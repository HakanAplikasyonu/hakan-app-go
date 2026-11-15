// Package repository persists the AI cost ledger (ai_usage_events).
package repository

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
	"github.com/jackc/pgx/v5/pgtype"
)

// Queries is the subset of db.Querier this repository uses.
type Queries interface {
	InsertAIUsageEvent(ctx context.Context, arg db.InsertAIUsageEventParams) error
	AIUsageReport(ctx context.Context, arg db.AIUsageReportParams) ([]db.AIUsageReportRow, error)
}

// Postgres implements usecase.UsageStore.
type Postgres struct{ q Queries }

// NewPostgres builds the store.
func NewPostgres(q Queries) *Postgres { return &Postgres{q: q} }

var _ usecase.UsageStore = (*Postgres)(nil)

// RecordUsage inserts one ledger row.
func (p *Postgres) RecordUsage(ctx context.Context, rec usecase.UsageRecord) error {
	params := db.InsertAIUsageEventParams{
		OrganizationID:   rec.OrganizationID,
		TaxpayerID:       optInt8(rec.TaxpayerID),
		UserID:           optInt8(rec.UserID),
		Purpose:          truncate(rec.Purpose, 64),
		Provider:         truncate(rec.Provider, 32),
		Model:            truncate(rec.Model, 128),
		IsFallback:       rec.Fallback,
		InputTokens:      rec.Usage.InputTokens,
		OutputTokens:     rec.Usage.OutputTokens,
		CacheReadTokens:  rec.Usage.CacheReadTokens,
		CacheWriteTokens: rec.Usage.CacheWriteTokens,
		MaskedValues:     int32(rec.MaskedValues), //nolint:gosec // distinct masked values per call are small
	}
	if rec.CostUSD != "" {
		if err := params.CostUsd.Scan(rec.CostUSD); err != nil {
			return fmt.Errorf("ai usage cost %q: %w", rec.CostUSD, err)
		}
	}
	if err := p.q.InsertAIUsageEvent(ctx, params); err != nil {
		return fmt.Errorf("insert ai usage: %w", err)
	}
	return nil
}

// UsageReport returns the grouped report for [from, to).
func (p *Postgres) UsageReport(ctx context.Context, organizationID int64, from, to time.Time) ([]usecase.ReportRow, error) {
	rows, err := p.q.AIUsageReport(ctx, db.AIUsageReportParams{
		OrganizationID: organizationID,
		FromAt:         pgtype.Timestamptz{Time: from, Valid: true},
		ToAt:           pgtype.Timestamptz{Time: to, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("ai usage report: %w", err)
	}
	out := make([]usecase.ReportRow, 0, len(rows))
	for _, r := range rows {
		row := usecase.ReportRow{
			Model:            r.Model,
			Calls:            r.Calls,
			FallbackCalls:    r.FallbackCalls,
			UnpricedCalls:    r.UnpricedCalls,
			InputTokens:      r.InputTokens,
			OutputTokens:     r.OutputTokens,
			CacheReadTokens:  r.CacheReadTokens,
			CacheWriteTokens: r.CacheWriteTokens,
			CostUSD:          numericString(r.CostUsd),
		}
		if r.Month.Valid {
			row.Month = r.Month.Time.Format("2006-01")
		}
		if r.TaxpayerID.Valid {
			id := r.TaxpayerID.Int64
			row.TaxpayerID = &id
		}
		out = append(out, row)
	}
	return out, nil
}

func optInt8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

// numericString renders a numeric as a 6-fraction-digit decimal string
// (pgtype's own text form drops trailing zeros).
func numericString(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil || n.NaN {
		return usecase.FormatUSD(0)
	}
	// value = Int x 10^Exp; nano-USD = Int x 10^(Exp+9).
	v := new(big.Int).Set(n.Int)
	if shift := int64(n.Exp) + 9; shift >= 0 {
		v.Mul(v, new(big.Int).Exp(big.NewInt(10), big.NewInt(shift), nil))
	} else {
		v.Quo(v, new(big.Int).Exp(big.NewInt(10), big.NewInt(-shift), nil))
	}
	if !v.IsInt64() {
		return usecase.FormatUSD(0)
	}
	return usecase.FormatUSD(v.Int64())
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
