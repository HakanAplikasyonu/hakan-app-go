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

// Ay başı belge toplama (#780): a few taxpayers sent documents two months
// ago but nothing last month, so the "Belge toplama" screen has rows on any
// day. Selin Yılmaz misses all four kinds (Ziraat statement, Z reports,
// expense receipts, purchase invoices); Kaya Kırtasiye and Deniz Market miss
// one bank statement each. The months follow today: last month (the period
// collected on days 1–5) stays empty, the month before gets one record.
const docCollectionSource = "demo-belge-toplama"

type docCollectionPlan struct {
	taxID string
	bank  string
	// all: also Z report, expense and purchase invoice.
	all bool
}

var docCollectionPlans = []docCollectionPlan{
	{taxID: "10000000146", bank: "Ziraat Bankası", all: true},
	{taxID: "20780000120", bank: "Halkbank"},
	{taxID: "20780000298", bank: "Vakıfbank"},
}

// seedDocumentCollection only adds rows (each keyed by docCollectionSource);
// running it again changes nothing.
func seedDocumentCollection(ctx context.Context, pool *pgxpool.Pool, now time.Time) error {
	t := now.In(istanbul)
	// The month before last: the documents that "usually" arrive.
	before := time.Date(t.Year(), t.Month()-2, 15, 12, 0, 0, 0, time.UTC)
	period := before.Format("2006-01")
	day := before.Format(time.DateOnly)
	return rls.System(ctx, pool, "demo-seed: document collection (#780)", func(ctx context.Context, tx pgx.Tx) error {
		for _, p := range docCollectionPlans {
			var tp int64
			err := tx.QueryRow(ctx, `SELECT id FROM taxpayers WHERE tax_id = $1`, p.taxID).Scan(&tp)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return fmt.Errorf("taxpayer %s: %w", p.taxID, err)
			}
			var box int64
			err = tx.QueryRow(ctx, `SELECT id FROM cash_boxes WHERE taxpayer_id = $1 AND type = 'bank_account' AND name = $2 ORDER BY id LIMIT 1`,
				tp, p.bank).Scan(&box)
			if errors.Is(err, pgx.ErrNoRows) {
				err = tx.QueryRow(ctx, `INSERT INTO cash_boxes (taxpayer_id, type, name, currency) VALUES ($1, 'bank_account', $2, 'TRY') RETURNING id`,
					tp, p.bank).Scan(&box)
			}
			if err != nil {
				return fmt.Errorf("bank account %s: %w", p.bank, err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO bank_transactions (taxpayer_id, cash_box_id, source_system, source_id, capture_method,
					booked_at, transaction_date, debit_credit, amount, currency, description, period, counterparty_name)
				SELECT $1, $2, 'bank_demo', $3, 'excel_import', $4::date + time '10:00', $4::date, 'B', 1850, 'TRY',
				       'Kira ödemesi', $5, 'Mülk Sahibi'
				WHERE NOT EXISTS (SELECT 1 FROM bank_transactions WHERE taxpayer_id = $1 AND source_system = 'bank_demo' AND source_id = $3)`,
				tp, box, docCollectionSource+"-"+period, day, period); err != nil {
				return fmt.Errorf("bank transaction: %w", err)
			}
			if !p.all {
				continue
			}
			src := docCollectionSource + "-" + period
			if _, err := tx.Exec(ctx, `
				INSERT INTO retail_sales (taxpayer_id, source_system, source_id, capture_method, sale_date, period,
					z_report_number, subtotal_amount, tax_amount, total_amount, approval_status)
				VALUES ($1, 'manual', $2, 'manual', $3::date, $4, '0142', 2500, 500, 3000, 'approved')
				ON CONFLICT (taxpayer_id, source_system, source_id) DO NOTHING`, tp, src, day, period); err != nil {
				return fmt.Errorf("z report: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO expenses (taxpayer_id, source_system, source_id, capture_method, expense_date, period,
					amount, currency, description, receipt_type, vendor_name, approval_status)
				VALUES ($1, 'manual', $2, 'manual', $3::date, $4, 420, 'TRY', 'Akaryakıt', 'fuel_receipt', 'Opet', 'approved')
				ON CONFLICT (taxpayer_id, source_system, source_id) DO NOTHING`, tp, src, day, period); err != nil {
				return fmt.Errorf("expense: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number,
					direction, type, issue_date, period, currency, line_extension_amount, tax_exclusive_amount,
					tax_amount, tax_inclusive_amount, payable_amount, tax_summary_source, customer_name, customer_tax_number)
				VALUES ($1, 'manual', $2, 'manual', $3, 'incoming', 'SATIS', $4::date, $5, 'TRY', 1000, 1000, 200, 1200, 1200, 'header_estimate',
					'Toptan Kırtasiye Ltd.', '9000000210')
				ON CONFLICT (taxpayer_id, source_system, source_id) DO NOTHING`,
				tp, src, "BTA"+before.Format("200601")+"000001", day, period); err != nil {
				return fmt.Errorf("purchase invoice: %w", err)
			}
		}
		return nil
	})
}
