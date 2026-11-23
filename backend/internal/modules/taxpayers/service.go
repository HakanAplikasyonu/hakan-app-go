// Package taxpayers holds the global mükellef kimliği of tenant-core faz 6b
// (issue #3): one row per VKN/TCKN across the platform, independent of any
// office (docs/01 §2, G2).
//
// Offices never look taxpayers up by VKN/TCKN here. They reach a taxpayer
// only through an active engagement (ForOfficeMember). Karşı taraflar (cari)
// are never linked to this identity (D1b).
package taxpayers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxid"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Taxpayer kinds.
const (
	KindLegal      = "legal"      // tüzel kişi: VKN
	KindIndividual = "individual" // gerçek kişi: VKN veya TCKN
)

const (
	maxTitleLen     = 255
	maxTaxOfficeLen = 128
	maxAddressLen   = 500
)

// Service errors.
var (
	ErrInvalidRequest = errors.New("taxpayers: invalid request")
	ErrAlreadyExists  = errors.New("taxpayers: tax id already registered")
	ErrNotFound       = errors.New("taxpayers: not found")

	// Validate's causes, wrapped under ErrInvalidRequest. The bulk import
	// shows each one to the office in Turkish (rowMessage, #397).
	errLegalNeedsVKN = errors.New("a legal entity needs a 10-digit VKN")
	errTitle         = fmt.Errorf("title is required (max %d characters)", maxTitleLen)
	errTaxOfficeLen  = fmt.Errorf("tax_office is too long (max %d characters)", maxTaxOfficeLen)
	errAddressLen    = fmt.Errorf("address is too long (max %d characters)", maxAddressLen)
	errActivityCode  = errors.New("activity_code must be a 6-digit NACE code (e.g. 47.11.01)")
)

// Taxpayer is the API shape of a taxpayer identity.
type Taxpayer struct {
	UUID      uuid.UUID `json:"uuid"`
	Kind      string    `json:"kind"`
	TaxID     string    `json:"tax_id"`
	TaxIDType string    `json:"tax_id_type"`
	Title     string    `json:"title"`
	TaxOffice string    `json:"tax_office"`
	// Address and ActivityCode (ana faaliyet kodu, NACE "47.11.01") come
	// from the vergi levhası; "" when unknown.
	Address      string    `json:"address"`
	ActivityCode string    `json:"activity_code"`
	CreatedAt    time.Time `json:"created_at"`
}

// CreateInput is a new taxpayer identity.
type CreateInput struct {
	Kind         string `json:"kind"`
	TaxID        string `json:"tax_id"`
	Title        string `json:"title"`
	TaxOffice    string `json:"tax_office"`
	Address      string `json:"address"`
	ActivityCode string `json:"activity_code"`
}

// PatchInput changes the descriptive fields. The tax id and kind never change.
type PatchInput struct {
	Title        *string `json:"title"`
	TaxOffice    *string `json:"tax_office"`
	Address      *string `json:"address"`
	ActivityCode *string `json:"activity_code"`
}

// Service implements the taxpayer use cases.
type Service struct {
	store Store
	// rlsDB, when set, runs office-side reads under Postgres RLS (issue #6).
	rlsDB rls.Beginner
}

// New builds the service.
func New(store Store) *Service { return &Service{store: store} }

// SetRLS makes office-side reads (ForOfficeMember) run in an RLS-scoped
// transaction built from the request context (rls.FromRequest), so the
// database enforces the office and taxpayer boundary as well.
func (s *Service) SetRLS(db rls.Beginner) { s.rlsDB = db }

// Validate normalizes and checks a CreateInput. It is exported so other
// flows (office adding a mükellef, issue #7) apply the same rules before
// touching the database.
func Validate(in CreateInput) (CreateInput, taxid.Type, error) {
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	if kind != KindLegal && kind != KindIndividual {
		return in, "", fmt.Errorf("%w: kind must be legal or individual", ErrInvalidRequest)
	}
	n, typ, err := taxid.Parse(in.TaxID)
	if err != nil {
		return in, "", fmt.Errorf("%w: tax_id: %w", ErrInvalidRequest, err)
	}
	if kind == KindLegal && typ != taxid.VKN {
		return in, "", fmt.Errorf("%w: %w", ErrInvalidRequest, errLegalNeedsVKN)
	}
	title := strings.Join(strings.Fields(in.Title), " ")
	if title == "" || utf8.RuneCountInString(title) > maxTitleLen {
		return in, "", fmt.Errorf("%w: %w", ErrInvalidRequest, errTitle)
	}
	office := strings.Join(strings.Fields(in.TaxOffice), " ")
	if utf8.RuneCountInString(office) > maxTaxOfficeLen {
		return in, "", fmt.Errorf("%w: %w", ErrInvalidRequest, errTaxOfficeLen)
	}
	address, activity, err := validateCertificateExtras(in.Address, in.ActivityCode)
	if err != nil {
		return in, "", err
	}
	return CreateInput{Kind: kind, TaxID: n, Title: title, TaxOffice: office, Address: address, ActivityCode: activity}, typ, nil
}

// validateCertificateExtras normalizes the address and the activity code
// (ana faaliyet kodu, NACE: 6 digits, stored as "47.11.01"). Both may be
// empty.
func validateCertificateExtras(address, activity string) (string, string, error) {
	address = strings.Join(strings.Fields(address), " ")
	if utf8.RuneCountInString(address) > maxAddressLen {
		return "", "", fmt.Errorf("%w: %w", ErrInvalidRequest, errAddressLen)
	}
	code, ok := normalizeActivityCode(activity)
	if !ok {
		return "", "", fmt.Errorf("%w: %w", ErrInvalidRequest, errActivityCode)
	}
	return address, code, nil
}

// normalizeActivityCode accepts "471101", "47.11.01" or "47 11 01"; "" is
// allowed (unknown).
func normalizeActivityCode(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", true
	}
	var d []byte
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= '0' && c <= '9':
			d = append(d, c)
		case c == '.' || c == ' ' || c == '-':
		default:
			return "", false
		}
	}
	if len(d) != 6 {
		return "", false
	}
	return string(d[0:2]) + "." + string(d[2:4]) + "." + string(d[4:6]), true
}

// Create registers a taxpayer identity. A VKN/TCKN exists at most once on
// the platform: a second create answers ErrAlreadyExists. Only platform
// routes may surface that error (D1b: offices must not learn it).
func (s *Service) Create(ctx context.Context, in CreateInput) (Taxpayer, error) {
	v, _, err := Validate(in)
	if err != nil {
		return Taxpayer{}, err
	}
	row, err := s.store.Create(ctx, db.CreateTaxpayerParams{
		Kind: v.Kind, TaxID: v.TaxID, Title: v.Title, TaxOffice: v.TaxOffice,
		Address: v.Address, ActivityCode: v.ActivityCode,
	})
	if err != nil {
		if errors.Is(err, errDuplicate) {
			return Taxpayer{}, ErrAlreadyExists
		}
		return Taxpayer{}, err
	}
	return toDTO(row), nil
}

// Get returns a taxpayer by uuid (platform routes).
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Taxpayer, error) {
	row, err := s.store.GetByUUID(ctx, id)
	if err != nil {
		return Taxpayer{}, mapNotFound(err)
	}
	return toDTO(row), nil
}

// FindByTaxID returns the taxpayer with that VKN/TCKN. It is for internal
// flows only (issue #7 add-mükellef, which must answer the office the same
// way whether or not this finds a row). Never expose its result directly.
func (s *Service) FindByTaxID(ctx context.Context, raw string) (Taxpayer, bool, error) {
	n, _, err := taxid.Parse(raw)
	if err != nil {
		return Taxpayer{}, false, fmt.Errorf("%w: tax_id: %s", ErrInvalidRequest, err.Error())
	}
	row, err := s.store.GetByTaxID(ctx, n)
	if errors.Is(err, errNoRows) {
		return Taxpayer{}, false, nil
	}
	if err != nil {
		return Taxpayer{}, false, err
	}
	return toDTO(row), true, nil
}

// List pages through taxpayers (platform routes). q matches an exact tax id
// or part of the title.
func (s *Service) List(ctx context.Context, q string, limit, offset int32) ([]Taxpayer, int64, error) {
	qp := pgtype.Text{}
	if v := strings.TrimSpace(q); v != "" {
		if n := taxid.Normalize(v); allDigits(n) {
			v = n
		}
		qp = pgtype.Text{String: v, Valid: true}
	}
	rows, total, err := s.store.List(ctx, qp, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Taxpayer, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDTO(r))
	}
	return out, total, nil
}

// Patch changes the title and/or tax office (platform routes).
func (s *Service) Patch(ctx context.Context, id uuid.UUID, in PatchInput) (Taxpayer, error) {
	params := db.UpdateTaxpayerProfileParams{Uuid: id}
	if in.Title != nil {
		t := strings.Join(strings.Fields(*in.Title), " ")
		if t == "" || utf8.RuneCountInString(t) > maxTitleLen {
			return Taxpayer{}, fmt.Errorf("%w: %w", ErrInvalidRequest, errTitle)
		}
		params.Title = pgtype.Text{String: t, Valid: true}
	}
	if in.TaxOffice != nil {
		o := strings.Join(strings.Fields(*in.TaxOffice), " ")
		if utf8.RuneCountInString(o) > maxTaxOfficeLen {
			return Taxpayer{}, fmt.Errorf("%w: %w", ErrInvalidRequest, errTaxOfficeLen)
		}
		params.TaxOffice = pgtype.Text{String: o, Valid: true}
	}
	if in.Address != nil || in.ActivityCode != nil {
		var a, c string
		if in.Address != nil {
			a = *in.Address
		}
		if in.ActivityCode != nil {
			c = *in.ActivityCode
		}
		address, code, err := validateCertificateExtras(a, c)
		if err != nil {
			return Taxpayer{}, err
		}
		if in.Address != nil {
			params.Address = pgtype.Text{String: address, Valid: true}
		}
		if in.ActivityCode != nil {
			params.ActivityCode = pgtype.Text{String: code, Valid: true}
		}
	}
	row, err := s.store.UpdateProfile(ctx, params)
	if err != nil {
		return Taxpayer{}, mapNotFound(err)
	}
	return toDTO(row), nil
}

// ForOfficeMember applies the office-side access rule (docs/01 §2.3): the
// taxpayer only through an active engagement of orgID, to a member assigned
// to it or holding tenant.engagements.read_all. Anything else is ErrNotFound.
func (s *Service) ForOfficeMember(ctx context.Context, orgID, userID int64, readAll bool, taxpayerID int64) (Taxpayer, error) {
	params := db.GetTaxpayerForOfficeMemberParams{
		OrganizationID: orgID, TaxpayerID: taxpayerID, ReadAll: readAll, UserID: userID,
	}
	var (
		row db.Taxpayer
		err error
	)
	if s.rlsDB != nil {
		err = rls.WithScope(ctx, s.rlsDB, rls.FromRequest(ctx), func(ctx context.Context, tx pgx.Tx) error {
			var e error
			row, e = NewPostgres(db.New(tx)).ForOfficeMember(ctx, params)
			return e
		})
	} else {
		row, err = s.store.ForOfficeMember(ctx, params)
	}
	if err != nil {
		return Taxpayer{}, mapNotFound(err)
	}
	return toDTO(row), nil
}

func toDTO(r db.Taxpayer) Taxpayer {
	typ := taxid.VKN
	if len(r.TaxID) == 11 {
		typ = taxid.TCKN
	}
	return Taxpayer{
		UUID: r.Uuid, Kind: r.Kind, TaxID: r.TaxID, TaxIDType: string(typ),
		Title: r.Title, TaxOffice: r.TaxOffice, Address: r.Address, ActivityCode: r.ActivityCode,
		CreatedAt: r.CreatedAt.Time,
	}
}

func mapNotFound(err error) error {
	if errors.Is(err, errNoRows) {
		return ErrNotFound
	}
	return err
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
