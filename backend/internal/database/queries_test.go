package database

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTenantQueriesReferenceOrganization is the isolation lint: a query on an
// organization-owned table (tenant_tables.txt) must mention organization_id,
// so a forgotten filter cannot leak rows across organizations. Queries that are
// global on purpose (token lookups, a user's own memberships, platform admin
// views) opt out with "-- tenant-lint: cross-org <reason>".
//
// Tables marked "engagement" in tenant_tables.txt also need engagement_id, so
// a forgotten filter cannot leak one mükellef's rows to a member assigned to
// another. Office-wide queries opt out with "-- tenant-lint: org-wide <reason>".
func TestTenantQueriesReferenceOrganization(t *testing.T) {
	tables := readTenantTables(t)
	files, err := filepath.Glob(filepath.Join("queries", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no query files: %v", err)
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, problem := range lintQueries(tables, string(raw)) {
			t.Errorf("%s: %s", file, problem)
		}
	}
}

// TestLintCatchesMissingScopes proves the lint itself: each bad query must be
// reported, each good one must pass.
func TestLintCatchesMissingScopes(t *testing.T) {
	tables := []tenantTable{
		{name: "engagements"},
		{name: "engagement_notes", engagement: true},
	}
	cases := []struct {
		name    string
		sql     string
		wantBad bool
	}{
		{"org table without org filter", "-- name: Q :many\nSELECT * FROM engagements WHERE uuid = $1;", true},
		{"org table with org filter", "-- name: Q :many\nSELECT * FROM engagements WHERE organization_id = $1;", false},
		{"engagement table without engagement filter", "-- name: Q :many\nSELECT * FROM engagement_notes WHERE organization_id = $1;", true},
		{"engagement table without org filter", "-- name: Q :many\nSELECT * FROM engagement_notes WHERE engagement_id = $1;", true},
		{"engagement table with both", "-- name: Q :many\nSELECT * FROM engagement_notes WHERE organization_id = $1 AND engagement_id = $2;", false},
		{"engagement table insert with both", "-- name: Q :exec\nINSERT INTO engagement_notes (organization_id, engagement_id, body) VALUES ($1, $2, $3);", false},
		{"engagement table insert missing engagement", "-- name: Q :exec\nINSERT INTO engagement_notes (organization_id, body) VALUES ($1, $2);", true},
		{"engagement table joined on engagement_id", "-- name: Q :many\nSELECT n.* FROM engagements e JOIN engagement_notes n ON n.engagement_id = e.id WHERE e.organization_id = $1 AND n.organization_id = e.organization_id;", false},
		{"engagement column only selected", "-- name: Q :many\nSELECT engagement_id FROM engagement_notes WHERE organization_id = $1;", true},
		{"org-wide exemption", "-- tenant-lint: org-wide ofis raporu, yalnızca read_all\n-- name: Q :many\nSELECT * FROM engagement_notes WHERE organization_id = $1;", false},
		{"org-wide exemption still needs org", "-- tenant-lint: org-wide ofis raporu\n-- name: Q :many\nSELECT * FROM engagement_notes;", true},
		{"org-wide exemption needs a reason", "-- tenant-lint: org-wide\n-- name: Q :many\nSELECT * FROM engagement_notes WHERE organization_id = $1;", true},
		{"cross-org exemption", "-- tenant-lint: cross-org token lookup\n-- name: Q :one\nSELECT * FROM engagement_notes WHERE uuid = $1;", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := lintQueries(tables, tc.sql)
			if tc.wantBad && len(got) == 0 {
				t.Fatalf("lint passed a bad query:\n%s", tc.sql)
			}
			if !tc.wantBad && len(got) > 0 {
				t.Fatalf("lint rejected a good query: %v", got)
			}
		})
	}
}

type tenantTable struct {
	name       string
	engagement bool
}

var nameRe = regexp.MustCompile(`^--\s*name:\s*(\w+)`)

// lintQueries returns one message per scope a query forgets.
func lintQueries(tables []tenantTable, src string) []string {
	var problems []string
	for _, q := range splitQueries(src) {
		if q.crossOrg {
			if q.crossOrgReason == "" {
				problems = append(problems, q.name+": tenant-lint cross-org exemption needs a reason")
			}
			continue
		}
		if q.orgWide && q.orgWideReason == "" {
			problems = append(problems, q.name+": tenant-lint org-wide exemption needs a reason")
		}
		body := strings.ToLower(stripComments(q.body))
		for _, table := range tables {
			if !regexp.MustCompile(`\b` + table.name + `\b`).MatchString(body) {
				continue
			}
			if !orgFilterRe.MatchString(body) {
				problems = append(problems, fmt.Sprintf("%s touches %s but never filters or sets organization_id", q.name, table.name))
			}
			if table.engagement && !q.orgWide && !engagementFilterRe.MatchString(body) {
				problems = append(problems, fmt.Sprintf("%s touches %s but never filters or sets engagement_id", q.name, table.name))
			}
		}
	}
	return problems
}

// orgFilterRe wants organization_id in a comparison or an INSERT column list,
// so selecting the column alone does not pass.
var orgFilterRe = scopeFilterRe("organization_id")

// engagementFilterRe is the same rule for engagement_id.
var engagementFilterRe = scopeFilterRe("engagement_id")

func scopeFilterRe(col string) *regexp.Regexp {
	return regexp.MustCompile(col + `\s*(=|in\b)|=\s*(\w+\.)?` + col + `\b|insert\s+into\s+\w+\s*\([^)]*` + col)
}

type namedQuery struct {
	name           string
	body           string
	crossOrg       bool
	crossOrgReason string
	orgWide        bool
	orgWideReason  string
}

const (
	crossOrgMarker = "-- tenant-lint: cross-org"
	orgWideMarker  = "-- tenant-lint: org-wide"
)

func splitQueries(src string) []namedQuery {
	var out []namedQuery
	var cur *namedQuery
	var pendingCross, pendingWide *string
	sc := bufio.NewScanner(strings.NewReader(src))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, crossOrgMarker) {
			r := strings.TrimSpace(strings.TrimPrefix(trimmed, crossOrgMarker))
			pendingCross = &r
			continue
		}
		if strings.HasPrefix(trimmed, orgWideMarker) {
			r := strings.TrimSpace(strings.TrimPrefix(trimmed, orgWideMarker))
			pendingWide = &r
			continue
		}
		if m := nameRe.FindStringSubmatch(trimmed); m != nil {
			if cur != nil {
				out = append(out, *cur)
			}
			cur = &namedQuery{name: m[1]}
			if pendingCross != nil {
				cur.crossOrg, cur.crossOrgReason = true, *pendingCross
				pendingCross = nil
			}
			if pendingWide != nil {
				cur.orgWide, cur.orgWideReason = true, *pendingWide
				pendingWide = nil
			}
			continue
		}
		if cur != nil {
			cur.body += line + "\n"
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

func stripComments(sql string) string {
	var b strings.Builder
	for _, line := range strings.Split(sql, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func readTenantTables(t *testing.T) []tenantTable {
	t.Helper()
	raw, err := os.ReadFile("tenant_tables.txt")
	if err != nil {
		t.Fatal(err)
	}
	var out []tenantTable
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		tt := tenantTable{name: fields[0]}
		switch {
		case len(fields) == 1:
		case len(fields) == 2 && fields[1] == "engagement":
			tt.engagement = true
		default:
			t.Fatalf("tenant_tables.txt: bad line %q (want \"table\" or \"table engagement\")", line)
		}
		out = append(out, tt)
	}
	if len(out) == 0 {
		t.Fatal("tenant_tables.txt is empty")
	}
	return out
}
