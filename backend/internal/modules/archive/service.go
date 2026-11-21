// Package archive is the taxpayer document archive (#393): past documents the
// office keeps for a mükellef (contracts, tax, SGK, trade registry) and İSG
// documents (training certificates, risk assessments, health reports) with
// their validity, read by the office mobile app.
//
// Rows are taxpayer-scoped (taxpayer_documents, rls_apply_taxpayer) like the
// payroll PDFs: the office reaches them through an active engagement.
package archive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// Errors.
var (
	ErrNotFound       = errors.New("archive: not found")
	ErrInvalid        = errors.New("archive: invalid input")
	ErrUnsupported    = errors.New("archive: only PDF, JPEG and PNG files are accepted")
	ErrExists         = errors.New("archive: the same file is already in the archive")
	ErrStorageMissing = errors.New("archive: storage is not configured")
)

// MaxFileBytes is the upload limit.
const MaxFileBytes = 20 << 20

// ExpiringDays: a document whose validity ends within this many days is
// "expiring" (yaklaşan bitiş uyarısı).
const ExpiringDays = 30

// Categories and İSG kinds.
var (
	Categories = []string{"general", "contract", "tax", "sgk", "trade_registry", "isg", "other"}
	ISGKinds   = []string{"isg_training", "risk_assessment", "health_report", "other"}
	// ImportantKinds are the kinds of a mükellef's important documents
	// (#780 B): vergi levhası, imza sirküleri, ticaret sicil gazetesi, kira
	// sözleşmesi, ruhsat, vekâletname, diğer.
	ImportantKinds = []string{"tax_certificate", "signature_circular", "trade_registry_gazette",
		"lease_contract", "license", "power_of_attorney", "other"}
)

func oneOf(v string, set []string) bool {
	for _, s := range set {
		if v == s {
			return true
		}
	}
	return false
}

// ObjectStore is the object storage the files live in.
type ObjectStore interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, path string) error
}

// Service runs the archive.
type Service struct {
	q     *db.Queries
	store ObjectStore
	now   func() time.Time
}

// New builds the service; a nil store makes uploads and downloads fail with
// ErrStorageMissing (listing still works).
func New(pool database.Ambient, store ObjectStore) *Service {
	return &Service{q: db.New(pool), store: store, now: time.Now}
}

// Document is the API view of an archived document.
type Document struct {
	UUID              uuid.UUID `json:"uuid"`
	Category          string    `json:"category"`
	ISGKind           *string   `json:"isg_kind"`
	Title             string    `json:"title"`
	Note              string    `json:"note"`
	DocumentDate      *string   `json:"document_date"`
	ValidUntil        *string   `json:"valid_until"`
	ExpiryStatus      string    `json:"expiry_status"`
	DaysLeft          *int      `json:"days_left"`
	FileName          string    `json:"file_name"`
	SizeBytes         int64     `json:"size_bytes"`
	ContentType       string    `json:"content_type"`
	VisibleToTaxpayer bool      `json:"visible_to_taxpayer"`
	// ImportantKind marks an important document (#780 B); nil: not important.
	ImportantKind *string   `json:"important_kind"`
	CreatedAt     time.Time `json:"created_at"`
}

// Counts are the tab counters and the expiry warning over the whole archive.
type Counts struct {
	Total       int64 `json:"total"`
	ISG         int64 `json:"isg"`
	Expired     int64 `json:"expired"`
	Expiring    int64 `json:"expiring"`
	ISGExpired  int64 `json:"isg_expired"`
	ISGExpiring int64 `json:"isg_expiring"`
	// Important documents (#780 B) and those expired or ending within
	// ExpiringDays.
	Important         int64 `json:"important"`
	ImportantExpiring int64 `json:"important_expiring"`
}

// Page is a page of documents with the archive counters.
type Page struct {
	Items        []Document `json:"items"`
	Total        int64      `json:"total"`
	Limit        int32      `json:"limit"`
	Offset       int32      `json:"offset"`
	Counts       Counts     `json:"counts"`
	ExpiringDays int        `json:"expiring_days"`
}

// Filter narrows the list.
type Filter struct {
	Category string
	ISGKind  string
	Q        string
	// ExpiringOnly lists documents that expired or end within ExpiringDays,
	// the soonest first.
	ExpiringOnly bool
	// OnlyVisible: the mükellef panel sees only documents the office marked
	// "Mükellef de görsün"; the counters follow.
	OnlyVisible bool
	// OnlyImportant lists the important documents (#780 B), the soonest
	// validity end first.
	OnlyImportant bool
	Limit, Offset int32
}

// istanbul is the day boundary for validity (a document valid "until 17
// Kasım" is valid all of that day in Türkiye).
var istanbul = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return l
	}
	return time.FixedZone("TRT", 3*3600)
}()

func (s *Service) today() time.Time {
	y, m, d := s.now().In(istanbul).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func date(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }

// expiry classifies a validity end date against today: none (no end date),
// expired, expiring (within ExpiringDays) or valid.
func expiry(validUntil pgtype.Date, today time.Time) (string, *int) {
	if !validUntil.Valid {
		return "none", nil
	}
	days := int(validUntil.Time.Sub(today).Hours() / 24)
	switch {
	case days < 0:
		return "expired", &days
	case days <= ExpiringDays:
		return "expiring", &days
	default:
		return "valid", &days
	}
}

func dateStr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format(time.DateOnly)
	return &s
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

func (s *Service) view(r db.TaxpayerDocument, today time.Time) Document {
	st, days := expiry(r.ValidUntil, today)
	return Document{
		UUID: r.Uuid, Category: r.Category, ISGKind: textPtr(r.IsgKind), Title: r.Title, Note: r.Note,
		DocumentDate: dateStr(r.DocumentDate), ValidUntil: dateStr(r.ValidUntil), ExpiryStatus: st, DaysLeft: days,
		FileName: r.FileName, SizeBytes: r.SizeBytes, ContentType: r.ContentType,
		VisibleToTaxpayer: r.VisibleToTaxpayer, ImportantKind: textPtr(r.ImportantKind), CreatedAt: r.CreatedAt.Time,
	}
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// List lists the archive.
func (s *Service) List(ctx context.Context, taxpayerID int64, f Filter) (Page, error) {
	if f.Category != "" && !oneOf(f.Category, Categories) {
		return Page{}, fmt.Errorf("%w: unknown category", ErrInvalid)
	}
	today := s.today()
	soon := today.AddDate(0, 0, ExpiringDays)
	p := db.ListTaxpayerDocumentsParams{TaxpayerID: taxpayerID, OnlyVisible: f.OnlyVisible, OnlyImportant: f.OnlyImportant, Lim: f.Limit, Off: f.Offset}
	if f.Category != "" {
		p.Category = pgtype.Text{String: f.Category, Valid: true}
	}
	if f.ISGKind != "" {
		if !oneOf(f.ISGKind, ISGKinds) {
			return Page{}, fmt.Errorf("%w: unknown isg_kind", ErrInvalid)
		}
		p.IsgKind = pgtype.Text{String: f.ISGKind, Valid: true}
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Q = pgtype.Text{String: likeEscaper.Replace(q), Valid: true}
	}
	if f.ExpiringOnly {
		p.ExpiresBefore = date(soon)
	}
	rows, err := s.q.ListTaxpayerDocuments(ctx, p)
	if err != nil {
		return Page{}, fmt.Errorf("archive: list: %w", err)
	}
	c, err := s.q.CountTaxpayerDocuments(ctx, db.CountTaxpayerDocumentsParams{TaxpayerID: taxpayerID, OnlyVisible: f.OnlyVisible, Today: date(today), Soon: date(soon)})
	if err != nil {
		return Page{}, fmt.Errorf("archive: counts: %w", err)
	}
	out := Page{Items: make([]Document, len(rows)), Limit: f.Limit, Offset: f.Offset, ExpiringDays: ExpiringDays,
		Counts: Counts{Total: c.Total, ISG: c.Isg, Expired: c.Expired, Expiring: c.Expiring, ISGExpired: c.IsgExpired, ISGExpiring: c.IsgExpiring,
			Important: c.Important, ImportantExpiring: c.ImportantExpiring}}
	for i, r := range rows {
		out.Items[i] = s.view(db.TaxpayerDocument{
			ID: r.ID, Uuid: r.Uuid, TaxpayerID: r.TaxpayerID, Category: r.Category, IsgKind: r.IsgKind, Title: r.Title,
			Note: r.Note, DocumentDate: r.DocumentDate, ValidUntil: r.ValidUntil, FileName: r.FileName, SizeBytes: r.SizeBytes,
			ContentType: r.ContentType, VisibleToTaxpayer: r.VisibleToTaxpayer, ImportantKind: r.ImportantKind, CreatedAt: r.CreatedAt,
		}, today)
		out.Total = r.TotalCount
	}
	return out, nil
}

// Input describes an uploaded document.
type Input struct {
	Category          string
	ISGKind           string
	Title             string
	Note              string
	DocumentDate      string // YYYY-MM-DD, optional
	ValidUntil        string // YYYY-MM-DD, optional
	VisibleToTaxpayer bool
	ImportantKind     string // optional: marks the document important (#780 B)
}

func importantKind(v string) (pgtype.Text, error) {
	if v == "" {
		return pgtype.Text{}, nil
	}
	if !oneOf(v, ImportantKinds) {
		return pgtype.Text{}, fmt.Errorf("%w: unknown important_kind", ErrInvalid)
	}
	return pgtype.Text{String: v, Valid: true}, nil
}

func parseDate(field, v string) (pgtype.Date, error) {
	if v == "" {
		return pgtype.Date{}, nil
	}
	t, err := time.Parse(time.DateOnly, v)
	if err != nil {
		return pgtype.Date{}, fmt.Errorf("%w: %s must be YYYY-MM-DD", ErrInvalid, field)
	}
	return date(t), nil
}

// sniff returns the accepted content type of data.
func sniff(data []byte) (string, string, bool) {
	switch ct := http.DetectContentType(data); {
	case strings.HasPrefix(ct, "application/pdf"):
		return "application/pdf", ".pdf", true
	case ct == "image/jpeg":
		return ct, ".jpg", true
	case ct == "image/png":
		return ct, ".png", true
	default:
		return "", "", false
	}
}

func cleanFileName(n, ext string) string {
	n = strings.TrimSpace(n)
	if i := strings.LastIndexAny(n, `/\`); i >= 0 {
		n = n[i+1:]
	}
	if n == "" {
		n = "belge" + ext
	}
	if len(n) > 200 {
		n = n[len(n)-200:]
	}
	return n
}

// Upload stores a document. The same file twice is refused (ErrExists).
func (s *Service) Upload(ctx context.Context, taxpayerID, actorID int64, fileName string, data []byte, in Input) (Document, error) {
	if s.store == nil {
		return Document{}, ErrStorageMissing
	}
	if in.Category == "" {
		in.Category = "general"
	}
	if !oneOf(in.Category, Categories) {
		return Document{}, fmt.Errorf("%w: unknown category", ErrInvalid)
	}
	var isg pgtype.Text
	switch {
	case in.Category == "isg" && !oneOf(in.ISGKind, ISGKinds):
		return Document{}, fmt.Errorf("%w: isg_kind must be isg_training, risk_assessment, health_report or other", ErrInvalid)
	case in.Category == "isg":
		isg = pgtype.Text{String: in.ISGKind, Valid: true}
	case in.ISGKind != "":
		return Document{}, fmt.Errorf("%w: isg_kind is only for the isg category", ErrInvalid)
	}
	docDate, err := parseDate("document_date", in.DocumentDate)
	if err != nil {
		return Document{}, err
	}
	validUntil, err := parseDate("valid_until", in.ValidUntil)
	if err != nil {
		return Document{}, err
	}
	if docDate.Valid && validUntil.Valid && validUntil.Time.Before(docDate.Time) {
		return Document{}, fmt.Errorf("%w: valid_until is before document_date", ErrInvalid)
	}
	if len([]rune(in.Note)) > 1000 {
		return Document{}, fmt.Errorf("%w: note is longer than 1000 characters", ErrInvalid)
	}
	important, err := importantKind(in.ImportantKind)
	if err != nil {
		return Document{}, err
	}
	if len(data) == 0 || len(data) > MaxFileBytes {
		return Document{}, fmt.Errorf("%w: the file is empty or larger than 20 MB", ErrInvalid)
	}
	ct, ext, ok := sniff(data)
	if !ok {
		return Document{}, ErrUnsupported
	}
	name := cleanFileName(fileName, ext)
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = strings.TrimSuffix(name, ext)
	}
	if r := []rune(title); len(r) > 200 {
		title = string(r[:200])
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	key := fmt.Sprintf("archive/%d/%s%s", taxpayerID, digest, ext)
	if err := s.store.Upload(ctx, storage.File{Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: ct, Filename: name}, key); err != nil {
		return Document{}, fmt.Errorf("archive: upload: %w", err)
	}
	var by pgtype.Int8
	if actorID > 0 {
		by = pgtype.Int8{Int64: actorID, Valid: true}
	}
	row, err := s.q.InsertTaxpayerDocument(ctx, db.InsertTaxpayerDocumentParams{
		TaxpayerID: taxpayerID, Category: in.Category, IsgKind: isg, Title: title, Note: strings.TrimSpace(in.Note),
		DocumentDate: docDate, ValidUntil: validUntil, StorageKey: key, Sha256: digest, SizeBytes: int64(len(data)),
		ContentType: ct, FileName: name, VisibleToTaxpayer: in.VisibleToTaxpayer, CreatedBy: by, ImportantKind: important,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrExists
	}
	if err != nil {
		return Document{}, fmt.Errorf("archive: insert: %w", err)
	}
	return s.view(row, s.today()), nil
}

// File is an opened document file.
type File struct {
	Body        io.ReadCloser
	Size        int64
	ContentType string
	Name        string
}

// Open opens a document's file. onlyVisible (mükellef panel): a document
// not marked "Mükellef de görsün" is not found.
func (s *Service) Open(ctx context.Context, taxpayerID int64, id uuid.UUID, onlyVisible bool) (File, error) {
	if s.store == nil {
		return File{}, ErrStorageMissing
	}
	d, err := s.q.GetTaxpayerDocument(ctx, db.GetTaxpayerDocumentParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) || err == nil && onlyVisible && !d.VisibleToTaxpayer {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, err
	}
	rc, size, err := s.store.Download(ctx, d.StorageKey)
	if err != nil {
		return File{}, fmt.Errorf("archive: download: %w", err)
	}
	return File{Body: rc, Size: size, ContentType: d.ContentType, Name: d.FileName}, nil
}

// Delete removes a document and its object.
func (s *Service) Delete(ctx context.Context, taxpayerID int64, id uuid.UUID) error {
	d, err := s.q.GetTaxpayerDocument(ctx, db.GetTaxpayerDocumentParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := s.q.DeleteTaxpayerDocument(ctx, db.DeleteTaxpayerDocumentParams{TaxpayerID: taxpayerID, ID: d.ID}); err != nil {
		return err
	}
	if s.store != nil {
		_ = s.store.Delete(ctx, d.StorageKey) // the row is gone; a leftover object is harmless
	}
	return nil
}
