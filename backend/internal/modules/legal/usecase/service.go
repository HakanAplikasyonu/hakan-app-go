// Package usecase is the KVKK legal text registry (issue #57): versioned
// documents (privacy notice, explicit consent for sharing past data, data
// processing agreement, retention policy) and the evidence of who accepted
// which version, in which flow, on whose behalf.
//
// A published version never changes (a database trigger enforces it); a
// correction is a new version. Flows accept only the current version, so the
// record always names the exact text the person saw.
package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// Document kinds.
const (
	KindPrivacyNotice  = "privacy_notice"
	KindHistoryConsent = "history_share_consent"
	KindDPA            = "data_processing_agreement"
	KindRetention      = "retention_policy"
)

// Acceptance contexts (the flow a text was accepted in).
const (
	ContextSelf       = "self"
	ContextHistory    = "history_share"
	ContextOnboarding = "onboarding_consent"
	ContextOfficeDPA  = "office_dpa"
)

var kinds = map[string]bool{KindPrivacyNotice: true, KindHistoryConsent: true, KindDPA: true, KindRetention: true}

var (
	// ErrNotFound: no such document (or no published version).
	ErrNotFound = errors.New("legal: not found")
	// ErrInvalid: bad input.
	ErrInvalid = errors.New("legal: invalid input")
	// ErrOutdated: the accepted version is not the current one (the person
	// saw an older text; the client must show the current text again).
	ErrOutdated = errors.New("legal: not the current version")
	// ErrState: the document cannot change in its state.
	ErrState = errors.New("legal: invalid state")
)

// Queries is the sqlc subset.
type Queries interface {
	GetCurrentLegalDocument(ctx context.Context, arg db.GetCurrentLegalDocumentParams) (db.LegalDocument, error)
	GetLegalDocumentVersion(ctx context.Context, arg db.GetLegalDocumentVersionParams) (db.LegalDocument, error)
	ListCurrentLegalDocuments(ctx context.Context, language string) ([]db.LegalDocument, error)
	ListAllLegalDocuments(ctx context.Context) ([]db.LegalDocument, error)
	GetLegalDocumentByUUID(ctx context.Context, id uuid.UUID) (db.LegalDocument, error)
	InsertLegalDocumentDraft(ctx context.Context, arg db.InsertLegalDocumentDraftParams) (db.LegalDocument, error)
	PublishLegalDocument(ctx context.Context, arg db.PublishLegalDocumentParams) (db.LegalDocument, error)
	RetireOlderLegalDocuments(ctx context.Context, arg db.RetireOlderLegalDocumentsParams) error
	InsertLegalAcceptance(ctx context.Context, arg db.InsertLegalAcceptanceParams) (db.LegalAcceptance, error)
	ListUserLegalAcceptances(ctx context.Context, userID int64) ([]db.ListUserLegalAcceptancesRow, error)
	GetOrganizationLegalAcceptance(ctx context.Context, arg db.GetOrganizationLegalAcceptanceParams) (db.GetOrganizationLegalAcceptanceRow, error)
	WithdrawLegalAcceptancesBySubject(ctx context.Context, subjectUUID pgtype.UUID) (int64, error)
}

// TxRunner runs fn in a transaction (publish retires older versions).
type TxRunner func(ctx context.Context, fn func(q Queries) error) error

// Service is the registry.
type Service struct {
	q  Queries
	tx TxRunner
}

// New builds the service. tx may be nil (publish then runs without a
// transaction; tests only).
func New(q Queries, tx TxRunner) *Service { return &Service{q: q, tx: tx} }

// Document is the API view.
type Document struct {
	UUID               uuid.UUID  `json:"uuid"`
	Kind               string     `json:"kind"`
	Language           string     `json:"language"`
	Version            int        `json:"version"`
	Title              string     `json:"title"`
	Body               string     `json:"body"`
	Status             string     `json:"status"`
	LegalReviewPending bool       `json:"legal_review_pending"`
	ContentSHA256      string     `json:"content_sha256"`
	PublishedAt        *time.Time `json:"published_at"`
}

func view(d db.LegalDocument) Document {
	v := Document{
		UUID: d.Uuid, Kind: d.Kind, Language: d.Language, Version: int(d.Version), Title: d.Title, Body: d.Body,
		Status: d.Status, LegalReviewPending: d.LegalReviewPending, ContentSHA256: d.ContentSha256,
	}
	if d.PublishedAt.Valid {
		t := d.PublishedAt.Time
		v.PublishedAt = &t
	}
	return v
}

// ContentHash is sha256(title + "\n" + body), the same as the migration seed.
func ContentHash(title, body string) string {
	s := sha256.Sum256([]byte(title + "\n" + body))
	return hex.EncodeToString(s[:])
}

func lang(l string) string {
	if strings.HasPrefix(strings.ToLower(l), "en") {
		return "en"
	}
	return "tr"
}

// Current returns the published version of kind in language, falling back to
// Turkish (the legally binding text).
func (s *Service) Current(ctx context.Context, kind, language string) (Document, error) {
	if !kinds[kind] {
		return Document{}, ErrNotFound
	}
	d, err := s.current(ctx, s.q, kind, language)
	if err != nil {
		return Document{}, err
	}
	return view(d), nil
}

func (s *Service) current(ctx context.Context, q Queries, kind, language string) (db.LegalDocument, error) {
	d, err := q.GetCurrentLegalDocument(ctx, db.GetCurrentLegalDocumentParams{Kind: kind, Language: lang(language)})
	if errors.Is(err, pgx.ErrNoRows) && lang(language) != "tr" {
		d, err = q.GetCurrentLegalDocument(ctx, db.GetCurrentLegalDocumentParams{Kind: kind, Language: "tr"})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return db.LegalDocument{}, ErrNotFound
	}
	return d, err
}

// Version returns a published (or retired) version: what someone accepted.
func (s *Service) Version(ctx context.Context, kind, language string, version int) (Document, error) {
	if !kinds[kind] || version < 1 {
		return Document{}, ErrNotFound
	}
	d, err := s.q.GetLegalDocumentVersion(ctx, db.GetLegalDocumentVersionParams{Kind: kind, Language: lang(language), Version: int32(version)}) //nolint:gosec // checked >= 1, small
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, err
	}
	return view(d), nil
}

// ListCurrent returns the current version of every kind.
func (s *Service) ListCurrent(ctx context.Context, language string) ([]Document, error) {
	rows, err := s.q.ListCurrentLegalDocuments(ctx, lang(language))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 && lang(language) != "tr" {
		return s.ListCurrent(ctx, "tr")
	}
	out := make([]Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, view(r))
	}
	return out, nil
}

// AcceptInput records that a user accepted the current version of a text.
type AcceptInput struct {
	Kind           string
	Language       string
	Version        int
	UserID         int64
	Context        string
	OrganizationID int64
	TaxpayerID     int64
	SubjectUUID    *uuid.UUID
	IP             string
	UserAgent      string
}

// Acceptance is the stored evidence.
type Acceptance struct {
	UUID       uuid.UUID `json:"uuid"`
	Kind       string    `json:"kind"`
	Version    int       `json:"version"`
	Context    string    `json:"context"`
	AcceptedAt time.Time `json:"accepted_at"`
}

// Accept records the acceptance (outside any other transaction).
func (s *Service) Accept(ctx context.Context, in AcceptInput) (Acceptance, error) {
	return s.AcceptWith(ctx, s.q, in)
}

// AcceptWith records the acceptance with q, so a flow can write its own row
// (e.g. a history share) and the evidence in one transaction. The version
// must be the current one.
func (s *Service) AcceptWith(ctx context.Context, q Queries, in AcceptInput) (Acceptance, error) {
	if !kinds[in.Kind] || in.UserID <= 0 || in.Version < 1 {
		return Acceptance{}, fmt.Errorf("%w: kind, version and user are required", ErrInvalid)
	}
	d, err := s.current(ctx, q, in.Kind, in.Language)
	if err != nil {
		return Acceptance{}, err
	}
	if int(d.Version) != in.Version {
		return Acceptance{}, fmt.Errorf("%w: %s current version is %d", ErrOutdated, in.Kind, d.Version)
	}
	ua := in.UserAgent
	if len(ua) > 512 {
		ua = ua[:512]
	}
	p := db.InsertLegalAcceptanceParams{
		DocumentID: d.ID, UserID: in.UserID, Context: in.Context, ContentSha256: d.ContentSha256,
		Ip: in.IP, UserAgent: ua,
	}
	if in.OrganizationID > 0 {
		p.OrganizationID = pgtype.Int8{Int64: in.OrganizationID, Valid: true}
	}
	if in.TaxpayerID > 0 {
		p.TaxpayerID = pgtype.Int8{Int64: in.TaxpayerID, Valid: true}
	}
	if in.SubjectUUID != nil {
		p.SubjectUuid = pgtype.UUID{Bytes: *in.SubjectUUID, Valid: true}
	}
	row, err := q.InsertLegalAcceptance(ctx, p)
	if err != nil {
		return Acceptance{}, fmt.Errorf("legal: record acceptance: %w", err)
	}
	return Acceptance{UUID: row.Uuid, Kind: d.Kind, Version: int(d.Version), Context: row.Context, AcceptedAt: row.AcceptedAt.Time}, nil
}

// WithdrawWith marks the acceptances tied to subject withdrawn (explicit
// consent taken back, KVKK md. 5).
func (s *Service) WithdrawWith(ctx context.Context, q Queries, subject uuid.UUID) error {
	_, err := q.WithdrawLegalAcceptancesBySubject(ctx, pgtype.UUID{Bytes: subject, Valid: true})
	return err
}

// MyAcceptance is a row of the user's own consent history.
type MyAcceptance struct {
	UUID         uuid.UUID  `json:"uuid"`
	Kind         string     `json:"kind"`
	Language     string     `json:"language"`
	Version      int        `json:"version"`
	Title        string     `json:"title"`
	Context      string     `json:"context"`
	Organization *string    `json:"organization"`
	Taxpayer     *string    `json:"taxpayer"`
	AcceptedAt   time.Time  `json:"accepted_at"`
	WithdrawnAt  *time.Time `json:"withdrawn_at"`
}

// Mine lists the user's acceptances (KVKK md. 11: the person can see what
// they consented to).
func (s *Service) Mine(ctx context.Context, userID int64) ([]MyAcceptance, error) {
	rows, err := s.q.ListUserLegalAcceptances(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]MyAcceptance, 0, len(rows))
	for _, r := range rows {
		m := MyAcceptance{
			UUID: r.Uuid, Kind: r.Kind, Language: r.Language, Version: int(r.Version), Title: r.Title,
			Context: r.Context, AcceptedAt: r.AcceptedAt.Time,
		}
		if r.OrganizationName.Valid {
			v := r.OrganizationName.String
			m.Organization = &v
		}
		if r.TaxpayerTitle.Valid {
			v := r.TaxpayerTitle.String
			m.Taxpayer = &v
		}
		if r.WithdrawnAt.Valid {
			t := r.WithdrawnAt.Time
			m.WithdrawnAt = &t
		}
		out = append(out, m)
	}
	return out, nil
}

// DPAStatus is the office's data processing agreement state.
type DPAStatus struct {
	Document Document          `json:"document"`
	Accepted *OfficeAcceptance `json:"accepted"`
	// UpToDate: the office accepted the current version.
	UpToDate bool `json:"up_to_date"`
}

// OfficeAcceptance is who accepted for the office.
type OfficeAcceptance struct {
	Version    int       `json:"version"`
	AcceptedAt time.Time `json:"accepted_at"`
	AcceptedBy string    `json:"accepted_by"`
}

// DPA returns the office's agreement state.
func (s *Service) DPA(ctx context.Context, organizationID int64, language string) (DPAStatus, error) {
	cur, err := s.Current(ctx, KindDPA, language)
	if err != nil {
		return DPAStatus{}, err
	}
	st := DPAStatus{Document: cur}
	a, err := s.q.GetOrganizationLegalAcceptance(ctx, db.GetOrganizationLegalAcceptanceParams{
		OrganizationID: pgtype.Int8{Int64: organizationID, Valid: true}, Context: ContextOfficeDPA, Kind: KindDPA,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return DPAStatus{}, err
	default:
		st.Accepted = &OfficeAcceptance{Version: int(a.Version), AcceptedAt: a.AcceptedAt.Time, AcceptedBy: a.AcceptedByEmail}
		st.UpToDate = int(a.Version) == cur.Version
	}
	return st, nil
}

// DraftInput creates a new draft version (platform admins).
type DraftInput struct {
	Kind     string `json:"kind"`
	Language string `json:"language"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

// CreateDraft adds the next version of kind as a draft.
func (s *Service) CreateDraft(ctx context.Context, userID int64, in DraftInput) (Document, error) {
	in.Title, in.Body = strings.TrimSpace(in.Title), strings.TrimSpace(in.Body)
	if !kinds[in.Kind] || (in.Language != "tr" && in.Language != "en") {
		return Document{}, fmt.Errorf("%w: unknown kind or language", ErrInvalid)
	}
	if in.Title == "" || len(in.Title) > 200 || in.Body == "" || len(in.Body) > 100_000 {
		return Document{}, fmt.Errorf("%w: title (≤200) and body (≤100000) are required", ErrInvalid)
	}
	p := db.InsertLegalDocumentDraftParams{
		Kind: in.Kind, Language: in.Language, Title: in.Title, Body: in.Body, ContentSha256: ContentHash(in.Title, in.Body),
	}
	if userID > 0 {
		p.CreatedBy = pgtype.Int8{Int64: userID, Valid: true}
	}
	d, err := s.q.InsertLegalDocumentDraft(ctx, p)
	if err != nil {
		return Document{}, err
	}
	return view(d), nil
}

// Publish makes a draft the current version and retires older published
// ones. reviewed clears the "awaiting legal review" flag.
func (s *Service) Publish(ctx context.Context, id uuid.UUID, reviewed bool) (Document, error) {
	var out db.LegalDocument
	run := func(q Queries) error {
		d, err := q.PublishLegalDocument(ctx, db.PublishLegalDocumentParams{Uuid: id, LegalReviewPending: !reviewed})
		if errors.Is(err, pgx.ErrNoRows) {
			if _, gerr := q.GetLegalDocumentByUUID(ctx, id); errors.Is(gerr, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("%w: only a draft can be published", ErrState)
		}
		if err != nil {
			return err
		}
		out = d
		return q.RetireOlderLegalDocuments(ctx, db.RetireOlderLegalDocumentsParams{Kind: d.Kind, Language: d.Language, Version: d.Version})
	}
	var err error
	if s.tx != nil {
		err = s.tx(ctx, run)
	} else {
		err = run(s.q)
	}
	if err != nil {
		return Document{}, err
	}
	return view(out), nil
}

// All lists every version (platform admins).
func (s *Service) All(ctx context.Context) ([]Document, error) {
	rows, err := s.q.ListAllLegalDocuments(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, view(r))
	}
	return out, nil
}

// Pool is what NewPostgres needs.
type Pool interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// NewPostgres builds the service over a pool, with transactional publish.
func NewPostgres(pool Pool) *Service {
	return New(db.New(pool), func(ctx context.Context, fn func(q Queries) error) error {
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error { return fn(db.New(tx)) })
	})
}
