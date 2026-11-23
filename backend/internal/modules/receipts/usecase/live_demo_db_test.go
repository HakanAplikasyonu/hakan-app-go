package usecase

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/posting"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// #780 canlı demo: mobilden masraf fişi → oran kırılımıyla taslak, okunamayan
// alan boş ve "kontrol et"; tarihsiz fiş yükleme ayına düşer; ikinci
// satıcıda mükellefin son hesabı önerilir.
func TestLiveDemoExpenseAgainstPostgres(t *testing.T) {
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
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'Demo') RETURNING id`, "demo-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', '3881056172', 'Demo') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))
	reader := &fakeReader{}
	svc := New(pool, nopStore{}, reader, nil, nil, nil).WithJournals(journals.New(pool, nopStore{}))
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}

	reader.answer = `{"kind":"expense_receipt","kind_confidence":0.95,"fields":{
"date":{"value":"14.10.2026","confidence":0.95},"vendor_tax_number":{"value":"4560012345","confidence":0.93},
"vendor_name":{"value":"DEMO MARKET","confidence":0.9},"receipt_number":{"value":"0042","confidence":0.9},
"total_amount":{"value":"921,00","confidence":0.94},"tax_amount":{"value":"121,00","confidence":0.92},
"vat_1_total":{"value":"101,00","confidence":0.9},"vat_1_tax":{"value":"1,00","confidence":0.9},
"vat_10_total":{"value":"220,00","confidence":0.9},
"vat_20_total":{"value":"600,00","confidence":0.9},"vat_20_tax":{"value":"100,00","confidence":0.9}}}`
	c1, err := svc.Capture(ctx, sc, user, "f1.png", png("demo-f1"), extract.KindExpense)
	must(err)
	if c1.Draft == nil || c1.Draft.Type != "expense" {
		t.Fatalf("capture = %+v", c1)
	}
	e1, err := svc.Expense(ctx, sc, c1.Draft.UUID)
	must(err)
	if e1.Period == nil || *e1.Period != "2026-10" {
		t.Fatalf("period = %v", e1.Period)
	}
	if len(e1.VatLines) != 2 || e1.VatLines[0].Rate.String() != "1" || e1.VatLines[1].Base.String() != "500" {
		t.Fatalf("vat lines = %+v (the %%10 line without VAT must not be stored)", e1.VatLines)
	}
	if !slices.Equal(e1.ReviewFields, []string{"vat_10_tax"}) {
		t.Fatalf("review = %v", e1.ReviewFields)
	}

	// Office chooses the account and approves: the taxpayer memory now has it.
	acc := "770.01.003"
	_, err = svc.SetExpensePosting(ctx, sc, c1.Draft.UUID, user, PostingInput{AccountCode: &acc})
	must(err)
	_, err = svc.DecideExpense(ctx, sc, c1.Draft.UUID, user, true)
	must(err)

	// Second receipt: another vendor, no date and no total read.
	reader.answer = `{"kind":"expense_receipt","kind_confidence":0.9,"fields":{
"vendor_tax_number":{"value":"7770012345","confidence":0.9},"vendor_name":{"value":"BAŞKA DÜKKAN","confidence":0.9},
"receipt_number":{"value":"7","confidence":0.9},"tax_amount":{"value":"10,00","confidence":0.9}}}`
	c2, err := svc.Capture(ctx, sc, user, "f2.png", png("demo-f2"), extract.KindExpense)
	must(err)
	e2, err := svc.Expense(ctx, sc, c2.Draft.UUID)
	must(err)
	if e2.Date != nil || e2.Amount != nil {
		t.Fatalf("unread date / total must stay empty: %+v", e2)
	}
	if e2.Period == nil || *e2.Period != DocPeriod(time.Time{}, false, time.Now()) {
		t.Fatalf("undated receipt period = %v", e2.Period)
	}
	if !slices.Contains(e2.ReviewFields, "date") || !slices.Contains(e2.ReviewFields, "total_amount") {
		t.Fatalf("review = %v", e2.ReviewFields)
	}
	if !e2.Suggested || e2.AccountCode == nil || *e2.AccountCode != acc || *e2.AccountSource != posting.SourceTaxpayerMemory {
		t.Fatalf("taxpayer memory suggestion = %+v", e2)
	}
	if _, err := svc.DecideExpense(ctx, sc, c2.Draft.UUID, user, true); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("approving without date / total = %v, want ErrIncomplete", err)
	}
	var n int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM expense_vat_lines WHERE taxpayer_id = $1`, tp).Scan(&n))
	if n != 2 {
		t.Fatalf("expense_vat_lines = %d", n)
	}
}
