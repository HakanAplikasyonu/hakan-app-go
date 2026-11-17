package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warnrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

func zAnswer(date, number, extra string) string {
	return `{"kind":"z_report","kind_confidence":0.95,"fields":{"date":{"value":"` + date + `","confidence":0.95},` +
		`"z_report_number":{"value":"` + number + `","confidence":0.95},"okc_serial_number":{"value":"JH20012345","confidence":0.9}` + extra + `}}`
}

// #384: Z raporu → KDV / ödeme kırılımı → ÖKC bazında liste → süreklilik
// uyarıları → düzeltme → onay → satış fişi taslağı (100/108 – 600 – 391).
func TestRetailSalesAgainstPostgres(t *testing.T) {
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
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'Z') RETURNING id`, "z-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', '2981056178', 'Z') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))
	q := db.New(pool)
	reader := &fakeReader{}
	svc := New(pool, nopStore{}, reader, nil, engine.New(warnrepo.NewWarnings(q), warnrepo.NewConfig(q)), nil).
		WithJournals(journals.New(pool, nopStore{}))
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}

	reader.answer = zAnswer("15.10.2026", "0123", `,"total_amount":{"value":"12.100,00","confidence":0.9},"tax_amount":{"value":"1.902,00","confidence":0.9},
		"cash_amount":{"value":"5.100,00","confidence":0.9},"card_amount":{"value":"7.000,00","confidence":0.9},"receipt_count":{"value":"84","confidence":0.9},
		"vat_20_total":{"value":"10.800,00","confidence":0.9},"vat_20_tax":{"value":"1.800,00","confidence":0.9},
		"vat_10_total":{"value":"1.100,00","confidence":0.9},"vat_10_tax":{"value":"100,00","confidence":0.9},
		"vat_1_total":{"value":"200,00","confidence":0.9},"vat_1_tax":{"value":"2,00","confidence":0.9}`)
	z1, err := svc.Capture(ctx, sc, user, "z1.png", png("z1"), extract.KindZReport)
	must(err)
	reader.answer = zAnswer("16.10.2026", "0124", `,"total_amount":{"value":"3.000,00","confidence":0.9},"tax_amount":{"value":"500,00","confidence":0.9}`)
	z2, err := svc.Capture(ctx, sc, user, "z2.png", png("z2"), extract.KindZReport)
	must(err)
	reader.answer = zAnswer("19.10.2026", "0127", `,"total_amount":{"value":"2.400,00","confidence":0.9},"tax_amount":{"value":"400,00","confidence":0.9}`)
	_, err = svc.Capture(ctx, sc, user, "z3.png", png("z3"), extract.KindZReport)
	must(err)

	page, err := svc.RetailSales(ctx, sc, RetailFilter{Period: "2026-10", Limit: 50})
	must(err)
	if page.Total != 3 || page.Pending != 3 || page.Amount.String() != "17500" || len(page.Devices) != 1 || page.Devices[0].Reports != 3 {
		t.Fatalf("page %+v", page)
	}
	if len(page.VatRates) != 3 || page.VatRates[2].Rate.String() != "20" || page.VatRates[2].Base.String() != "9000" {
		t.Fatalf("rates %+v", page.VatRates)
	}
	byNo := map[string]RetailSaleView{}
	for _, it := range page.Items {
		byNo[*it.ZNumber] = it
	}
	first := byNo["0123"]
	if first.Cash.String() != "5100" || first.Card.String() != "7000" || *first.ReceiptCount != 84 || len(first.VatLines) != 3 || first.BreakdownMismatch {
		t.Fatalf("breakdown %+v", first)
	}
	last := byNo["0127"]
	if len(last.Warnings) != 2 || last.Warnings[0] != "Z_DAY_GAP" || last.Warnings[1] != "Z_NUMBER_GAP" {
		t.Fatalf("warnings on 0127: %v", last.Warnings)
	}
	if len(byNo["0124"].Warnings) != 0 {
		t.Fatalf("warnings on 0124: %v", byNo["0124"].Warnings)
	}

	// Düzeltme: numara 0125 → numara atlaması kapanır, gün boşluğu kalır.
	num := "0125"
	v, err := svc.UpdateRetailSale(ctx, sc, last.UUID, RetailInput{ZNumber: &num})
	must(err)
	if len(v.Warnings) != 1 || v.Warnings[0] != "Z_DAY_GAP" {
		t.Fatalf("after fix: %v", v.Warnings)
	}

	// Onay → satış fişi taslağı.
	v, err = svc.DecideRetailSale(ctx, sc, first.UUID, user, true)
	must(err)
	if v.ApprovalStatus != "approved" || v.Journal == nil {
		t.Fatalf("approved %+v", v)
	}
	var debit, credit string
	var lines int
	must(pool.QueryRow(ctx, `SELECT sum(l.debit)::text, sum(l.credit)::text, count(*) FROM journal_draft_lines l
		JOIN journal_drafts d ON d.id = l.journal_draft_id WHERE d.uuid = $1`, v.Journal.UUID).Scan(&debit, &credit, &lines))
	if debit != "12100.0000" || credit != "12100.0000" || lines != 8 {
		t.Fatalf("entry %s %s %d", debit, credit, lines)
	}
	var cardLine string
	must(pool.QueryRow(ctx, `SELECT l.debit::text FROM journal_draft_lines l JOIN journal_drafts d ON d.id = l.journal_draft_id
		WHERE d.uuid = $1 AND l.account_code = '108'`, v.Journal.UUID).Scan(&cardLine))
	if cardLine != "7000.0000" {
		t.Fatalf("108 = %s", cardLine)
	}
	if _, err := svc.UpdateRetailSale(ctx, sc, first.UUID, RetailInput{ZNumber: &num}); err != ErrState {
		t.Fatalf("edit after approval: %v", err)
	}
	// Fiş yakalama ekranından onay da taslak üretir.
	_, err = svc.Decide(ctx, sc, z2.UUID, user, true)
	must(err)
	var drafts int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM journal_drafts WHERE engagement_id = $1`, eng).Scan(&drafts))
	if drafts != 2 {
		t.Fatalf("drafts = %d", drafts)
	}
	cv, err := svc.Get(ctx, sc, z1.UUID)
	must(err)
	if cv.Status != StatusApproved {
		t.Fatalf("capture %s", cv.Status)
	}
	n, err := svc.RunZContinuity(ctx, sc)
	must(err)
	if n != 1 {
		t.Fatalf("reports with findings = %d", n)
	}
}
