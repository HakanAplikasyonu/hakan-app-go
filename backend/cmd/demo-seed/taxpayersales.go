package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// salesDemoSource marks the current-month demo invoices (source_system).
const salesDemoSource = "demo_satis"

// salesDemoFrom is the first month the current-month invoices are added
// for: the demo's fixed month (2026-10) keeps only its UBL invoices, so
// its returns, reconciliation and tests stay as they are.
const salesDemoFrom = "2026-11"

// salesDemoInvoices are the current-month invoices of the demo taxpayer
// (#780): sales to a few buyers and two purchases, so the mükellef panel's
// VAT card and sales report have figures in the month they open on.
var salesDemoInvoices = []struct {
	day       int
	direction string
	name, vkn string
	base      int // TL, excluding VAT
	rate      int // percent
}{
	{2, "outgoing", "Marmara Market Zinciri A.Ş.", "6130412887", 84000, 20},
	{4, "outgoing", "Ege Toptan Gıda Ltd. Şti.", "3270558341", 46500, 10},
	{6, "outgoing", "Marmara Market Zinciri A.Ş.", "6130412887", 52000, 20},
	{9, "outgoing", "Başkent Lokantacılık Ltd. Şti.", "1460873925", 18750, 10},
	{12, "outgoing", "Karadeniz Otelcilik A.Ş.", "5094216638", 27300, 20},
	{3, "incoming", "Anadolu Un Sanayi A.Ş.", "0710934452", 61000, 1},
	{8, "incoming", "Trakya Ambalaj Ltd. Şti.", "8823105764", 14200, 20},
}

// seedCurrentSales adds salesDemoInvoices to the month of now for the demo
// taxpayer. Keyed by month and row (source_id), so a rerun in the same
// month adds nothing; days after today move to today.
func seedCurrentSales(ctx context.Context, pool *pgxpool.Pool, tpID int64, now time.Time) error {
	now = now.In(time.FixedZone("TRT", 3*60*60))
	period := now.Format("2006-01")
	if period < salesDemoFrom {
		return nil
	}
	return rls.System(ctx, pool, "demo-seed: current-month sales (#780)", func(ctx context.Context, tx pgx.Tx) error {
		for i, inv := range salesDemoInvoices {
			day := min(inv.day, now.Day())
			date := time.Date(now.Year(), now.Month(), day, 0, 0, 0, 0, time.UTC)
			vat := inv.base * inv.rate / 100
			number := fmt.Sprintf("ATS%s%09d", now.Format("2006"), now.Year()%100*1000000+int(now.Month())*1000+i+1)
			var id int64
			err := tx.QueryRow(ctx, `
				INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number,
					direction, type, issue_date, period, currency, line_extension_amount, tax_exclusive_amount,
					tax_amount, tax_inclusive_amount, payable_amount, tax_summary_source, customer_name,
					customer_tax_number)
				VALUES ($1, $2, $3, 'api', $4, $5, $6, $7::date, $8, 'TRY', $9::numeric, $9::numeric, $10::numeric,
					$11::numeric, $11::numeric, 'source_summary', $12, $13)
				ON CONFLICT (taxpayer_id, source_system, source_id) DO NOTHING
				RETURNING id`,
				tpID, salesDemoSource, fmt.Sprintf("%s-%02d", period, i+1), number, inv.direction, "SATIS",
				date, period, inv.base, vat, inv.base+vat, inv.name, inv.vkn).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				continue // already seeded this month
			}
			if err != nil {
				return fmt.Errorf("invoice %d: %w", i+1, err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO invoice_tax_summaries (taxpayer_id, invoice_id, tax_rate, base, vat)
				VALUES ($1, $2, $3::numeric, $4::numeric, $5::numeric)`, tpID, id, inv.rate, inv.base, vat); err != nil {
				return fmt.Errorf("invoice %d summary: %w", i+1, err)
			}
		}
		return nil
	})
}
