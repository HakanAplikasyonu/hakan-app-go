package repository

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// #277: a mükellef panel request runs in a taxpayer scope (no office). The
// OCR call's cost row belongs to the office and must be written without
// breaking the request's own transaction.
func TestLedgerFromTaxpayerScope(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", "9000000017")
	user := d.User(t)
	rec := usecase.UsageRecord{OrganizationID: org, TaxpayerID: &tp, UserID: &user, Purpose: "receipt_ocr", Provider: "fake", Model: "m",
		Usage: provider.Usage{InputTokens: 10, OutputTokens: 5}}
	scope := rls.Scope{UserID: user, TaxpayerIDs: []int64{tp}}

	// Before: inside the scope the insert is refused and poisons the transaction.
	err := rls.WithScope(ctx, d.Pool, scope, func(ctx context.Context, tx pgx.Tx) error {
		if err := NewPostgres(database.QueriesFor(d.Pool)).RecordUsage(ctx, rec); err == nil {
			t.Fatal("expected the scoped insert to be refused (test no longer reproduces #277)")
		}
		_, err := tx.Exec(ctx, "SELECT 1")
		return err
	})
	if err == nil {
		t.Fatal("expected the scoped transaction to be aborted")
	}

	// After: the ledger pool writes it in its own system transaction.
	store := NewPostgres(database.QueriesFor(d.Pool)).WithLedgerPool(d.Pool)
	err = rls.WithScope(ctx, d.Pool, scope, func(ctx context.Context, tx pgx.Tx) error {
		if err := store.RecordUsage(ctx, rec); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "SELECT 1") // the request's transaction is still usable
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM ai_usage_events WHERE organization_id = $1 AND taxpayer_id = $2`, org, tp).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("ledger rows = %d, want 1", n)
	}
}
