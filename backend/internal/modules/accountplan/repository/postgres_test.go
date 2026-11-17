package repository

import (
	"context"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warningsrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

const planCSV = "Hesap Kodu;Hesap Adı\n100;KASA\n100.01;Merkez\n120;ALICILAR\n120.01;Alıcı A\n600;YURTİÇİ SATIŞLAR\n600.01;Satış\n391;HESAPLANAN KDV\n391.01;KDV %20\n"

// TestGateBlocksMissingCodePerSetting is the #30 acceptance criterion on a
// real database: an entry with a code outside the imported Luca plan is
// blocked by default (blocker), only warned when the office lowers the
// severity, and passes silently when the code is disabled. Fixing the code
// auto-resolves the warning.
func TestGateBlocksMissingCodePerSetting(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", "1234567890")
	eng := d.Engagement(t, org, tp)
	other := d.Engagement(t, org, d.Taxpayer(t, "legal", "1234567891"))
	q := db.New(d.Pool)

	svc := usecase.New(New(d.Pool, q))
	cfg := warningsrepo.NewConfig(q)
	gate := usecase.NewGate(svc, engine.New(warningsrepo.NewWarnings(q), cfg), cfg)
	subj := engine.Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}
	ent := engine.Entity{Type: "invoice", ID: ptr(int64(4242)), Period: "2026-10"}

	// No plan yet: nothing checked, nothing blocked.
	v, err := gate.Evaluate(ctx, subj, ent, "", []string{"999.99"})
	if err != nil || v.PlanLoaded || v.Blocked {
		t.Fatalf("no plan: %+v %v", v, err)
	}

	res, err := svc.Import(ctx, usecase.Subject{OrganizationID: org, EngagementID: eng}, 0, "", "plan.csv", "", []byte(planCSV))
	if err != nil {
		t.Fatal(err)
	}
	if res.Import.AccountCount != 8 || res.Import.SourceKind != usecase.KindAccountPlan {
		t.Fatalf("import = %+v", res)
	}

	codes := []string{"120.01", "600.01", "391.01", "770.01"}
	v, err = gate.Evaluate(ctx, subj, ent, "luca", codes)
	if err != nil {
		t.Fatal(err)
	}
	if !v.PlanLoaded || !v.Blocked || len(v.Missing) != 1 || v.Missing[0] != "770.01" {
		t.Fatalf("default blocker: %+v", v)
	}
	if n := openWarnings(t, d, eng); n != 1 {
		t.Fatalf("open warnings = %d", n)
	}

	// Office lowers the code to warning: still reported, no longer blocks.
	setOffice(t, d, org, `'warning'`, "NULL")
	v, err = gate.Evaluate(ctx, subj, ent, "luca", codes)
	if err != nil || v.Blocked || len(v.Missing) != 1 {
		t.Fatalf("warning severity: %+v %v", v, err)
	}
	// Disabled: neither blocks nor keeps the warning open.
	setOffice(t, d, org, "NULL", "false")
	v, err = gate.Evaluate(ctx, subj, ent, "luca", codes)
	if err != nil || v.Blocked || v.Enabled {
		t.Fatalf("disabled: %+v %v", v, err)
	}
	if n := openWarnings(t, d, eng); n != 0 {
		t.Fatalf("open warnings after disable = %d", n)
	}
	setOffice(t, d, org, "NULL", "true")

	// A group account (has sub-accounts) blocks too (#319): Luca does not
	// take postings on it. Fixing the code resolves the warning.
	v, err = gate.Evaluate(ctx, subj, ent, "luca", []string{"120", "600.01"})
	if err != nil || !v.Blocked || len(v.NonLeaf) != 1 {
		t.Fatalf("non-leaf: %+v %v", v, err)
	}
	v, err = gate.Evaluate(ctx, subj, ent, "luca", []string{"120.01", "600.01"})
	if err != nil || v.Blocked {
		t.Fatalf("fixed: %+v %v", v, err)
	}
	var autoResolved int
	if err := d.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM data_warnings WHERE engagement_id = $1 AND status = 'auto_resolved'`, eng).Scan(&autoResolved); err != nil || autoResolved == 0 {
		t.Fatalf("auto_resolved = %d %v", autoResolved, err)
	}

	// The plan is per engagement: the other mükellef has none.
	r2, err := svc.Check(ctx, usecase.Subject{OrganizationID: org, EngagementID: other}, "", []string{"770.01"})
	if err != nil || r2.PlanLoaded {
		t.Fatalf("other engagement: %+v %v", r2, err)
	}

	// A new import replaces the active plan; history keeps both versions.
	if _, err := svc.Import(ctx, usecase.Subject{OrganizationID: org, EngagementID: eng}, 0, "", "plan2.csv", "",
		[]byte("Hesap Kodu;Hesap Adı\n770;GENEL YÖNETİM\n770.01;Kira\n")); err != nil {
		t.Fatal(err)
	}
	r3, err := svc.Check(ctx, usecase.Subject{OrganizationID: org, EngagementID: eng}, "", []string{"770.01", "120.01"})
	if err != nil || len(r3.Missing) != 1 || r3.Missing[0] != "120.01" {
		t.Fatalf("after reimport: %+v %v", r3, err)
	}
	imps, err := svc.Imports(ctx, usecase.Subject{OrganizationID: org, EngagementID: eng}, 10)
	if err != nil || len(imps) != 2 || imps[0].FileName != "plan2.csv" {
		t.Fatalf("imports = %+v %v", imps, err)
	}
	page, err := svc.Accounts(ctx, usecase.Subject{OrganizationID: org, EngagementID: eng}, "", "770", 10, 0)
	if err != nil || page.Total != 2 {
		t.Fatalf("accounts = %+v %v", page, err)
	}
}

func setOffice(t *testing.T, d *testdb.DB, org int64, severity, enabled string) {
	t.Helper()
	_, err := d.Pool.Exec(context.Background(), `
		INSERT INTO warning_rule_configs (organization_id, code, severity, enabled)
		VALUES ($1, 'ACCOUNT_NOT_IN_TARGET_PLAN', `+severity+`, `+enabled+`)
		ON CONFLICT (organization_id, code) WHERE engagement_id IS NULL
		DO UPDATE SET severity = EXCLUDED.severity, enabled = EXCLUDED.enabled`, org)
	if err != nil {
		t.Fatal(err)
	}
}

func openWarnings(t *testing.T, d *testdb.DB, eng int64) int {
	t.Helper()
	var n int
	if err := d.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM data_warnings WHERE engagement_id = $1 AND code = 'ACCOUNT_NOT_IN_TARGET_PLAN' AND status = 'open'`, eng).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func ptr[T any](v T) *T { return &v }

// TestExportGateNormalizesAndRejectsGroupAccounts is the #319 export check:
// "120 01" is the plan's "120.01" (the line is rewritten to it), a group
// account and a missing code block per line, and a warning lowered to
// "warning" or an export setting that does not block only warns.
func TestExportGateNormalizesAndRejectsGroupAccounts(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", "1234567892")
	eng := d.Engagement(t, org, tp)
	q := db.New(d.Pool)
	svc := usecase.New(New(d.Pool, q))
	cfg := warningsrepo.NewConfig(q)
	check := usecase.NewGate(svc, engine.New(warningsrepo.NewWarnings(q), cfg), cfg).ExportGate()
	scope := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}
	date := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	entries := func() []accountingexport.Entry {
		return []accountingexport.Entry{{No: "1", Date: date, Lines: []accountingexport.Line{
			{Account: "120 01"}, {Account: "120"}, {Account: "770.01"}, {Account: "600.01"},
		}}}
	}

	// No plan: nothing checked.
	if is, err := check(ctx, scope, "luca", entries(), true); err != nil || len(is) != 0 {
		t.Fatalf("no plan: %v %v", is, err)
	}
	if _, err := svc.Import(ctx, usecase.Subject{OrganizationID: org, EngagementID: eng}, 0, "", "plan.csv", "", []byte(planCSV)); err != nil {
		t.Fatal(err)
	}
	es := entries()
	is, err := check(ctx, scope, "luca", es, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(is) != 2 || is[0].Line != 2 || is[0].Params["reason"] != "group_account" || is[1].Line != 3 ||
		is[0].Severity != accountingexport.SeverityBlocker || is[1].Severity != accountingexport.SeverityBlocker {
		t.Fatalf("issues = %+v", is)
	}
	if es[0].Lines[0].Account != "120.01" {
		t.Fatalf("normalized line = %q, want 120.01", es[0].Lines[0].Account)
	}
	if n := openWarnings(t, d, eng); n != 1 {
		t.Fatalf("open warnings = %d", n)
	}
	// Export setting off: the same lines only warn.
	if is, _ := check(ctx, scope, "luca", entries(), false); len(is) != 2 || is[0].Severity != accountingexport.SeverityWarning {
		t.Fatalf("non-blocking setting: %+v", is)
	}
	// Another target program with no plan: nothing checked.
	if is, _ := check(ctx, scope, "zirve", entries(), true); len(is) != 0 {
		t.Fatalf("zirve without a plan: %+v", is)
	}
}

// TestTrialBalanceReport (#393): mizan uploads keep their amounts per
// import; the report compares the latest mizan with the previous period,
// down to the main accounts, and another engagement sees none of it.
func TestTrialBalanceReport(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", "1234567892")
	eng := d.Engagement(t, org, tp)
	other := d.Engagement(t, org, d.Taxpayer(t, "legal", "1234567893"))
	svc := usecase.New(New(d.Pool, db.New(d.Pool)))
	sub := usecase.Subject{OrganizationID: org, EngagementID: eng}

	const head = "Hesap Kodu;Hesap Adı;Borç;Alacak;Borç Bakiye;Alacak Bakiye\n"
	sep := "MİZAN 01.01.2026 - 30.09.2026\n" + head +
		"100;KASA;1.000,00;400,00;600,00;\n100.01;MERKEZ KASA;1.000,00;400,00;600,00;\n" +
		"320;SATICILAR;0,00;250,00;;250,00\n320.01;SATICI A;0,00;250,00;;250,00\n"
	oct := head +
		"100;KASA;1.500,00;500,00;1.000,00;\n100.01;MERKEZ KASA;1.500,00;500,00;1.000,00;\n" +
		"600;SATIŞLAR;0,00;900,00;;900,00\n600.01;YURTİÇİ;0,00;900,00;;900,00\n"

	// A plain account plan is not a trial balance.
	if _, err := svc.Import(ctx, sub, 0, "", "plan.csv", "", []byte(planCSV)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TrialBalanceReport(ctx, sub, "latest", "", 0); err != usecase.ErrNotFound {
		t.Fatalf("no mizan yet: %v", err)
	}
	res, err := svc.Import(ctx, sub, 0, "", "eylul.csv", "", []byte(sep))
	if err != nil || res.Import.Period == nil || *res.Import.Period != "2026-09" {
		t.Fatalf("eylül import = %+v %v", res.Import, err)
	}
	if _, err := svc.Import(ctx, sub, 0, "", "ekim.csv", "2026-13", []byte(oct)); err == nil {
		t.Fatal("invalid period accepted")
	}
	if _, err := svc.Import(ctx, sub, 0, "", "ekim.csv", "2026-10", []byte(oct)); err != nil {
		t.Fatal(err)
	}

	list, err := svc.TrialBalances(ctx, sub)
	if err != nil || len(list) != 2 || *list[0].Period != "2026-10" || list[0].DebitTotal != "1500.00" || list[0].CreditTotal != "1400.00" {
		t.Fatalf("list = %+v %v", list, err)
	}
	rep, err := svc.TrialBalanceReport(ctx, sub, "latest", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ComparedTo == nil || rep.ComparedTo.UUID != list[1].UUID || len(rep.Lines) != 2 {
		t.Fatalf("report = %+v", rep)
	}
	byCode := map[string]usecase.TrialBalanceLine{}
	for _, l := range rep.Lines {
		byCode[l.Code] = l
	}
	if l := byCode["100"]; l.Balance != "1000.00" || *l.PreviousBalance != "600.00" || *l.Change != "400.00" {
		t.Fatalf("100 = %+v", l)
	}
	if l := byCode["600"]; l.Balance != "-900.00" || l.PreviousBalance != nil || *l.Change != "-900.00" {
		t.Fatalf("600 = %+v", l)
	}
	if rep, err := svc.TrialBalanceReport(ctx, sub, list[1].UUID.String(), usecase.CompareNone, 0); err != nil || rep.ComparedTo != nil || len(rep.Lines) != 4 {
		t.Fatalf("eylül without comparison = %+v %v", rep, err)
	}

	// The other engagement sees nothing, not even by uuid.
	osub := usecase.Subject{OrganizationID: org, EngagementID: other}
	if l, err := svc.TrialBalances(ctx, osub); err != nil || len(l) != 0 {
		t.Fatalf("other engagement list = %+v %v", l, err)
	}
	if _, err := svc.TrialBalanceReport(ctx, osub, list[0].UUID.String(), "", 0); err != usecase.ErrNotFound {
		t.Fatalf("other engagement by uuid: %v", err)
	}
}
