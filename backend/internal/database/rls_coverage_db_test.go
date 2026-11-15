package database

import (
	"context"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestRLSCoversScopedTables keeps RLS (issue #6) in step with the lint lists:
// every table in tenant_tables.txt and taxpayer_tables.txt must have row
// level security enabled and a policy for the scoped roles. A new table
// fails here until its migration calls rls_apply_org, rls_apply_engagement
// or rls_apply_taxpayer (DATABASE_RULES.md).
func TestRLSCoversScopedTables(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	type want struct{ table, policy string }
	var wants []want
	for _, tt := range readTenantTables(t) {
		p := "rls_org"
		if tt.engagement {
			p = "rls_engagement"
		}
		wants = append(wants, want{tt.name, p})
	}
	for _, name := range readTaxpayerTables(t) {
		wants = append(wants, want{name, "rls_taxpayer"})
	}
	for _, w := range wants {
		var enabled bool
		err := d.Pool.QueryRow(ctx, `
			SELECT c.relrowsecurity FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relname = $1`, w.table).Scan(&enabled)
		if err != nil {
			t.Errorf("%s: %v", w.table, err)
			continue
		}
		if !enabled {
			t.Errorf("%s: row level security is not enabled (its migration must call the rls_apply_* template that creates %s)", w.table, w.policy)
			continue
		}
		var ok bool
		if err := d.Pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM pg_policies
			               WHERE schemaname = 'public' AND tablename = $1 AND policyname = $2)`,
			w.table, w.policy).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Errorf("%s: policy %s missing", w.table, w.policy)
		}
	}
}
