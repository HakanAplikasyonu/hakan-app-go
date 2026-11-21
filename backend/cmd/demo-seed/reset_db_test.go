package main

import (
	"context"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/upload"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

func count(t *testing.T, d *testdb.DB, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := d.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

// uploadUBL loads the demo UBL files with prefix into an engagement through
// the real upload flow (customers, invoices, lines, sources, warnings, …).
// ownTaxID, when set, is the party the files are checked against (a real
// office's taxpayer reusing the demo files); empty uses the taxpayer's own.
func uploadUBL(t *testing.T, d *testdb.DB, scope engagementctx.Scope, actor int64, prefix, ownTaxID string) {
	t.Helper()
	files, err := demo.UBL(prefix)
	if err != nil || len(files) == 0 {
		t.Fatalf("demo UBL %q: %d files, %v", prefix, len(files), err)
	}
	svc := upload.New(d.Pool, storage.NewMemory(), nil, nil)
	if ownTaxID != "" {
		svc = svc.WithOwnTaxIDResolver(func(context.Context, int64) (string, error) { return ownTaxID, nil })
	}
	for _, f := range files {
		job, err := svc.Upload(context.Background(), scope, actor, f.Name, f.Data)
		if err != nil {
			t.Fatalf("upload %s: %v", f.Name, err)
		}
		if job.Report == nil || job.Report.Added == 0 {
			t.Fatalf("upload %s: %+v", f.Name, job.Report)
		}
	}
}

func demoScope(t *testing.T, d *testdb.DB, key string) engagementctx.Scope {
	t.Helper()
	var taxID string
	for _, tp := range demo.Taxpayers {
		if tp.Key == key {
			taxID = tp.TaxID
		}
	}
	var s engagementctx.Scope
	err := d.Pool.QueryRow(context.Background(), `
		SELECT e.id, e.organization_id, e.taxpayer_id FROM engagements e
		JOIN organizations o ON o.id = e.organization_id AND o.slug = $1
		JOIN taxpayers t ON t.id = e.taxpayer_id AND t.tax_id = $2`, demo.OrgSlug, taxID).
		Scan(&s.ID, &s.OrganizationID, &s.TaxpayerID)
	if err != nil {
		t.Fatalf("demo engagement %s: %v", key, err)
	}
	return s
}

// TestSeedLinksTaxpayerUser (#145): the taxpayer-side account is an owner of
// Atlas Gıda, so it lands in the mükellef panel instead of the CMS shell.
func TestSeedLinksTaxpayerUser(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	if err := seed(ctx, d.Pool, "Demo-Test-1!", ""); err != nil {
		t.Fatal(err)
	}
	var atlasTaxID string
	for _, tp := range demo.Taxpayers {
		if tp.Key == demo.TaxpayerUserOf {
			atlasTaxID = tp.TaxID
		}
	}
	if n := count(t, d, `
		SELECT count(*) FROM taxpayer_members m
		JOIN users u ON u.id = m.user_id AND u.email = $1
		JOIN taxpayers t ON t.id = m.taxpayer_id AND t.tax_id = $2
		WHERE m.role = $3`, demo.TaxpayerUser.Email, atlasTaxID, demo.TaxpayerUserRole); n != 1 {
		t.Fatalf("taxpayer user membership = %d, want 1", n)
	}
	// Only Atlas: the other demo taxpayer has no taxpayer-side member.
	if n := count(t, d, `SELECT count(*) FROM taxpayer_members`); n != 1 {
		t.Fatalf("taxpayer_members = %d, want 1", n)
	}
	// -reset and seed again: still exactly one membership.
	if err := dropDemo(ctx, d.Pool); err != nil {
		t.Fatal(err)
	}
	if err := seed(ctx, d.Pool, "Demo-Test-1!", ""); err != nil {
		t.Fatal(err)
	}
	if n := count(t, d, `SELECT count(*) FROM taxpayer_members`); n != 1 {
		t.Fatalf("after reseed taxpayer_members = %d, want 1", n)
	}
	// #318: the review queue has the demo receipt's pending fields, once.
	if n := count(t, d, `
		SELECT count(*) FROM suggestions s JOIN organizations o ON o.id = s.organization_id AND o.slug = $1
		WHERE s.kind = 'receipt_field' AND s.status = 'pending'`, demo.OrgSlug); n != len(demoReceiptFields) {
		t.Fatalf("pending receipt fields = %d, want %d", n, len(demoReceiptFields))
	}
	// #391: one imported Luca transfer and one failed Zirve transfer, once.
	if n := count(t, d, `
		SELECT count(*) FROM accounting_transfers a JOIN organizations o ON o.id = a.organization_id AND o.slug = $1
		WHERE (a.driver = 'luca' AND a.status = 'imported') OR (a.driver = 'zirve' AND a.status = 'failed' AND a.error_message <> '')`, demo.OrgSlug); n != 2 {
		t.Fatalf("demo transfers = %d, want 2", n)
	}
	// #385: the Easytrade connection, its history and October's data, once.
	if n := count(t, d, `SELECT count(*) FROM source_connections WHERE source_system = 'easytrade' AND sync_interval_minutes = 240 AND next_sync_at > NOW()`); n != 1 {
		t.Fatalf("easytrade connections = %d, want 1", n)
	}
	if n := count(t, d, `SELECT count(*) FROM sync_runs r JOIN source_connections c ON c.id = r.connection_id AND c.source_system = 'easytrade' WHERE r.status = 'partial'`); n != 1 {
		t.Fatalf("partial runs = %d, want 1", n)
	}
	if n := count(t, d, `SELECT count(*) FROM source_financial_transactions WHERE source_system = 'easytrade' AND period = '2026-10'`); n != 10 {
		t.Fatalf("financial transactions = %d, want 10", n)
	}
	if n := count(t, d, `SELECT count(*) FROM retail_sales WHERE source_system = 'easytrade' AND z_report_number IS NOT NULL`); n != 5 {
		t.Fatalf("Z reports = %d, want 5", n)
	}
	// #382: Atlas'ın iki dönem bordrosu, beş personeli, Eylül SGK'sı ödenmiş,
	// iki ayın tahakkuk fişi (Luca'ya aktarılmış) ve mükellefe açık dört PDF.
	for q, want := range map[string]int{
		`SELECT count(*) FROM payroll_runs`:                                    2,
		`SELECT count(*) FROM employees`:                                       5,
		`SELECT count(*) FROM payroll_lines`:                                   8,
		`SELECT count(*) FROM sgk_premiums WHERE paid_at IS NOT NULL`:          1,
		`SELECT count(*) FROM payroll_runs WHERE journal_draft_id IS NOT NULL`: 2,
		`SELECT count(*) FROM payroll_documents WHERE visible_to_taxpayer`:     4,
		`SELECT count(*) FROM employees WHERE national_id_masked LIKE '100%'`:  4,
		// #393: Eylül ve Ekim mizanları, tutarlarıyla.
		`SELECT count(*) FROM account_plan_imports WHERE source_kind = 'trial_balance' AND period IN ('2026-09', '2026-10')`: 2,
		`SELECT count(DISTINCT import_id) FROM trial_balance_lines WHERE code = '600.01' AND balance < 0`:                    2,
		// #393: yedi arşiv evrakı; üçü İSG, biri 30 gün içinde bitiyor, birinin süresi dolmuş.
		`SELECT count(*) FROM taxpayer_documents`: 8,
		// #780 B: önemli belgeler, biri 30 gün içinde bitiyor.
		`SELECT count(*) FROM taxpayer_documents WHERE important_kind IS NOT NULL AND visible_to_taxpayer`:                                    4,
		`SELECT count(*) FROM taxpayer_documents WHERE important_kind IS NOT NULL AND valid_until BETWEEN CURRENT_DATE AND CURRENT_DATE + 30`: 1,
		// #395: başka programın masraf raporu ortak Excel akışından, ofis şablonuyla.
		`SELECT count(*) FROM excel_imports WHERE data_type = 'expenses' AND status = 'committed'`:      1,
		`SELECT count(*) FROM excel_import_templates WHERE data_type = 'expenses' AND scope = 'tenant'`: 1,
		`SELECT count(*) FROM expenses WHERE capture_method = 'excel_import' AND period = '2026-11'`:    3,
		// #389: Ekim mutabakatı, mektuplar (biri yanıtlanmış), bir kalem inceleme kuyruğunda.
		`SELECT count(*) FROM reconciliation_runs WHERE period = '2026-10'`:                                                         1,
		`SELECT count(*) FROM reconciliation_letters`:                                                                               2,
		`SELECT count(*) FROM reconciliation_letters WHERE response = 'agreed'`:                                                     1,
		`SELECT count(*) FROM reconciliation_items WHERE status = 'in_review'`:                                                      1,
		`SELECT count(*) FROM reconciliation_items WHERE status = 'explained' AND note IS NOT NULL`:                                 1,
		`SELECT count(*) FROM taxpayer_documents WHERE category = 'isg' AND valid_until BETWEEN CURRENT_DATE AND CURRENT_DATE + 30`: 1,
		`SELECT count(*) FROM taxpayer_documents WHERE category = 'isg' AND valid_until < CURRENT_DATE`:                             1,
		// #390: Eylül KDV1 ve MUHSGK verildi, Ba/Bs hazırlandı; Ekim KDV1 taslak.
		`SELECT count(*) FROM tax_returns WHERE period = '2026-09' AND status = 'filed' AND accrual_number IS NOT NULL`: 2,
		`SELECT count(*) FROM tax_returns WHERE period = '2026-09' AND kind = 'babs' AND status = 'prepared'`:           1,
		`SELECT count(*) FROM tax_returns WHERE period = '2026-10' AND kind = 'kdv1' AND status = 'draft'`:              1,
		// #780 C: Eylül beyanından sonra gelen iki masraf; biri cari döneme alındı.
		`SELECT count(*) FROM late_documents l JOIN expenses x ON l.document_type = 'expense' AND x.id = l.document_id
		 WHERE x.source_id LIKE 'demo-gec-belge-%' AND l.period = '2026-09' AND l.close_reason = 'filed'`: 2,
		`SELECT count(*) FROM late_documents l JOIN expenses x ON l.document_type = 'expense' AND x.id = l.document_id
		 WHERE x.source_id = 'demo-gec-belge-2' AND l.status = 'current_period' AND l.target_period = '2026-11'`: 1,
	} {
		if n := count(t, d, q); n != want {
			t.Errorf("%s = %d, want %d", q, n, want)
		}
	}
	// #384: Excel'den masraflar ve Z raporları; ilk günler onaylı ve
	// yevmiye taslağında, Z'de bir numara ve bir gün boşluğu uyarısı.
	if n := count(t, d, `
		SELECT count(*) FROM receipt_postings p JOIN organizations o ON o.id = p.organization_id AND o.slug = $1
		WHERE p.journal_draft_id IS NOT NULL`, demo.OrgSlug); n != 6 {
		t.Fatalf("receipt journal drafts = %d, want 6", n)
	}
	if n := count(t, d, `
		SELECT count(*) FROM data_warnings w JOIN organizations o ON o.id = w.organization_id AND o.slug = $1
		WHERE w.code IN ('Z_NUMBER_GAP', 'Z_DAY_GAP') AND w.status = 'open'`, demo.OrgSlug); n != 2 {
		t.Fatalf("Z continuity warnings = %d, want 2", n)
	}
	// #780 B: ofis KDV eşiği 10.000 TL; Atlas, Selin ve Deniz Market bu ay
	// eşiği geçer, nöbetçi olayı ve zil bildirimi oluşur.
	if n := count(t, d, `
		SELECT count(*) FROM warning_rule_configs c JOIN organizations o ON o.id = c.organization_id AND o.slug = $1
		WHERE c.engagement_id IS NULL AND c.code = 'VAT_PAYABLE_THRESHOLD' AND c.params->>'limit' = '10000.00'`, demo.OrgSlug); n != 1 {
		t.Fatalf("office VAT threshold rows = %d, want 1", n)
	}
	if n := count(t, d, `
		SELECT count(*) FROM vat_watch_events w JOIN organizations o ON o.id = w.organization_id AND o.slug = $1
		WHERE w.event = 'vat.threshold_exceeded'`, demo.OrgSlug); n < 3 {
		t.Fatalf("VAT threshold events = %d, want at least 3", n)
	}
	if n := count(t, d, `
		SELECT count(*) FROM notifications n JOIN organizations o ON o.id = n.organization_id AND o.slug = $1
		WHERE n.channel = 'inapp' AND n.source_event = 'vat.threshold_exceeded'`, demo.OrgSlug); n < 3 {
		t.Fatalf("VAT threshold notifications = %d, want at least 3", n)
	}
}

// TestResetAfterUpload (#148): after UBL was uploaded to the demo office,
// -reset deletes the demo-only taxpayers with all their data and leaves real
// tenants alone, including a demo tax id that another office also serves.
func TestResetAfterUpload(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	if err := seed(ctx, d.Pool, "Demo-Test-1!", ""); err != nil {
		t.Fatal(err)
	}
	actor := d.User(t)
	atlas := demoScope(t, d, "atlas")
	selin := demoScope(t, d, "selin")
	uploadUBL(t, d, atlas, actor, "atlas-", "")
	uploadUBL(t, d, selin, actor, "selin-", "")

	// A real office with its own taxpayer and data.
	realOrg := d.Organization(t)
	realTP := d.Taxpayer(t, "legal", "1410000001")
	realEng := d.Engagement(t, realOrg, realTP)
	uploadUBL(t, d, engagementctx.Scope{ID: realEng, OrganizationID: realOrg, TaxpayerID: realTP}, actor, "atlas-", demo.Taxpayers[0].TaxID)
	// The real office served the demo taxpayer "selin" before (an ended
	// engagement, office change #8): its identity and data must stay.
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO engagements (organization_id, taxpayer_id, status, ended_at)
		VALUES ($1, $2, 'ended', NOW())`, realOrg, selin.TaxpayerID); err != nil {
		t.Fatal(err)
	}

	realInvoices := count(t, d, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`, realTP)
	realCustomers := count(t, d, `SELECT count(*) FROM customers WHERE taxpayer_id = $1`, realTP)
	selinInvoices := count(t, d, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`, selin.TaxpayerID)
	if realInvoices == 0 || realCustomers == 0 || selinInvoices == 0 {
		t.Fatalf("setup: real invoices=%d customers=%d selin invoices=%d", realInvoices, realCustomers, selinInvoices)
	}
	if n := count(t, d, `SELECT count(*) FROM customers WHERE taxpayer_id = $1`, atlas.TaxpayerID); n == 0 {
		t.Fatal("setup: no demo customers (the RESTRICT case of #148)")
	}

	if err := dropDemo(ctx, d.Pool); err != nil {
		t.Fatalf("reset: %v", err)
	}

	// Demo office and the demo-only taxpayer with every row of its data.
	if n := count(t, d, `SELECT count(*) FROM organizations WHERE slug = $1`, demo.OrgSlug); n != 0 {
		t.Fatalf("demo office left: %d", n)
	}
	if n := count(t, d, `SELECT count(*) FROM taxpayers WHERE id = $1`, atlas.TaxpayerID); n != 0 {
		t.Fatal("demo-only taxpayer left")
	}
	for _, table := range []string{"invoices", "customers", "invoice_sources", "data_warnings", "attachments", "taxpayer_members"} {
		if n := count(t, d, `SELECT count(*) FROM `+table+` WHERE taxpayer_id = $1`, atlas.TaxpayerID); n != 0 {
			t.Errorf("%s rows of the demo taxpayer left: %d", table, n)
		}
	}

	// Real tenant untouched.
	if n := count(t, d, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`, realTP); n != realInvoices {
		t.Errorf("real invoices = %d, want %d", n, realInvoices)
	}
	if n := count(t, d, `SELECT count(*) FROM customers WHERE taxpayer_id = $1`, realTP); n != realCustomers {
		t.Errorf("real customers = %d, want %d", n, realCustomers)
	}
	if n := count(t, d, `SELECT count(*) FROM engagements WHERE organization_id = $1`, realOrg); n != 2 {
		t.Errorf("real engagements = %d, want 2", n)
	}
	// A demo tax id another office serves keeps its identity and data.
	if n := count(t, d, `SELECT count(*) FROM taxpayers WHERE id = $1`, selin.TaxpayerID); n != 1 {
		t.Error("shared demo taxpayer was deleted")
	}
	if n := count(t, d, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`, selin.TaxpayerID); n != selinInvoices {
		t.Errorf("shared taxpayer invoices = %d, want %d", n, selinInvoices)
	}

	// The demo can be rebuilt afterwards.
	if err := seed(ctx, d.Pool, "Demo-Test-1!", ""); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	if err := dropDemo(ctx, d.Pool); err != nil {
		t.Fatalf("second reset: %v", err)
	}
}

// TestSeedClickFixWidget (#387): with a widget id the demo office's support
// setting is written, so its taxpayers see the support button in the portal.
func TestSeedClickFixWidget(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	if err := seed(ctx, d.Pool, "Demo-Test-1!", "demo-ofis-destek"); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := d.Pool.QueryRow(ctx, `
		SELECT s.clickfix_widget_id FROM office_support_settings s
		JOIN organizations o ON o.id = s.organization_id WHERE o.slug = $1`, demo.OrgSlug).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != "demo-ofis-destek" {
		t.Fatalf("widget id = %q", id)
	}
}

// TestSeedBankMonth: the demo atlas engagement has a bank month to match
// (#383): the alias answering for X Yazılım, the payments approved and
// booked (#456), one collection waiting in the queue.
func TestSeedBankMonth(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	if err := seed(ctx, d.Pool, "Demo-Test-1!", ""); err != nil {
		t.Fatal(err)
	}
	atlas := demoScope(t, d, "atlas")
	if n := count(t, d, `SELECT count(*) FROM bank_transactions WHERE taxpayer_id = $1`, atlas.TaxpayerID); n != 9 {
		t.Fatalf("movements = %d", n)
	}
	if n := count(t, d, `SELECT count(*) FROM bank_transaction_postings WHERE engagement_id = $1 AND status = 'approved'`, atlas.ID); n < 3 {
		t.Fatalf("approved = %d (alias rule auto-approved + fees)", n)
	}
	if n := count(t, d, `SELECT count(*) FROM bank_transaction_postings WHERE engagement_id = $1 AND status = 'suggested'`, atlas.ID); n == 0 {
		t.Fatal("nothing waits in the queue")
	}
	if n := count(t, d, `SELECT count(*) FROM bank_transaction_postings WHERE engagement_id = $1 AND account_code = '120.01.015' AND source = 'alias'`, atlas.ID); n != 1 {
		t.Fatalf("alias suggestion = %d", n)
	}
	if err := dropDemo(ctx, d.Pool); err != nil {
		t.Fatalf("reset: %v", err)
	}
}
