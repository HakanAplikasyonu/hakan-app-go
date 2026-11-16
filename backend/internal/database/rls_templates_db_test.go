package database

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// scopedTemplates is the exact policy each rls_apply_* template creates for
// the scoped roles. Their cross-tenant behaviour is tested once per template
// (internal/platform/rls: TestRLSHidesOtherOffices, TestRLSEngagementScope,
// TestRLSRejectsWritesOutsideScope); TestScopedPoliciesMatchTemplates proves
// every scoped table carries exactly one of them and nothing wider, so the
// leak test holds for every table (issue #166).
var scopedTemplates = map[string]string{
	"rls_org":        "(organization_id = app_current_organization_id())",
	"rls_engagement": "app_engagement_visible(organization_id, engagement_id)",
	"rls_taxpayer":   "(taxpayer_id = ANY (app_current_taxpayer_ids()))",
}

// TestScopedPoliciesMatchTemplates: for every table in tenant_tables.txt and
// taxpayer_tables.txt the only policy that reaches the scoped roles (or
// PUBLIC) is its template, unchanged: command ALL, roles exactly
// hakan_rls_app and hakan_rls_worker, USING and WITH CHECK as created. A
// second permissive policy would be OR-ed in and widen what a tenant sees.
func TestScopedPoliciesMatchTemplates(t *testing.T) {
	d := testdb.Open(t)
	want := map[string]string{}
	for _, tt := range readTenantTables(t) {
		want[tt.name] = "rls_org"
		if tt.engagement {
			want[tt.name] = "rls_engagement"
		}
	}
	for _, name := range readTaxpayerTables(t) {
		want[name] = "rls_taxpayer"
	}
	type pol struct {
		table, name, cmd, permissive string
		roles                        []string
		qual, check                  *string
	}
	rows, err := d.Pool.Query(context.Background(), `
		SELECT tablename::text, policyname::text, cmd, permissive, roles::text[], qual, with_check
		FROM pg_policies WHERE schemaname = 'public'`)
	if err != nil {
		t.Fatal(err)
	}
	pols, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (pol, error) {
		var p pol
		err := r.Scan(&p.table, &p.name, &p.cmd, &p.permissive, &p.roles, &p.qual, &p.check)
		return p, err
	})
	if err != nil {
		t.Fatal(err)
	}
	byTable := map[string][]pol{}
	for _, p := range pols {
		byTable[p.table] = append(byTable[p.table], p)
	}
	reachesScoped := func(roles []string) bool {
		for _, r := range roles {
			if r == "hakan_rls_app" || r == "hakan_rls_worker" || r == "public" {
				return true
			}
		}
		return false
	}
	tables := make([]string, 0, len(want))
	for name := range want {
		tables = append(tables, name)
	}
	sort.Strings(tables)
	for _, table := range tables {
		tmpl := want[table]
		var scoped []pol
		for _, p := range byTable[table] {
			if reachesScoped(p.roles) {
				scoped = append(scoped, p)
			}
		}
		if len(scoped) != 1 {
			names := []string{}
			for _, p := range scoped {
				names = append(names, p.name)
			}
			t.Errorf("%s: %d policies reach the scoped roles (%s), want only %s", table, len(scoped), strings.Join(names, ", "), tmpl)
			continue
		}
		p := scoped[0]
		roles := append([]string(nil), p.roles...)
		sort.Strings(roles)
		expr := scopedTemplates[tmpl]
		switch {
		case p.name != tmpl:
			t.Errorf("%s: scoped policy %s, want %s", table, p.name, tmpl)
		case p.cmd != "ALL" || p.permissive != "PERMISSIVE":
			t.Errorf("%s: %s is %s %s, want PERMISSIVE ALL", table, p.name, p.permissive, p.cmd)
		case strings.Join(roles, ",") != "hakan_rls_app,hakan_rls_worker":
			t.Errorf("%s: %s roles %v", table, p.name, p.roles)
		case p.qual == nil || *p.qual != expr:
			t.Errorf("%s: %s USING %v, want %s", table, p.name, deref(p.qual), expr)
		case p.check == nil || *p.check != expr:
			t.Errorf("%s: %s WITH CHECK %v, want %s", table, p.name, deref(p.check), expr)
		}
	}
	t.Logf("checked %d scoped tables", len(tables))
}

func deref(s *string) string {
	if s == nil {
		return "<none>"
	}
	return *s
}
