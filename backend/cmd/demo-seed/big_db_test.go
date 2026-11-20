package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// testScale is the big demo shrunk for the database test.
var testScale = bigScale{Offices: 2, Taxpayers: 4, Months: 3}

// testNow pins the calendar: the big demo is relative to the current month.
var testNow = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

func seedBigForTest(t *testing.T, d *testdb.DB) {
	t.Helper()
	ctx := context.Background()
	if err := seed(ctx, d.Pool, "Demo-Test-1!", ""); err != nil {
		t.Fatal(err)
	}
	if err := seedBig(ctx, d.Pool, "Demo-Test-1!", testScale, testNow); err != nil {
		t.Fatal(err)
	}
}

// TestBigDemoSeedsAndResets: küçük ölçekli büyük demo ofisleri, mükellefleri
// ve kullanıcıları kurar; -reset hepsini (yalnızca demo verisini) siler.
func TestBigDemoSeedsAndResets(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	// A real office's taxpayer outside the demo prefix range must survive.
	realOrg := d.Organization(t)
	realTP := d.Taxpayer(t, "legal", "1234567890")
	d.Engagement(t, realOrg, realTP)

	seedBigForTest(t, d)
	slugs := demo.BigOfficeSlugs()[:testScale.Offices]
	if n := count(t, d, `SELECT count(*) FROM organizations WHERE slug = ANY($1::text[])`, slugs); n != testScale.Offices {
		t.Fatalf("offices = %d, want %d", n, testScale.Offices)
	}
	for _, slug := range slugs {
		n := count(t, d, `SELECT count(*) FROM engagements e JOIN organizations o ON o.id = e.organization_id
			JOIN taxpayers t ON t.id = e.taxpayer_id WHERE o.slug = $1 AND t.tax_id NOT IN ('9000000017', '10000000146')`, slug)
		if n != testScale.Taxpayers {
			t.Errorf("%s big taxpayers = %d, want %d", slug, n, testScale.Taxpayers)
		}
		if n := count(t, d, `SELECT count(*) FROM engagement_assignments a JOIN organizations o ON o.id = a.organization_id
			WHERE o.slug = $1`, slug); n < testScale.Taxpayers {
			t.Errorf("%s assignments = %d", slug, n)
		}
	}
	if n := count(t, d, `SELECT count(*) FROM taxpayer_members m JOIN users u ON u.id = m.user_id
		WHERE u.email = $1 AND m.role = 'owner'`, demo.BigOffices[1].TaxpayerUser.Email); n != 1 {
		t.Errorf("portal user memberships = %d, want 1", n)
	}

	checkBigLedger(t, d)
	checkBigReturns(t, d)
	checkBigRealLife(t, d)
	// #518 B2: kilitli döneme geç gelen fatura talep olarak bekler; talep
	// tam fatura içeriğini taşır (uygulanınca gerçek fatura yazılır).
	if n := count(t, d, `SELECT count(*) FROM change_requests c JOIN period_locks l
		ON l.engagement_id = c.engagement_id AND l.period = c.period
		WHERE c.status = 'open' AND c.entity_type = 'invoice' AND c.entity_id IS NULL
		  AND c.new_payload->'invoice'->>'invoice_number' LIKE 'GEC%'
		  AND c.diff->>'kind' = 'create'`); n == 0 {
		t.Error("no open change request on a locked period")
	}
	if n := count(t, d, `SELECT count(*) FROM invoices WHERE invoice_number LIKE 'GEC%'`); n != 0 {
		t.Errorf("held invoices were written: %d", n)
	}

	if err := dropDemo(ctx, d.Pool); err != nil {
		t.Fatal(err)
	}
	if n := count(t, d, `SELECT count(*) FROM organizations WHERE slug = ANY($1::text[])`, slugs); n != 0 {
		t.Errorf("offices after reset = %d", n)
	}
	if n := count(t, d, `SELECT count(*) FROM taxpayers WHERE tax_id LIKE '93%' OR tax_id LIKE '1000%'`); n != 0 {
		t.Errorf("demo taxpayers after reset = %d", n)
	}
	if n := count(t, d, `SELECT count(*) FROM taxpayers WHERE id = $1`, realTP); n != 1 {
		t.Error("reset deleted a real taxpayer")
	}
}

// checkBigLedger: faturalar, Z raporları, banka, yevmiye ve mizan aynı
// rakamları anlatır.
func checkBigLedger(t *testing.T, d *testdb.DB) {
	t.Helper()
	big := `SELECT e.id FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id
		JOIN organizations o ON o.id = e.organization_id
		WHERE o.slug = ANY($1::text[]) AND t.tax_id NOT IN ('9000000017', '10000000146')`
	slugs := demo.BigOfficeSlugs()[:testScale.Offices]
	for _, c := range []struct{ name, sql string }{
		{"engagement without invoices in a period", `SELECT count(*) FROM (` + big + `) e
			JOIN engagements g ON g.id = e.id
			CROSS JOIN generate_series(0, 3) m
			WHERE g.started_at < DATE '2026-11-01' - make_interval(months => m)
			  AND NOT EXISTS (SELECT 1 FROM invoices i WHERE i.taxpayer_id = g.taxpayer_id
			       AND i.period = to_char(DATE '2026-11-01' - make_interval(months => m), 'YYYY-MM'))`},
		{"invoice tax summary ≠ header", `SELECT count(*) FROM invoices i
			WHERE i.source_system = 'demo_big' AND i.tax_amount <> (SELECT sum(vat) FROM invoice_tax_summaries s
			WHERE s.invoice_id = i.id AND s.taxpayer_id = i.taxpayer_id)`},
		{"invoice items ≠ header", `SELECT count(*) FROM invoices i
			WHERE i.source_system = 'demo_big' AND i.tax_exclusive_amount <> (SELECT sum(tax_exclusive_amount) FROM invoice_items s
			WHERE s.invoice_id = i.id AND s.taxpayer_id = i.taxpayer_id)`},
		{"Z report VAT lines ≠ header", `SELECT count(*) FROM retail_sales r
			WHERE r.source_system = 'demo_big' AND r.tax_amount <> (SELECT sum(tax_amount) FROM retail_sale_vat_lines l
			WHERE l.retail_sale_id = r.id AND l.taxpayer_id = r.taxpayer_id)`},
		{"unbalanced voucher", `SELECT count(*) FROM (SELECT journal_draft_id FROM journal_draft_lines l
			WHERE l.engagement_id IN (` + big + `) GROUP BY 1 HAVING sum(debit) <> sum(credit)) x`},
		{"unbalanced mizan", `SELECT count(*) FROM (SELECT import_id FROM trial_balance_lines
			WHERE engagement_id IN (` + big + `) AND is_leaf GROUP BY 1 HAVING sum(debit) <> sum(credit)) x`},
		{"negative bank balance", `SELECT count(*) FROM bank_transactions WHERE source_system = 'bank_demo_big' AND balance_after < 0`},
		// Mizan 102 (Ekim) = ekstrenin Ekim sonu bakiyesi − kayda alınmamış hareketler.
		{"mizan bank account ≠ bank", `SELECT count(*) FROM (` + big + `) e
			JOIN bank_ledger_accounts a ON a.engagement_id = e.id
			JOIN account_plan_imports i ON i.engagement_id = e.id AND i.period = '2026-10'
			JOIN trial_balance_lines l ON l.import_id = i.id AND l.code = a.account_code
			WHERE l.balance <> (SELECT b.balance_after FROM bank_transactions b WHERE b.cash_box_id = a.cash_box_id
			      AND b.period <= '2026-10' ORDER BY b.booked_at DESC, b.id DESC LIMIT 1)
			  - COALESCE((SELECT sum(CASE WHEN b.debit_credit = 'A' THEN b.amount ELSE -b.amount END) FROM bank_transactions b
			      WHERE b.cash_box_id = a.cash_box_id AND b.period <= '2026-10' AND b.match_status IN ('unmatched', 'suggested')), 0)`},
	} {
		var args []any
		if strings.Contains(c.sql, "$1") {
			args = append(args, slugs)
		}
		if n := count(t, d, c.sql, args...); n != 0 {
			t.Errorf("%s: %d", c.name, n)
		}
	}
	if n := count(t, d, `SELECT count(*) FROM bank_transaction_matches m JOIN bank_transactions b ON b.id = m.bank_transaction_id
		WHERE b.source_system = 'bank_demo_big'`); n == 0 {
		t.Error("no bank ↔ invoice matches")
	}
}

// checkBigReturns: beyannameler servisle hazırlandı; banka ödemeleri
// verilen beyanla, SGK ödemesi tahakkukla aynı; güncel ayda dikkat
// gerektiren kalemler var.
func checkBigReturns(t *testing.T, d *testdb.DB) {
	t.Helper()
	slugs := demo.BigOfficeSlugs()[:testScale.Offices]
	big := `SELECT e.id FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id
		JOIN organizations o ON o.id = e.organization_id
		WHERE o.slug = ANY($1::text[]) AND t.tax_id NOT IN ('9000000017', '10000000146')`
	for _, c := range []struct {
		name string
		sql  string
		min  int
	}{
		{"filed KDV1 (2026-08)", `SELECT count(*) FROM tax_returns WHERE engagement_id IN (` + big + `) AND kind = 'kdv1' AND period = '2026-08' AND status = 'filed'`, testScale.Offices * testScale.Taxpayers},
		{"late KDV1 (2026-09, not filed)", `SELECT count(*) FROM tax_returns WHERE engagement_id IN (` + big + `) AND kind = 'kdv1' AND period = '2026-09' AND status <> 'filed'`, 1},
		{"MUHSGK", `SELECT count(*) FROM tax_returns WHERE engagement_id IN (` + big + `) AND kind = 'muhsgk'`, 1},
		{"payroll runs", `SELECT count(*) FROM payroll_runs p JOIN engagements e ON e.taxpayer_id = p.taxpayer_id WHERE e.id IN (` + big + `)`, 1},
		{"unmatched bank lines", `SELECT count(*) FROM bank_transactions b JOIN engagements e ON e.taxpayer_id = b.taxpayer_id
			WHERE e.id IN (` + big + `) AND b.match_status = 'unmatched'`, 1},
		{"open blockers", `SELECT count(*) FROM data_warnings WHERE engagement_id IN (` + big + `) AND status = 'open' AND severity = 'blocker'`, 1},
		{"open document tasks", `SELECT count(*) FROM document_tasks WHERE engagement_id IN (` + big + `) AND status = 'open'`, 1},
		{"pending review items", `SELECT count(*) FROM suggestions WHERE engagement_id IN (` + big + `) AND status = 'pending'`, 1},
		{"reconciliation differences", `SELECT count(*) FROM reconciliation_items it JOIN reconciliation_runs r ON r.id = it.run_id
			WHERE r.engagement_id IN (` + big + `) AND it.result <> 'matched'`, 1},
		{"letters", `SELECT count(*) FROM reconciliation_letters WHERE engagement_id IN (` + big + `)`, 1},
		{"period locks", `SELECT count(*) FROM period_locks WHERE engagement_id IN (` + big + `)`, 1},
		{"notifications", `SELECT count(*) FROM notifications n JOIN organizations o ON o.id = n.organization_id WHERE o.slug = ANY($1::text[])`, 1},
		{"report deliveries", `SELECT count(*) FROM report_deliveries WHERE engagement_id IN (` + big + `)`, 1},
	} {
		if n := count(t, d, c.sql, slugs); n < c.min {
			t.Errorf("%s = %d, want ≥ %d", c.name, n, c.min)
		}
	}
	for name, sql := range map[string]string{
		"KDV bank payment ≠ filed return": `SELECT count(*) FROM tax_returns r JOIN engagements e ON e.id = r.engagement_id
			LEFT JOIN bank_transactions b ON b.taxpayer_id = e.taxpayer_id AND b.description = 'GIB VERGI ODEMESI KDV ' || replace(r.period, '-', '/')
			WHERE r.engagement_id IN (` + big + `) AND r.kind = 'kdv1' AND r.status = 'filed' AND r.payable_amount > 0
			  AND b.amount IS DISTINCT FROM r.payable_amount`,
		"SGK bank payment ≠ premium": `SELECT count(*) FROM sgk_premiums s JOIN engagements e ON e.taxpayer_id = s.taxpayer_id
			JOIN bank_transactions b ON b.taxpayer_id = s.taxpayer_id AND b.description = 'SGK PRIM ODEMESI ' || replace(s.period, '-', '/')
			WHERE e.id IN (` + big + `) AND (b.amount <> s.payable_amount OR b.transaction_date <> s.paid_at)`,
	} {
		if n := count(t, d, sql, slugs); n != 0 {
			t.Errorf("%s: %d", name, n)
		}
	}
}

// checkBigRealLife (#568): ofis kadrosu, rutin dışı banka işlemleri ve
// gerçek bir nedene bağlı hatalar.
func checkBigRealLife(t *testing.T, d *testdb.DB) {
	t.Helper()
	slugs := demo.BigOfficeSlugs()[:testScale.Offices]
	for _, slug := range slugs {
		if n := count(t, d, `SELECT count(*) FROM organization_members m JOIN organizations o ON o.id = m.organization_id
			WHERE o.slug = $1`, slug); n != demo.BigStaffPerOffice {
			t.Errorf("%s members = %d, want %d", slug, n, demo.BigStaffPerOffice)
		}
	}
	big := `SELECT e.id FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id
		JOIN organizations o ON o.id = e.organization_id
		WHERE o.slug = ANY($1::text[]) AND t.tax_id NOT IN ('9000000017', '10000000146')`
	for _, c := range []struct {
		name string
		sql  string
		min  int
	}{
		{"partial payments", `SELECT count(*) FROM bank_transactions WHERE source_system = 'bank_demo_big' AND description LIKE '%KISMI ODEME'`, 1},
		{"foreign currency collections", `SELECT count(*) FROM bank_transactions WHERE source_system = 'bank_demo_big' AND kind = 'YABANCI_PARA_HAVALE'`, 1},
		{"kur farkı lines", `SELECT count(*) FROM journal_draft_lines WHERE engagement_id IN (` + big + `) AND account_code IN ('646.01', '656.01') AND description LIKE 'Kur farkı DOVIZ%'`, 1},
		{"bank accounts without the declaration month's statement", `SELECT count(*) FROM cash_boxes b
			JOIN engagements e ON e.taxpayer_id = b.taxpayer_id WHERE e.id IN (` + big + `)
			AND NOT EXISTS (SELECT 1 FROM bank_transactions t WHERE t.cash_box_id = b.id
				AND t.period = '2026-10')
			AND EXISTS (SELECT 1 FROM bank_transactions t WHERE t.cash_box_id = b.id
				AND t.period = '2026-09')`, 1},
		{"own account transfers", `SELECT count(*) FROM bank_transactions WHERE source_system = 'bank_demo_big' AND kind_detail = 'own_account_transfer'`, 2},
		{"duplicate invoices", `SELECT count(*) FROM data_warnings WHERE engagement_id IN (` + big + `) AND code = 'POSSIBLE_DUPLICATE'`, 1},
		{"drafts on a parent account", `SELECT count(*) FROM journal_draft_lines WHERE engagement_id IN (` + big + `) AND account_code ~ '^7[0-9]{2}$'`, 1},
		{"trainee drafts", `SELECT count(*) FROM journal_drafts j JOIN organization_members m ON m.user_id = j.created_by
			AND m.organization_id = j.organization_id JOIN users u ON u.id = m.user_id
			WHERE j.engagement_id IN (` + big + `) AND j.status = 'draft' AND u.email LIKE 'stajyer%'`, 1},
	} {
		var args []any
		if strings.Contains(c.sql, "$1") {
			args = append(args, slugs)
		}
		if n := count(t, d, c.sql, args...); n < c.min {
			t.Errorf("%s = %d, want ≥ %d", c.name, n, c.min)
		}
	}
	// Mükerrer uyarısı boşuna değil: aday aynı karşı taraf, tarih ve tutarda.
	if n := count(t, d, `SELECT count(*) FROM data_warnings w JOIN invoices c ON c.id = w.entity_id
		WHERE w.code = 'POSSIBLE_DUPLICATE' AND w.engagement_id IN (`+big+`)
		  AND NOT EXISTS (SELECT 1 FROM invoices o WHERE o.uuid::text = w.params->'candidates'->>0 AND o.id <> c.id
		      AND o.taxpayer_id = c.taxpayer_id AND o.issue_date = c.issue_date AND o.payable_amount = c.payable_amount
		      AND o.customer_tax_number IS NOT DISTINCT FROM c.customer_tax_number AND o.invoice_number <> c.invoice_number)`, slugs); n != 0 {
		t.Errorf("duplicate warnings without a real duplicate: %d", n)
	}
}
