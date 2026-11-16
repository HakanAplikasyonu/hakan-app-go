package database

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

func readSharedTables(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("shared_tables.txt")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		t.Fatal("shared_tables.txt is empty")
	}
	return out
}

// TestSharedTablesPolicies (issue #307): every table in shared_tables.txt
// carries exactly the rls_apply_org_shared policies for the scoped roles —
// a SELECT policy that also opens the organization_id NULL rows and a write
// policy limited to the scope's office — plus rls_legacy and the owner's
// rls_system, with FORCE on. Nothing else reaches the scoped roles.
func TestSharedTablesPolicies(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	for _, table := range readSharedTables(t) {
		var enabled, forced bool
		if err := d.Pool.QueryRow(ctx, `
			SELECT c.relrowsecurity, c.relforcerowsecurity FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relname = $1`, table).Scan(&enabled, &forced); err != nil {
			t.Errorf("%s: %v", table, err)
			continue
		}
		if !enabled || !forced {
			t.Errorf("%s: rls=%v force=%v (its migration must call rls_apply_org_shared)", table, enabled, forced)
		}
		rows, err := d.Pool.Query(ctx, `
			SELECT policyname::text, cmd, roles::text[], COALESCE(qual, ''), COALESCE(with_check, '')
			FROM pg_policies WHERE schemaname = 'public' AND tablename = $1`, table)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for rows.Next() {
			var name, cmd, qual, check string
			var roles []string
			if err := rows.Scan(&name, &cmd, &roles, &qual, &check); err != nil {
				t.Fatal(err)
			}
			got[name] = true
			switch name {
			case "rls_shared_read":
				if cmd != "SELECT" || !strings.Contains(qual, "organization_id IS NULL") || !strings.Contains(qual, "app_current_organization_id()") {
					t.Errorf("%s.rls_shared_read: cmd %s qual %q", table, cmd, qual)
				}
			case "rls_shared_write":
				if cmd != "ALL" || strings.Contains(qual, "IS NULL") || strings.Contains(check, "IS NULL") ||
					!strings.Contains(qual, "app_current_organization_id()") || !strings.Contains(check, "app_current_organization_id()") {
					t.Errorf("%s.rls_shared_write: cmd %s qual %q check %q (must not open NULL rows)", table, cmd, qual, check)
				}
			case "rls_legacy", "rls_system":
			default:
				for _, r := range roles {
					if r == "hakan_rls_app" || r == "hakan_rls_worker" || r == "public" {
						t.Errorf("%s: extra policy %s reaches the scoped roles", table, name)
					}
				}
			}
		}
		rows.Close()
		for _, want := range []string{"rls_shared_read", "rls_shared_write", "rls_legacy", "rls_system"} {
			if !got[want] {
				t.Errorf("%s: policy %s missing", table, want)
			}
		}
	}
}
