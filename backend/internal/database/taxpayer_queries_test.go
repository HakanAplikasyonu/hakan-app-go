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

// TestTaxpayerQueriesReferenceTaxpayer is the isolation lint for source data
// (taxpayer_tables.txt): a query on a mükellef-owned table must filter on or
// set taxpayer_id, so one taxpayer's invoices cannot leak into another's.
// Deliberate exceptions carry "-- tenant-lint: cross-taxpayer <reason>".
func TestTaxpayerQueriesReferenceTaxpayer(t *testing.T) {
	tables := readTaxpayerTables(t)
	files, err := filepath.Glob(filepath.Join("queries", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no query files: %v", err)
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, problem := range lintTaxpayerQueries(tables, string(raw)) {
			t.Errorf("%s: %s", file, problem)
		}
	}
}

func TestTaxpayerLintCatchesMissingScope(t *testing.T) {
	tables := []string{"invoices", "invoice_items"}
	cases := []struct {
		name, sql string
		bad       bool
	}{
		{"no filter", "-- name: Q :many\nSELECT * FROM invoices WHERE ettn = $1;", true},
		{"filter", "-- name: Q :many\nSELECT * FROM invoices WHERE taxpayer_id = $1 AND ettn = $2;", false},
		{"named param", "-- name: Q :many\nSELECT * FROM invoices WHERE taxpayer_id = @taxpayer_id;", false},
		{"insert", "-- name: Q :exec\nINSERT INTO invoice_items (taxpayer_id, invoice_id) VALUES ($1, $2);", false},
		{"insert without", "-- name: Q :exec\nINSERT INTO invoice_items (invoice_id) VALUES ($1);", true},
		{"selected only", "-- name: Q :many\nSELECT taxpayer_id FROM invoices WHERE id = $1;", true},
		{"other table", "-- name: Q :many\nSELECT * FROM users WHERE id = $1;", false},
		{"prefix name is not a match", "-- name: Q :many\nSELECT * FROM invoices_archive WHERE id = $1;", false},
		{"exempt", "-- tenant-lint: cross-taxpayer platform raporu\n-- name: Q :many\nSELECT count(*) FROM invoices;", false},
		{"exempt without reason", "-- tenant-lint: cross-taxpayer\n-- name: Q :many\nSELECT count(*) FROM invoices;", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := lintTaxpayerQueries(tables, tc.sql)
			if tc.bad != (len(got) > 0) {
				t.Fatalf("bad=%v, problems=%v", tc.bad, got)
			}
		})
	}
}

var taxpayerFilterRe = regexp.MustCompile(`taxpayer_id\s*(=|in\b)|=\s*(\w+\.)?taxpayer_id\b|insert\s+into\s+\w+\s*\([^)]*taxpayer_id`)

const crossTaxpayerMarker = "-- tenant-lint: cross-taxpayer"

var taxpayerNameRe = regexp.MustCompile(`^--\s*name:\s*(\w+)`)

func lintTaxpayerQueries(tables []string, src string) []string {
	type q struct {
		name, body, reason string
		exempt             bool
	}
	var qs []q
	var cur *q
	var pending *string
	sc := bufio.NewScanner(strings.NewReader(src))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, crossTaxpayerMarker) {
			r := strings.TrimSpace(strings.TrimPrefix(trimmed, crossTaxpayerMarker))
			pending = &r
			continue
		}
		if m := taxpayerNameRe.FindStringSubmatch(trimmed); m != nil {
			if cur != nil {
				qs = append(qs, *cur)
			}
			cur = &q{name: m[1]}
			if pending != nil {
				cur.exempt, cur.reason, pending = true, *pending, nil
			}
			continue
		}
		if cur != nil {
			if i := strings.Index(line, "--"); i >= 0 {
				line = line[:i]
			}
			cur.body += line + "\n"
		}
	}
	if cur != nil {
		qs = append(qs, *cur)
	}
	var problems []string
	for _, x := range qs {
		if x.exempt {
			if x.reason == "" {
				problems = append(problems, x.name+": tenant-lint cross-taxpayer exemption needs a reason")
			}
			continue
		}
		body := strings.ToLower(x.body)
		for _, table := range tables {
			if !regexp.MustCompile(`\b` + table + `\b`).MatchString(body) {
				continue
			}
			if !taxpayerFilterRe.MatchString(body) {
				problems = append(problems, fmt.Sprintf("%s touches %s but never filters or sets taxpayer_id", x.name, table))
			}
		}
	}
	return problems
}

func readTaxpayerTables(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("taxpayer_tables.txt")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, strings.Fields(line)[0])
	}
	return out
}
