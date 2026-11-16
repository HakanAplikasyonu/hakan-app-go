package database

import (
	"context"
	"sort"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// rlsInvariantExceptions are the tables with an organization_id or
// taxpayer_id column that are deliberately outside row level security, each
// with the reason. Every other such table must have RLS enabled, FORCE on
// and at least one policy (TestRLSInvariant). Remove an entry when its
// table gets its policies; the test fails while a listed table is already
// protected or no longer exists.
//
// All of these come from the boilerplate and keep rows without an office
// (organization_id NULL): an office-only policy would hide those rows in
// every scoped transaction and break the lookups that read them there (the
// system roles in every permission check, a user's own notifications). A
// policy that also handles the NULL rows is a design decision of its own
// (issue #302, "Karar gerekiyor").
var rlsInvariantExceptions = map[string]string{
	"roles":                  "system roles have organization_id NULL and are read by every tenant permission check",
	"notifications":          "user notifications (organization_id NULL) are written inside office and taxpayer scopes",
	"activity_events":        "platform activity log; office rows sit next to user / platform rows with organization_id NULL",
	"bulk_jobs":              "IO engine jobs (boilerplate) started by platform users without an office",
	"import_jobs":            "IO engine jobs (boilerplate) started by platform users without an office",
	"export_jobs":            "IO engine jobs (boilerplate) started by platform users without an office",
	"excel_import_templates": "system templates have organization_id and taxpayer_id NULL and are offered to every office",
}

// TestRLSInvariant (issue #302): after all migrations, every public table
// with an organization_id or taxpayer_id column has row level security
// enabled and forced and at least one policy, and every forced table has
// the owner's rls_system policy (rls.System) — unless it is a listed
// exception.
func TestRLSInvariant(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	rows, err := d.Pool.Query(ctx, `
		SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity,
		       (SELECT count(*) FROM pg_policies p WHERE p.schemaname = 'public' AND p.tablename = c.relname),
		       EXISTS (SELECT 1 FROM pg_policies p WHERE p.schemaname = 'public' AND p.tablename = c.relname
		               AND p.policyname = 'rls_system')
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p')
		  AND EXISTS (SELECT 1 FROM pg_attribute a WHERE a.attrelid = c.oid AND NOT a.attisdropped
		              AND a.attname IN ('organization_id', 'taxpayer_id'))
		ORDER BY c.relname`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	var checked int
	for rows.Next() {
		var name string
		var enabled, forced, system bool
		var policies int
		if err := rows.Scan(&name, &enabled, &forced, &policies, &system); err != nil {
			t.Fatal(err)
		}
		seen[name] = true
		if reason, ok := rlsInvariantExceptions[name]; ok {
			if enabled && forced && policies > 0 {
				t.Errorf("%s is protected now: remove it from rlsInvariantExceptions (%s)", name, reason)
			}
			continue
		}
		checked++
		switch {
		case !enabled:
			t.Errorf("%s: row level security is not enabled (its migration must call an rls_apply_* template)", name)
		case !forced:
			t.Errorf("%s: row level security is not forced (rls_owner_force, migration 000086)", name)
		case policies == 0:
			t.Errorf("%s: no policy", name)
		case !system:
			t.Errorf("%s: forced without the owner's rls_system policy: rls.System cannot reach it", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var stale []string
	for name := range rlsInvariantExceptions {
		if !seen[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	for _, name := range stale {
		t.Errorf("rlsInvariantExceptions lists %s, which has no organization_id / taxpayer_id table any more: remove it", name)
	}
	if checked < 60 {
		t.Fatalf("only %d tables checked: the query no longer finds the scoped tables", checked)
	}
}
