package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/foreigninvoices"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/inbox"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Gelen kutusu (#780 b): the demo taxpayer has work of every kind waiting in
// the month of now. The current-month invoices (seedCurrentSales) have no
// journal entry and the Eylül masraf fişi waits as a late document
// (seedLateDocuments); this adds what the month lacks: an unmatched bank
// movement, a masraf fişi and a Z raporu awaiting approval, and a foreign
// service invoice that came in by e-mail. Rows are keyed by month
// (source_id / source_uuid), so a rerun in the same month adds nothing.
func seedInbox(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, now time.Time) error {
	t := now.In(inbox.Istanbul)
	period := t.Format("2006-01")
	day := func(d int) time.Time {
		if d > t.Day() {
			d = t.Day()
		}
		return time.Date(t.Year(), t.Month(), d, 0, 0, 0, 0, time.UTC)
	}
	key := "demo:gelen-kutusu:" + period

	// Banka: eşleşmemiş bir gelen havale (karşı hesabı belli değil).
	var boxID int64
	err := pool.QueryRow(ctx, `SELECT id FROM cash_boxes WHERE taxpayer_id = $1 AND iban = $2 ORDER BY id LIMIT 1`,
		scope.TaxpayerID, DemoBankIBAN).Scan(&boxID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return fmt.Errorf("inbox cash box: %w", err)
	default:
		d := day(5)
		if _, err := banking.NewService(pool, nil).Ingest(ctx, scope.TaxpayerID, boxID, nil, []banking.Transaction{{
			SourceSystem: "bank_file", SourceID: key + ":banka", CaptureMethod: banking.CaptureExcel,
			BookedAt: d, TransactionDate: d, Direction: banking.DirIn, Amount: "7350.00", Currency: "TRY",
			Description: "HAVALE GELEN SERDAR AKIN ACIKLAMASIZ", CounterpartyName: "Serdar Akın",
		}}); err != nil {
			return fmt.Errorf("inbox bank movement: %w", err)
		}
	}

	return rls.System(ctx, pool, "demo-seed: gelen kutusu (#780 b)", func(ctx context.Context, tx pgx.Tx) error {
		// Onay bekleyen masraf fişi.
		if _, err := tx.Exec(ctx, `
			INSERT INTO expenses (taxpayer_id, source_system, source_id, capture_method, expense_date, period,
			                      amount, currency, tax_rate, tax_exclusive_amount, tax_amount, description, receipt_type,
			                      vendor_name, approval_status)
			VALUES ($1, 'manual', $2, 'manual', $3::date, $4, 1440.00, 'TRY', 20, 1200.00, 240.00,
			        'Toner ve kağıt alımı', 'okc_receipt', 'Bilgi Ofis Malzemeleri', 'pending')
			ON CONFLICT (taxpayer_id, source_system, source_id) DO NOTHING`,
			scope.TaxpayerID, key+":masraf", day(7), period); err != nil {
			return fmt.Errorf("inbox expense: %w", err)
		}
		// Onay bekleyen Z raporu.
		if _, err := tx.Exec(ctx, `
			INSERT INTO retail_sales (taxpayer_id, source_system, source_id, capture_method, sale_date, period,
			                          z_report_number, okc_serial_number, subtotal_amount, tax_amount, total_amount, approval_status)
			VALUES ($1, 'demo', $2, 'manual', $3::date, $4, 'Z-0917', 'AV0000123456', 5000.00, 1000.00, 6000.00, 'pending')
			ON CONFLICT (taxpayer_id, source_system, source_id) DO NOTHING`,
			scope.TaxpayerID, key+":z", day(8), period); err != nil {
			return fmt.Errorf("inbox z report: %w", err)
		}
		// E-postayla gelen, işlenmemiş yabancı hizmet faturası (taslak).
		src := uuid.NewSHA1(uuid.NameSpaceURL, []byte(key+":yabanci"))
		if _, err := tx.Exec(ctx, `
			INSERT INTO foreign_service_invoices (organization_id, engagement_id, period, vendor, country, invoice_no,
			                                      invoice_date, amount, currency, service_kind, treatment, status,
			                                      source_kind, source_uuid, note)
			VALUES ($1, $2, $3, 'Google Cloud EMEA Ltd.', 'IE', $4, $5::date, 120.00, 'USD', $6, $7, 'draft',
			        'email', $8, 'Demo: e-postayla gelen fatura')
			ON CONFLICT (organization_id, engagement_id, source_kind, source_uuid) WHERE source_uuid IS NOT NULL DO NOTHING`,
			scope.OrganizationID, scope.ID, period, "GCP-"+period, day(3), foreigninvoices.KindCloud,
			foreigninvoices.TreatmentKDV2, src); err != nil {
			return fmt.Errorf("inbox foreign invoice: %w", err)
		}
		return nil
	})
}
