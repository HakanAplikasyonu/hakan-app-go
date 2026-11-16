package rls_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// TestOrgSharedPolicy (issue #307, migration 000087, rls_apply_org_shared):
// an office scope reads its own rows and the platform's (organization_id
// NULL) rows, never another office's; it writes only its own rows; a NULL
// row is written only through rls.System. A taxpayer scope (no office)
// reads the NULL rows and writes nothing. Unscoped queries are unchanged.
func TestOrgSharedPolicy(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	const insert = `INSERT INTO excel_import_templates (scope, organization_id, fingerprint, data_type, source_system, header_row, columns)
		VALUES ($1, $2, repeat($3, 64), 'outgoing_invoices', 'test', 1, '[]')`
	err := rls.System(ctx, f.d.Pool, "test fixture", func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, insert, "global", nil, "a"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, insert, "tenant", f.orgA, "b"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, insert, "tenant", f.orgB, "c")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	visible := func(ctx context.Context, tx pgx.Tx) map[string]bool {
		rows, err := tx.Query(ctx, `SELECT scope || ':' || COALESCE(organization_id::text, 'null') FROM excel_import_templates`)
		if err != nil {
			t.Fatal(err)
		}
		got, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, g := range got {
			out[g] = true
		}
		return out
	}
	refused := func(ctx context.Context, tx pgx.Tx, what, sql string, args ...any) {
		t.Helper()
		sp, err := tx.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tag, err := sp.Exec(ctx, sql, args...)
		switch {
		case isRLSError(err):
		case err == nil && tag.RowsAffected() == 0 && !tag.Insert():
		default:
			t.Errorf("%s: %v (rows %d), want it refused", what, err, tag.RowsAffected())
		}
		_ = sp.Rollback(ctx)
	}
	idA, idB := strconv.FormatInt(f.orgA, 10), strconv.FormatInt(f.orgB, 10)

	// Office A.
	err = rls.WithScope(ctx, f.d.Pool, rls.Scope{OrganizationID: f.orgA, UserID: f.ownerA}, func(ctx context.Context, tx pgx.Tx) error {
		got := visible(ctx, tx)
		if !got["global:null"] || !got["tenant:"+idA] || got["tenant:"+idB] || len(got) != 2 {
			t.Errorf("office A sees %v, want global and its own", got)
		}
		if _, err := tx.Exec(ctx, insert, "tenant", f.orgA, "d"); err != nil {
			t.Errorf("office A writing its own row: %v", err)
		}
		refused(ctx, tx, "office A writing a NULL row", insert, "global", nil, "e")
		refused(ctx, tx, "office A writing office B's row", insert, "tenant", f.orgB, "e")
		refused(ctx, tx, "office A updating the NULL row", `UPDATE excel_import_templates SET source_system = 'x' WHERE organization_id IS NULL`)
		refused(ctx, tx, "office A deleting the NULL row", `DELETE FROM excel_import_templates WHERE organization_id IS NULL`)
		refused(ctx, tx, "office A taking the NULL row", `UPDATE excel_import_templates SET organization_id = $1, scope = 'tenant' WHERE organization_id IS NULL`, f.orgA)
		refused(ctx, tx, "office A giving away its row", `UPDATE excel_import_templates SET organization_id = NULL, scope = 'global' WHERE organization_id = $1`, f.orgA)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Taxpayer scope: the platform's rows only, no writes.
	err = rls.WithScope(ctx, f.d.Pool, rls.Scope{UserID: f.ownerA, TaxpayerIDs: []int64{f.tpA}}, func(ctx context.Context, tx pgx.Tx) error {
		if got := visible(ctx, tx); !got["global:null"] || len(got) != 1 {
			t.Errorf("taxpayer scope sees %v, want only the global row", got)
		}
		refused(ctx, tx, "taxpayer scope writing a NULL row", insert, "global", nil, "f")
		refused(ctx, tx, "taxpayer scope writing an office row", insert, "tenant", f.orgA, "f")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// rls.System writes the platform's rows; unscoped queries see everything.
	if err := rls.System(ctx, f.d.Pool, "test: platform template", func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, insert, "global", nil, "9")
		return err
	}); err != nil {
		t.Fatalf("System writing a NULL row: %v", err)
	}
	var n int
	if err := f.d.Pool.QueryRow(ctx, `SELECT count(*) FROM excel_import_templates`).Scan(&n); err != nil || n != 5 {
		t.Fatalf("unscoped count = %d %v, want 5", n, err)
	}
}
