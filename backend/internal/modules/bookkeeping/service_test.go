package bookkeeping

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ledger"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name                                 string
		lt, source, direction, typ, docClass string
		side, cat                            string
	}{
		{"alış faturası", ledger.Operating, SourceInvoice, "incoming", "SATIS", "invoice", SideExpense, CatGoodsPurchase},
		{"müstahsil", ledger.Operating, SourceInvoice, "incoming", "SATIS", "mustahsil", SideExpense, CatGoodsPurchase},
		{"gelen SMM", ledger.Operating, SourceInvoice, "incoming", "SATIS", "smm", SideExpense, CatProfessionalExpense},
		{"gelen iade", ledger.Operating, SourceInvoice, "incoming", "IADE", "invoice", SideIncome, CatSalesReturn},
		{"satış faturası", ledger.Operating, SourceInvoice, "outgoing", "SATIS", "invoice", SideIncome, CatSalesRevenue},
		{"giden iade", ledger.Operating, SourceInvoice, "outgoing", "IADE", "invoice", SideExpense, CatPurchaseReturn},
		{"serbest meslek makbuzu", ledger.SelfEmployedBook, SourceInvoice, "outgoing", "SATIS", "smm", SideIncome, CatProfessionalIncome},
		{"serbest meslek alış", ledger.SelfEmployedBook, SourceInvoice, "incoming", "SATIS", "invoice", SideExpense, CatGeneralExpense},
		{"Z raporu", ledger.Operating, SourceRetailSale, "outgoing", "Z", "", SideIncome, CatRetailRevenue},
		{"maaş", ledger.Operating, SourceExpense, "incoming", "salary", "", SideExpense, CatPersonnelExpense},
		{"akaryakıt", ledger.Operating, SourceExpense, "incoming", "fuel_receipt", "", SideExpense, CatVehicleExpense},
		{"banka masrafı", ledger.Operating, SourceExpense, "incoming", "bank_fee", "", SideExpense, CatFinanceExpense},
		{"vergi", ledger.Operating, SourceExpense, "incoming", "tax_payment", "", SideExpense, CatTaxExpense},
		{"ÖKC fişi", ledger.Operating, SourceExpense, "incoming", "okc_receipt", "", SideExpense, CatGeneralExpense},
	}
	for _, c := range cases {
		side, cat := Classify(c.lt, c.source, c.direction, c.typ, c.docClass)
		if side != c.side || cat != c.cat {
			t.Errorf("%s: got %s/%s, want %s/%s", c.name, side, cat, c.side, c.cat)
		}
	}
}

func num(s string) pgtype.Numeric { return repository.Num(decimal.RequireFromString(s)) }

func TestToRowOverrideAndReturns(t *testing.T) {
	base := db.ListOperatingAccountRowsRow{
		Source: SourceInvoice, RecordUuid: uuid.New(), Period: "2026-10", Direction: "incoming",
		DocType: "SATIS", DocClass: "invoice", Net: num("1000.00"), Vat: num("200.00"),
	}
	r := toRow(ledger.Operating, base)
	if r.Side != SideExpense || r.Category != CatGoodsPurchase || r.Overridden || !r.Total.Equal(decimal.RequireFromString("1200")) {
		t.Fatalf("default row: %+v", r)
	}

	o := base
	o.OverrideCategory = CatGeneralExpense
	if r := toRow(ledger.Operating, o); r.Category != CatGeneralExpense || !r.Overridden || r.Side != SideExpense {
		t.Fatalf("override: %+v", r)
	}
	// An override to an income category moves the row to the income side.
	o.OverrideCategory = CatOtherIncome
	if r := toRow(ledger.Operating, o); r.Side != SideIncome {
		t.Fatalf("income override: %+v", r)
	}
	// An unknown stored category is ignored.
	o.OverrideCategory = "unknown_thing"
	if r := toRow(ledger.Operating, o); r.Overridden || r.Category != CatGoodsPurchase {
		t.Fatalf("unknown override: %+v", r)
	}

	ret := base
	ret.DocType = "IADE"
	r = toRow(ledger.Operating, ret)
	if r.Category != CatSalesReturn || !r.Net.Equal(decimal.RequireFromString("-1000")) || !r.VAT.Equal(decimal.RequireFromString("-200")) {
		t.Fatalf("return row: %+v", r)
	}

	exp := db.ListOperatingAccountRowsRow{Source: SourceExpense, RecordUuid: uuid.New(), DocType: "okc_receipt", CategoryName: "Kırtasiye", Net: num("50"), Vat: num("10")}
	if r := toRow(ledger.Operating, exp); r.CategoryName != "Kırtasiye" || r.Category != CatGeneralExpense {
		t.Fatalf("expense category name: %+v", r)
	}
}

func TestLedgerValid(t *testing.T) {
	for _, v := range []string{ledger.Balance, ledger.Operating, ledger.SelfEmployedBook} {
		if !ledger.Valid(v) {
			t.Errorf("%s should be valid", v)
		}
	}
	if ledger.Valid("tek_duzen") || ledger.SingleEntry(ledger.Balance) || !ledger.SingleEntry(ledger.Operating) {
		t.Fatal("ledger helpers")
	}
}
