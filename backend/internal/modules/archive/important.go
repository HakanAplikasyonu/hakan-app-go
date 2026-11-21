package archive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Önemli belgeler (#780 B): a mükellef's important documents (vergi levhası,
// imza sirküleri, ticaret sicil gazetesi, kira sözleşmesi, ruhsat,
// vekâletname...) are archive documents with an important_kind. The office
// uploads them or marks an archived document important; the mükellef panel
// shows the ones marked "Mükellef de görsün". An office setting turns the
// section off (default on).

// Patch edits an archived document. A nil field is left as it is; an empty
// ImportantKind removes the important mark and an empty ValidUntil removes
// the validity end.
type Patch struct {
	ImportantKind     *string `json:"important_kind"`
	VisibleToTaxpayer *bool   `json:"visible_to_taxpayer"`
	Title             *string `json:"title"`
	Note              *string `json:"note"`
	ValidUntil        *string `json:"valid_until"`
}

// Update applies p to the document.
func (s *Service) Update(ctx context.Context, taxpayerID int64, id uuid.UUID, p Patch) (Document, error) {
	d, err := s.q.GetTaxpayerDocument(ctx, db.GetTaxpayerDocumentParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, err
	}
	arg := db.UpdateTaxpayerDocumentParams{
		TaxpayerID: taxpayerID, ID: d.ID, ImportantKind: d.ImportantKind, VisibleToTaxpayer: d.VisibleToTaxpayer,
		Title: d.Title, Note: d.Note, ValidUntil: d.ValidUntil,
	}
	if p.ImportantKind != nil {
		if arg.ImportantKind, err = importantKind(strings.TrimSpace(*p.ImportantKind)); err != nil {
			return Document{}, err
		}
	}
	if p.VisibleToTaxpayer != nil {
		arg.VisibleToTaxpayer = *p.VisibleToTaxpayer
	}
	if p.Title != nil {
		t := strings.TrimSpace(*p.Title)
		if t == "" || len([]rune(t)) > 200 {
			return Document{}, fmt.Errorf("%w: title must be 1..200 characters", ErrInvalid)
		}
		arg.Title = t
	}
	if p.Note != nil {
		n := strings.TrimSpace(*p.Note)
		if len([]rune(n)) > 1000 {
			return Document{}, fmt.Errorf("%w: note is longer than 1000 characters", ErrInvalid)
		}
		arg.Note = n
	}
	if p.ValidUntil != nil {
		if arg.ValidUntil, err = parseDate("valid_until", strings.TrimSpace(*p.ValidUntil)); err != nil {
			return Document{}, err
		}
		if arg.ValidUntil.Valid && d.DocumentDate.Valid && arg.ValidUntil.Time.Before(d.DocumentDate.Time) {
			return Document{}, fmt.Errorf("%w: valid_until is before document_date", ErrInvalid)
		}
	}
	row, err := s.q.UpdateTaxpayerDocument(ctx, arg)
	if err != nil {
		return Document{}, fmt.Errorf("archive: update: %w", err)
	}
	return s.view(row, s.today()), nil
}

// ImportantSettings is the office's "Önemli belgeler" setting.
type ImportantSettings struct {
	Enabled   bool       `json:"enabled"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// Settings returns the office's setting; without a row the section is on.
func (s *Service) Settings(ctx context.Context, orgID int64) (ImportantSettings, error) {
	row, err := s.q.GetImportantDocumentSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportantSettings{Enabled: true}, nil
	}
	if err != nil {
		return ImportantSettings{}, fmt.Errorf("important document settings: %w", err)
	}
	return settingsOf(row), nil
}

// SaveSettings stores the office's setting.
func (s *Service) SaveSettings(ctx context.Context, orgID, actorID int64, enabled bool) (ImportantSettings, error) {
	var by pgtype.Int8
	if actorID > 0 {
		by = pgtype.Int8{Int64: actorID, Valid: true}
	}
	row, err := s.q.UpsertImportantDocumentSettings(ctx, db.UpsertImportantDocumentSettingsParams{
		OrganizationID: orgID, Enabled: enabled, UpdatedBy: by,
	})
	if err != nil {
		return ImportantSettings{}, fmt.Errorf("save important document settings: %w", err)
	}
	return settingsOf(row), nil
}

func settingsOf(row db.ImportantDocumentSetting) ImportantSettings {
	out := ImportantSettings{Enabled: row.Enabled}
	if row.UpdatedAt.Valid {
		t := row.UpdatedAt.Time
		out.UpdatedAt = &t
	}
	return out
}

// TaxpayerImportant is the mükellef panel's "Önemli belgeler" section.
type TaxpayerImportant struct {
	// Enabled is false when the company's office turned the section off.
	Enabled      bool       `json:"enabled"`
	Items        []Document `json:"items"`
	ExpiringDays int        `json:"expiring_days"`
}

// TaxpayerImportant lists the important documents the office shares with
// the mükellef ("Mükellef de görsün"), unless the company's current office
// turned the section off. A company without an office sees its documents.
func (s *Service) TaxpayerImportant(ctx context.Context, taxpayerID int64) (TaxpayerImportant, error) {
	out := TaxpayerImportant{Enabled: true, Items: []Document{}, ExpiringDays: ExpiringDays}
	eng, err := s.q.GetActiveEngagementForTaxpayer(ctx, taxpayerID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return out, fmt.Errorf("archive: office: %w", err)
	case eng.Status != "active":
		// Not yet (or no longer) served: the office's setting does not apply.
	default:
		// The setting is the office's row: read it in the office scope
		// narrowed to this taxpayer.
		if err := rls.ForOffice(ctx, eng.OrganizationID, taxpayerID, func(ctx context.Context) error {
			st, err := s.Settings(ctx, eng.OrganizationID)
			out.Enabled = st.Enabled
			return err
		}); err != nil {
			return out, err
		}
	}
	if !out.Enabled {
		return out, nil
	}
	p, err := s.List(ctx, taxpayerID, Filter{OnlyVisible: true, OnlyImportant: true, Limit: 200})
	if err != nil {
		return out, err
	}
	out.Items = p.Items
	return out, nil
}
