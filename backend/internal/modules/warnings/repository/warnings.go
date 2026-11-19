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
	GroupTenantDataWarnings(ctx context.Context, arg db.GroupTenantDataWarningsParams) ([]db.GroupTenantDataWarningsRow, error)
	AcknowledgeTenantDataWarningsByCode(ctx context.Context, arg db.AcknowledgeTenantDataWarningsByCodeParams) (int64, error)
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
	// Note is the reason of a bulk acknowledgement (#529).
	Note          string
	Limit, Offset int32
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
	// ResolvedByName and ResolutionNote: who decided and why (#529).
	ResolvedByName *string   `json:"resolved_by_name"`
	ResolutionNote *string   `json:"resolution_note"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// List returns one page of warnings and the total.
func (p *Warnings) List(ctx context.Context, f Filter) ([]Row, int64, error) {
	eng := optID(f.EngagementID)
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
		// Banka hareketi, masraf, Z raporu, fiş, finansal hareket; fatura
		// kalemi için faturası (#528).
		for _, v := range []pgtype.UUID{r.BankTransactionUuid, r.ExpenseUuid, r.RetailSaleUuid,
			r.ReceiptCaptureUuid, r.FinancialTransactionUuid, r.ItemInvoiceUuid} {
			if row.EntityUUID == nil && v.Valid {
				id := uuid.UUID(v.Bytes)
				row.EntityUUID = &id
			}
		}
		if r.ResolvedAt.Valid {
			t := r.ResolvedAt.Time
			row.ResolvedAt = &t
		}
		if r.ResolvedByName != "" {
			v := r.ResolvedByName
			row.ResolvedByName = &v
		}
		if r.ResolutionNote.Valid {
			v := r.ResolutionNote.String
			row.ResolutionNote = &v
		}
		out = append(out, row)
	}
	return out, total, nil
}

// Decide records the accountant's decision (acknowledged / resolved) on an
// open warning; false when no open warning has that uuid in the engagement.
func (p *Warnings) Decide(ctx context.Context, organizationID, engagementID int64, id uuid.UUID, status catalog.Status, userID int64, note string) (bool, error) {
	by := pgtype.Int8{}
	if userID > 0 {
		by = pgtype.Int8{Int64: userID, Valid: true}
	}
	n, err := p.q.DecideDataWarning(ctx, db.DecideDataWarningParams{
		Status: string(status), ResolvedBy: by, OrganizationID: organizationID, EngagementID: engagementID, Uuid: id,
		Note: optText(note),
	})
	if err != nil {
		return false, fmt.Errorf("decide data warning: %w", err)
	}
	return n > 0, nil
}

// Group is the warnings of one code in scope (issue #333).
type Group struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Count    int64  `json:"count"`
	// Engagements is the number of distinct engagements (office view only).
	Engagements *int64    `json:"engagements,omitempty"`
	LatestAt    time.Time `json:"latest_at"`
}

// Groups counts the warnings matching f (Code, EntityType, Limit and
// Offset are ignored) by code: heaviest severity first, then most frequent.
func (p *Warnings) Groups(ctx context.Context, f Filter) ([]Group, error) {
	rows, err := p.q.GroupTenantDataWarnings(ctx, db.GroupTenantDataWarningsParams{
		OrganizationID: f.OrganizationID, EngagementID: optID(f.EngagementID), AllEngagements: f.AllEngagements,
		UserID: f.UserID, Period: optText(f.Period), Severity: optText(f.Severity), Status: optText(f.Status),
	})
	if err != nil {
		return nil, fmt.Errorf("group data warnings: %w", err)
	}
	out := make([]Group, 0, len(rows))
	for _, r := range rows {
		g := Group{Code: r.Code, Severity: r.Severity, Count: r.WarningCount, LatestAt: r.LatestAt.Time}
		if f.EngagementID == 0 {
			n := r.EngagementCount
			g.Engagements = &n
		}
		out = append(out, g)
	}
	return out, nil
}

// AcknowledgeByCode acknowledges every open warning of f.Code (and f.Period
// when set) in scope, as Decide does for one: who and when are stored on
// the row. Only active engagements are touched. Returns how many changed.
func (p *Warnings) AcknowledgeByCode(ctx context.Context, f Filter) (int64, error) {
	by := pgtype.Int8{}
	if f.UserID > 0 {
		by = pgtype.Int8{Int64: f.UserID, Valid: true}
	}
	n, err := p.q.AcknowledgeTenantDataWarningsByCode(ctx, db.AcknowledgeTenantDataWarningsByCodeParams{
		ResolvedBy: by, OrganizationID: f.OrganizationID, EngagementID: optID(f.EngagementID),
		AllEngagements: f.AllEngagements, UserID: f.UserID, Code: f.Code, Period: optText(f.Period),
		Note: optText(f.Note),
	})
	if err != nil {
		return 0, fmt.Errorf("acknowledge data warnings: %w", err)
	}
	return n, nil
}

func optID(id int64) pgtype.Int8 {
	if id <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: id, Valid: true}
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
