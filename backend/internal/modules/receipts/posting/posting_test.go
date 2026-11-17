package posting

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
)

func TestMergeLayersByCode(t *testing.T) {
	office := []Category{{Code: "yemek", Name: "Yemek (ofis)", AccountCode: "770.05", Active: true, Scope: ScopeOffice},
		{Code: "kkeg", Active: false, Scope: ScopeOffice, Name: "KKEG"}}
	own := []Category{{Code: "yemek", Name: "Yemek (mükellef)", AccountCode: "760.01", Active: true, Scope: ScopeTaxpayer},
		{Code: "ozel", Name: "Özel", AccountCode: "770.09", Active: true, Scope: ScopeTaxpayer}}
	got := Merge(DefaultCategories, office, own)
	c, ok := Find(got, "yemek")
	if !ok || c.AccountCode != "760.01" || c.Scope != ScopeTaxpayer || c.Overrides != ScopeOffice {
		t.Fatalf("yemek = %+v", c)
	}
	if _, ok := Find(got, "kkeg"); ok {
		t.Fatal("inactive office row must hide the default")
	}
	if _, ok := Find(got, "ozel"); !ok {
		t.Fatal("taxpayer category missing")
	}
	if got[len(got)-1].Code != FallbackCategory {
		t.Fatalf("fallback must be last, got %s", got[len(got)-1].Code)
	}
}

func TestPick(t *testing.T) {
	cats := Merge(DefaultCategories, nil, nil)
	cases := []struct{ typ, vendor, want string }{
		{"fuel_receipt", "", "akaryakit"},
		{"okc_receipt", "OPET PETROLCÜLÜK A.Ş.", "akaryakit"},
		{"okc_receipt", "Köşe Lokantası", "yemek"},
		{"okc_receipt", "Yurtiçi Kargo", "kargo"},
		{"bank_fee", "", "banka"},
		{"okc_receipt", "ABC Ticaret", FallbackCategory},
		{"other", "", FallbackCategory},
	}
	for _, c := range cases {
		got, ok := Pick(cats, c.typ, c.vendor, "")
		if !ok || got.Code != c.want {
			t.Errorf("Pick(%s, %q) = %s, want %s", c.typ, c.vendor, got.Code, c.want)
		}
	}
}

var plan = []engine.PlanAccount{
	{Code: "770", Name: "GENEL YÖNETİM GİDERLERİ", IsLeaf: false},
	{Code: "770.01", Name: "PERSONEL GİDERLERİ", IsLeaf: true},
	{Code: "770.02", Name: "AKARYAKIT GİDERLERİ", IsLeaf: true},
	{Code: "770.03", Name: "YEMEK GİDERLERİ", IsLeaf: true},
	{Code: "191", Name: "İNDİRİLECEK KDV", IsLeaf: false},
	{Code: "191.01", Name: "İNDİRİLECEK KDV %20", IsLeaf: true},
	{Code: "191.02", Name: "İNDİRİLECEK KDV %10", IsLeaf: true},
	{Code: "100.01", Name: "MERKEZ KASA", IsLeaf: true},
}

func TestResolveLeaf(t *testing.T) {
	if r := ResolveLeaf("770", "Akaryakıt", plan); r.Code != "770.02" || !r.InPlan || !r.Confidence.Equal(confByName) {
		t.Fatalf("by name %+v", r)
	}
	if r := ResolveLeaf("770", "Kargo ve posta", plan); r.Code != "770.01" || !r.Confidence.Equal(confGuess) {
		t.Fatalf("guess %+v", r)
	}
	if r := ResolveLeaf("770.03", "", plan); r.Code != "770.03" || !r.Confidence.Equal(confExact) {
		t.Fatalf("exact %+v", r)
	}
	if r := ResolveLeaf("100", "kasa", plan); r.Code != "100.01" {
		t.Fatalf("single %+v", r)
	}
	if r := ResolveLeaf("689", "", plan); r.Code != "689" || r.InPlan {
		t.Fatalf("not in plan %+v", r)
	}
}

func TestSuggestPrefersVendorMemory(t *testing.T) {
	cats := Merge(DefaultCategories, nil, nil)
	s, ok := Suggest(cats, "fuel_receipt", "Opet", "", &Memory{CategoryCode: "akaryakit", AccountCode: "770.09"}, plan)
	if !ok || s.AccountCode != "770.09" || s.Source != SourceVendorMemory || s.CategoryCode != "akaryakit" {
		t.Fatalf("memory %+v", s)
	}
	s, _ = Suggest(cats, "fuel_receipt", "Opet", "", nil, plan)
	if s.AccountCode != "770.02" || s.Source != SourcePlan {
		t.Fatalf("plan %+v", s)
	}
	s, _ = Suggest(cats, "fuel_receipt", "Opet", "", nil, nil)
	if s.AccountCode != "770" || s.Source != SourceCategory {
		t.Fatalf("no plan %+v", s)
	}
}

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestExpenseEntry(t *testing.T) {
	rate := d("20")
	e, err := ExpenseEntry(ExpenseInput{SourceID: "expense:x", Date: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC),
		Total: d("1200"), Tax: d("200"), TaxRate: &rate, Deductible: true, ExpenseAccount: "770.02", VATAccount: "191",
		PaymentAccount: "100.01", VendorName: "Opet", ReceiptNumber: "0042", CategoryName: "Akaryakıt", Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"770.02 D1000.00 C0.00", "191.01 D200.00 C0.00", "100.01 D0.00 C1200.00"}
	if len(e.Lines) != 3 || !e.Balanced() {
		t.Fatalf("lines %v", e.Lines)
	}
	for i, w := range want {
		if e.Lines[i].String() != w {
			t.Errorf("line %d = %s, want %s", i, e.Lines[i], w)
		}
	}
	// Not deductible: VAT stays in the expense.
	e, _ = ExpenseEntry(ExpenseInput{Total: d("118"), Tax: d("18"), ExpenseAccount: "689", PaymentAccount: "100"})
	if len(e.Lines) != 2 || e.Lines[0].String() != "689 D118.00 C0.00" || !e.Balanced() {
		t.Fatalf("kkeg %v", e.Lines)
	}
	if _, err := ExpenseEntry(ExpenseInput{Total: decimal.Zero}); err == nil {
		t.Fatal("zero amount accepted")
	}
}

func TestPaymentMain(t *testing.T) {
	for m, want := range map[string]string{"": "100", "cash": "100", "bank_transfer": "102", "credit_card": "309", "credit": "320"} {
		if got, _ := PaymentMain(m); got != want {
			t.Errorf("%q = %s want %s", m, got, want)
		}
	}
}
