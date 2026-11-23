// Package docuploads is the office's typed document upload (Oğuz'un demo
// geri bildirimi): uploading a document for a taxpayer, the office user picks
// its type (fatura, Z raporu, masraf, banka ekstresi, diğer) and its period
// (a month). The month is suggested from the document date (typed by the
// user or read from the photo) and falls back to the current month. When the
// chosen month is locked the user rejects the document, takes it in as a
// correction of the closed month (a correction declaration may be needed) or
// applies it to another, open month; the choice is kept with the upload.
//
// The file goes where that type already lives: Z reports and expense receipts
// to receipt capture (AI read → draft), e-invoice XML / ZIP to the UBL upload
// job, everything else to the taxpayer's document archive.
package docuploads

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/archive"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/upload"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	receiptsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// MaxFileBytes caps one file.
const MaxFileBytes = 20 << 20

// ReceiptCapturer is the receipt capture subset (*receiptsusecase.Service).
type ReceiptCapturer interface {
	CaptureKeyed(ctx context.Context, sc engagementctx.Scope, userID int64, fileName string, data []byte, kindHint, clientKey string) (receiptsusecase.View, error)
}

// InvoiceUploader is the UBL upload subset (*upload.Service).
type InvoiceUploader interface {
	Upload(ctx context.Context, scope engagementctx.Scope, actorID int64, fileName string, data []byte) (upload.Job, error)
}

// Archiver is the document archive subset (*archive.Service).
type Archiver interface {
	Upload(ctx context.Context, taxpayerID, actorID int64, fileName string, data []byte, in archive.Input) (archive.Document, error)
}

// Service runs typed uploads.
type Service struct {
	q        *db.Queries
	receipts ReceiptCapturer
	invoices InvoiceUploader
	archive  Archiver
	now      func() time.Time
}

// New builds the service; a nil target makes uploads of that type fail.
func New(pool database.Pool, receipts ReceiptCapturer, invoices InvoiceUploader, arch Archiver) *Service {
	return &Service{q: database.QueriesFor(pool), receipts: receipts, invoices: invoices, archive: arch, now: time.Now}
}

// Input is one upload.
type Input struct {
	DocType string
	// Period is the month the user picked (YYYY-MM).
	Period string
	// PeriodAuto: the user left the suggested month; a document date read
	// from the photo may then move it.
	PeriodAuto bool
	// DocumentDate (YYYY-MM-DD) typed by the user, optional.
	DocumentDate string
	// LockChoice / TargetPeriod answer a locked month.
	LockChoice   string
	TargetPeriod string
	FileName     string
	Data         []byte
}

// Upload is the API view of one typed upload.
type Upload struct {
	UUID            uuid.UUID `json:"uuid"`
	DocType         string    `json:"doc_type"`
	Period          string    `json:"period"`
	RequestedPeriod string    `json:"requested_period"`
	PeriodSource    string    `json:"period_source"`
	DocumentDate    *string   `json:"document_date"`
	LockChoice      *string   `json:"lock_choice"`
	LockedPeriod    *string   `json:"locked_period"`
	// OCRPeriod: the photo's date falls in this locked month, so the record
	// was written to the chosen month instead.
	OCRPeriod *string `json:"ocr_period"`
	// CorrectionNeeded: taken in as a correction of a locked month; a
	// correction declaration may be needed.
	CorrectionNeeded    bool       `json:"correction_needed"`
	Status              string     `json:"status"`
	FileName            string     `json:"file_name"`
	SizeBytes           int64      `json:"size_bytes"`
	ReceiptUUID         *uuid.UUID `json:"receipt_uuid"`
	UploadJobUUID       *uuid.UUID `json:"upload_job_uuid"`
	ArchiveDocumentUUID *uuid.UUID `json:"archive_document_uuid"`
	Error               string     `json:"error"`
	CreatedByName       string     `json:"created_by_name"`
	CreatedAt           time.Time  `json:"created_at"`
}

func oneOf(v string, set []string) bool {
	for _, s := range set {
		if v == s {
			return true
		}
	}
	return false
}

// fileKind sorts the bytes: image (what receipt capture reads), pdf, xml,
// zip; "" for anything else.
func fileKind(data []byte) string {
	ct := http.DetectContentType(data)
	switch {
	case ct == "image/jpeg", ct == "image/png", ct == "image/webp", ct == "image/gif":
		return "image"
	case strings.HasPrefix(ct, "application/pdf"):
		return "pdf"
	case strings.HasPrefix(ct, "application/zip"):
		return "zip"
	case strings.HasPrefix(ct, "text/xml"), strings.HasPrefix(ct, "application/xml"):
		return "xml"
	}
	trimmed := bytes.TrimLeft(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), " \t\r\n")
	if bytes.HasPrefix(trimmed, []byte("<")) {
		return "xml"
	}
	return ""
}

func (s *Service) locked(ctx context.Context, taxpayerID int64, period string) (bool, error) {
	_, err := s.q.GetActivePeriodLockForTaxpayer(ctx, db.GetActivePeriodLockForTaxpayerParams{TaxpayerID: taxpayerID, Period: period})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("docuploads: lock lookup: %w", err)
	}
	return true, nil
}

func text(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }

func optUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil} }

// Upload takes one document in.
func (s *Service) Upload(ctx context.Context, sc engagementctx.Scope, userID int64, in Input) (Upload, error) {
	if !oneOf(in.DocType, Types) {
		return Upload{}, fmt.Errorf("%w: doc_type must be invoice, z_report, expense, bank_statement or other", ErrInvalid)
	}
	if len(in.Data) == 0 || len(in.Data) > MaxFileBytes {
		return Upload{}, fmt.Errorf("%w: the file is empty or larger than %d MB", ErrInvalid, MaxFileBytes>>20)
	}
	var docDate *time.Time
	if d := strings.TrimSpace(in.DocumentDate); d != "" {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			return Upload{}, fmt.Errorf("%w: document_date must be YYYY-MM-DD", ErrInvalid)
		}
		docDate = &t
	}
	requested, source := strings.TrimSpace(in.Period), SourceManual
	if in.PeriodAuto {
		suggested, src := SuggestPeriod(docDate, s.now())
		if docDate != nil || !ValidPeriod(requested) {
			requested = suggested
		}
		source = src
	}
	if !ValidPeriod(requested) {
		return Upload{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	kind := fileKind(in.Data)
	sum := sha256.Sum256(in.Data)
	sha := hex.EncodeToString(sum[:])
	rec := db.InsertOfficeDocumentUploadParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, DocType: in.DocType,
		Period: requested, RequestedPeriod: requested, PeriodSource: source, FileName: cleanName(in.FileName),
		ContentType: http.DetectContentType(in.Data), SizeBytes: int64(len(in.Data)), Sha256: sha,
		CreatedBy: pgtype.Int8{Int64: userID, Valid: userID > 0},
	}
	if docDate != nil {
		rec.DocumentDate = pgtype.Date{Time: *docDate, Valid: true}
	}

	// e-Fatura XML / ZIP: the invoice's own date sets its period; a locked
	// month is held by the invoice write path as a change request.
	if in.DocType == TypeInvoice && (kind == "xml" || kind == "zip") {
		if s.invoices == nil {
			return Upload{}, errors.New("docuploads: invoice upload is not configured")
		}
		job, err := s.invoices.Upload(ctx, sc, userID, rec.FileName, in.Data)
		if err != nil {
			return Upload{}, err
		}
		rec.Status, rec.UploadJobUuid = StatusProcessed, optUUID(job.UUID)
		return s.insert(ctx, rec)
	}

	isLocked, err := s.locked(ctx, sc.TaxpayerID, requested)
	if err != nil {
		return Upload{}, err
	}
	targetLocked := false
	if isLocked && in.LockChoice == ChoiceApplySelected && ValidPeriod(in.TargetPeriod) {
		if targetLocked, err = s.locked(ctx, sc.TaxpayerID, in.TargetPeriod); err != nil {
			return Upload{}, err
		}
	}
	plan, err := Decide(requested, isLocked, in.LockChoice, strings.TrimSpace(in.TargetPeriod), targetLocked)
	if err != nil {
		return Upload{}, err
	}
	rec.Period, rec.LockChoice, rec.LockedPeriod = plan.Period, text(plan.LockChoice), text(plan.LockedPeriod)
	if plan.Reject {
		rec.Status = StatusRejected
		return s.insert(ctx, rec)
	}

	switch in.DocType {
	case TypeZReport, TypeExpense:
		if kind != "image" {
			return Upload{}, ErrNeedsImage
		}
		if err := s.capture(ctx, sc, userID, in, docDate == nil && in.PeriodAuto && plan.LockChoice == "", &rec); err != nil {
			return Upload{}, err
		}
	default:
		if kind != "pdf" && kind != "image" {
			return Upload{}, ErrUnsupported
		}
		if s.archive == nil {
			return Upload{}, errors.New("docuploads: document archive is not configured")
		}
		doc, err := s.archive.Upload(ctx, sc.TaxpayerID, userID, rec.FileName, in.Data, archive.Input{
			Category: "general", DocumentDate: strings.TrimSpace(in.DocumentDate),
			Note: fmt.Sprintf("Evrak yükleme: %s, dönem %s", typeLabel(in.DocType), rec.Period),
		})
		if errors.Is(err, archive.ErrExists) {
			return Upload{}, ErrExists
		}
		if errors.Is(err, archive.ErrUnsupported) {
			return Upload{}, ErrUnsupported
		}
		if err != nil {
			return Upload{}, err
		}
		rec.Status, rec.ArchiveDocumentUuid = StatusProcessed, optUUID(doc.UUID)
	}
	return s.insert(ctx, rec)
}

// capture sends a Z report / expense photo to receipt capture and writes the
// draft to the decided month. With followOCR the date read from the photo
// moves the month, unless that month is locked: then the record stays in
// the chosen month and OcrPeriod says why.
func (s *Service) capture(ctx context.Context, sc engagementctx.Scope, userID int64, in Input, followOCR bool, rec *db.InsertOfficeDocumentUploadParams) error {
	if s.receipts == nil {
		return errors.New("docuploads: receipt capture is not configured")
	}
	hint := extract.KindExpense
	if in.DocType == TypeZReport {
		hint = extract.KindZReport
	}
	v, err := s.receipts.CaptureKeyed(ctx, sc, userID, rec.FileName, in.Data, hint, "office-upload:"+in.DocType+":"+rec.Sha256)
	if err != nil {
		return err
	}
	rec.ReceiptUuid = optUUID(v.UUID)
	switch {
	case v.Replayed || v.Status == receiptsusecase.StatusDuplicate:
		rec.Status = StatusDuplicate
		return nil
	case v.Status == receiptsusecase.StatusFailed:
		rec.Status = StatusFailed
		if v.Error != nil {
			rec.Error = *v.Error
		}
		return nil
	}
	rec.Status = StatusProcessed
	if v.Draft == nil {
		return nil
	}
	var date pgtype.Date
	if v.Draft.Type == "retail_sale" {
		date, err = s.q.GetRetailSaleDraftDate(ctx, db.GetRetailSaleDraftDateParams{TaxpayerID: sc.TaxpayerID, Uuid: v.Draft.UUID})
	} else {
		date, err = s.q.GetExpenseDraftDate(ctx, db.GetExpenseDraftDateParams{TaxpayerID: sc.TaxpayerID, Uuid: v.Draft.UUID})
	}
	if err != nil {
		return fmt.Errorf("docuploads: draft date: %w", err)
	}
	if followOCR && date.Valid {
		read := date.Time.Format("2006-01")
		if read == rec.Period {
			rec.PeriodSource = SourceOCR
		} else {
			lockedRead, err := s.locked(ctx, sc.TaxpayerID, read)
			if err != nil {
				return err
			}
			if lockedRead {
				rec.OcrPeriod = text(read)
			} else {
				rec.Period, rec.PeriodSource = read, SourceOCR
			}
		}
	}
	period := pgtype.Text{String: rec.Period, Valid: true}
	if v.Draft.Type == "retail_sale" {
		_, err = s.q.SetRetailSaleDraftPeriod(ctx, db.SetRetailSaleDraftPeriodParams{Period: period, TaxpayerID: sc.TaxpayerID, Uuid: v.Draft.UUID})
	} else {
		_, err = s.q.SetExpenseDraftPeriod(ctx, db.SetExpenseDraftPeriodParams{Period: period, TaxpayerID: sc.TaxpayerID, Uuid: v.Draft.UUID})
	}
	if err != nil {
		return fmt.Errorf("docuploads: draft period: %w", err)
	}
	return nil
}

func (s *Service) insert(ctx context.Context, rec db.InsertOfficeDocumentUploadParams) (Upload, error) {
	row, err := s.q.InsertOfficeDocumentUpload(ctx, rec)
	if err != nil {
		return Upload{}, fmt.Errorf("docuploads: insert: %w", err)
	}
	return view(row, ""), nil
}

// List returns the engagement's latest uploads; period (YYYY-MM) narrows.
func (s *Service) List(ctx context.Context, sc engagementctx.Scope, period string) ([]Upload, error) {
	if period != "" && !ValidPeriod(period) {
		return nil, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	rows, err := s.q.ListOfficeDocumentUploads(ctx, db.ListOfficeDocumentUploadsParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Period: text(period), Lim: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("docuploads: list: %w", err)
	}
	out := make([]Upload, 0, len(rows))
	for _, r := range rows {
		out = append(out, view(db.OfficeDocumentUpload{
			ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, EngagementID: r.EngagementID, TaxpayerID: r.TaxpayerID,
			DocType: r.DocType, Period: r.Period, RequestedPeriod: r.RequestedPeriod, PeriodSource: r.PeriodSource,
			DocumentDate: r.DocumentDate, LockChoice: r.LockChoice, LockedPeriod: r.LockedPeriod, OcrPeriod: r.OcrPeriod,
			Status: r.Status, FileName: r.FileName, ContentType: r.ContentType, SizeBytes: r.SizeBytes, Sha256: r.Sha256,
			ReceiptUuid: r.ReceiptUuid, UploadJobUuid: r.UploadJobUuid, ArchiveDocumentUuid: r.ArchiveDocumentUuid,
			Error: r.Error, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
		}, r.CreatedByName))
	}
	return out, nil
}

func strPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func uuidPtr(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	id := uuid.UUID(u.Bytes)
	return &id
}

func view(r db.OfficeDocumentUpload, createdBy string) Upload {
	v := Upload{
		UUID: r.Uuid, DocType: r.DocType, Period: r.Period, RequestedPeriod: r.RequestedPeriod, PeriodSource: r.PeriodSource,
		LockChoice: strPtr(r.LockChoice), LockedPeriod: strPtr(r.LockedPeriod), OCRPeriod: strPtr(r.OcrPeriod),
		CorrectionNeeded: r.LockChoice.String == ChoiceCorrection, Status: r.Status, FileName: r.FileName,
		SizeBytes: r.SizeBytes, ReceiptUUID: uuidPtr(r.ReceiptUuid), UploadJobUUID: uuidPtr(r.UploadJobUuid),
		ArchiveDocumentUUID: uuidPtr(r.ArchiveDocumentUuid), Error: r.Error, CreatedByName: createdBy, CreatedAt: r.CreatedAt.Time,
	}
	if r.DocumentDate.Valid {
		d := r.DocumentDate.Time.Format("2006-01-02")
		v.DocumentDate = &d
	}
	return v
}

func typeLabel(t string) string {
	switch t {
	case TypeInvoice:
		return "fatura"
	case TypeBankStatement:
		return "banka ekstresi"
	default:
		return "diğer evrak"
	}
}

func cleanName(n string) string {
	n = strings.TrimSpace(n)
	if i := strings.LastIndexAny(n, `/\`); i >= 0 {
		n = n[i+1:]
	}
	if n == "" {
		n = "evrak"
	}
	if len(n) > 200 {
		n = n[len(n)-200:]
	}
	return n
}

// Settings is the office's "tür ve dönem seçerek yükleme" setting.
type Settings struct {
	Enabled   bool       `json:"enabled"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// Settings returns the office's setting; without a row it is on.
func (s *Service) Settings(ctx context.Context, orgID int64) (Settings, error) {
	row, err := s.q.GetDocumentUploadSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{Enabled: true}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("docuploads: settings: %w", err)
	}
	return settingsOf(row), nil
}

// SaveSettings stores the office's setting.
func (s *Service) SaveSettings(ctx context.Context, orgID, actorID int64, enabled bool) (Settings, error) {
	row, err := s.q.UpsertDocumentUploadSettings(ctx, db.UpsertDocumentUploadSettingsParams{
		OrganizationID: orgID, Enabled: enabled, UpdatedBy: pgtype.Int8{Int64: actorID, Valid: actorID > 0},
	})
	if err != nil {
		return Settings{}, fmt.Errorf("docuploads: save settings: %w", err)
	}
	return settingsOf(row), nil
}

func settingsOf(row db.DocumentUploadSetting) Settings {
	out := Settings{Enabled: row.Enabled}
	if row.UpdatedAt.Valid {
		t := row.UpdatedAt.Time
		out.UpdatedAt = &t
	}
	return out
}
