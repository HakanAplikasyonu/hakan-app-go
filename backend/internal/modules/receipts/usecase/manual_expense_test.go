package usecase

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func dd(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// Elle masraf (#780): kırılımlı girişte toplam Σ(matrah + KDV), tek oranlı
// girişte masrafın oranı o oran; kırılımsız giriş tutarla kabul edilir.
func TestCheckManualExpense(t *testing.T) {
	m, err := checkManualExpense(ManualExpenseInput{Date: "2026-10-12", VatLines: []VatLine{
		{Rate: dd("20"), Base: dd("500"), Tax: dd("100")},
		{Rate: dd("10"), Base: dd("200"), Tax: dd("20")},
		{Rate: dd("1"), Base: dd("0"), Tax: dd("0")}, // boş satır atlanır
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.lines) != 2 || !m.total.Equal(dd("820")) || !m.tax.Equal(dd("120")) || !m.base.Equal(dd("700")) || m.rate != nil {
		t.Fatalf("%+v", m)
	}
	if m.receiptType != "okc_receipt" {
		t.Fatalf("receipt type %q", m.receiptType)
	}
	m, err = checkManualExpense(ManualExpenseInput{Date: "2026-10-12", VatLines: []VatLine{{Rate: dd("20"), Base: dd("695"), Tax: dd("139")}}})
	if err != nil || m.rate == nil || !m.rate.Equal(dd("20")) || !m.total.Equal(dd("834")) {
		t.Fatalf("%+v %v", m, err)
	}
	a := dd("250")
	m, err = checkManualExpense(ManualExpenseInput{Date: "2026-10-12", Amount: &a})
	if err != nil || len(m.lines) != 0 || !m.total.Equal(a) || !m.tax.IsZero() {
		t.Fatalf("kırılım yok: %+v %v", m, err)
	}
	for name, in := range map[string]ManualExpenseInput{
		"no date":     {VatLines: []VatLine{{Rate: dd("20"), Base: dd("1"), Tax: dd("0.2")}}},
		"no amount":   {Date: "2026-10-12"},
		"twice":       {Date: "2026-10-12", VatLines: []VatLine{{Rate: dd("20"), Base: dd("1")}, {Rate: dd("20.00"), Base: dd("2")}}},
		"negative":    {Date: "2026-10-12", VatLines: []VatLine{{Rate: dd("20"), Base: dd("-1")}}},
		"rate":        {Date: "2026-10-12", VatLines: []VatLine{{Rate: dd("120"), Base: dd("1")}}},
		"vkn":         {Date: "2026-10-12", Amount: &a, VendorTaxNumber: "12ab"},
		"payment":     {Date: "2026-10-12", Amount: &a, PaymentMethod: "barter"},
		"receipt typ": {Date: "2026-10-12", Amount: &a, ReceiptType: "nope"},
	} {
		if _, err := checkManualExpense(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
