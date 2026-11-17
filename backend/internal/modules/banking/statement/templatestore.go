package statement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// ErrTemplateNotFound: no such template in the office.
var ErrTemplateNotFound = errors.New("statement: template not found")

// WithColumnSuggester sets the AI column mapper of the analysis step
// (nil: heuristics only).
func (im *Importer) WithColumnSuggester(s ColumnSuggester) *Importer {
	im.ai = s
	return im
}

// TemplateView is a template as the API shows it.
type TemplateView struct {
	UUID        *uuid.UUID       `json:"uuid,omitempty"`
	Source      string           `json:"source"`
	BankID      string           `json:"bank_id,omitempty"`
	Name        string           `json:"name"`
	Format      string           `json:"format,omitempty"`
	Fingerprint string           `json:"fingerprint"`
	Columns     []TemplateColumn `json:"columns"`
	Hits        int32            `json:"hits"`
	LastUsedAt  *time.Time       `json:"last_used_at,omitempty"`
	UpdatedAt   *time.Time       `json:"updated_at,omitempty"`
}

// TemplateInput saves a template.
type TemplateInput struct {
	BankID  string           `json:"bank_id"`
	Name    string           `json:"name"`
	Format  string           `json:"format"`
	Columns []TemplateColumn `json:"columns"`
}

func (im *Importer) officeTemplates(ctx context.Context, org int64) ([]Template, error) {
	if org <= 0 {
		return nil, nil
	}
	rows, err := database.QueriesFor(im.pool).ListBankStatementTemplates(ctx, org)
	if err != nil {
		return nil, err
	}
	out := make([]Template, 0, len(rows))
	for _, r := range rows {
		var cs []TemplateColumn
		if err := json.Unmarshal(r.Columns, &cs); err != nil {
			continue
		}
		out = append(out, Template{ID: r.ID, BankID: r.BankID.String, Name: r.Name, Format: r.Format, Columns: cs, Source: SourceOffice})
	}
	return out, nil
}

// Templates lists the office's templates, then the built-in ones.
func (im *Importer) Templates(ctx context.Context, org int64) ([]TemplateView, error) {
	rows, err := database.QueriesFor(im.pool).ListBankStatementTemplates(ctx, org)
	if err != nil {
		return nil, err
	}
	out := make([]TemplateView, 0, len(rows)+len(builtinTemplates))
	for _, r := range rows {
		out = append(out, templateView(r))
	}
	for _, t := range builtinTemplates {
		out = append(out, TemplateView{Source: SourceBuiltin, BankID: t.BankID, Name: t.Name, Format: t.Format,
			Fingerprint: t.Fingerprint(), Columns: t.Columns})
	}
	return out, nil
}

func templateView(r db.BankStatementTemplate) TemplateView {
	var cs []TemplateColumn
	_ = json.Unmarshal(r.Columns, &cs)
	id := r.Uuid
	v := TemplateView{UUID: &id, Source: SourceOffice, BankID: r.BankID.String, Name: r.Name, Format: r.Format,
		Fingerprint: r.Fingerprint, Columns: cs, Hits: r.Hits}
	if r.LastUsedAt.Valid {
		t := r.LastUsedAt.Time
		v.LastUsedAt = &t
	}
	if r.UpdatedAt.Valid {
		t := r.UpdatedAt.Time
		v.UpdatedAt = &t
	}
	return v
}

// SaveTemplate stores (or replaces, same format + fingerprint) an office
// template after the accountant confirmed the columns.
func (im *Importer) SaveTemplate(ctx context.Context, org, userID int64, in TemplateInput) (TemplateView, error) {
	in.BankID = strings.TrimSpace(strings.ToLower(in.BankID))
	in.Name = strings.TrimSpace(in.Name)
	t := Template{BankID: in.BankID, Name: in.Name, Format: in.Format, Columns: in.Columns}
	if in.Format == "" {
		return TemplateView{}, fmt.Errorf("%w: format is required (excel, csv or pdf)", ErrInvalidTemplate)
	}
	if err := t.Validate(); err != nil {
		return TemplateView{}, err
	}
	if in.BankID != "" && !validBankID(in.BankID) {
		return TemplateView{}, fmt.Errorf("%w: bank_id %q", ErrInvalidTemplate, in.BankID)
	}
	if t.Name == "" {
		t.Name = "Ekstre şablonu"
	}
	cs, err := json.Marshal(t.Columns)
	if err != nil {
		return TemplateView{}, err
	}
	row, err := database.QueriesFor(im.pool).UpsertBankStatementTemplate(ctx, db.UpsertBankStatementTemplateParams{
		OrganizationID: org, BankID: optText(in.BankID), Name: t.Name, Format: t.Format, Fingerprint: t.Fingerprint(),
		Columns: cs, CreatedBy: pgtype.Int8{Int64: userID, Valid: userID > 0},
	})
	if err != nil {
		return TemplateView{}, err
	}
	return templateView(row), nil
}

// DeleteTemplate removes one of the office's templates.
func (im *Importer) DeleteTemplate(ctx context.Context, org int64, id uuid.UUID) error {
	n, err := database.QueriesFor(im.pool).DeleteBankStatementTemplate(ctx, db.DeleteBankStatementTemplateParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

func validBankID(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return s != ""
}
