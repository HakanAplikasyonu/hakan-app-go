package posting

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func rate(s string) *decimal.Decimal { r := d(s); return &r }

var plan = []engine.PlanAccount{
	{Code: "191", Name: "İNDİRİLECEK KDV"},
	{Code: "191.01", Name: "İNDİRİLECEK KDV %20", IsLeaf: true},
	{Code: "191.02", Name: "İNDİRİLECEK KDV %10", IsLeaf: true},
	{Code: "391", Name: "HESAPLANAN KDV"},
	{Code: "391.01", Name: "HESAPLANAN KDV %20", IsLeaf: true},
}

func purchase() Input {
	return Input{
		Invoice: Invoice{
			ID: 42, Number: "ABC2026000000123", Direction: "incoming", Type: "SATIS", Currency: "TRY", Status: "active",
			IssueDate: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), Counterpart: "Tedarikçi A.Ş.",
			TaxExclusive: d("1500"), Tax: d("250"), Payable: d("1750"),
		},
		Items: []Item{
			{LineNo: 1, Name: "Mal", Net: d("1000"), VAT: d("200"), Rate: rate("20"), Account: "153.01"},
			{LineNo: 2, Name: "Hizmet", Net: d("500"), VAT: d("50"), Rate: rate("10"), Account: "770.01"},
		},
		Cari: "320.01.005",
		Plan: plan,
	}
}

func sales() Input {
	return Input{
		Invoice: Invoice{
			ID: 43, Number: "SAT2026000000007", Direction: "outgoing", Type: "SATIS", Currency: "TRY", Status: "active",
			IssueDate: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), Counterpart: "Müşteri Ltd.",
			TaxExclusive: d("2000"), Tax: d("400"), Payable: d("2400"),
		},
		Items: []Item{
			{LineNo: 1, Net: d("1200"), VAT: d("240"), Rate: rate("20"), Account: "600.01"},
			{LineNo: 2, Net: d("800"), VAT: d("160"), Rate: rate("20"), Account: "600.01"},
		},
		Cari: "120.01.002",
		Plan: plan,
	}
}

type want struct {
	account       string
	debit, credit string
}

func check(t *testing.T, e Entry, ws []want) {
	t.Helper()
	if len(e.Lines) != len(ws) {
		t.Fatalf("lines = %+v", e.Lines)
	}
	for i, w := range ws {
		l := e.Lines[i]
		if l.Account != w.account || !l.Debit.Equal(d(w.debit)) || !l.Credit.Equal(d(w.credit)) {
			t.Fatalf("line %d = %s B%s A%s, want %+v", i+1, l.Account, l.Debit, l.Credit, w)
		}
	}
	if !e.Balanced() {
		t.Fatal("not balanced")
	}
}

func TestPurchaseEntry(t *testing.T) {
	e, err := Build(purchase())
	if err != nil {
		t.Fatal(err)
	}
	check(t, e, []want{
		{"153.01", "1000", "0"},
		{"770.01", "500", "0"},
		{"191.01", "200", "0"},
		{"191.02", "50", "0"},
		{"320.01.005", "0", "1750"},
	})
	if !e.Date.Equal(time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)) || e.DocumentNo != "ABC2026000000123" ||
		e.SourceID != "42" || e.DocumentType != "invoice_purchase" {
		t.Fatalf("entry = %+v", e)
	}
	if e.Description != "Alış faturası ABC2026000000123 (14.10.2026) Tedarikçi A.Ş." {
		t.Fatalf("description = %q", e.Description)
	}
}

func TestSalesEntryGroupsByAccount(t *testing.T) {
	e, err := Build(sales())
	if err != nil {
		t.Fatal(err)
	}
	check(t, e, []want{
		{"120.01.002", "2400", "0"},
		{"600.01", "0", "2000"},
		{"391.01", "0", "400"},
	})
	if e.DocumentType != "invoice_sales" {
		t.Fatalf("type = %s", e.DocumentType)
	}
}

// A late purchase (#640): the entry is dated the first day of the deduction
// month; the document date stays on the lines and in the description.
func TestLatePurchaseDate(t *testing.T) {
	in := purchase()
	in.Invoice.VATDeductionPeriod = "2026-12"
	e, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if !e.Date.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date = %s", e.Date)
	}
	if !e.DocumentDate.Equal(in.Invoice.IssueDate) {
		t.Fatalf("document date = %s", e.DocumentDate)
	}
	// Sales never move.
	s := sales()
	s.Invoice.VATDeductionPeriod = "2026-12"
	if got := EntryDate(s.Invoice); !got.Equal(s.Invoice.IssueDate) {
		t.Fatalf("sales date = %s", got)
	}
}

func TestFallbackAccounts(t *testing.T) {
	in := purchase()
	in.Cari = ""
	in.Plan = nil
	in.Items[1].VATAccount = "191.99"
	e, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	check(t, e, []want{
		{"153.01", "1000", "0"},
		{"770.01", "500", "0"},
		{"191", "200", "0"},
		{"191.99", "50", "0"},
		{"320", "0", "1750"},
	})
}

func TestRefusals(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Input)
		code   string
	}{
		{"cancelled", func(in *Input) { in.Invoice.Status = "cancelled" }, ReasonNotActive},
		{"usd", func(in *Input) { in.Invoice.Currency = "USD" }, ReasonForeignCurrency},
		{"iade", func(in *Input) { in.Invoice.Type = "IADE" }, ReasonReturn},
		{"tevkifat type", func(in *Input) { in.Invoice.Type = "TEVKIFAT" }, ReasonWithholding},
		{"istisna", func(in *Input) { in.Invoice.Type = "ISTISNA" }, ReasonExemption},
		{"ihrac kayitli", func(in *Input) { in.Invoice.Type = "IHRACKAYITLI" }, ReasonExemption},
		{"ozel matrah", func(in *Input) { in.Invoice.Type = "OZELMATRAH" }, ReasonUnsupportedType},
		{"header withholding", func(in *Input) { in.Invoice.Withholding = d("10") }, ReasonWithholding},
		{"otv", func(in *Input) { in.Items[0].Excise = d("5") }, ReasonSpecialTax},
		{"konaklama", func(in *Input) {
			in.Items[0].Taxes = []Tax{{Kind: "tax", TypeCode: "0059", Amount: d("20")}}
		}, ReasonSpecialTax},
		{"line exemption", func(in *Input) {
			in.Items[0].Taxes = []Tax{{Kind: "tax", TypeCode: "0015", ExemptionCode: "301"}}
		}, ReasonExemption},
		{"global exemption", func(in *Input) { in.Invoice.GlobalExemptionCode = "351" }, ReasonExemption},
		{"no items", func(in *Input) { in.Items = nil }, ReasonNoItems},
		{"no account", func(in *Input) { in.Items[1].Account = "" }, ReasonItemAccount},
		{"totals", func(in *Input) { in.Invoice.Payable = d("1760") }, ReasonTotalsMismatch},
		{"header allowance", func(in *Input) { in.Invoice.TaxExclusive = d("1400") }, ReasonTotalsMismatch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := purchase()
			c.mutate(&in)
			_, err := Build(in)
			var r *Refusal
			if !errors.As(err, &r) || r.Code != c.code {
				t.Fatalf("err = %v, want %s", err, c.code)
			}
			if c.code == ReasonItemAccount && r.LineNo != 2 {
				t.Fatalf("line = %d", r.LineNo)
			}
		})
	}
	// VAT on the line with the VAT type code is fine.
	in := purchase()
	in.Items[0].Taxes = []Tax{{Kind: "tax", TypeCode: "0015", Amount: d("200")}}
	if _, err := Build(in); err != nil {
		t.Fatal(err)
	}
}
