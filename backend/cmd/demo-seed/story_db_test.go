package main

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestDemoStoryIsConsistent (#456): Atlas Gıda'nın bordrosu, beyannameleri,
// banka ekstresi, mizanı ve mutabakatı aynı rakamları anlatır (ledger.go).
func TestDemoStoryIsConsistent(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	if err := seed(ctx, d.Pool, "Demo-Test-1!", ""); err != nil {
		t.Fatal(err)
	}
	atlas := demoScope(t, d, "atlas")
	str := func(sql string, args ...any) string {
		t.Helper()
		var s string
		if err := d.Pool.QueryRow(ctx, sql, args...).Scan(&s); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return s
	}

	// Bordro ve beyannameler ledger.go sabitleriyle aynı.
	for sql, want := range map[string]string{
		`SELECT net_total::numeric(18,2)::text FROM payroll_runs WHERE taxpayer_id = $1 AND period = '2026-09'`:      sepPayrollNet,
		`SELECT net_total::numeric(18,2)::text FROM payroll_runs WHERE taxpayer_id = $1 AND period = '2026-10'`:      sepPayrollNet,
		`SELECT payable_amount::numeric(18,2)::text FROM sgk_premiums WHERE taxpayer_id = $1 AND period = '2026-09'`: sepSGKPayable,
		`SELECT paid_at::text FROM sgk_premiums WHERE taxpayer_id = $1 AND period = '2026-09'`:                       sepSGKPaidOn,
	} {
		if got := str(sql, atlas.TaxpayerID); got != want {
			t.Errorf("%s = %s, want %s", sql, got, want)
		}
	}
	for kind, want := range map[string]string{"kdv1": sepKDVPayable, "muhsgk": sepMUHSGKPayable} {
		got := str(`SELECT payable_amount::text FROM tax_returns WHERE engagement_id = $1 AND kind = $2 AND period = '2026-09' AND status = 'filed'`, atlas.ID, kind)
		if got != want {
			t.Errorf("%s 2026-09 payable = %s, want %s", kind, got, want)
		}
	}

	// Ekim ekstresi Eylül yükümlülüklerini öder; SGK bordrodaki ödeme günü.
	for desc, want := range map[string]string{
		"MAAS ODEMESI EYLUL PERSONEL":      sepPayrollNet,
		"SGK PRIM ODEMESI 2026/09":         sepSGKPayable,
		"GIB VERGI ODEMESI KDV 2026/09":    sepKDVPayable,
		"GIB VERGI ODEMESI MUHSGK 2026/09": sepMUHSGKPayable,
	} {
		got := str(`SELECT amount::numeric(18,2)::text FROM bank_transactions WHERE taxpayer_id = $1 AND description = $2`, atlas.TaxpayerID, desc)
		if got != want {
			t.Errorf("bank %q = %s, want %s", desc, got, want)
		}
	}
	if got := str(`SELECT transaction_date::text FROM bank_transactions WHERE taxpayer_id = $1 AND description = 'SGK PRIM ODEMESI 2026/09'`, atlas.TaxpayerID); got != sepSGKPaidOn {
		t.Errorf("SGK bank date = %s, want %s", got, sepSGKPaidOn)
	}

	// Mizanlar dengeli (yaprak hesaplar).
	for _, period := range []string{"2026-09", "2026-10"} {
		if diff := str(`
			SELECT (SUM(l.debit) - SUM(l.credit))::numeric(18,2)::text FROM trial_balance_lines l
			JOIN account_plan_imports i ON i.id = l.import_id
			WHERE i.engagement_id = $1 AND i.period = $2 AND l.is_leaf`, atlas.ID, period); diff != "0.00" {
			t.Errorf("mizan %s borç − alacak = %s", period, diff)
		}
	}

	// Ekim mutabakatı: farklar yalnızca aktarılmamış Z taslakları ve Luca'daki
	// elle kur farkı fişinden; banka, bordro, vergi ve SGK hesapları eşleşir.
	rows, err := d.Pool.Query(ctx, `
		SELECT main_code::text, difference::text, COALESCE(suggestion_code, '') FROM reconciliation_items it
		JOIN reconciliation_runs r ON r.id = it.run_id
		WHERE r.engagement_id = $1 AND r.period = '2026-10' AND it.result <> 'matched'`, atlas.ID)
	if err != nil {
		t.Fatal(err)
	}
	var open []string
	for rows.Next() {
		var code, diff, sug string
		if err := rows.Scan(&code, &diff, &sug); err != nil {
			t.Fatal(err)
		}
		open = append(open, code)
		if code == "120" && diff != "1200.00" {
			t.Errorf("120 difference = %s, want the manual Luca entry 1200.00", diff)
		}
		if code != "120" && sug != "draft_not_exported" {
			t.Errorf("%s suggestion = %q, want draft_not_exported (Z drafts)", code, sug)
		}
	}
	rows.Close()
	sort.Strings(open)
	if got := strings.Join(open, ","); got != "100,108,120,391,600" {
		t.Errorf("open reconciliation accounts = %s, want 100,108,120,391,600", got)
	}
	for _, main := range []string{"102", "335", "360", "361", "770"} {
		if r := str(`SELECT it.result FROM reconciliation_items it JOIN reconciliation_runs r ON r.id = it.run_id
			WHERE r.engagement_id = $1 AND r.period = '2026-10' AND it.main_code = $2`, atlas.ID, main); r != "matched" {
			t.Errorf("%s = %s, want matched", main, r)
		}
	}
}
