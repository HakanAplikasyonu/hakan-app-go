package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/foreigninvoices"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Yabancı faturadan fiş taslağı (#780 KDV2 B). Çelik Robotik serbest
// meslek kazanç defteri tutar: yevmiye fişi oluşmaz. Fiş örneği bilanço
// esasındaki demo mükellefte (scope): Çelik'in geçen ayki iki onaylı
// faturasının (Google Ads, SolidCAD lisansı) aynısı eklenir ve fiş
// taslakları oluşturulur. Tekrar çalıştırmada fatura da fiş de çoğalmaz
// (satıcı + fatura no; fatura başına tek fiş bağı).
func seedForeignInvoiceJournals(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64, now time.Time) error {
	prev, _ := demoForeignMonths(now)
	err := rls.System(ctx, pool, "demo-seed: foreign invoices for journals", func(ctx context.Context, tx pgx.Tx) error {
		for _, f := range demoForeignInvoices {
			if !f.prev || !f.confirmed {
				continue
			}
			no := fmt.Sprintf(f.invoiceNo, prev[:4]+prev[5:])
			if _, err := tx.Exec(ctx, `INSERT INTO foreign_service_invoices (organization_id, engagement_id, period, vendor, country,
					invoice_no, invoice_date, amount, currency, exchange_rate, service_kind, treatment, stopaj_rate, gross_up, vat_rate,
					status, confirmed_at, note)
				SELECT $1, $2, $3, $4, $5, $6, $7::date, $8::numeric, $9, $10::numeric,
					$11, $12, $13::numeric, $14, 20, 'confirmed', NOW(), 'Demo kaydı'
				WHERE NOT EXISTS (SELECT 1 FROM foreign_service_invoices
					WHERE organization_id = $1 AND engagement_id = $2 AND vendor = $4 AND invoice_no = $6)`,
				scope.OrganizationID, scope.ID, prev, f.vendor, f.country, no, fmt.Sprintf("%s-%02d", prev, f.day), f.amount,
				f.currency, f.rate, f.kind, f.treatment, f.stopaj, f.grossUp); err != nil {
				return fmt.Errorf("foreign invoice %s: %w", no, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	svc := foreigninvoices.New(db.New(pool)).WithJournals(journals.New(pool, seedStore))
	res, err := svc.CreatePeriodJournals(ctx, scope, prev, actor)
	if errors.Is(err, foreigninvoices.ErrDisabled) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("foreign invoice journals: %w", err)
	}
	if len(res.Failed) > 0 {
		return fmt.Errorf("foreign invoice journals: %s", res.Failed[0].Message)
	}
	return nil
}
