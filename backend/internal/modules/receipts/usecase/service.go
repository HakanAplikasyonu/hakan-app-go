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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// MaxImageBytes caps one photo.
const MaxImageBytes = 10 << 20

// ReviewBelow: fields under this confidence go to the review queue.
const ReviewBelow = 0.80

// Capture statuses.
const (
	StatusDraft     = "draft"
	StatusDuplicate = "duplicate"
	StatusFailed    = "failed"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
)

// PlateProducer is the data_warning producer of the plate check.
const PlateProducer = "control:plate"

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
	log      *slog.Logger
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
	if len(data) == 0 || len(data) > MaxImageBytes {
		return View{}, fmt.Errorf("%w: image must be 1 byte to 10 MB", ErrInvalid)
	}
	media := http.DetectContentType(data)
	if !mediaTypes[media] {
		return View{}, fmt.Errorf("%w: image must be JPEG, PNG, WebP or GIF (got %s)", ErrInvalid, media)
	}
	switch kindHint {
	case "", extract.KindExpense, extract.KindFuel, extract.KindZReport:
	default:
		return View{}, fmt.Errorf("%w: kind", ErrInvalid)
	}
	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])
	key := fmt.Sprintf("receipts/%d/%s%s", sc.TaxpayerID, hexSum, ext(media))
	if err := s.store.Upload(ctx, storage.File{Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: media, Filename: cleanName(fileName)}, key); err != nil {
		return View{}, fmt.Errorf("receipts: store image: %w", err)
	}
	att, err := s.q.UpsertAttachment(ctx, db.UpsertAttachmentParams{
		TaxpayerID: sc.TaxpayerID, Kind: "image", StorageKey: key, Sha256: hexSum, SizeBytes: int64(len(data)),
		ContentType: text(media), FileName: text(cleanName(fileName)),
	})
	if err != nil {
		return View{}, fmt.Errorf("receipts: attachment: %w", err)
	}
	base := db.InsertReceiptCaptureParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, UploadedBy: optID(userID),
		AttachmentID: att.ID, MediaType: media, Kind: text(kindHint), Fields: []byte("{}"),
	}
	// Same image again: no AI call, point at the earlier record.
	if !att.Inserted {
		if prev, ok := s.previousForAttachment(ctx, sc, att.ID); ok {
			base.Status, base.DuplicateOf = StatusDuplicate, pgtype.UUID{Bytes: prev, Valid: true}
			base.Error = text("same image was uploaded before")
			return s.insertCapture(ctx, base, nil)
		}
	}
	res, err := s.reader.Read(ctx, extract.Request{
		OrganizationID: sc.OrganizationID, TaxpayerID: sc.TaxpayerID, UserID: userID,
		MediaType: media, Image: data, KindHint: kindHint,
	})
	if err != nil {
		s.log.Warn("receipt_read_failed", "taxpayer", sc.TaxpayerID, "error", err)
		base.Status, base.Error = StatusFailed, text(err.Error())
		return s.insertCapture(ctx, base, nil)
	}
	fieldsJSON, _ := json.Marshal(res.Fields)
	base.Kind, base.Fields, base.Model = text(res.Kind), fieldsJSON, text(res.Model)

	if prev, ok := s.duplicateByFields(ctx, sc, res); ok {
		base.Status, base.DuplicateOf = StatusDuplicate, pgtype.UUID{Bytes: prev, Valid: true}
		base.Error = text("same vendor, date and amount were captured before")
		return s.insertCapture(ctx, base, &res)
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
	period := pgtype.Text{}
	if hasDate {
		period = text(date.Format("2006-01"))
	}
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
		draft = Draft{Type: "expense", ID: row.ID, UUID: row.Uuid}
		base.ExpenseID = pgtype.Int8{Int64: row.ID, Valid: true}
	}
	base.Status = StatusDraft
	capture, err := q.InsertReceiptCapture(ctx, base)
	if err != nil {
		return View{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return View{}, err
	}
	v := s.view(capture, &res)
	v.Draft = &draft

	// After the commit: checks that only add information.
	if draft.Type == "expense" {
		v.PlateCheck = s.checkPlate(ctx, sc, draft.ID, res, date, hasDate)
	}
	v.InReview = s.sendLowConfidence(ctx, sc, draft, res, period.String, att.ID)
	return v, nil
}

// checkPlate raises or resolves PLATE_NOT_REGISTERED (05 §3.3, T8).
func (s *Service) checkPlate(ctx context.Context, sc engagementctx.Scope, expenseID int64, res extract.Result, date time.Time, hasDate bool) string {
	plate := res.Get(extract.FieldPlate)
	if plate == "" {
		return "no_plate"
	}
	on := pgtype.Date{}
	if hasDate {
		on = pgtype.Date{Time: date, Valid: true}
	}
	ok, err := s.q.VehicleRegisteredOn(ctx, db.VehicleRegisteredOnParams{TaxpayerID: sc.TaxpayerID, Plate: plate, OnDate: on})
	if err != nil {
		s.log.Warn("plate_check_failed", "error", err)
		return "error"
	}
	result := "registered"
	var findings []engine.Finding
	if !ok {
		result = "not_registered"
		findings = []engine.Finding{{Code: "PLATE_NOT_REGISTERED", Params: map[string]any{"plate": plate, "date": res.Get(extract.FieldDate)}}}
	}
	if s.warnings != nil && hasDate {
		id := expenseID
		if _, err := s.warnings.Apply(ctx, engine.Subject{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID},
			PlateProducer, engine.Entity{Type: catalog.EntityExpense, ID: &id, Period: date.Format("2006-01")},
			[]string{"PLATE_NOT_REGISTERED"}, findings); err != nil {
			s.log.Warn("plate_warning_failed", "error", err)
		}
	}
	return result
}

// sendLowConfidence submits every present field under ReviewBelow (or
// one we could not normalize) to the review queue.
func (s *Service) sendLowConfidence(ctx context.Context, sc engagementctx.Scope, d Draft, res extract.Result, period string, attachmentID int64) []string {
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
		if err := s.reviewer.SubmitField(ctx, FieldReview{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID,
			TargetType: d.Type, TargetID: d.ID, Period: period, Field: f, Value: v, Model: res.Model, AttachmentID: attachmentID,
		}); err != nil {
			s.log.Warn("receipt_review_submit_failed", "field", f, "error", err)
		}
	}
	return out
}

// Decide approves or rejects a draft (accountant, T6: "beyana sadece onaylı
// masraf girer").
func (s *Service) Decide(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, userID int64, approve bool) (View, error) {
	c, err := s.get(ctx, sc, id)
	if err != nil {
		return View{}, err
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
	switch {
	case c.ExpenseID.Valid:
		if _, err := q.SetExpenseApproval(ctx, db.SetExpenseApprovalParams{ApprovalStatus: status, ApprovedBy: optID(userID), TaxpayerID: c.TaxpayerID, ID: c.ExpenseID.Int64}); err != nil {
			return View{}, fmt.Errorf("receipts: approve expense: %w", err)
		}
	case c.RetailSaleID.Valid:
		if _, err := q.SetRetailSaleApproval(ctx, db.SetRetailSaleApprovalParams{ApprovalStatus: status, ApprovedBy: optID(userID), TaxpayerID: c.TaxpayerID, ID: c.RetailSaleID.Int64}); err != nil {
			return View{}, fmt.Errorf("receipts: approve retail sale: %w", err)
		}
	default:
		return View{}, ErrState
	}
	c, err = q.SetReceiptCaptureStatus(ctx, db.SetReceiptCaptureStatusParams{Status: captureStatus, OrganizationID: c.OrganizationID, EngagementID: c.EngagementID, ID: c.ID})
	if err != nil {
		return View{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return View{}, err
	}
	return s.view(c, nil), nil
}

// Get returns one capture.
func (s *Service) Get(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (View, error) {
	c, err := s.get(ctx, sc, id)
	if err != nil {
		return View{}, err
	}
	return s.view(c, nil), nil
}

// List returns one page of captures.
func (s *Service) List(ctx context.Context, sc engagementctx.Scope, status string, limit, offset int32) ([]View, int64, error) {
	rows, err := s.q.ListReceiptCaptures(ctx, db.ListReceiptCapturesParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Status: text(status), Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountReceiptCaptures(ctx, db.CountReceiptCapturesParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Status: text(status)})
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
