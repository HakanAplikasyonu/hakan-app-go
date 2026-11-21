package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/reports"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// DemoReportEmail is the contact address of the demo taxpayers' reports
// (#386): MailHog catches it locally.
func DemoReportEmail(key string) string { return "rapor." + key + "@" + demo.EmailDomain }

// demoReportMonths are the months the demo report shows (the year up to
// the demo period 2026-10).
var demoReportMonths = reports.Months("2025-11", "2026-10")

// demoExpenses are the approved expenses of a demo month: document type,
// amount excluding VAT, VAT rate (percent). The month index varies them.
// The serbest meslek makbuzu has its issuer (name, TCKN) so it reaches
// Form Ba (#575); ÖKC / fuel receipts and the expense voucher do not.
var demoExpenses = []struct {
	kind        string
	base        int
	rate        int
	vendor, vkn string
}{
	{"fuel_receipt", 4200, 20, "", ""},
	{"okc_receipt", 2600, 20, "", ""},
	{"expense_voucher", 1800, 0, "", ""},
	{"bank_fee", 650, 0, "", ""},
	{"smm", 7500, 20, "Av. Deniz Arslan", "10000000146"},
}

// seedReportContact gives a demo taxpayer the address its reports go to, so
// one-click and bulk sending work on every demo taxpayer (#386).
func seedReportContact(ctx context.Context, pool *pgxpool.Pool, tpID int64, key string) error {
	return rls.System(ctx, pool, "demo-seed: report contact email", func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE taxpayers SET contact_email = $2 WHERE id = $1`, tpID, DemoReportEmail(key))
		return err
	})
}

// seedReport fills the taxpayer report of one demo taxpayer (#386): twelve months of approved Z reports and expenses (the
// charts have a trend and categories without uploading anything), an
// approved October comment, an enabled monthly schedule and two sent
// emails in the history. Invoices still come from the demo UBL uploads.
func seedReport(ctx context.Context, pool *pgxpool.Pool, orgID, engID, tpID int64, key, officeName string, now time.Time) error {
	return rls.System(ctx, pool, "demo-seed: taxpayer report", func(ctx context.Context, tx pgx.Tx) error {
		for i, m := range demoReportMonths {
			day := m + "-28"
			z := 38000 + i*2400 + (i%3)*5500
			if _, err := tx.Exec(ctx, `INSERT INTO retail_sales (taxpayer_id, source_system, source_id, capture_method, sale_date, period,
				z_report_number, subtotal_amount, tax_amount, total_amount, approval_status, approved_at)
				VALUES ($1, 'demo', $2, 'manual', $3::date, $4, $5, $6::numeric, $6::numeric * 0.20, $6::numeric * 1.20, 'approved', NOW())
				ON CONFLICT (taxpayer_id, source_system, source_id) DO NOTHING`,
				tpID, "demo:z-"+m, day, m, fmt.Sprintf("Z-%04d", 100+i), z); err != nil {
				return fmt.Errorf("z report %s: %w", m, err)
			}
			for j, e := range demoExpenses {
				base := e.base + ((i+j)%4)*350
				vat := base * e.rate / 100
				// Mevcut kayıt silinmez; yalnız boş satıcı alanları doldurulur.
				if _, err := tx.Exec(ctx, `INSERT INTO expenses (taxpayer_id, source_system, source_id, capture_method, expense_date, period,
					amount, tax_rate, tax_exclusive_amount, tax_amount, description, receipt_type, approval_status, approved_at,
					vendor_name, vendor_tax_number)
					VALUES ($1, 'demo', $2, 'manual', $3::date, $4, $5::numeric + $6::numeric, $7::numeric, $5::numeric, $6::numeric, 'Demo gider', $8, 'approved', NOW(),
					NULLIF($9, ''), NULLIF($10, ''))
					ON CONFLICT (taxpayer_id, source_system, source_id) DO UPDATE
					SET vendor_name = COALESCE(expenses.vendor_name, EXCLUDED.vendor_name),
					    vendor_tax_number = COALESCE(expenses.vendor_tax_number, EXCLUDED.vendor_tax_number)`,
					tpID, "demo:expense-"+m+"-"+e.kind, m+"-15", m, base, vat, e.rate, e.kind, e.vendor, e.vkn); err != nil {
					return fmt.Errorf("expense %s %s: %w", m, e.kind, err)
				}
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO period_summaries (organization_id, engagement_id, taxpayer_id, period, facts, template, body, status, approved_at)
			VALUES ($1, $2, $3, '2026-10', '[]', $4, $4, 'approved', NOW())`, orgID, engID, tpID,
			"Ekim ayında perakende satışlarınız bir önceki aya göre arttı; giderleriniz ise yatay seyretti. "+
				"Akaryakıt ve serbest meslek makbuzları giderlerin en büyük kalemleri.\n\n"+
				"Beyan öncesinde açık uyarılar incelendi; KDV tutarı tahminidir ve devreden KDV dahil değildir."); err != nil {
			return fmt.Errorf("summary: %w", err)
		}
		// Enabled schedule; last month counts as sent so a fresh demo does
		// not email at once.
		last := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0).Format("2006-01")
		if _, err := tx.Exec(ctx, `INSERT INTO report_schedules (organization_id, engagement_id, taxpayer_id, enabled, day_of_month, last_period)
			VALUES ($1, $2, $3, true, 1, $4) ON CONFLICT (organization_id, engagement_id) DO NOTHING`, orgID, engID, tpID, last); err != nil {
			return fmt.Errorf("schedule: %w", err)
		}
		q := db.New(tx)
		for _, h := range []struct {
			period, origin string
			sent           time.Time
		}{
			{"2026-09", reports.OriginScheduled, time.Date(2026, 10, 1, 7, 0, 0, 0, time.UTC)},
			{"2026-10", reports.OriginManual, time.Date(2026, 11, 3, 10, 30, 0, 0, time.UTC)},
		} {
			rg, _ := reports.ParseRange(h.period, "")
			rep, err := reports.Build(ctx, q, tpID, rg, reports.Options{OrganizationID: orgID, EngagementID: engID, ApprovedOnly: true}, now)
			if err != nil {
				return fmt.Errorf("report %s: %w", h.period, err)
			}
			snap, _ := json.Marshal(rep)
			if _, err := tx.Exec(ctx, `INSERT INTO report_deliveries (organization_id, engagement_id, taxpayer_id, kind, period, recipient, origin,
				status, attempts, report, office_name, sent_at)
				VALUES ($1, $2, $3, 'month', $4, $5, $6, 'sent', 1, $7, $8, $9)`,
				orgID, engID, tpID, h.period, DemoReportEmail(key), h.origin, snap, officeName, h.sent); err != nil {
				return fmt.Errorf("delivery %s: %w", h.period, err)
			}
		}
		return nil
	})
}
