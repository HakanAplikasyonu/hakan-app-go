package foreigninvoices

// Mükelleften gelen yabancı hizmet faturası (#780 KDV2 A2): mükellef paneli
// yüklemesi ve WhatsApp uzantısının "Yabancı hizmet faturası" hedefi. Dosya
// mükellefin attachments deposuna yazılır (WhatsApp / belge görevi ile aynı
// içerik adresli anahtar); satır taslak açılır. Görsel ise AI satıcı, ülke,
// fatura no / tarih, tutar ve dövizi önerir (prefilled); hiçbir şey
// kendiliğinden onaylanmaz, ofis düzeltip onaylar.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// Source kinds of an invoice that came with a file.
const (
	SourceTaxpayerUpload = "taxpayer_upload"
	SourceWhatsApp       = "whatsapp"
	// SourceAgent: masaüstü ajanının izlediği klasörden gelen PDF (#780 e).
	SourceAgent = "agent"
)

// MaxFileBytes is the upload limit (same as the archive: 20 MB).
const MaxFileBytes = 20 << 20

// Intake errors.
var (
	// ErrUnsupported: the file is not a PDF or a photo.
	ErrUnsupported = errors.New("foreigninvoices: only PDF, JPEG, PNG, WebP and HEIC files are accepted")
	// ErrStorageMissing: no object storage configured.
	ErrStorageMissing = errors.New("foreigninvoices: storage is not configured")
)

// IntakeQueries is the db.Querier subset of the intake.
type IntakeQueries interface {
	GetForeignInvoiceBySource(ctx context.Context, arg db.GetForeignInvoiceBySourceParams) (db.ForeignServiceInvoice, error)
	CreateForeignInvoiceIntake(ctx context.Context, arg db.CreateForeignInvoiceIntakeParams) (db.ForeignServiceInvoice, error)
	GetForeignInvoiceSourceFile(ctx context.Context, arg db.GetForeignInvoiceSourceFileParams) (db.GetForeignInvoiceSourceFileRow, error)
	ListForeignInvoices(ctx context.Context, arg db.ListForeignInvoicesParams) ([]db.ForeignServiceInvoice, error)
	UpsertAttachment(ctx context.Context, arg db.UpsertAttachmentParams) (db.UpsertAttachmentRow, error)
}

// ObjectStore is the object storage subset.
type ObjectStore interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// Prefiller reads a foreign invoice image (*extract.Extractor).
type Prefiller interface {
	ReadForeignInvoice(ctx context.Context, req extract.Request) (extract.ForeignInvoice, error)
}

// Intake turns an uploaded file into a draft invoice.
type Intake struct {
	svc    *Service
	q      IntakeQueries
	store  ObjectStore
	reader Prefiller
	log    *slog.Logger
	now    func() time.Time
}

// NewIntake builds the intake. store nil: uploads fail with
// ErrStorageMissing; reader nil: no AI suggestion (manual entry).
func NewIntake(svc *Service, q IntakeQueries, store ObjectStore, reader Prefiller, log *slog.Logger) *Intake {
	if log == nil {
		log = slog.Default()
	}
	return &Intake{svc: svc, q: q, store: store, reader: reader, log: log, now: time.Now}
}

// Enabled reports whether the office has foreign invoices on.
func (in *Intake) Enabled(ctx context.Context, orgID int64) (bool, error) {
	return in.svc.enabled(ctx, orgID)
}

// Enabled reports whether the office has the section on (the automation
// gate of the "Yabancı faturalarınızı gönderin" rule).
func (s *Service) Enabled(ctx context.Context, orgID int64) (bool, error) {
	return s.enabled(ctx, orgID)
}

// IntakeInput is one file.
type IntakeInput struct {
	Source   string
	FileName string
	Data     []byte
	Note     string
	UserID   int64
}

// IntakeResult is the draft row; Replayed: the same file came before from
// the same source, the first row is answered.
type IntakeResult struct {
	Item     Item `json:"item"`
	Replayed bool `json:"replayed"`
}

// Sniff returns the content type, the attachment kind and the extension of
// an accepted file.
func Sniff(data []byte) (contentType, kind, ext string, ok bool) {
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		switch string(data[8:12]) {
		case "heic", "heix", "hevc", "heim", "heis", "mif1", "msf1":
			return "image/heic", "image", ".heic", true
		}
	}
	switch ct := http.DetectContentType(data); {
	case strings.HasPrefix(ct, "application/pdf"):
		return "application/pdf", "pdf", ".pdf", true
	case ct == "image/jpeg":
		return ct, "image", ".jpg", true
	case ct == "image/png":
		return ct, "image", ".png", true
	case ct == "image/webp":
		return ct, "image", ".webp", true
	}
	return "", "", "", false
}

func cleanFileName(name, ext string) string {
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		name = "yabanci-fatura"
	}
	if r := []rune(name); len(r) > 120 {
		name = string(r[:120])
	}
	if !strings.HasSuffix(strings.ToLower(name), ext) {
		name += ext
	}
	return name
}

// Submit stores the file and opens a draft invoice for the engagement.
// taxpayerID is the engagement's taxpayer (the attachments owner).
func (in *Intake) Submit(ctx context.Context, sub Subject, taxpayerID int64, f IntakeInput) (IntakeResult, error) {
	if f.Source != SourceTaxpayerUpload && f.Source != SourceWhatsApp && f.Source != SourceAgent {
		return IntakeResult{}, fmt.Errorf("%w: unknown source", ErrInvalid)
	}
	on, err := in.svc.enabled(ctx, sub.OrganizationID)
	if err != nil {
		return IntakeResult{}, err
	}
	if !on {
		return IntakeResult{}, ErrDisabled
	}
	note := strings.TrimSpace(f.Note)
	if len([]rune(note)) > 1000 {
		return IntakeResult{}, fmt.Errorf("%w: note is longer than 1000 characters", ErrInvalid)
	}
	if len(f.Data) == 0 || len(f.Data) > MaxFileBytes {
		return IntakeResult{}, fmt.Errorf("%w: the file is empty or larger than 20 MB", ErrInvalid)
	}
	ct, kind, ext, ok := Sniff(f.Data)
	if !ok {
		return IntakeResult{}, ErrUnsupported
	}
	if in.store == nil {
		return IntakeResult{}, ErrStorageMissing
	}
	sum := sha256.Sum256(f.Data)
	sha := hex.EncodeToString(sum[:])
	name := cleanFileName(f.FileName, ext)
	key := fmt.Sprintf("attachments/%d/%s%s", taxpayerID, sha, ext)
	if err := in.store.Upload(ctx, storage.File{Body: bytes.NewReader(f.Data), Size: int64(len(f.Data)), ContentType: ct, Filename: name}, key); err != nil {
		return IntakeResult{}, fmt.Errorf("foreigninvoices: store file: %w", err)
	}
	att, err := in.q.UpsertAttachment(ctx, db.UpsertAttachmentParams{
		TaxpayerID: taxpayerID, Kind: kind, StorageKey: key, Sha256: sha, SizeBytes: int64(len(f.Data)),
		ContentType: pgtype.Text{String: ct, Valid: true}, FileName: pgtype.Text{String: name, Valid: true},
	})
	if err != nil {
		return IntakeResult{}, fmt.Errorf("foreigninvoices: attachment: %w", err)
	}
	if row, ok, err := in.bySource(ctx, sub, f.Source, att.Uuid); err != nil || ok {
		return IntakeResult{Item: itemFromRow(row), Replayed: ok}, err
	}

	// AI suggestion (images only; a PDF stays for manual entry).
	var read extract.ForeignInvoice
	if kind == "image" && ct != "image/heic" && in.reader != nil {
		r, err := in.reader.ReadForeignInvoice(ctx, extract.Request{
			OrganizationID: sub.OrganizationID, TaxpayerID: taxpayerID, UserID: f.UserID, MediaType: ct, Image: f.Data,
		})
		switch {
		case err == nil:
			read = r
		case errors.Is(err, extract.ErrAIUnavailable):
		default:
			in.log.Warn("foreign_invoice_prefill_failed", "organization_id", sub.OrganizationID, "error", err)
		}
	}
	p, err := in.draftParams(ctx, sub, read)
	if err != nil {
		return IntakeResult{}, err
	}
	p.SourceKind, p.SourceUuid, p.Note, p.CreatedBy = f.Source, att.Uuid, text(note), optID(f.UserID)
	row, err := in.q.CreateForeignInvoiceIntake(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		// A concurrent request opened the same source first.
		row, ok, err := in.bySource(ctx, sub, f.Source, att.Uuid)
		if err == nil && !ok {
			err = ErrNotFound
		}
		return IntakeResult{Item: itemFromRow(row), Replayed: true}, err
	}
	if err != nil {
		return IntakeResult{}, fmt.Errorf("foreigninvoices: create draft: %w", err)
	}
	return IntakeResult{Item: itemFromRow(row)}, nil
}

func (in *Intake) bySource(ctx context.Context, sub Subject, source string, id uuid.UUID) (db.ForeignServiceInvoice, bool, error) {
	row, err := in.q.GetForeignInvoiceBySource(ctx, db.GetForeignInvoiceBySourceParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID,
		SourceKind: pgtype.Text{String: source, Valid: true}, SourceUuid: pgtype.UUID{Bytes: id, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, false, nil
	}
	if err != nil {
		return row, false, fmt.Errorf("foreigninvoices: find source: %w", err)
	}
	return row, true, nil
}

// draftParams fills a draft from what the AI read: the period is the
// invoice month (not in the future), otherwise the upload month; the
// service kind's office defaults give treatment and rates.
func (in *Intake) draftParams(ctx context.Context, sub Subject, r extract.ForeignInvoice) (db.CreateForeignInvoiceIntakeParams, error) {
	now := in.now()
	p := db.CreateForeignInvoiceIntakeParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Period: CurrentPeriod(now),
		Vendor: r.Vendor, Country: r.Country, InvoiceNo: r.InvoiceNo, Currency: r.Currency,
		Amount: num(decimal.Zero), Prefilled: !r.Empty(),
	}
	if p.Currency == "" {
		p.Currency = "USD"
	}
	if r.InvoiceDate != "" {
		if t, err := time.Parse("2006-01-02", r.InvoiceDate); err == nil && !t.After(now) {
			p.InvoiceDate = pgtype.Date{Time: t, Valid: true}
			p.Period = t.Format("2006-01")
		}
	}
	if r.Amount != "" {
		if a, err := decimal.NewFromString(r.Amount); err == nil && a.IsPositive() && a.LessThan(maxAmount) {
			p.Amount = num(a.Round(2))
			if p.Currency == "TRY" {
				p.ExchangeRate = num(decimal.NewFromInt(1))
			}
		}
	}
	kinds, err := in.svc.kindDefaults(ctx, sub.OrganizationID)
	if err != nil {
		return p, err
	}
	kind := r.ServiceKind
	if _, ok := kinds[kind]; !ok {
		kind = KindOther
	}
	d := defaultFrom(kinds[kind])
	p.ServiceKind, p.Treatment, p.GrossUp = kind, d.Treatment, d.GrossUp
	p.StopajRate, p.VatRate = num(d.StopajRate), num(d.VATRate)
	if d.Treatment == TreatmentKDV2 {
		p.StopajRate, p.GrossUp = num(decimal.Zero), false
	}
	// Otomatik TCMB kuru (#780): faturanın tarihinden önceki son bülten.
	if p.Currency != "TRY" && p.InvoiceDate.Valid {
		n := normalized{Currency: p.Currency, InvoiceDate: &p.InvoiceDate.Time}
		if err := in.svc.autoRate(ctx, sub.OrganizationID, &n); err != nil {
			return p, err
		}
		if n.Rate != nil {
			p.ExchangeRate, p.ExchangeRateSource, p.ExchangeRateDate = num(*n.Rate), text(n.RateSource), date(n.RateDate)
		}
	}
	return p, nil
}

// File is a source document to stream back.
type File struct {
	Body        io.ReadCloser
	Size        int64
	ContentType string
	Name        string
}

// SourceFile opens the file an invoice came with.
func (in *Intake) SourceFile(ctx context.Context, sub Subject, id uuid.UUID) (File, error) {
	r, err := in.q.GetForeignInvoiceSourceFile(ctx, db.GetForeignInvoiceSourceFileParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Uuid: id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, fmt.Errorf("foreigninvoices: source file: %w", err)
	}
	if in.store == nil {
		return File{}, ErrStorageMissing
	}
	rc, size, err := in.store.Download(ctx, r.StorageKey)
	if err != nil {
		return File{}, fmt.Errorf("foreigninvoices: download: %w", err)
	}
	f := File{Body: rc, Size: size, ContentType: "application/octet-stream", Name: "yabanci-fatura"}
	if r.ContentType.Valid && r.ContentType.String != "" {
		f.ContentType = r.ContentType.String
	}
	if r.FileName.Valid && r.FileName.String != "" {
		f.Name = r.FileName.String
	}
	return f, nil
}

// TaxpayerItem is one invoice the taxpayer sent, read only.
type TaxpayerItem struct {
	UUID        uuid.UUID `json:"uuid"`
	Vendor      string    `json:"vendor"`
	InvoiceDate *string   `json:"invoice_date"`
	// Status: in_review (ofiste inceleniyor) or processed (işlendi).
	Status     string    `json:"status"`
	SourceKind string    `json:"source_kind"`
	CreatedAt  time.Time `json:"created_at"`
}

// TaxpayerList is GET /v1/taxpayer/foreign-invoices. Enabled false: the
// office turned the section off; the panel hides the upload.
type TaxpayerList struct {
	Enabled bool           `json:"enabled"`
	Period  string         `json:"period"`
	Count   int            `json:"count"`
	Items   []TaxpayerItem `json:"items"`
}

// TaxpayerStatus maps a row status to the taxpayer's words.
func TaxpayerStatus(status string) string {
	if status == StatusConfirmed {
		return "processed"
	}
	return "in_review"
}

// ForTaxpayer lists the invoices the taxpayer sent (panel upload or
// WhatsApp) in period (current month when empty).
func (in *Intake) ForTaxpayer(ctx context.Context, sub Subject, period string) (TaxpayerList, error) {
	if period == "" {
		period = CurrentPeriod(in.now())
	}
	if !ValidPeriod(period) {
		return TaxpayerList{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	out := TaxpayerList{Period: period, Items: []TaxpayerItem{}}
	on, err := in.svc.enabled(ctx, sub.OrganizationID)
	if err != nil || !on {
		return out, err
	}
	out.Enabled = true
	rows, err := in.q.ListForeignInvoices(ctx, db.ListForeignInvoicesParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Period: period,
	})
	if err != nil {
		return TaxpayerList{}, fmt.Errorf("foreigninvoices: list: %w", err)
	}
	for _, r := range rows {
		if !r.SourceKind.Valid || (r.SourceKind.String != SourceTaxpayerUpload && r.SourceKind.String != SourceWhatsApp) {
			continue
		}
		it := TaxpayerItem{UUID: r.Uuid, Vendor: r.Vendor, Status: TaxpayerStatus(r.Status), SourceKind: r.SourceKind.String, CreatedAt: r.CreatedAt.Time}
		if r.InvoiceDate.Valid {
			s := r.InvoiceDate.Time.Format("2006-01-02")
			it.InvoiceDate = &s
		}
		out.Items = append(out.Items, it)
	}
	out.Count = len(out.Items)
	return out, nil
}
