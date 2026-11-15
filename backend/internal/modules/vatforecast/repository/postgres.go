// Package repository reads the VAT forecast inputs from Postgres
// (queries/vat_forecast.sql) and stores opening carry-forwards.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// Queries is the subset of db.Querier used here.
type Queries interface {
	VATForecastLines(ctx context.Context, arg db.VATForecastLinesParams) ([]db.VATForecastLinesRow, error)
	VATForecastExcluded(ctx context.Context, arg db.VATForecastExcludedParams) (db.VATForecastExcludedRow, error)
	VATForecastHistory(ctx context.Context, arg db.VATForecastHistoryParams) ([]db.VATForecastHistoryRow, error)
	GetVATOpeningAtOrBefore(ctx context.Context, arg db.GetVATOpeningAtOrBeforeParams) (db.GetVATOpeningAtOrBeforeRow, error)
	ListVATOpenings(ctx context.Context, arg db.ListVATOpeningsParams) ([]db.ListVATOpeningsRow, error)
	UpsertVATOpening(ctx context.Context, arg db.UpsertVATOpeningParams) (db.UpsertVATOpeningRow, error)
	DeleteVATOpening(ctx context.Context, arg db.DeleteVATOpeningParams) (int64, error)
	ListEngagementsByOrg(ctx context.Context, arg db.ListEngagementsByOrgParams) ([]db.ListEngagementsByOrgRow, error)
	CountEngagementsByOrg(ctx context.Context, arg db.CountEngagementsByOrgParams) (int64, error)
	ListAssignedEngagements(ctx context.Context, arg db.ListAssignedEngagementsParams) ([]db.ListAssignedEngagementsRow, error)
	CountAssignedEngagements(ctx context.Context, arg db.CountAssignedEngagementsParams) (int64, error)
}

// Postgres implements usecase.Store.
type Postgres struct{ q Queries }

// New builds the store.
func New(q Queries) *Postgres { return &Postgres{q: q} }

var _ usecase.Store = (*Postgres)(nil)

// Lines implements usecase.Store.
func (p *Postgres) Lines(ctx context.Context, taxpayerID int64, period string) ([]usecase.Line, error) {
	rows, err := p.q.VATForecastLines(ctx, db.VATForecastLinesParams{TaxpayerID: taxpayerID, Period: period})
	if err != nil {
		return nil, fmt.Errorf("vat forecast lines: %w", err)
	}
	out := make([]usecase.Line, 0, len(rows))
	for _, r := range rows {
		out = append(out, usecase.Line{
			Direction: r.Direction, Type: r.Type, TaxRate: dec(r.TaxRate), WithholdingCode: r.WithholdingCode,
			InvoiceCount: r.InvoiceCount, Base: dec(r.Base), VAT: dec(r.Vat), Withholding: dec(r.Withholding),
		})
	}
	return out, nil
}

// Excluded implements usecase.Store.
func (p *Postgres) Excluded(ctx context.Context, taxpayerID int64, period string) (usecase.Excluded, error) {
	r, err := p.q.VATForecastExcluded(ctx, db.VATForecastExcludedParams{TaxpayerID: taxpayerID, Period: period})
	if err != nil {
		return usecase.Excluded{}, fmt.Errorf("vat forecast excluded: %w", err)
	}
	return usecase.Excluded{
		Included: r.Included, Cancelled: r.Cancelled, Draft: r.Draft, DeletedAtSource: r.DeletedAtSource, MissingExchangeRate: r.MissingExchangeRate,
	}, nil
}

// History implements usecase.Store.
func (p *Postgres) History(ctx context.Context, taxpayerID int64, from, before string) ([]usecase.PeriodTotal, error) {
	rows, err := p.q.VATForecastHistory(ctx, db.VATForecastHistoryParams{TaxpayerID: taxpayerID, FromPeriod: from, BeforePeriod: before})
	if err != nil {
		return nil, fmt.Errorf("vat forecast history: %w", err)
	}
	out := make([]usecase.PeriodTotal, 0, len(rows))
	for _, r := range rows {
		out = append(out, usecase.PeriodTotal{Period: r.Period, Output: dec(r.OutputVat), Input: dec(r.InputVat)})
	}
	return out, nil
}

// OpeningAtOrBefore implements usecase.Store.
func (p *Postgres) OpeningAtOrBefore(ctx context.Context, organizationID, engagementID int64, period string) (*usecase.Opening, error) {
	r, err := p.q.GetVATOpeningAtOrBefore(ctx, db.GetVATOpeningAtOrBeforeParams{
		OrganizationID: organizationID, EngagementID: engagementID, Period: period,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("vat opening: %w", err)
	}
	o := opening(r.Period, r.Amount, r.Note, r.UpdatedAt)
	return &o, nil
}

// ListOpenings implements usecase.Store.
func (p *Postgres) ListOpenings(ctx context.Context, organizationID, engagementID int64) ([]usecase.Opening, error) {
	rows, err := p.q.ListVATOpenings(ctx, db.ListVATOpeningsParams{OrganizationID: organizationID, EngagementID: engagementID})
	if err != nil {
		return nil, fmt.Errorf("list vat openings: %w", err)
	}
	out := make([]usecase.Opening, 0, len(rows))
	for _, r := range rows {
		out = append(out, opening(r.Period, r.Amount, r.Note, r.UpdatedAt))
	}
	return out, nil
}

// UpsertOpening implements usecase.Store.
func (p *Postgres) UpsertOpening(ctx context.Context, organizationID, engagementID int64, o usecase.Opening, userID int64) (usecase.Opening, error) {
	note := pgtype.Text{}
	if o.Note != "" {
		note = pgtype.Text{String: o.Note, Valid: true}
	}
	by := pgtype.Int8{}
	if userID > 0 {
		by = pgtype.Int8{Int64: userID, Valid: true}
	}
	r, err := p.q.UpsertVATOpening(ctx, db.UpsertVATOpeningParams{
		OrganizationID: organizationID, EngagementID: engagementID, Period: o.Period,
		Amount: num(o.Amount.Dec()), Note: note, UpdatedBy: by,
	})
	if err != nil {
		return usecase.Opening{}, fmt.Errorf("upsert vat opening: %w", err)
	}
	return opening(r.Period, r.Amount, r.Note, r.UpdatedAt), nil
}

// DeleteOpening implements usecase.Store.
func (p *Postgres) DeleteOpening(ctx context.Context, organizationID, engagementID int64, period string) (bool, error) {
	n, err := p.q.DeleteVATOpening(ctx, db.DeleteVATOpeningParams{OrganizationID: organizationID, EngagementID: engagementID, Period: period})
	if err != nil {
		return false, fmt.Errorf("delete vat opening: %w", err)
	}
	return n > 0, nil
}

// ActiveEngagements implements usecase.Store.
func (p *Postgres) ActiveEngagements(ctx context.Context, f usecase.OfficeFilter) ([]usecase.EngagementRef, int64, error) {
	status := pgtype.Text{String: engagementctx.StatusActive, Valid: true}
	var out []usecase.EngagementRef
	var total int64
	if f.AllEngagements {
		rows, err := p.q.ListEngagementsByOrg(ctx, db.ListEngagementsByOrgParams{
			OrganizationID: f.OrganizationID, Status: status, LimitCount: f.Limit, OffsetCount: f.Offset,
		})
		if err != nil {
			return nil, 0, fmt.Errorf("list engagements: %w", err)
		}
		for _, r := range rows {
			out = append(out, usecase.EngagementRef{ID: r.ID, UUID: r.Uuid, TaxpayerID: r.TaxpayerID, TaxpayerUUID: r.TaxpayerUuid})
		}
		if total, err = p.q.CountEngagementsByOrg(ctx, db.CountEngagementsByOrgParams{OrganizationID: f.OrganizationID, Status: status}); err != nil {
			return nil, 0, fmt.Errorf("count engagements: %w", err)
		}
		return out, total, nil
	}
	rows, err := p.q.ListAssignedEngagements(ctx, db.ListAssignedEngagementsParams{
		OrganizationID: f.OrganizationID, UserID: f.UserID, Status: status, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list assigned engagements: %w", err)
	}
	for _, r := range rows {
		out = append(out, usecase.EngagementRef{ID: r.ID, UUID: r.Uuid, TaxpayerID: r.TaxpayerID, TaxpayerUUID: r.TaxpayerUuid})
	}
	if total, err = p.q.CountAssignedEngagements(ctx, db.CountAssignedEngagementsParams{
		OrganizationID: f.OrganizationID, UserID: f.UserID, Status: status,
	}); err != nil {
		return nil, 0, fmt.Errorf("count assigned engagements: %w", err)
	}
	return out, total, nil
}

func opening(period string, amount pgtype.Numeric, note pgtype.Text, updated pgtype.Timestamptz) usecase.Opening {
	return usecase.Opening{Period: period, Amount: usecase.M(dec(amount)), Note: note.String, UpdatedAt: updated.Time}
}

func dec(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

func num(d decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}
