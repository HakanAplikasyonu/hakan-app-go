// Package taxpayerportal serves the mükellef web panel (issue #39, D3):
// a taxpayer user reads its own invoices and receipts and uploads e-invoice
// files. Every route runs behind RequireTaxpayer (X-Taxpayer); queries are
// bounded by the resolved taxpayer_id, so a user never sees another company.
package taxpayerportal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/upload"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/view"
	receiptshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/handler"
	receiptsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/pagecursor"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Uploader is the UBL upload service subset the panel uses.
type Uploader interface {
	Upload(ctx context.Context, scope engagementctx.Scope, actorID int64, fileName string, data []byte) (upload.Job, error)
}

// Handler serves /v1/taxpayer/{invoices,receipts,uploads}.
type Handler struct {
	q        *db.Queries
	uploads  Uploader
	receipts ReceiptCapturer
}

// ReceiptCapturer is the receipt capture service subset (#199).
// *receiptsusecase.Service implements it.
type ReceiptCapturer interface {
	CaptureKeyed(ctx context.Context, sc engagementctx.Scope, userID int64, fileName string, data []byte, kindHint, clientKey string) (receiptsusecase.View, error)
	Correct(ctx context.Context, sc engagementctx.Scope, captureUUID uuid.UUID, userID int64, in []receiptsusecase.Correction) ([]string, error)
}

// WithReceipts enables receipt upload and corrections from the taxpayer
// panel and the mobile app (#199). Without it those routes answer 503.
func (h *Handler) WithReceipts(svc ReceiptCapturer) *Handler {
	h.receipts = svc
	return h
}

// NewHandler builds the handler.
func NewHandler(q *db.Queries, uploads Uploader) *Handler { return &Handler{q: q, uploads: uploads} }

// RegisterRoutes mounts the panel routes. A nil resolver registers nothing.
func RegisterRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader, tpResolver taxpayerctx.Resolver, h *Handler) {
	if tpResolver == nil || h == nil {
		return
	}
	mount(mux, h, middleware.Authenticate(tokens, loader), middleware.RequireTaxpayer(tpResolver))
}

// mount registers the routes behind mws (authentication, then RequireTaxpayer).
func mount(mux *http.ServeMux, h *Handler, mws ...func(http.Handler) http.Handler) {
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, middleware.Chain(fn, mws...))
	}
	route("GET /v1/taxpayer/invoices", h.listInvoices)
	route("GET /v1/taxpayer/invoices/{uuid}", h.getInvoice)
	route("GET /v1/taxpayer/receipts", h.listReceipts)
	route("POST /v1/taxpayer/receipts", h.createReceipt)
	route("POST /v1/taxpayer/receipts/{uuid}/corrections", h.correctReceipt)
	route("GET /v1/taxpayer/uploads", h.listUploads)
	route("POST /v1/taxpayer/uploads", h.createUpload)
	route("GET /v1/taxpayer/uploads/{uuid}", h.getUpload)
}

// Invoice is the panel's invoice row. Amounts are decimal strings.
type Invoice = view.Invoice

type invoicePage struct {
	Items  []Invoice `json:"items"`
	Total  int64     `json:"total"`
	Limit  int32     `json:"limit"`
	Offset int32     `json:"offset"`
	// NextCursor fetches the next page with ?cursor= (#182); null on the
	// last page.
	NextCursor *string `json:"next_cursor"`
}

func (h *Handler) listInvoices(w http.ResponseWriter, r *http.Request) {
	s := taxpayerctx.Must(r.Context())
	limit, offset := page(r)
	period := strings.TrimSpace(r.URL.Query().Get("period"))
	direction := strings.TrimSpace(r.URL.Query().Get("direction"))
	if direction != "" && direction != "outgoing" && direction != "incoming" {
		response.Error(w, r, http.StatusBadRequest, "INVALID_DIRECTION", "direction must be outgoing or incoming")
		return
	}
	var rows []db.ListTaxpayerInvoicesRow
	if c := strings.TrimSpace(r.URL.Query().Get("cursor")); c != "" {
		date, id, err := pagecursor.Decode(c)
		if err != nil {
			response.Error(w, r, http.StatusBadRequest, "INVALID_CURSOR", "cursor is not valid")
			return
		}
		offset = 0
		after, err := h.q.ListTaxpayerInvoicesAfter(r.Context(), db.ListTaxpayerInvoicesAfterParams{
			TaxpayerID: s.ID, CursorDate: pgtype.Date{Time: date, Valid: true}, CursorID: id,
			Period: repository.Text(period), Direction: repository.Text(direction), Lim: limit,
		})
		if err != nil {
			response.InternalErr(w, r, err, "could not list invoices")
			return
		}
		for _, a := range after {
			rows = append(rows, db.ListTaxpayerInvoicesRow(a))
		}
	} else {
		var err error
		rows, err = h.q.ListTaxpayerInvoices(r.Context(), db.ListTaxpayerInvoicesParams{
			TaxpayerID: s.ID, Period: repository.Text(period), Direction: repository.Text(direction), Lim: limit, Off: offset,
		})
		if err != nil {
			response.InternalErr(w, r, err, "could not list invoices")
			return
		}
	}
	total, err := h.q.CountTaxpayerInvoices(r.Context(), db.CountTaxpayerInvoicesParams{TaxpayerID: s.ID, Period: repository.Text(period), Direction: repository.Text(direction)})
	if err != nil {
		response.InternalErr(w, r, err, "could not count invoices")
		return
	}
	out := invoicePage{Items: make([]Invoice, 0, len(rows)), Total: total, Limit: limit, Offset: offset}
	for _, row := range rows {
		out.Items = append(out.Items, Invoice{
			UUID: row.Uuid, ETTN: uuidText(row.Ettn), InvoiceNumber: row.InvoiceNumber, Direction: row.Direction, Type: row.Type,
			Profile: row.Profile.String, IssueDate: row.IssueDate.Time.Format("2006-01-02"), Period: row.Period,
			CounterpartyName: row.CustomerName.String, CounterpartyID: firstNonEmpty(row.CustomerTaxNumber.String, row.CustomerIdentityNumber.String),
			Currency: row.Currency, TaxExclusive: money(row.TaxExclusiveAmount), TaxAmount: money(row.TaxAmount),
			Withholding: money(row.WithholdingAmount), Payable: money(row.PayableAmount),
			Status: row.StatusNormalized, SourceSystem: row.SourceSystem, CaptureMethod: row.CaptureMethod,
		})
	}
	if n := len(rows); n > 0 && n == int(limit) {
		next := pagecursor.Encode(rows[n-1].IssueDate.Time, rows[n-1].ID)
		out.NextCursor = &next
	}
	response.JSON(w, r, http.StatusOK, out)
}

// InvoiceDetail is one invoice with lines, KDV summary and sources.
type InvoiceDetail = view.Detail

func (h *Handler) getInvoice(w http.ResponseWriter, r *http.Request) {
	s := taxpayerctx.Must(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "invoice not found")
		return
	}
	ctx := r.Context()
	inv, err := h.q.GetInvoice(ctx, db.GetInvoiceParams{TaxpayerID: s.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "invoice not found")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not load invoice")
		return
	}
	out, err := view.LoadDetail(ctx, h.q, inv)
	if err != nil {
		response.InternalErr(w, r, err, "could not load invoice details")
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Receipt is a captured receipt (fiş) as the panel shows it.
type Receipt struct {
	UUID        uuid.UUID `json:"uuid"`
	Kind        string    `json:"kind,omitempty"`
	Status      string    `json:"status"`
	MediaType   string    `json:"media_type"`
	VendorName  string    `json:"vendor_name,omitempty"`
	Date        string    `json:"date,omitempty"`
	TotalAmount string    `json:"total_amount,omitempty"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type receiptPage struct {
	Items  []Receipt `json:"items"`
	Total  int64     `json:"total"`
	Limit  int32     `json:"limit"`
	Offset int32     `json:"offset"`
}

func (h *Handler) listReceipts(w http.ResponseWriter, r *http.Request) {
	s := taxpayerctx.Must(r.Context())
	limit, offset := page(r)
	rows, err := h.q.ListTaxpayerReceiptCaptures(r.Context(), db.ListTaxpayerReceiptCapturesParams{TaxpayerID: s.ID, Lim: limit, Off: offset})
	if err != nil {
		response.InternalErr(w, r, err, "could not list receipts")
		return
	}
	total, err := h.q.CountTaxpayerReceiptCaptures(r.Context(), s.ID)
	if err != nil {
		response.InternalErr(w, r, err, "could not count receipts")
		return
	}
	out := receiptPage{Items: make([]Receipt, 0, len(rows)), Total: total, Limit: limit, Offset: offset}
	for _, row := range rows {
		f := receiptFields(row.Fields)
		out.Items = append(out.Items, Receipt{
			UUID: row.Uuid, Kind: row.Kind.String, Status: row.Status, MediaType: row.MediaType,
			VendorName: f["vendor_name"], Date: f["date"], TotalAmount: f["total_amount"], Error: row.Error.String, CreatedAt: row.CreatedAt.Time,
		})
	}
	response.JSON(w, r, http.StatusOK, out)
}

// receiptFields reads {"name": {"value": "..."}} from the extraction result.
func receiptFields(raw []byte) map[string]string {
	var m map[string]struct {
		Value string `json:"value"`
	}
	out := map[string]string{}
	if json.Unmarshal(raw, &m) != nil {
		return out
	}
	for k, v := range m {
		out[k] = v.Value
	}
	return out
}

type uploadPage struct {
	Items  []upload.Job `json:"items"`
	Limit  int32        `json:"limit"`
	Offset int32        `json:"offset"`
}

func (h *Handler) listUploads(w http.ResponseWriter, r *http.Request) {
	s := taxpayerctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	limit, offset := page(r)
	rows, err := h.q.ListTaxpayerUploadJobs(r.Context(), db.ListTaxpayerUploadJobsParams{TaxpayerID: s.ID, ActorID: p.UserInternal, Lim: limit, Off: offset})
	if err != nil {
		response.InternalErr(w, r, err, "could not list uploads")
		return
	}
	out := uploadPage{Items: make([]upload.Job, 0, len(rows)), Limit: limit, Offset: offset}
	for _, row := range rows {
		j := upload.JobView(row)
		if j.Report != nil {
			j.Report.Items = nil
		}
		out.Items = append(out.Items, j)
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) getUpload(w http.ResponseWriter, r *http.Request) {
	s := taxpayerctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "upload not found")
		return
	}
	row, err := h.q.GetTaxpayerUploadJob(r.Context(), db.GetTaxpayerUploadJobParams{TaxpayerID: s.ID, ActorID: p.UserInternal, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "upload not found")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not load upload")
		return
	}
	response.JSON(w, r, http.StatusOK, upload.JobView(row))
}

// createUpload accepts an e-invoice XML / ZIP from the taxpayer. The file is
// processed for the taxpayer's active office engagement (D1: one at a time),
// exactly like an office upload; without an office there is nobody to
// process it.
func (h *Handler) createUpload(w http.ResponseWriter, r *http.Request) {
	p, _ := authctx.PrincipalFrom(r.Context())
	scope, ok := h.activeScope(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, upload.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		response.Error(w, r, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", "file is too large or the form is invalid")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "FILE_REQUIRED", "multipart field \"file\" is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, upload.MaxUploadBytes+1))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "FILE_UNREADABLE", "could not read the file")
		return
	}
	var job upload.Job
	err = forOffice(r.Context(), scope, func(ctx context.Context) error {
		var err error
		job, err = h.uploads.Upload(ctx, scope, p.UserInternal, header.Filename, data)
		return err
	})
	switch {
	case errors.Is(err, upload.ErrEmpty):
		response.Error(w, r, http.StatusBadRequest, "FILE_EMPTY", "the file is empty")
	case errors.Is(err, upload.ErrTooLarge):
		response.Error(w, r, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", "file is too large")
	case errors.Is(err, upload.ErrUnsupported):
		response.Error(w, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_FILE", "upload an XML, ZIP or base64 file")
	case err != nil:
		response.InternalErr(w, r, err, "upload failed")
	default:
		response.JSON(w, r, http.StatusAccepted, job)
	}
}

func page(r *http.Request) (limit, offset int32) {
	limit = 20
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 100 {
		limit = int32(v) //nolint:gosec // bounded
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 && v <= 1_000_000 {
		offset = int32(v) //nolint:gosec // bounded
	}
	return limit, offset
}

func money(n pgtype.Numeric) string { return repository.Dec(n).StringFixed(2) }

func uuidText(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuid.UUID(u.Bytes).String()
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// activeScope is the engagement of the company's current office: uploads and
// receipts are processed there. Without one it answers 409.
func (h *Handler) activeScope(w http.ResponseWriter, r *http.Request) (engagementctx.Scope, bool) {
	s := taxpayerctx.Must(r.Context())
	eng, err := h.q.GetActiveEngagementForTaxpayer(r.Context(), s.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		response.Error(w, r, http.StatusConflict, "NO_ACTIVE_ENGAGEMENT", "the company has no active office to process uploads")
		return engagementctx.Scope{}, false
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not resolve office")
		return engagementctx.Scope{}, false
	}
	return engagementctx.Scope{ID: eng.ID, UUID: eng.Uuid, OrganizationID: eng.OrganizationID,
		TaxpayerID: s.ID, TaxpayerUUID: s.UUID, Status: eng.Status}, true
}

// forOffice runs a mükellef upload as work handed to the engagement's office
// (issue #281). The panel request is in a taxpayer scope; the receipt,
// review queue, data warning and upload code writes the office's records
// (suggestions, data_warnings, ...), which the office policies refuse in
// that scope. rls.ForOffice switches the transaction to the office scope
// narrowed to this taxpayer for the call, after checking that the office
// serves the scope's own taxpayer.
func forOffice(ctx context.Context, sc engagementctx.Scope, fn func(ctx context.Context) error) error {
	return rls.ForOffice(ctx, sc.OrganizationID, sc.TaxpayerID, fn)
}

// createReceipt: POST /v1/taxpayer/receipts (multipart: image, kind;
// Idempotency-Key header). The mobile app's capture (#199): the photo is read
// for the company's current office like an office-side capture. A retry with
// the same Idempotency-Key returns the first record with 200.
func (h *Handler) createReceipt(w http.ResponseWriter, r *http.Request) {
	if h.receipts == nil {
		response.Error(w, r, http.StatusServiceUnavailable, "RECEIPTS_UNAVAILABLE", "receipt capture is not available")
		return
	}
	scope, ok := h.activeScope(w, r)
	if !ok {
		return
	}
	name, data, ok := receiptshandler.ReadImage(w, r)
	if !ok {
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	var v receiptsusecase.View
	err := forOffice(r.Context(), scope, func(ctx context.Context) error {
		var err error
		v, err = h.receipts.CaptureKeyed(ctx, scope, p.UserInternal, name, data, r.FormValue("kind"), r.Header.Get("Idempotency-Key"))
		return err
	})
	receiptshandler.WriteCapture(w, r, v, err)
}

type correctionRequest struct {
	Corrections []receiptsusecase.Correction `json:"corrections"`
}

type correctionResult struct {
	// Submitted are the fields sent to the office's review queue.
	Submitted []string `json:"submitted"`
}

// correctReceipt: POST /v1/taxpayer/receipts/{uuid}/corrections. The values
// go to the office's review queue; the accountant decides (#199).
func (h *Handler) correctReceipt(w http.ResponseWriter, r *http.Request) {
	if h.receipts == nil {
		response.Error(w, r, http.StatusServiceUnavailable, "RECEIPTS_UNAVAILABLE", "receipt capture is not available")
		return
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "receipt not found")
		return
	}
	var in correctionRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	scope, ok := h.activeScope(w, r)
	if !ok {
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	var fields []string
	err = forOffice(r.Context(), scope, func(ctx context.Context) error {
		var err error
		fields, err = h.receipts.Correct(ctx, scope, id, p.UserInternal, in.Corrections)
		return err
	})
	if err != nil {
		receiptshandler.WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, correctionResult{Submitted: fields})
}
