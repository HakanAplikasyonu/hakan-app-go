package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// WarningQueries is the subset of db.Querier used for data_warnings.
type WarningQueries interface {
	GetLatestDataWarning(ctx context.Context, arg db.GetLatestDataWarningParams) (db.DataWarning, error)
	UpsertOpenDataWarning(ctx context.Context, arg db.UpsertOpenDataWarningParams) (db.DataWarning, error)
	AutoResolveDataWarnings(ctx context.Context, arg db.AutoResolveDataWarningsParams) (int64, error)
	ListTenantDataWarnings(ctx context.Context, arg db.ListTenantDataWarningsParams) ([]db.ListTenantDataWarningsRow, error)
	CountTenantDataWarnings(ctx context.Context, arg db.CountTenantDataWarningsParams) (int64, error)
	DecideDataWarning(ctx context.Context, arg db.DecideDataWarningParams) (int64, error)
}

// Warnings is the Postgres engine.Store over data_warnings.
type Warnings struct{ q WarningQueries }

// NewWarnings builds the store.
func NewWarnings(q WarningQueries) *Warnings { return &Warnings{q: q} }

var _ engine.Store = (*Warnings)(nil)

// Raise implements engine.Store. The uq_data_warnings_open index keeps one
// open row per key even under concurrent syncs.
func (p *Warnings) Raise(ctx context.Context, w engine.Warning) (engine.RaiseResult, error) {
	last, err := p.q.GetLatestDataWarning(ctx, db.GetLatestDataWarningParams{
		OrganizationID: w.OrganizationID, EngagementID: w.EngagementID,
		EntityType: w.EntityType, EntityID: optInt8(w.EntityID), Code: w.Code, Period: w.Period,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return 0, fmt.Errorf("latest data warning: %w", err)
	case last.Status == string(catalog.StatusAcknowledged) || last.Status == string(catalog.StatusResolved):
		if sameJSON(last.Params, w.Params) {
			return engine.Suppressed, nil
		}
	}
	row, err := p.q.UpsertOpenDataWarning(ctx, db.UpsertOpenDataWarningParams{
		OrganizationID: w.OrganizationID, EngagementID: w.EngagementID, TaxpayerID: w.TaxpayerID,
		Period: optText(w.Period), EntityType: w.EntityType, EntityID: optInt8(w.EntityID),
		Code: w.Code, Severity: string(w.Severity), Params: w.Params, Producer: w.Producer,
	})
	if err != nil {
		return 0, fmt.Errorf("upsert data warning: %w", err)
	}
	// Insert sets created_at = updated_at = NOW(); the conflict branch sets
	// updated_at = NOW() of a later statement.
	if row.CreatedAt.Time.Equal(row.UpdatedAt.Time) {
		return engine.Created, nil
	}
	return engine.Refreshed, nil
}

// AutoResolve implements engine.Store.
func (p *Warnings) AutoResolve(ctx context.Context, s engine.Subject, producer string, e engine.Entity, codes, keep []string) (int, error) {
	if keep == nil {
		keep = []string{}
	}
	n, err := p.q.AutoResolveDataWarnings(ctx, db.AutoResolveDataWarningsParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Producer: producer,
		EntityType: e.Type, EntityID: optInt8(e.ID), Period: e.Period, Codes: codes, Keep: keep,
	})
	if err != nil {
		return 0, fmt.Errorf("auto-resolve data warnings: %w", err)
	}
	return int(n), nil
}

// Filter narrows a warning list (05 §1 filters).
type Filter struct {
	OrganizationID int64
	// EngagementID > 0 limits to one engagement.
	EngagementID int64
	// AllEngagements is true for tenant.engagements.read_all callers; others
	// see only engagements they are assigned to (UserID).
	AllEngagements bool
	UserID         int64
	Period         string
	Code           string
	Severity       string
	Status         string
	EntityType     string
	Limit, Offset  int32
}

// Row is one listed warning.
type Row struct {
	UUID           uuid.UUID       `json:"uuid"`
	EngagementUUID uuid.UUID       `json:"engagement_uuid"`
	Period         *string         `json:"period"`
	EntityType     string          `json:"entity_type"`
	EntityUUID     *uuid.UUID      `json:"entity_uuid"`
	Code           string          `json:"code"`
	Severity       string          `json:"severity"`
	Params         json.RawMessage `json:"params"`
	Producer       string          `json:"producer"`
	Status         string          `json:"status"`
	ResolvedAt     *time.Time      `json:"resolved_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// List returns one page of warnings and the total.
func (p *Warnings) List(ctx context.Context, f Filter) ([]Row, int64, error) {
	eng := pgtype.Int8{}
	if f.EngagementID > 0 {
		eng = pgtype.Int8{Int64: f.EngagementID, Valid: true}
	}
	rows, err := p.q.ListTenantDataWarnings(ctx, db.ListTenantDataWarningsParams{
		OrganizationID: f.OrganizationID, EngagementID: eng, AllEngagements: f.AllEngagements, UserID: f.UserID,
		Period: optText(f.Period), Code: optText(f.Code), Severity: optText(f.Severity),
		Status: optText(f.Status), EntityType: optText(f.EntityType), Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list data warnings: %w", err)
	}
	total, err := p.q.CountTenantDataWarnings(ctx, db.CountTenantDataWarningsParams{
		OrganizationID: f.OrganizationID, EngagementID: eng, AllEngagements: f.AllEngagements, UserID: f.UserID,
		Period: optText(f.Period), Code: optText(f.Code), Severity: optText(f.Severity),
		Status: optText(f.Status), EntityType: optText(f.EntityType),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count data warnings: %w", err)
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		row := Row{
			UUID: r.Uuid, EngagementUUID: r.EngagementUuid, EntityType: r.EntityType,
			Code: r.Code, Severity: r.Severity, Params: json.RawMessage(r.Params), Producer: r.Producer,
			Status: r.Status, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		}
		if r.Period.Valid {
			v := r.Period.String
			row.Period = &v
		}
		switch {
		case r.InvoiceUuid.Valid:
			id := uuid.UUID(r.InvoiceUuid.Bytes)
			row.EntityUUID = &id
		case r.CustomerUuid.Valid:
			id := uuid.UUID(r.CustomerUuid.Bytes)
			row.EntityUUID = &id
		}
		if r.ResolvedAt.Valid {
			t := r.ResolvedAt.Time
			row.ResolvedAt = &t
		}
		out = append(out, row)
	}
	return out, total, nil
}

// Decide records the accountant's decision (acknowledged / resolved) on an
// open warning; false when no open warning has that uuid in the engagement.
func (p *Warnings) Decide(ctx context.Context, organizationID, engagementID int64, id uuid.UUID, status catalog.Status, userID int64) (bool, error) {
	by := pgtype.Int8{}
	if userID > 0 {
		by = pgtype.Int8{Int64: userID, Valid: true}
	}
	n, err := p.q.DecideDataWarning(ctx, db.DecideDataWarningParams{
		Status: string(status), ResolvedBy: by, OrganizationID: organizationID, EngagementID: engagementID, Uuid: id,
	})
	if err != nil {
		return false, fmt.Errorf("decide data warning: %w", err)
	}
	return n > 0, nil
}

func optInt8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func optText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// sameJSON compares two JSON documents by value (jsonb reorders keys).
func sameJSON(a, b []byte) bool {
	var va, vb any
	if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vb) != nil {
		return false
	}
	return reflect.DeepEqual(va, vb)
}
