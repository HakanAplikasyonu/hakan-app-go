package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/latedocs"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Demo late documents (#780 C): two Eylül 2026 masraf fişleri arrive after
// the Eylül beyanları were filed (seedTaxReturns). One waits for the office's
// decision; the other was booked in Kasım 2026 ("cari döneme alındı").
const (
	demoLatePendingSource  = "demo-gec-belge-1"
	demoLateResolvedSource = "demo-gec-belge-2"
)

// seedLateDocuments runs after seedTaxReturns. It only adds rows: two new
// expenses and their late document rows; running it again changes nothing.
func seedLateDocuments(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	return rls.System(ctx, pool, "demo-seed: late documents (#780 C)", func(ctx context.Context, tx pgx.Tx) error {
		for _, x := range []struct {
			source, date, desc, vendor, amount string
		}{
			{demoLatePendingSource, "2026-09-29", "Kargo gönderim bedeli", "Yurtiçi Kargo", "1250.00"},
			{demoLateResolvedSource, "2026-09-30", "Kırtasiye alımı", "Ofis Dünyası Kırtasiye", "640.00"},
		} {
			if _, err := tx.Exec(ctx, `
				INSERT INTO expenses (taxpayer_id, source_system, source_id, capture_method, expense_date, period,
				                      amount, currency, description, receipt_type, vendor_name, approval_status)
				VALUES ($1, 'manual', $2, 'manual', $3::date, to_char($3::date, 'YYYY-MM'), $4::numeric, 'TRY', $5,
				        'expense_voucher', $6, 'approved')
				ON CONFLICT (taxpayer_id, source_system, source_id) DO NOTHING`,
				scope.TaxpayerID, x.source, x.date, x.amount, x.desc, x.vendor); err != nil {
				return fmt.Errorf("late expense %s: %w", x.source, err)
			}
		}
		svc := latedocs.New(db.New(tx))
		sub := latedocs.Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID}
		list, err := svc.List(ctx, sub, latedocs.Filter{Period: "2026-09"})
		if err != nil {
			return fmt.Errorf("late documents: %w", err)
		}
		var resolved string
		if err := tx.QueryRow(ctx, `SELECT uuid::text FROM expenses WHERE taxpayer_id = $1 AND source_system = 'manual' AND source_id = $2`,
			scope.TaxpayerID, demoLateResolvedSource).Scan(&resolved); err != nil {
			return fmt.Errorf("late expense: %w", err)
		}
		for _, it := range list.Items {
			if it.DocumentUUID.String() != resolved || it.Status != latedocs.StatusPending {
				continue
			}
			if _, err := svc.Decide(ctx, sub, it.UUID, latedocs.Decision{
				Status: latedocs.StatusCurrentPeriod, TargetPeriod: "2026-11", Note: "Kasım masraflarıyla işlendi.",
			}, actor); err != nil {
				return fmt.Errorf("late document decision: %w", err)
			}
		}
		return nil
	})
}
