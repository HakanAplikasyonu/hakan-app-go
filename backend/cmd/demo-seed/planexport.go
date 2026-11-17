package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/planexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// DemoNewSupplierTaxID is the demo's new supplier: Luca has no account for
// it yet, so the Luca hesap planı "yeni hesaplar" file lists it (#464).
const DemoNewSupplierTaxID = "4810054422"

// seedPlanExport gives the engagement a new supplier card, its generated
// 320 sub account (next free code after the imported Luca plan) and one
// "yeni hesaplar" download in the history. Runs after the reconciliation so
// the October story keeps its numbers.
func seedPlanExport(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	err := rls.System(ctx, pool, "demo-seed: plan export supplier", func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO customers (taxpayer_id, source_system, source_id, capture_method, type, name,
				tax_number, tax_office, address, city, email)
			SELECT $1, 'manual_upload', 'demo-ege-lojistik', 'manual', 'supplier', 'Ege Lojistik Taşımacılık Ltd. Şti.',
				$2::text, 'Karşıyaka', 'Ataşehir Mah. 6012 Sok. No: 4', 'İzmir', 'muhasebe@egelojistik.example'
			WHERE NOT EXISTS (SELECT 1 FROM customers WHERE taxpayer_id = $1 AND source_system = 'manual_upload' AND source_id = 'demo-ege-lojistik')`,
			scope.TaxpayerID, DemoNewSupplierTaxID)
		return err
	})
	if err != nil {
		return fmt.Errorf("supplier card: %w", err)
	}
	svc := planexport.New(pool)
	if _, _, err := svc.Allocate(ctx, scope, actor, planexport.AllocateInput{
		MainCode: planexport.MainSuppliers, TaxID: DemoNewSupplierTaxID, Name: "Ege Lojistik Taşımacılık Ltd. Şti.",
	}); err != nil {
		return fmt.Errorf("sub account: %w", err)
	}
	history, err := svc.Exports(ctx, scope, 1, 0)
	if err != nil {
		return err
	}
	if len(history) > 0 {
		return nil
	}
	if _, err := svc.BuildFile(ctx, scope, actor, planexport.FileInput{Kind: planexport.KindNew}); err != nil {
		return fmt.Errorf("plan file: %w", err)
	}
	return nil
}
