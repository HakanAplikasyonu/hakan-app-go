package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/posting"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warnrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// #384: masraf listesi → hesap önerisi (mizan) → müşavir seçimi → onay →
// yevmiye taslağı → satıcı hafızası; kategori katmanları.
func TestExpensePostingAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t).Pool
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	var org, tp, eng, user int64
	tag := time.Now().Format("150405.000000")
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'R') RETURNING id`, "ex-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', '4730074556', 'T') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))
	// Mükellefin Luca planı: 770 altında yalnızca akaryakıt, KDV %20, kasa.
	var imp int64
	must(pool.QueryRow(ctx, `INSERT INTO account_plan_imports (organization_id, engagement_id, source_format, source_kind, sha256)
		VALUES ($1, $2, 'xlsx', 'trial_balance', repeat('a', 64)) RETURNING id`, org, eng).Scan(&imp))
	_, err := pool.Exec(ctx, `INSERT INTO account_plan_accounts (organization_id, engagement_id, import_id, code, name, main_code, level, is_leaf) VALUES
		($1, $2, $3, '770', 'GENEL YÖNETİM GİDERLERİ', '770', 1, false),
		($1, $2, $3, '770.02', 'AKARYAKIT GİDERLERİ', '770', 2, true),
		($1, $2, $3, '770.05', 'YEMEK GİDERLERİ', '770', 2, true),
		($1, $2, $3, '191.01', 'İNDİRİLECEK KDV %20', '191', 2, true),
		($1, $2, $3, '100.01', 'MERKEZ KASA', '100', 2, true)`, org, eng, imp)
	must(err)

	reader := &fakeReader{answer: fuel}
	q := db.New(pool)
	svc := New(pool, nopStore{}, reader, nil, engine.New(warnrepo.NewWarnings(q), warnrepo.NewConfig(q)), nil).WithJournals(journals.New(pool, nopStore{}))
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}
	// UNAPPROVED_EXPENSE (#529): açık uyarı sayısı ve son durum.
	unapproved := func() (open int, last string) {
		t.Helper()
		must(pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status = 'open'),
			COALESCE((array_agg(status ORDER BY updated_at DESC))[1], '')
			FROM data_warnings WHERE engagement_id = $1 AND code = 'UNAPPROVED_EXPENSE' AND producer = $2`,
			eng, ExpenseApprovalProducer).Scan(&open, &last))
		return open, last
	}

	c, err := svc.Capture(ctx, sc, user, "fis.png", png("ex-a"), extract.KindFuel)
	must(err)
	if open, _ := unapproved(); open != 1 {
		t.Fatalf("pending expense must raise UNAPPROVED_EXPENSE, open = %d", open)
	}
	page, err := svc.Expenses(ctx, sc, ExpenseFilter{Limit: 50})
	must(err)
	if page.Total != 1 || page.Pending != 1 || len(page.Items) != 1 || page.Amount.String() != "1200" {
		t.Fatalf("page %+v", page)
	}
	x := page.Items[0]
	if !x.Suggested || *x.CategoryCode != "akaryakit" || *x.AccountCode != "770.02" || *x.AccountSource != posting.SourcePlan {
		t.Fatalf("suggestion %+v", x)
	}

	// Kategori seçimi ve elle hesap.
	cat := "yemek"
	v, err := svc.SetExpensePosting(ctx, sc, x.UUID, user, PostingInput{CategoryCode: &cat})
	must(err)
	if v.Suggested || *v.AccountCode != "770.05" || *v.CategoryName != "Yemek ve ikram" {
		t.Fatalf("category %+v", v)
	}
	bad := "77"
	if _, err := svc.SetExpensePosting(ctx, sc, x.UUID, user, PostingInput{AccountCode: &bad}); err == nil {
		t.Fatal("bad account accepted")
	}
	acc := "770.02"
	v, err = svc.SetExpensePosting(ctx, sc, x.UUID, user, PostingInput{AccountCode: &acc})
	must(err)
	if *v.AccountSource != posting.SourceManual || *v.CategoryCode != "yemek" {
		t.Fatalf("manual %+v", v)
	}

	// Onay → yevmiye taslağı; fiş yakalama kaydı da onaylı.
	v, err = svc.DecideExpense(ctx, sc, x.UUID, user, true)
	must(err)
	if v.ApprovalStatus != "approved" || v.Journal == nil || v.Journal.Status != "draft" {
		t.Fatalf("approved %+v", v)
	}
	if open, last := unapproved(); open != 0 || last != "auto_resolved" {
		t.Fatalf("approval must resolve UNAPPROVED_EXPENSE: open %d, last %q", open, last)
	}
	rows, err := pool.Query(ctx, `SELECT l.account_code, l.debit::text, l.credit::text, l.source_kind, l.source_id
		FROM journal_draft_lines l JOIN journal_drafts d ON d.id = l.journal_draft_id WHERE d.uuid = $1 ORDER BY l.line_no`, v.Journal.UUID)
	must(err)
	var got []string
	for rows.Next() {
		var a, dr, cr, kind, src string
		must(rows.Scan(&a, &dr, &cr, &kind, &src))
		if kind != "receipt" || src != "expense:"+x.UUID.String() {
			t.Fatalf("source %s %s", kind, src)
		}
		got = append(got, a+" "+dr+" "+cr)
	}
	want := []string{"770.02 1000.0000 0.0000", "191.01 200.0000 0.0000", "100.01 0.0000 1200.0000"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("lines %v", got)
	}
	cv, err := svc.Get(ctx, sc, c.UUID)
	must(err)
	if cv.Status != StatusApproved {
		t.Fatalf("capture status %s", cv.Status)
	}
	if _, err := svc.DecideExpense(ctx, sc, x.UUID, user, false); err != ErrState {
		t.Fatalf("second decision %v", err)
	}

	// Aynı satıcıdan yeni fiş: satıcı hafızası.
	reader.answer = `{"kind":"fuel_receipt","fields":{"date":{"value":"20.10.2026","confidence":0.9},"vendor_tax_number":{"value":"1234567890","confidence":0.9},"total_amount":{"value":"600,00","confidence":0.9},"tax_amount":{"value":"100,00","confidence":0.9},"tax_rate":{"value":"20","confidence":0.9}}}`
	c2, err := svc.Capture(ctx, sc, user, "fis2.png", png("ex-b"), "")
	must(err)
	page, err = svc.Expenses(ctx, sc, ExpenseFilter{Status: "pending", Limit: 50})
	must(err)
	if len(page.Items) != 1 || *page.Items[0].AccountSource != posting.SourceVendorMemory || *page.Items[0].AccountCode != "770.02" {
		t.Fatalf("memory %+v", page.Items)
	}
	// Fiş yakalama ekranından onay da taslak üretir.
	if open, _ := unapproved(); open != 1 {
		t.Fatalf("second pending expense: open = %d", open)
	}
	_, err = svc.Decide(ctx, sc, c2.UUID, user, true)
	must(err)
	if open, _ := unapproved(); open != 0 {
		t.Fatalf("capture approval must resolve UNAPPROVED_EXPENSE: open = %d", open)
	}
	var drafts int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM journal_drafts WHERE engagement_id = $1`, eng).Scan(&drafts))
	if drafts != 2 {
		t.Fatalf("drafts = %d", drafts)
	}
	page, err = svc.Expenses(ctx, sc, ExpenseFilter{Category: "yemek", Limit: 50})
	must(err)
	if page.Total != 2 {
		t.Fatalf("category filter total %d", page.Total)
	}

	// Kategori katmanları: ofis şablonu ve mükellef.
	off, err := svc.SaveCategory(ctx, org, 0, user, CategoryInput{Code: "yemek", Name: "Yemek (ofis)", AccountCode: "770.05"})
	must(err)
	own, err := svc.SaveCategory(ctx, org, eng, user, CategoryInput{Code: "yemek", Name: "Yemek (Atlas)", AccountCode: "760", Deductible: "no"})
	must(err)
	list, err := svc.Categories(ctx, org, eng)
	must(err)
	if cy, _ := posting.Find(list, "yemek"); cy.Name != "Yemek (Atlas)" || cy.Overrides != posting.ScopeOffice {
		t.Fatalf("effective %+v", cy)
	}
	list, err = svc.Categories(ctx, org, 0)
	must(err)
	if cy, _ := posting.Find(list, "yemek"); cy.Name != "Yemek (ofis)" || cy.Overrides != posting.ScopePlatform {
		t.Fatalf("template %+v", cy)
	}
	if err := svc.DeleteCategory(ctx, org, 0, *own.UUID); err != ErrNotFound {
		t.Fatalf("office delete of taxpayer row: %v", err)
	}
	must(svc.DeleteCategory(ctx, org, eng, *own.UUID))
	must(svc.DeleteCategory(ctx, org, 0, *off.UUID))
	if _, err := svc.SaveCategory(ctx, org, 0, user, CategoryInput{Code: "Kötü Kod", Name: "x", AccountCode: "770"}); err == nil {
		t.Fatal("bad code accepted")
	}
}
