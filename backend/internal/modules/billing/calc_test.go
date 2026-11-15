package billing

import (
	"testing"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestCompute(t *testing.T) {
	p := Plan{Name: "Standart", BaseFee: d("1500"), PerTaxpayerFee: d("75"), IncludedTaxpayers: 10, VATRate: d("20")}
	a := Compute(p, 23)
	if a.BillableTaxpayers != 13 || a.TaxpayerFeeTotal.String() != "975" || a.Subtotal.String() != "2475" ||
		a.VATAmount.String() != "495" || a.Total.String() != "2970" || len(a.Lines) != 2 || a.Lines[1].Quantity != 13 {
		t.Fatalf("%+v", a)
	}
	if b := Compute(p, 4); b.BillableTaxpayers != 0 || b.Total.String() != "1800" {
		t.Fatalf("under the included count: %+v", b)
	}
	if z := Compute(Plan{Name: "Standart", VATRate: d("20")}, 7); !z.Total.IsZero() || z.ActiveTaxpayers != 7 {
		t.Fatalf("unpriced plan: %+v", z)
	}
	// Kuruş rounding on VAT.
	if r := Compute(Plan{BaseFee: d("99.99"), VATRate: d("20")}, 0); r.VATAmount.String() != "20" || r.Total.String() != "119.99" {
		t.Fatalf("rounding: %+v", r)
	}
}

func TestPeriodBounds(t *testing.T) {
	s, e, err := PeriodBounds("2026-12")
	if err != nil || s.Format("2006-01-02") != "2026-12-01" || e.Format("2006-01-02") != "2027-01-01" {
		t.Fatalf("%v %v %v", s, e, err)
	}
	if _, _, err := PeriodBounds("2026-13"); err == nil {
		t.Fatal("bad period accepted")
	}
}

func d2(s string) decimal.Decimal { return decimal.RequireFromString(s) }
