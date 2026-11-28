// Package usecase runs receipt capture (issue #23): photo → AI read with
// per-field confidence → expense / retail_sale draft (approval_status =
// pending) with the image attached → plate check → low-confidence fields
// to the review queue. Commit to the declaration needs an accountant's
// approval.
package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// MaxImageBytes caps one photo.
const MaxImageBytes = 10 << 20

// ReviewBelow: fields under this confidence go to the review queue.
const ReviewBelow = extract.ReviewBelow

// Capture statuses.
const (
	// StatusProcessing: the photo is stored and its AI read runs in the
	// background (#780, CapturePartsAsync).
	StatusProcessing = "processing"
	StatusDraft      = "draft"
	StatusDuplicate  = "duplicate"
	StatusFailed     = "failed"
	StatusApproved   = "approved"
	StatusRejected   = "rejected"
)

// PlateProducer is the data_warning producer of the plate check.
const PlateProducer = "control:plate"

// ExpenseApprovalProducer raises UNAPPROVED_EXPENSE on a pending expense
// and resolves it once the expense is approved or rejected (#529).
const ExpenseApprovalProducer = "control:expense_approval"

// Errors.
var (
	ErrInvalid  = errors.New("receipts: invalid input")
	ErrNotFound = errors.New("receipts: not found")
	ErrState    = errors.New("receipts: not allowed in the capture's status")
)

// DB is the pool subset.
type DB interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// ObjectStore is the storage subset.
type ObjectStore interface {
	Upload(ctx context.Context, file storage.File, path string) error
}

// Reader reads a photo (extract.Extractor).
type Reader interface {
	Read(ctx context.Context, req extract.Request) (extract.Result, error)
}

// FieldReview is one low-confidence field for the review queue (#14).
type FieldReview struct {
	OrganizationID int64
	EngagementID   int64
	TaxpayerID     int64
	TargetType     string // expense | retail_sale
	TargetID       int64
	Period         string
	Field          string
	Value          extract.Value
	Model          string
	AttachmentID   int64
	// ReceiptUUID is the capture the field was read from: the review
	// queue shows its photo next to the value (#569).
	ReceiptUUID uuid.UUID
	// ProposedBy is the user who typed the value (taxpayer correction,
	// #199); 0 = read by the model.
	ProposedBy int64
	// Note is the proposer's comment.
	Note string
}

// Reviewer sends a field to the review queue. Nil: no queue.
type Reviewer interface {
	SubmitField(ctx context.Context, f FieldReview) error
}

// WarningEngine raises / resolves data warnings (#13).
type WarningEngine interface {
	Apply(ctx context.Context, s engine.Subject, producer string, ent engine.Entity, owned []string, findings []engine.Finding) (engine.Report, error)
}

// Service runs captures.
type Service struct {
	pool     DB
	q        *db.Queries
	store    ObjectStore
	reader   Reader
	reviewer Reviewer
	warnings WarningEngine
	journals JournalWriter
	log      *slog.Logger
	// notifier / frontendURL: receipt notifications (#780 Senaryo B).
	notifier    Notifier
	frontendURL string
	// bg: background reads of mobile receipts (#780, CapturePartsAsync).
	bg background
}

// New builds the service; reviewer and warnings may be nil.
func New(pool DB, store ObjectStore, reader Reader, reviewer Reviewer, warnings WarningEngine, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{pool: pool, q: database.QueriesFor(pool), store: store, reader: reader, reviewer: reviewer, warnings: warnings, log: log}
}

// WithReviewer sets the review queue adapter.
func (s *Service) WithReviewer(r Reviewer) *Service { s.reviewer = r; return s }

var mediaTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true}

// Capture stores a photo and turns it into a draft.
func (s *Service) Capture(ctx context.Context, sc engagementctx.Scope, userID int64, fileName string, data []byte, kindHint string) (View, error) {
	return s.CaptureKeyed(ctx, sc, userID, fileName, data, kindHint, "")
}

// MaxClientKey bounds the Idempotency-Key a client may send.
const MaxClientKey = 128

// CaptureKeyed is Capture with the client's Idempotency-Key (#199): a second
// upload with the same key for the taxpayer returns the first capture with
// Replayed set, without storing or reading the photo again.
func (s *Service) CaptureKeyed(ctx context.Context, sc engagementctx.Scope, userID int64, fileName string, data []byte, kindHint, clientKey string) (View, error) {
	return s.CaptureParts(ctx, sc, userID, []Part{{FileName: fileName, Data: data}}, kindHint, clientKey)
}

// MaxParts bounds the photos of one document (a long Z report in parts).
const MaxParts = 5

// Part is one photo of a document captured in several parts, in order
// (parça 1, 2, 3 …).
type Part struct {
	FileName string
	Data     []byte
}

// CaptureParts is CaptureKeyed for a document photographed in several parts
// (Oğuz'un demo geri bildirimi: a long Z report). Every part is stored and
// read; the reads are merged into one draft (extract.Merge) and the parts
// are kept in order on the capture (receipt_capture_parts). One part is a
// plain capture.
func (s *Service) CaptureParts(ctx context.Context, sc engagementctx.Scope, userID int64, parts []Part, kindHint, clientKey string) (View, error) {
	return s.captureKeyed(ctx, sc, userID, parts, kindHint, clientKey, s.capture)
}

// CapturePartsAsync is CaptureParts that does not wait for the AI read (#780):
// the photos are stored and the capture is recorded as StatusProcessing; the
// read runs in the background once the caller's transaction has committed
// and turns the capture into a draft, a duplicate or a failed (unreadable)
// capture, exactly as CaptureParts would. The mobile app's upload uses it:
// a read inside the request took longer than the phone waited. A replay
// (same Idempotency-Key) and the same image sent again are answered at once
// as before.
func (s *Service) CapturePartsAsync(ctx context.Context, sc engagementctx.Scope, userID int64, parts []Part, kindHint, clientKey string) (View, error) {
	return s.captureKeyed(ctx, sc, userID, parts, kindHint, clientKey, s.asyncCapture(true))
}

// CaptureKeyedAsync is CaptureKeyed with the background read of
// CapturePartsAsync, for an office-side upload (the WhatsApp extension,
// #780): the office is not notified, its own user sent the photo.
func (s *Service) CaptureKeyedAsync(ctx context.Context, sc engagementctx.Scope, userID int64, fileName string, data []byte, kindHint, clientKey string) (View, error) {
	return s.captureKeyed(ctx, sc, userID, []Part{{FileName: fileName, Data: data}}, kindHint, clientKey, s.asyncCapture(false))
}

type captureFunc func(ctx context.Context, sc engagementctx.Scope, userID int64, parts []Part, kindHint, clientKey string) (View, error)

func (s *Service) captureKeyed(ctx context.Context, sc engagementctx.Scope, userID int64, parts []Part, kindHint, clientKey string, run captureFunc) (View, error) {
	clientKey = strings.TrimSpace(clientKey)
	if len(clientKey) > MaxClientKey {
		return View{}, fmt.Errorf("%w: Idempotency-Key is longer than %d characters", ErrInvalid, MaxClientKey)
	}
	if clientKey != "" {
		if v, ok, err := s.byClientKey(ctx, sc, clientKey); err != nil || ok {
			return v, err
		}
	}
	if len(parts) > 1 {
		st, err := s.PartsSettings(ctx, sc.OrganizationID)
		if err != nil {
			return View{}, err
		}
		if !st.Enabled {
			return View{}, ErrPartsOff
		}
	}
	v, err := run(ctx, sc, userID, parts, kindHint, clientKey)
	var pgErr *pgconn.PgError
	if clientKey != "" && errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_receipt_captures_client_key" {
		// A concurrent request with the same key won; answer with its record.
		if v, ok, err := s.byClientKey(ctx, sc, clientKey); err != nil || ok {
			return v, err
		}
	}
	return v, err
}

func (s *Service) byClientKey(ctx context.Context, sc engagementctx.Scope, key string) (View, bool, error) {
	c, err := s.q.GetReceiptCaptureByClientKey(ctx, db.GetReceiptCaptureByClientKeyParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, ClientKey: text(key),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, false, nil
	}
	if err != nil {
		return View{}, false, err
	}
	v := s.view(c, nil)
	v.Replayed = true
	return v, true, nil
}

// storedPart is a part's stored image.
type storedPart struct {
	media, sha, name string
	att              db.UpsertAttachmentRow
}

func (s *Service) storePart(ctx context.Context, sc engagementctx.Scope, p Part) (storedPart, error) {
	if len(p.Data) == 0 || len(p.Data) > MaxImageBytes {
		return storedPart{}, fmt.Errorf("%w: image must be 1 byte to 10 MB", ErrInvalid)
	}
	media := http.DetectContentType(p.Data)
	if !mediaTypes[media] {
		return storedPart{}, fmt.Errorf("%w: image must be JPEG, PNG, WebP or GIF (got %s)", ErrInvalid, media)
	}
	sum := sha256.Sum256(p.Data)
	sha := hex.EncodeToString(sum[:])
	name := cleanName(p.FileName)
	key := fmt.Sprintf("receipts/%d/%s%s", sc.TaxpayerID, sha, ext(media))
	if err := s.store.Upload(ctx, storage.File{Body: bytes.NewReader(p.Data), Size: int64(len(p.Data)), ContentType: media, Filename: name}, key); err != nil {
		return storedPart{}, fmt.Errorf("receipts: store image: %w", err)
	}
	att, err := s.q.UpsertAttachment(ctx, db.UpsertAttachmentParams{
		TaxpayerID: sc.TaxpayerID, Kind: "image", StorageKey: key, Sha256: sha, SizeBytes: int64(len(p.Data)),
		ContentType: text(media), FileName: text(name),
	})
	if err != nil {
		return storedPart{}, fmt.Errorf("receipts: attachment: %w", err)
	}
	return storedPart{media: media, sha: sha, name: name, att: att}, nil
}

// partsHash is the content hash of the whole document: the image's own
// hash for one part, else the hash of the parts' hashes in order.
func partsHash(stored []storedPart) string {
	if len(stored) == 1 {
		return stored[0].sha
	}
	h := sha256.New()
	for _, p := range stored {
		_, _ = h.Write([]byte(p.sha + "+"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// saveParts keeps the order of a multi-part capture; one part needs no row.
func (s *Service) saveParts(ctx context.Context, sc engagementctx.Scope, capture uuid.UUID, stored []storedPart) error {
	if len(stored) < 2 {
		return nil
	}
	for i, p := range stored {
		if err := s.q.InsertReceiptCapturePart(ctx, db.InsertReceiptCapturePartParams{
			PartNo: int16(i + 1), AttachmentID: p.att.ID, FileName: p.name, //nolint:gosec // i < MaxParts
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, CaptureUuid: capture,
		}); err != nil {
			return fmt.Errorf("receipts: capture part: %w", err)
		}
	}
	return nil
}

// prepared is a capture whose photos are stored and that waits for the read.
type prepared struct {
	base     db.InsertReceiptCaptureParams
	parts    []Part
	stored   []storedPart
	kindHint string
	hexSum   string
}

func (s *Service) capture(ctx context.Context, sc engagementctx.Scope, userID int64, parts []Part, kindHint, clientKey string) (View, error) {
	pre, v, done, err := s.prepare(ctx, sc, userID, parts, kindHint, clientKey)
	if err != nil || done {
		return v, err
	}
	return s.read(ctx, sc, userID, pre, 0)
}

// prepare validates and stores the photos. done: the capture is already
// answered (the same image was captured before) and needs no read.
func (s *Service) prepare(ctx context.Context, sc engagementctx.Scope, userID int64, parts []Part, kindHint, clientKey string) (prepared, View, bool, error) {
	if len(parts) == 0 || len(parts) > MaxParts {
		return prepared{}, View{}, false, fmt.Errorf("%w: send 1 to %d photos", ErrInvalid, MaxParts)
	}
	switch kindHint {
	case "", extract.KindExpense, extract.KindFuel, extract.KindZReport:
	default:
		return prepared{}, View{}, false, fmt.Errorf("%w: kind", ErrInvalid)
	}
	stored := make([]storedPart, 0, len(parts))
	anyNew := false
	for _, p := range parts {
		sp, err := s.storePart(ctx, sc, p)
		if err != nil {
			return prepared{}, View{}, false, err
		}
		anyNew = anyNew || sp.att.Inserted
		stored = append(stored, sp)
	}
	att, media := stored[0].att, stored[0].media
	base := db.InsertReceiptCaptureParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, UploadedBy: optID(userID),
		AttachmentID: att.ID, MediaType: media, Kind: text(kindHint), Fields: []byte("{}"),
		ClientKey: text(clientKey),
	}
	// Same image(s) again: no AI call, point at the earlier record (or at
	// the capture of the same image still being read, #780).
	if !anyNew {
		prev, ok := s.previousForAttachment(ctx, sc, att.ID)
		if !ok {
			prev, ok = s.processingForAttachment(ctx, sc, att.ID)
		}
		if ok {
			base.Status, base.DuplicateOf = StatusDuplicate, pgtype.UUID{Bytes: prev, Valid: true}
			base.Error = text("same image was uploaded before")
			v, err := s.insertCaptureParts(ctx, sc, base, nil, stored)
			return prepared{}, v, true, err
		}
	}
	return prepared{base: base, parts: parts, stored: stored, kindHint: kindHint, hexSum: partsHash(stored)}, View{}, false, nil
}

// read runs the AI read of a prepared capture and records the result:
// captureID 0 inserts the capture, else the StatusProcessing capture is
// completed (background read, #780).
func (s *Service) read(ctx context.Context, sc engagementctx.Scope, userID int64, pre prepared, captureID int64) (View, error) {
	reads, err := s.readParts(ctx, sc, userID, pre)
	return s.record(ctx, sc, pre, captureID, reads, err)
}

// readParts asks the reader for every part. It needs no database
// transaction (the AI ledger writes in its own), so the background read
// holds no connection while the model works (#780).
func (s *Service) readParts(ctx context.Context, sc engagementctx.Scope, userID int64, pre prepared) ([]extract.Result, error) {
	reads := make([]extract.Result, 0, len(pre.parts))
	for i, p := range pre.parts {
		r, err := s.reader.Read(ctx, extract.Request{
			OrganizationID: sc.OrganizationID, TaxpayerID: sc.TaxpayerID, UserID: userID,
			MediaType: pre.stored[i].media, Image: p.Data, KindHint: pre.kindHint,
		})
		if err != nil {
			s.log.Warn("receipt_read_failed", "taxpayer", sc.TaxpayerID, "part", i+1, "error", err)
			return nil, err
		}
		reads = append(reads, r)
	}
	return reads, nil
}

// record writes the result of readParts: a failed read (readErr) is an
// unreadable capture with the reader's error, else a duplicate or a draft.
func (s *Service) record(ctx context.Context, sc engagementctx.Scope, pre prepared, captureID int64, reads []extract.Result, readErr error) (View, error) {
	base, stored, kindHint, hexSum := pre.base, pre.stored, pre.kindHint, pre.hexSum
	att := stored[0].att
	if readErr != nil {
		base.Status, base.Error = StatusFailed, text(readErr.Error())
		return s.writeView(ctx, s.q, sc, base, captureID, nil, stored)
	}
	res := reads[0]
	if len(reads) > 1 {
		res = extract.Merge(reads...)
	}
	// The uploader chose "Z raporu" (WhatsApp "Z raporu olarak kaydet",
	// parçalı Z raporu): the photo becomes a retail sale draft even when the
	// model reads it as a receipt. Before, a single photo the model called
	// an expense receipt became an expense draft.
	if kindHint == extract.KindZReport {
		res.Kind = extract.KindZReport
	}
	fieldsJSON, _ := json.Marshal(res.Fields)
	base.Kind, base.Fields, base.Model = text(res.Kind), fieldsJSON, text(res.Model)

	if prev, ok := s.duplicateByFields(ctx, sc, res); ok {
		base.Status, base.DuplicateOf = StatusDuplicate, pgtype.UUID{Bytes: prev, Valid: true}
		base.Error = text("same vendor, date and amount were captured before")
		return s.writeView(ctx, s.q, sc, base, captureID, &res, stored)
	}

	tx, err := database.Begin(ctx, s.pool)
	if err != nil {
		return View{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	draft := Draft{}
	conf := toNumeric(decimal.NewFromFloat(res.MinConfidence()).Round(4))
	raw, _ := json.Marshal(map[string]any{"kind": res.Kind, "kind_confidence": res.KindConfidence, "model": res.Model})
	date, hasDate := res.Date()
	period := text(DocPeriod(date, hasDate, time.Now()))
	total, hasTotal := res.Decimal(extract.FieldTotal)
	tax, hasTax := res.Decimal(extract.FieldTax)
	sourceID := "capture:" + hexSum
	if res.Kind == extract.KindZReport {
		p := db.InsertRetailSaleDraftParams{
			TaxpayerID: sc.TaxpayerID, SourceID: sourceID, SaleDate: optDate(date, hasDate), Period: period,
			ZReportNumber: text(res.Get(extract.FieldZNumber)), OkcSerialNumber: text(res.Get(extract.FieldOKCSerial)),
			TotalAmount: optNum(total, hasTotal), TaxAmount: optNum(tax, hasTax), OcrConfidence: conf, OcrFields: fieldsJSON,
			AttachmentID: pgtype.Int8{Int64: att.ID, Valid: true}, ContentHash: text(hexSum),
		}
		if p.RawPayload, err = secrets.SealField(secrets.PurposeRetailSaleRawPayload, raw); err != nil {
			return View{}, err
		}
		if hasTotal && hasTax {
			p.SubtotalAmount = optNum(total.Sub(tax), true)
		}
		row, err := q.InsertRetailSaleDraft(ctx, p)
		if err != nil {
			return View{}, fmt.Errorf("receipts: retail sale: %w", err)
		}
		// #384: KDV oranı ve ödeme tipi kırılımı.
		if err := storeZBreakdown(dbtx.WithTx(ctx, tx), q, sc.TaxpayerID, row.ID, res); err != nil {
			return View{}, fmt.Errorf("receipts: z breakdown: %w", err)
		}
		draft = Draft{Type: "retail_sale", ID: row.ID, UUID: row.Uuid}
		base.RetailSaleID = pgtype.Int8{Int64: row.ID, Valid: true}
	} else {
		p := db.InsertExpenseDraftParams{
			TaxpayerID: sc.TaxpayerID, SourceID: sourceID, ExpenseDate: optDate(date, hasDate), Period: period,
			Amount: optNum(total, hasTotal), TaxAmount: optNum(tax, hasTax), ReceiptType: receiptType(res.Kind),
			VendorTaxNumber: text(res.Get(extract.FieldVendorTaxNumber)), VendorName: text(res.Get(extract.FieldVendorName)),
			ReceiptNumber: text(res.Get(extract.FieldReceiptNumber)), ZReportNumber: text(res.Get(extract.FieldZNumber)),
			OkcSerialNumber: text(res.Get(extract.FieldOKCSerial)), VehiclePlate: text(res.Get(extract.FieldPlate)),
			PaymentMethod: text(res.Get(extract.FieldPaymentMethod)), OcrConfidence: conf, OcrFields: fieldsJSON,
			AttachmentID: pgtype.Int8{Int64: att.ID, Valid: true}, ContentHash: text(hexSum),
		}
		if p.RawPayload, err = secrets.SealField(secrets.PurposeExpenseRawPayload, raw); err != nil {
			return View{}, err
		}
		if r, ok := res.Decimal(extract.FieldTaxRate); ok {
			p.TaxRate = optNum(r, true)
		}
		if hasTotal && hasTax {
			p.TaxExclusiveAmount = optNum(total.Sub(tax), true)
		}
		row, err := q.InsertExpenseDraft(ctx, p)
		if err != nil {
			return View{}, fmt.Errorf("receipts: expense: %w", err)
		}
		// #780: KDV oranı kırılımı (okunan oranlar; KDV'si okunamayan oran
		// yazılmaz, ofis ekranında "kontrol et" olur).
		if err := storeExpenseVatLines(dbtx.WithTx(ctx, tx), q, sc.TaxpayerID, row.ID, res); err != nil {
			return View{}, fmt.Errorf("receipts: expense vat lines: %w", err)
		}
		draft = Draft{Type: "expense", ID: row.ID, UUID: row.Uuid}
		base.ExpenseID = pgtype.Int8{Int64: row.ID, Valid: true}
	}
	base.Status = StatusDraft
	capture, err := s.writeCapture(dbtx.WithTx(ctx, tx), q, sc, base, captureID, stored)
	if err != nil {
		return View{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return View{}, err
	}
	v := s.view(capture, &res)
	v.Draft = &draft
	v.Parts = partViews(stored)

	// After the commit: checks that only add information.
	if draft.Type == "expense" {
		// Fiş yapay zekâ kontrolleri: the taxpayer sees them in the preview.
		var plateShown bool
		v.PlateCheck, plateShown = s.checkPlate(ctx, sc, draft.ID, res, date, hasDate, period.String)
		if s.checkBuyer(ctx, sc, draft.ID, res, period.String) {
			v.Checks = append(v.Checks, CodeBuyerMismatch)
		}
		if plateShown {
			code := CodePlateNotRegistered
			if v.PlateCheck == "unreadable" {
				code = CodePlateUnreadable
			}
			v.Checks = append(v.Checks, code)
		}
		s.syncExpenseApprovalByID(ctx, sc, draft.ID)
	}
	if draft.Type == "retail_sale" {
		s.CheckZContinuity(ctx, sc, res.Get(extract.FieldOKCSerial))
	}
	v.InReview = s.sendLowConfidence(ctx, sc, draft, res, period.String, att.ID, capture.Uuid)
	return v, nil
}

// officeZone is office time (Türkiye, UTC+3 all year since 2016).
var officeZone = time.FixedZone("TRT", 3*60*60)

// DocPeriod is the month a captured document belongs to (#780): the month
// of its own date. The engagement is the taxpayer's current office (the
// upload's scope), so nobody picks the month by hand. When the date could
// not be read the record goes to the upload month (office time), the same
// month the capture list uses, so it still shows in the office's month
// view; the date itself stays empty and is flagged "kontrol et". A date
// corrected in the review queue moves the record to its own month.
func DocPeriod(date time.Time, hasDate bool, now time.Time) string {
	if hasDate {
		return date.Format("2006-01")
	}
	return now.In(officeZone).Format("2006-01")
}

// storeExpenseVatLines stores the per-rate breakdown read from an expense
// receipt. A rate whose VAT was not read is skipped (never stored as zero).
func storeExpenseVatLines(ctx context.Context, q *db.Queries, taxpayerID, expenseID int64, res extract.Result) error {
	rows := make([]map[string]string, 0, 4)
	for _, l := range res.VatLines() {
		if !l.TaxRead || l.Tax.IsNegative() || l.Tax.GreaterThan(l.Total) {
			continue
		}
		rows = append(rows, map[string]string{"tax_rate": l.Rate.String(), "tax_base": l.Total.Sub(l.Tax).String(), "tax_amount": l.Tax.String()})
	}
	if len(rows) == 0 {
		return nil
	}
	raw, _ := json.Marshal(rows)
	_, err := q.InsertExpenseVatLines(ctx, db.InsertExpenseVatLinesParams{TaxpayerID: taxpayerID, ExpenseID: expenseID, Rows: raw})
	return err
}

// sendLowConfidence submits every present field under ReviewBelow (or
// one we could not normalize) to the review queue.
func (s *Service) sendLowConfidence(ctx context.Context, sc engagementctx.Scope, d Draft, res extract.Result, period string, attachmentID int64, receipt uuid.UUID) []string {
	var out []string
	for _, f := range extract.Fields {
		v, ok := res.Fields[f]
		if !ok || (v.Valid && v.Confidence >= ReviewBelow) {
			continue
		}
		out = append(out, f)
		if s.reviewer == nil {
			continue
		}
		// A field the queue could not take stays on the draft: a side
		// effect in its own savepoint (#311); the next field still goes.
		_ = database.BestEffort(ctx, "receipt_review_submit", func(ctx context.Context) error {
			return s.reviewer.SubmitField(ctx, FieldReview{
				OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID,
				TargetType: d.Type, TargetID: d.ID, Period: period, Field: f, Value: v, Model: res.Model, AttachmentID: attachmentID,
				ReceiptUUID: receipt,
			})
		})
	}
	return out
}

// Decide approves or rejects a draft (accountant, T6: "beyana sadece onaylı
// masraf girer").
func (s *Service) Decide(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, userID int64, approve bool) (View, error) {
	return s.DecideWithReason(ctx, sc, id, userID, approve, Rejection{})
}

// DecideWithReason is Decide with the office's reason for a rejection
// (#780, ignored on approval). The reason is kept on the capture and told to
// the mükellef.
func (s *Service) DecideWithReason(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, userID int64, approve bool, rej Rejection) (View, error) {
	rej, err := decisionReason(approve, rej)
	if err != nil {
		return View{}, err
	}
	c, err := s.get(ctx, sc, id)
	if err != nil {
		return View{}, err
	}
	// Okunamayan fotoğraf (#568): taslağı yoktur; ofis kapatır (yerine
	// mükelleften yenisini ister). Onaylanacak bir şey yoktur.
	if c.Status == StatusFailed && !approve {
		c, err = s.q.SetReceiptCaptureStatus(ctx, db.SetReceiptCaptureStatusParams{Status: StatusRejected, OrganizationID: c.OrganizationID, EngagementID: c.EngagementID, ID: c.ID,
			RejectReasonCode: rej.dbCode(), RejectReasonText: rej.dbText()})
		if err != nil {
			return View{}, err
		}
		s.notifyDecision(ctx, sc, byCapture(c.ID), false, rej)
		return s.view(c, nil), nil
	}
	if c.Status != StatusDraft {
		return View{}, ErrState
	}
	status, captureStatus := "rejected", StatusRejected
	if approve {
		status, captureStatus = "approved", StatusApproved
	}
	tx, err := database.Begin(ctx, s.pool)
	if err != nil {
		return View{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	var decided *db.Expense
	switch {
	case c.ExpenseID.Valid:
		e, err := q.SetExpenseApproval(ctx, db.SetExpenseApprovalParams{ApprovalStatus: status, ApprovedBy: optID(userID), TaxpayerID: c.TaxpayerID, ID: c.ExpenseID.Int64})
		if err != nil {
			return View{}, fmt.Errorf("receipts: approve expense: %w", err)
		}
		decided = &e
		// #384: the approved expense gets its account and journal draft.
		if approve {
			if err := s.postExpense(dbtx.WithTx(ctx, tx), q, sc, e, userID); err != nil {
				return View{}, err
			}
		}
	case c.RetailSaleID.Valid:
		rs, err := q.SetRetailSaleApproval(ctx, db.SetRetailSaleApprovalParams{ApprovalStatus: status, ApprovedBy: optID(userID), TaxpayerID: c.TaxpayerID, ID: c.RetailSaleID.Int64})
		if err != nil {
			return View{}, fmt.Errorf("receipts: approve retail sale: %w", err)
		}
		// #384: the approved Z report gets its sales journal draft.
		if approve {
			if err := s.postRetailSale(dbtx.WithTx(ctx, tx), q, sc, rs, userID); err != nil {
				return View{}, err
			}
		}
	default:
		return View{}, ErrState
	}
	c, err = q.SetReceiptCaptureStatus(ctx, db.SetReceiptCaptureStatusParams{Status: captureStatus, OrganizationID: c.OrganizationID, EngagementID: c.EngagementID, ID: c.ID,
		RejectReasonCode: rej.dbCode(), RejectReasonText: rej.dbText()})
	if err != nil {
		return View{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return View{}, err
	}
	if decided != nil {
		s.syncExpenseApproval(ctx, sc, *decided)
	}
	s.notifyDecision(ctx, sc, byCapture(c.ID), approve, rej)
	return s.view(c, nil), nil
}

// Get returns one capture.
func (s *Service) Get(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (View, error) {
	c, err := s.get(ctx, sc, id)
	if err != nil {
		return View{}, err
	}
	v := s.view(c, nil)
	// A capture read in the background (#780) is followed with Get: its
	// draft, parts, open receipt checks and the fields sent to review come
	// along, as in the capture response.
	switch {
	case c.ExpenseID.Valid:
		if e, err := s.q.GetExpense(ctx, db.GetExpenseParams{TaxpayerID: c.TaxpayerID, ID: c.ExpenseID.Int64}); err == nil {
			v.Draft = &Draft{Type: "expense", ID: e.ID, UUID: e.Uuid}
		}
		v.Checks = s.expenseChecks(ctx, sc, []int64{c.ExpenseID.Int64})[c.ExpenseID.Int64]
	case c.RetailSaleID.Valid:
		if r, err := s.q.GetRetailSale(ctx, db.GetRetailSaleParams{TaxpayerID: c.TaxpayerID, ID: c.RetailSaleID.Int64}); err == nil {
			v.Draft = &Draft{Type: "retail_sale", ID: r.ID, UUID: r.Uuid}
		}
	}
	if parts, err := s.q.ListReceiptCaptureParts(ctx, db.ListReceiptCapturePartsParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, CaptureUuid: c.Uuid,
	}); err == nil && len(parts) > 1 {
		for _, p := range parts {
			v.Parts = append(v.Parts, PartView{PartNo: int(p.PartNo), FileName: p.FileName})
		}
	}
	if c.Status == StatusDraft {
		v.InReview = lowConfidence(v.Fields)
	}
	return v, nil
}

// lowConfidence lists the present fields under ReviewBelow (or not
// normalized): the fields sendLowConfidence sends to the review queue.
func lowConfidence(fields map[string]extract.Value) []string {
	var out []string
	for _, f := range extract.Fields {
		v, ok := fields[f]
		if !ok || (v.Valid && v.Confidence >= ReviewBelow) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// List returns one page of captures.
// period (YYYY-MM, #333) is the draft's period, or the upload month when
// there is no draft; "" = any.
func (s *Service) List(ctx context.Context, sc engagementctx.Scope, status, period string, limit, offset int32) ([]View, int64, error) {
	rows, err := s.q.ListReceiptCaptures(ctx, db.ListReceiptCapturesParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Status: text(status), Period: text(period), Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountReceiptCaptures(ctx, db.CountReceiptCapturesParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Status: text(status), Period: text(period)})
	if err != nil {
		return nil, 0, err
	}
	out := make([]View, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.view(r, nil))
	}
	return out, total, nil
}

func (s *Service) previousForAttachment(ctx context.Context, sc engagementctx.Scope, attachmentID int64) (uuid.UUID, bool) {
	if e, err := s.q.FindExpenseByAttachment(ctx, db.FindExpenseByAttachmentParams{TaxpayerID: sc.TaxpayerID, AttachmentID: pgtype.Int8{Int64: attachmentID, Valid: true}}); err == nil {
		if c, err := s.q.FindCaptureByExpense(ctx, db.FindCaptureByExpenseParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, ExpenseID: e.ID}); err == nil {
			return c, true
		}
		return e.Uuid, true
	}
	if r, err := s.q.FindRetailSaleByAttachment(ctx, db.FindRetailSaleByAttachmentParams{TaxpayerID: sc.TaxpayerID, AttachmentID: pgtype.Int8{Int64: attachmentID, Valid: true}}); err == nil {
		return r.Uuid, true
	}
	return uuid.Nil, false
}

func (s *Service) duplicateByFields(ctx context.Context, sc engagementctx.Scope, res extract.Result) (uuid.UUID, bool) {
	date, hasDate := res.Date()
	total, hasTotal := res.Decimal(extract.FieldTotal)
	if !hasDate {
		return uuid.Nil, false
	}
	if res.Kind == extract.KindZReport {
		z := res.Get(extract.FieldZNumber)
		if z == "" && !hasTotal {
			return uuid.Nil, false
		}
		r, err := s.q.FindDuplicateRetailSale(ctx, db.FindDuplicateRetailSaleParams{
			TaxpayerID: sc.TaxpayerID, SaleDate: pgtype.Date{Time: date, Valid: true}, ZReportNumber: text(z), TotalAmount: optNum(total, hasTotal),
			OkcSerialNumber: text(res.Get(extract.FieldOKCSerial)),
		})
		return r.Uuid, err == nil
	}
	vkn := res.Get(extract.FieldVendorTaxNumber)
	if vkn == "" || !hasTotal {
		return uuid.Nil, false
	}
	e, err := s.q.FindDuplicateExpense(ctx, db.FindDuplicateExpenseParams{
		TaxpayerID: sc.TaxpayerID, VendorTaxNumber: text(vkn), ExpenseDate: pgtype.Date{Time: date, Valid: true},
		Amount: optNum(total, true), ReceiptNumber: text(res.Get(extract.FieldReceiptNumber)),
	})
	if err != nil {
		return uuid.Nil, false
	}
	if c, err := s.q.FindCaptureByExpense(ctx, db.FindCaptureByExpenseParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, ExpenseID: e.ID}); err == nil {
		return c, true
	}
	return e.Uuid, true
}

// insertCaptureParts is insertCapture keeping a multi-part capture's parts.
func (s *Service) insertCaptureParts(ctx context.Context, sc engagementctx.Scope, p db.InsertReceiptCaptureParams, res *extract.Result, stored []storedPart) (View, error) {
	v, err := s.insertCapture(ctx, p, res)
	if err != nil {
		return v, err
	}
	if err := s.saveParts(ctx, sc, v.UUID, stored); err != nil {
		return View{}, err
	}
	v.Parts = partViews(stored)
	return v, nil
}

// ErrNotProcessing: a background read found its capture no longer waiting
// for the read.
var ErrNotProcessing = errors.New("receipts: capture is not waiting for its read")

// writeCapture records a capture: inserts it with its parts (captureID 0) or
// completes the StatusProcessing capture captureID, whose parts are already
// stored.
func (s *Service) writeCapture(ctx context.Context, q *db.Queries, sc engagementctx.Scope, p db.InsertReceiptCaptureParams, captureID int64, stored []storedPart) (db.ReceiptCapture, error) {
	if captureID == 0 {
		c, err := q.InsertReceiptCapture(ctx, p)
		if err != nil {
			return c, err
		}
		return c, s.saveParts(ctx, sc, c.Uuid, stored)
	}
	c, err := q.CompleteReceiptCapture(ctx, db.CompleteReceiptCaptureParams{
		Status: p.Status, Kind: p.Kind, Fields: p.Fields, Model: p.Model, ExpenseID: p.ExpenseID,
		RetailSaleID: p.RetailSaleID, DuplicateOf: p.DuplicateOf, Error: p.Error,
		OrganizationID: p.OrganizationID, EngagementID: p.EngagementID, ID: captureID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotProcessing
	}
	return c, err
}

// writeView is writeCapture answering with the capture's view.
func (s *Service) writeView(ctx context.Context, q *db.Queries, sc engagementctx.Scope, p db.InsertReceiptCaptureParams, captureID int64, res *extract.Result, stored []storedPart) (View, error) {
	c, err := s.writeCapture(ctx, q, sc, p, captureID, stored)
	if err != nil {
		return View{}, err
	}
	v := s.view(c, res)
	v.Parts = partViews(stored)
	return v, nil
}

// processingForAttachment finds a capture of the same image still being
// read in the background (#780).
func (s *Service) processingForAttachment(ctx context.Context, sc engagementctx.Scope, attachmentID int64) (uuid.UUID, bool) {
	id, err := s.q.FindProcessingCaptureByAttachment(ctx, db.FindProcessingCaptureByAttachmentParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, AttachmentID: attachmentID,
	})
	return id, err == nil
}

func (s *Service) insertCapture(ctx context.Context, p db.InsertReceiptCaptureParams, res *extract.Result) (View, error) {
	c, err := s.q.InsertReceiptCapture(ctx, p)
	if err != nil {
		return View{}, err
	}
	return s.view(c, res), nil
}

func (s *Service) get(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (db.ReceiptCapture, error) {
	c, err := s.q.GetReceiptCapture(ctx, db.GetReceiptCaptureParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ReceiptCapture{}, ErrNotFound
	}
	return c, err
}

func receiptType(kind string) string {
	switch kind {
	case extract.KindFuel:
		return "fuel_receipt"
	case extract.KindExpense:
		return "okc_receipt"
	default:
		return "other"
	}
}

func ext(media string) string {
	return map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif"}[media]
}

func cleanName(n string) string {
	n = strings.TrimSpace(n)
	if i := strings.LastIndexAny(n, `/\`); i >= 0 {
		n = n[i+1:]
	}
	if n == "" {
		n = "receipt"
	}
	if len(n) > 200 {
		n = n[:200]
	}
	return n
}

func text(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func optID(v int64) pgtype.Int8 {
	if v <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}

func optDate(t time.Time, ok bool) pgtype.Date {
	if !ok {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: t, Valid: true}
}

func toNumeric(d decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

func optNum(d decimal.Decimal, ok bool) pgtype.Numeric {
	if !ok {
		return pgtype.Numeric{}
	}
	return toNumeric(d)
}

// syncExpenseApproval raises or resolves UNAPPROVED_EXPENSE for one
// expense: open while it waits for the office's decision (#529). Only adds
// information: failures are logged, never returned. An expense without a
// period (no date yet) is checked once it has one.
func (s *Service) syncExpenseApproval(ctx context.Context, sc engagementctx.Scope, e db.Expense) {
	if s.warnings == nil || !e.Period.Valid {
		return
	}
	var findings []engine.Finding
	if e.ApprovalStatus == "pending" {
		findings = []engine.Finding{{Code: "UNAPPROVED_EXPENSE", Params: map[string]any{}}}
	}
	id := e.ID
	_ = database.BestEffort(ctx, "receipt_expense_approval_warning", func(ctx context.Context) error {
		_, err := s.warnings.Apply(ctx, engine.Subject{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID},
			ExpenseApprovalProducer, engine.Entity{Type: catalog.EntityExpense, ID: &id, Period: e.Period.String},
			[]string{"UNAPPROVED_EXPENSE"}, findings)
		return err
	})
}

// syncExpenseApprovalByID loads the expense and syncs its warning.
func (s *Service) syncExpenseApprovalByID(ctx context.Context, sc engagementctx.Scope, id int64) {
	if s.warnings == nil {
		return
	}
	e, err := s.q.GetExpense(ctx, db.GetExpenseParams{TaxpayerID: sc.TaxpayerID, ID: id})
	if err != nil {
		return
	}
	s.syncExpenseApproval(ctx, sc, e)
}
