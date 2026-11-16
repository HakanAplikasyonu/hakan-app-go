package rls_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// TestForOffice (issue #281): from a taxpayer scope, ForOffice writes the
// office's rows for the scope's own taxpayer at its active office and puts
// the taxpayer scope back; it refuses another office or another taxpayer,
// and a failing fn leaves nothing behind.
func TestForOffice(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	panel := rls.Scope{UserID: f.ownerA, TaxpayerIDs: []int64{f.tpA}}
	insert := func(ctx context.Context, org, eng int64, code ...string) error {
		c := "TEST_281"
		if len(code) > 0 {
			c = code[0]
		}
		tx, _ := dbtx.Tx(ctx)
		_, err := tx.Exec(ctx, `INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, code, severity, producer, entity_type)
			SELECT $1, $2, taxpayer_id, $3, 'warning', 'test', 'expense' FROM engagements WHERE id = $2`, org, eng, c)
		return err
	}
	setting := func(ctx context.Context, name string) string {
		tx, _ := dbtx.Tx(ctx)
		var v string
		_ = tx.QueryRow(ctx, `SELECT COALESCE(current_setting($1, true), '')`, name).Scan(&v)
		return v
	}
	errBoom := errors.New("boom")
	err := rls.WithScope(ctx, f.d.Pool, panel, func(ctx context.Context, tx pgx.Tx) error {
		// The taxpayer scope itself cannot write the office's row.
		sp, _ := tx.Begin(ctx)
		if err := insert(dbtx.WithTx(ctx, sp), f.orgA, f.engA); !isRLSError(err) {
			t.Errorf("taxpayer scope insert: %v, want a row level security violation", err)
		}
		_ = sp.Rollback(ctx)

		if err := rls.ForOffice(ctx, f.orgA, f.tpA, func(ctx context.Context) error {
			if got := setting(ctx, "app.organization_id"); got == "" {
				t.Error("no office inside ForOffice")
			}
			return insert(ctx, f.orgA, f.engA)
		}); err != nil {
			t.Fatalf("ForOffice: %v", err)
		}
		if got := setting(ctx, "app.organization_id"); got != "" {
			t.Errorf("office scope leaked out of ForOffice: %q", got)
		}
		if got := setting(ctx, "app.engagements_read_all"); got != "" {
			t.Errorf("read_all leaked out of ForOffice: %q", got)
		}
		// A failing fn rolls its writes back and keeps the transaction usable.
		if err := rls.ForOffice(ctx, f.orgA, f.tpA, func(ctx context.Context) error {
			if err := insert(ctx, f.orgA, f.engA, "TEST_281_ROLLED_BACK"); err != nil {
				return err
			}
			return errBoom
		}); !errors.Is(err, errBoom) {
			t.Fatalf("failing fn: %v", err)
		}
		// Office B does not serve taxpayer A; taxpayer B is not in the scope.
		if err := rls.ForOffice(ctx, f.orgB, f.tpA, func(context.Context) error { return nil }); !errors.Is(err, rls.ErrNotOfficeOfTaxpayer) {
			t.Errorf("other office: %v", err)
		}
		if err := rls.ForOffice(ctx, f.orgB, f.tpB, func(context.Context) error { return nil }); !errors.Is(err, rls.ErrNotOfficeOfTaxpayer) {
			t.Errorf("other taxpayer: %v", err)
		}
		_, err := tx.Exec(ctx, "SELECT 1")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// From an office scope, ForOffice for another office is refused.
	err = rls.WithScope(ctx, f.d.Pool, rls.Scope{OrganizationID: f.orgB, UserID: f.ownerB, TaxpayerIDs: []int64{f.tpB}}, func(ctx context.Context, _ pgx.Tx) error {
		if err := rls.ForOffice(ctx, f.orgA, f.tpA, func(context.Context) error { return nil }); !errors.Is(err, rls.ErrNotOfficeOfTaxpayer) {
			t.Errorf("office B acting for office A: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.d.Pool.QueryRow(ctx, `SELECT count(*) FROM data_warnings WHERE code LIKE 'TEST_281%' AND organization_id = $1`, f.orgA).Scan(&n); err != nil || n != 1 {
		t.Fatalf("office rows = %d %v, want 1 (the failed call rolled back)", n, err)
	}
}
