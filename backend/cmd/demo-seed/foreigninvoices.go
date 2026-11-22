package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/foreigninvoices"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Yabancı hizmet faturaları (#780 A1): Emre Çelik - Çelik Robotik
// Mühendislik'in geçen ay ve bu ayki Google Ads, AWS ve yazılım lisansı
// faturaları. Geçen ayınkiler onaylı; bu ayki Google Ads faturasının kuru
// elle girilmedi, TCMB tablosundan otomatik gelir (#780). Aynı satıcı +
// fatura no ikinci kez eklenmez.

// demoForeignInvoice is one demo row; prev selects last month.
type demoForeignInvoice struct {
	prev                                 bool
	vendor, country, invoiceNo, currency string
	day                                  int
	amount, rate                         string // rate "": kur eksik
	kind, treatment, stopaj              string
	grossUp, confirmed                   bool
}

var demoForeignInvoices = []demoForeignInvoice{
	{true, "Google Ireland Ltd.", "IE", "GADS-%s-0412", "USD", 3, "1250.00", "34.20", foreigninvoices.KindAdvertising, foreigninvoices.TreatmentKDV2, "0", false, true},
	{true, "SolidCAD Software GmbH", "DE", "SC-%s-118", "EUR", 12, "900.00", "37.10", foreigninvoices.KindSoftware, foreigninvoices.TreatmentKDV2Stopaj, "15", true, true},
	{false, "Amazon Web Services, Inc.", "US", "AWS-%s-77031", "USD", 2, "203.15", "34.55", foreigninvoices.KindCloud, foreigninvoices.TreatmentKDV2, "0", false, false},
	{false, "Google Ireland Ltd.", "IE", "GADS-%s-0519", "USD", 3, "980.00", "", foreigninvoices.KindAdvertising, foreigninvoices.TreatmentKDV2, "0", false, false},
}

// demoForeignMonths returns last month and this month (Europe/Istanbul).
func demoForeignMonths(now time.Time) (prev, cur string) {
	t := now.In(foreigninvoices.Istanbul)
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, foreigninvoices.Istanbul)
	return first.AddDate(0, -1, 0).Format("2006-01"), first.Format("2006-01")
}

// seedForeignInvoices adds the demo foreign service invoices to Çelik's
// engagement. It runs on every seed and adds only missing rows.
func seedForeignInvoices(ctx context.Context, pool *pgxpool.Pool, orgID int64, now time.Time) error {
	var taxID string
	for _, tp := range demo.OperatingTaxpayers {
		if tp.Key == demo.CelikKey {
			taxID = tp.TaxID
		}
	}
	prev, cur := demoForeignMonths(now)
	if err := rls.System(ctx, pool, "demo-seed: foreign service invoices", func(ctx context.Context, tx pgx.Tx) error {
		var engID int64
		err := tx.QueryRow(ctx, `SELECT e.id FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id
			WHERE e.organization_id = $1 AND t.tax_id = $2 AND e.status = 'active' ORDER BY e.id DESC LIMIT 1`,
			orgID, taxID).Scan(&engID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("celik engagement: %w", err)
		}
		for _, f := range demoForeignInvoices {
			period := cur
			if f.prev {
				period = prev
			}
			no := fmt.Sprintf(f.invoiceNo, period[:4]+period[5:])
			status := foreigninvoices.StatusDraft
			if f.confirmed {
				status = foreigninvoices.StatusConfirmed
			}
			if _, err := tx.Exec(ctx, `INSERT INTO foreign_service_invoices (organization_id, engagement_id, period, vendor, country,
					invoice_no, invoice_date, amount, currency, exchange_rate, service_kind, treatment, stopaj_rate, gross_up, vat_rate,
					status, confirmed_at, note, exchange_rate_source)
				SELECT $1, $2, $3, $4, $5, $6, $7::date, $8::numeric, $9, NULLIF($10, '')::numeric,
					$11, $12, $13::numeric, $14, 20, $15, CASE WHEN $15 = 'confirmed' THEN NOW() END, 'Demo kaydı',
					CASE WHEN $10 <> '' THEN 'manual' END
				WHERE NOT EXISTS (SELECT 1 FROM foreign_service_invoices
					WHERE organization_id = $1 AND engagement_id = $2 AND vendor = $4 AND invoice_no = $6)`,
				orgID, engID, period, f.vendor, f.country, no, fmt.Sprintf("%s-%02d", period, f.day), f.amount, f.currency, f.rate,
				f.kind, f.treatment, f.stopaj, f.grossUp, status); err != nil {
				return fmt.Errorf("foreign invoice %s: %w", no, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	// Kuru girilmemiş taslak (bu ayın Google Ads faturası) TCMB kuruyla
	// dolar (#780); elle girilen kurlar değişmez.
	return fillDemoForeignRates(ctx, pool, orgID)
}
