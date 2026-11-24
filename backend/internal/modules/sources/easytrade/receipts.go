package easytrade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources"
)

// Fişten KDV oranı okuma (#780, sources/receipt_read.go). Easytrade sends
// one tax_rate per expense; a BİM receipt has lines at %1 and %20. After a
// sync the expenses whose receipt waits are read here, apart from the
// sync (queue task app:easytrade:receipts): the image is downloaded with
// size, type and time limits and handed to the receipt reader
// (receipts/extract), whose AI call masks personal data in the image like
// a photo upload (ai/usecase redactImages). The read rate lines replace
// Easytrade's single rate when their total matches Easytrade's total.

// Receipt download limits.
const (
	// MaxReceiptBytes caps one receipt image (as a photo upload).
	MaxReceiptBytes = 10 << 20
	// ReceiptTimeout bounds one download.
	ReceiptTimeout = 30 * time.Second
	// ReceiptBatch: receipts read per pass; the rest wait for the next.
	ReceiptBatch = 25
)

// receiptTypes are the image types the reader takes (sniffed, not the
// server's Content-Type).
var receiptTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true}

// errReceiptRejected: the image is too large or not an image; retrying
// does not help.
var errReceiptRejected = errors.New("easytrade: receipt image rejected")

// ReceiptReader reads a receipt image (*extract.Extractor).
type ReceiptReader interface {
	Read(ctx context.Context, req extract.Request) (extract.Result, error)
}

// ReceiptQueue enqueues one connection's receipt pass.
type ReceiptQueue func(ctx context.Context, taxpayerID int64, id uuid.UUID) error

// WithReceiptReader sets the receipt reader; nil: receipts are not read
// and Easytrade's single rate stays.
func (s *Service) WithReceiptReader(r ReceiptReader) *Service { s.reader = r; return s }

// WithReceiptQueue sets where a finished sync hands its receipt pass.
func (s *Service) WithReceiptQueue(q ReceiptQueue) *Service { s.receiptQueue = q; return s }

// ReceiptReport counts one pass.
type ReceiptReport struct {
	Read       int `json:"read"`
	Mismatch   int `json:"mismatch"`
	Unreadable int `json:"unreadable"`
	// Waiting: left pending (download to retry, AI not configured).
	Waiting int `json:"waiting"`
}

// enqueueReceipts hands the receipt pass of a finished sync to the queue.
// A queue failure never fails the sync: the next sync enqueues again.
func (s *Service) enqueueReceipts(ctx context.Context, row db.SourceConnection) {
	if s.receiptQueue == nil {
		return
	}
	_ = s.receiptQueue(context.WithoutCancel(ctx), row.TaxpayerID, row.Uuid)
}

// RunReceiptsTask is the worker's entry for one pass. Easytrade and
// download failures stay on the expenses (retried by the next pass); only
// infrastructure errors fail the task.
func (s *Service) RunReceiptsTask(ctx context.Context, taxpayerID int64, id uuid.UUID) error {
	_, err := s.ReadReceipts(ctx, taxpayerID, id)
	switch {
	case err == nil, errors.Is(err, ErrBadState), errors.Is(err, ErrNotFound), errors.Is(err, ErrNoVault),
		fatal(err), isSource(err):
		return nil
	}
	return err
}

// ReadReceipts reads the waiting receipts of a connection's expenses.
func (s *Service) ReadReceipts(ctx context.Context, taxpayerID int64, id uuid.UUID) (ReceiptReport, error) {
	var rep ReceiptReport
	if s.reader == nil {
		return rep, nil
	}
	row, c, err := s.client(ctx, taxpayerID, id)
	if err != nil {
		return rep, err
	}
	if row.Status == "disabled" || row.Status == "auth_failed" {
		return rep, ErrBadState
	}
	pending, err := s.q.ListPendingSourceReceiptReads(ctx, db.ListPendingSourceReceiptReadsParams{
		TaxpayerID: taxpayerID, SourceSystem: SourceSystem, Lim: ReceiptBatch,
	})
	if err != nil || len(pending) == 0 {
		return rep, err
	}
	// The AI call is billed to the office serving the taxpayer.
	eng, err := s.q.GetActiveEngagementForTaxpayer(ctx, taxpayerID)
	if errors.Is(err, pgx.ErrNoRows) {
		rep.Waiting = len(pending)
		return rep, nil
	}
	if err != nil {
		return rep, err
	}
	for i, p := range pending {
		st, ok := sources.ParseReceiptRead(p.OcrFields)
		if !ok {
			continue
		}
		next, stop, err := s.readReceipt(ctx, c, eng.OrganizationID, taxpayerID, p.SourceID, num(p.Amount), st)
		if err != nil || stop {
			rep.Waiting += len(pending) - i
			return rep, err
		}
		if err := s.storeReceipt(ctx, taxpayerID, p.ID, st.Key, next); err != nil {
			return rep, err
		}
		switch next.Status {
		case sources.ReceiptRead:
			rep.Read++
		case sources.ReceiptMismatch:
			rep.Mismatch++
		case sources.ReceiptUnreadable:
			rep.Unreadable++
		default:
			rep.Waiting++
		}
	}
	return rep, nil
}

// readReceipt fetches and reads one receipt. stop: no further receipt can
// be read in this pass (AI not configured, Easytrade refused the session).
func (s *Service) readReceipt(ctx context.Context, c *Client, orgID, taxpayerID int64, sourceID string, amount decimal.Decimal, st sources.ReceiptReadState) (sources.ReceiptReadState, bool, error) {
	next := st
	// A fresh detail: the receipt URL in the stored payload may have expired.
	var d struct {
		Data obj `json:"data"`
	}
	if err := c.get(ctx, "/company/expenses/"+url.PathEscape(sourceID), nil, &d); err != nil {
		if fatal(err) {
			return next, true, err
		}
		return retry(next, "detail_failed"), false, nil
	}
	u := ReceiptURL(d.Data)
	if u == "" {
		next.Status, next.Reason = sources.ReceiptUnreadable, "no_receipt"
		return next, false, nil
	}
	img, media, err := s.downloadReceipt(ctx, u)
	switch {
	case errors.Is(err, errReceiptRejected):
		next.Status, next.Reason = sources.ReceiptUnreadable, err.Error()
		return next, false, nil
	case err != nil:
		return retry(next, "download_failed"), false, nil
	}
	res, err := s.reader.Read(ctx, extract.Request{
		OrganizationID: orgID, TaxpayerID: taxpayerID, MediaType: media, Image: img, KindHint: extract.KindExpense,
	})
	if errors.Is(err, extract.ErrAIUnavailable) {
		return next, true, nil
	}
	if err != nil {
		// A read was attempted (and billed): not tried again.
		next.Status, next.Reason = sources.ReceiptUnreadable, "read_failed"
		return next, false, nil
	}
	now := s.now()
	next.ReadAt, next.Model, next.Reason, next.Attempts = &now, res.Model, "", 0
	next.Lines = ReceiptLines(res)
	next.Total = nil
	if t, ok := res.Decimal(extract.FieldTotal); ok {
		next.Total = &t
	}
	next.Status = ReceiptOutcome(next.Lines, amount)
	if next.Status == sources.ReceiptUnreadable {
		next.Reason = "no_rates"
	}
	return next, false, nil
}

// ReceiptOutcome is the status of read lines against Easytrade's total.
func ReceiptOutcome(lines []sources.ReceiptReadLine, amount decimal.Decimal) string {
	switch {
	case len(lines) == 0:
		return sources.ReceiptUnreadable
	case sources.ReceiptMatches(lines, amount):
		return sources.ReceiptRead
	}
	return sources.ReceiptMismatch
}

// retry counts a failed download; after MaxReceiptAttempts it gives up.
func retry(st sources.ReceiptReadState, reason string) sources.ReceiptReadState {
	st.Attempts++
	st.Reason = reason
	if st.Attempts >= sources.MaxReceiptAttempts {
		st.Status = sources.ReceiptUnreadable
	}
	return st
}

// ReceiptLines are the rate lines read from a receipt: each rate whose VAT
// was read (matrah = total − VAT). A receipt with one rate and no rate
// lines gives that rate with the receipt's total and VAT. Nothing is
// derived from a rate alone.
func ReceiptLines(res extract.Result) []sources.ReceiptReadLine {
	var out []sources.ReceiptReadLine
	for _, l := range res.VatLines() {
		if !l.TaxRead || l.Tax.IsNegative() || l.Tax.GreaterThan(l.Total) {
			continue
		}
		out = append(out, sources.ReceiptReadLine{Rate: l.Rate, Base: l.Total.Sub(l.Tax), Tax: l.Tax})
	}
	if len(out) > 0 {
		return out
	}
	rate, okR := res.Decimal(extract.FieldTaxRate)
	total, okT := res.Decimal(extract.FieldTotal)
	tax, okX := res.Decimal(extract.FieldTax)
	if okR && okT && okX && !tax.IsNegative() && tax.LessThanOrEqual(total) && !rate.IsNegative() {
		out = append(out, sources.ReceiptReadLine{Rate: rate, Base: total.Sub(tax), Tax: tax})
	}
	return out
}

// storeReceipt writes the state, and the read lines when they match, only
// if the same receipt still waits on a pending expense.
func (s *Service) storeReceipt(ctx context.Context, taxpayerID, expenseID int64, key string, st sources.ReceiptReadState) error {
	return database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		n, err := q.SetSourceReceiptRead(ctx, db.SetSourceReceiptReadParams{
			OcrFields: st.OcrFields(), TaxpayerID: taxpayerID, ID: expenseID, ReceiptKey: key,
		})
		if err != nil || n == 0 || st.Status != sources.ReceiptRead {
			return err
		}
		return sources.ReplaceExpenseVatLines(ctx, q, taxpayerID, expenseID, st.VatLines())
	})
}

// downloadReceipt fetches the image: https only (or the configured
// Easytrade host), at most MaxReceiptBytes, an image type, within
// ReceiptTimeout. No Easytrade token is sent: the URL is signed.
func (s *Service) downloadReceipt(ctx context.Context, raw string) ([]byte, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, "", fmt.Errorf("%w: bad url", errReceiptRejected)
	}
	if u.Scheme != "https" && !s.sameHost(u) {
		return nil, "", fmt.Errorf("%w: not https", errReceiptRejected)
	}
	ctx, cancel := context.WithTimeout(ctx, ReceiptTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("%w: bad url", errReceiptRejected)
	}
	hc := s.receiptHTTP
	if hc == nil {
		hc = s.hc
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = res.Body.Close() }()
	switch {
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone:
		return nil, "", fmt.Errorf("%w: HTTP %d", errReceiptRejected, res.StatusCode)
	case res.StatusCode >= 300:
		return nil, "", fmt.Errorf("easytrade: receipt download: HTTP %d", res.StatusCode)
	case res.ContentLength > MaxReceiptBytes:
		return nil, "", fmt.Errorf("%w: too large", errReceiptRejected)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, MaxReceiptBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > MaxReceiptBytes {
		return nil, "", fmt.Errorf("%w: too large", errReceiptRejected)
	}
	media := http.DetectContentType(body)
	if i := strings.IndexByte(media, ';'); i >= 0 {
		media = media[:i]
	}
	if !receiptTypes[media] {
		return nil, "", fmt.Errorf("%w: type %s", errReceiptRejected, media)
	}
	return body, media, nil
}

// sameHost: the URL is on the configured Easytrade host (a test server).
func (s *Service) sameHost(u *url.URL) bool {
	b, err := url.Parse(s.base)
	return err == nil && b.Host != "" && strings.EqualFold(b.Host, u.Host) && b.Scheme == u.Scheme
}

func num(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}
