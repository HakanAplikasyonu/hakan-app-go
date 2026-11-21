package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/planexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// DemoCariCustomerTaxID is the demo customer of the cari mizanı example.
const DemoCariCustomerTaxID = "6290341187"

// demoCariMarker is the description prefix of the cari mizanı vouchers; a
// second seed run finds them and adds nothing.
const demoCariMarker = "Cari örneği:"

// seedCariMizan adds the cari mizanı example (#780 C): November 2026
// vouchers, approved and not exported, on two named sub accounts. Ege
// Lojistik (320) was paid more than its invoice, so it carries a debit
// balance in 320: the screen marks it "Ters bakiye". Karadeniz Market (120)
// owes part of its invoice. Runs after seedPlanExport (Ege Lojistik's code)
// and touches no October number.
func seedCariMizan(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	var done bool
	err := rls.System(ctx, pool, "demo-seed: cari mizanı", func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM journal_drafts
			WHERE organization_id = $1 AND engagement_id = $2 AND description LIKE $3)`,
			scope.OrganizationID, scope.ID, demoCariMarker+"%").Scan(&done)
	})
	if err != nil || done {
		return err
	}
	plan := planexport.New(pool)
	ege, _, err := plan.Allocate(ctx, scope, actor, planexport.AllocateInput{
		MainCode: planexport.MainSuppliers, TaxID: DemoNewSupplierTaxID, Name: "Ege Lojistik Taşımacılık Ltd. Şti.",
	})
	if err != nil {
		return fmt.Errorf("supplier sub account: %w", err)
	}
	market, _, err := plan.Allocate(ctx, scope, actor, planexport.AllocateInput{
		MainCode: planexport.MainCustomers, TaxID: DemoCariCustomerTaxID, Name: "Karadeniz Market Gıda Ltd. Şti.",
	})
	if err != nil {
		return fmt.Errorf("customer sub account: %w", err)
	}
	day := func(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC) }
	amt := decimal.RequireFromString
	vouchers := []journals.DraftInput{
		{Date: day(3), Description: demoCariMarker + " Ege Lojistik nakliye faturası EGE2026000118", Lines: []journals.LineInput{
			{Account: "760.01", Description: "Nakliye", Debit: amt("10000.00")},
			{Account: "191.01", Description: "İndirilecek KDV", Debit: amt("2000.00")},
			{Account: ege.Code, Description: "EGE2026000118", Credit: amt("12000.00")},
		}},
		{Date: day(6), Description: demoCariMarker + " Ege Lojistik ödemesi (fazla gönderildi)", Lines: []journals.LineInput{
			{Account: ege.Code, Description: "Havale", Debit: amt("15000.00")},
			{Account: "102.01", Description: "Banka", Credit: amt("15000.00")},
		}},
		{Date: day(10), Description: demoCariMarker + " Karadeniz Market satış faturası ATL2026000412", Lines: []journals.LineInput{
			{Account: market.Code, Description: "ATL2026000412", Debit: amt("36000.00")},
			{Account: "600.01", Description: "Satış", Credit: amt("30000.00")},
			{Account: "391.01", Description: "Hesaplanan KDV", Credit: amt("6000.00")},
		}},
		{Date: day(17), Description: demoCariMarker + " Karadeniz Market tahsilatı", Lines: []journals.LineInput{
			{Account: "102.01", Description: "Banka", Debit: amt("20000.00")},
			{Account: market.Code, Description: "Havale", Credit: amt("20000.00")},
		}},
	}
	svc := journals.New(pool, seedStore)
	for _, v := range vouchers {
		d, err := svc.CreateDraft(ctx, scope, actor, v)
		if err != nil {
			return fmt.Errorf("cari voucher: %w", err)
		}
		if err := svc.Approve(ctx, scope, actor, d.UUID); err != nil {
			return fmt.Errorf("approve cari voucher: %w", err)
		}
	}
	return nil
}
