package posting

import (
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
)

var ratePlan = []engine.PlanAccount{
	{Code: "770.01", Name: "GENEL GİDERLER", IsLeaf: true},
	{Code: "191.01", Name: "İNDİRİLECEK KDV %20", IsLeaf: true},
	{Code: "191.02", Name: "İNDİRİLECEK KDV %10", IsLeaf: true},
	{Code: "100.01", Name: "MERKEZ KASA", IsLeaf: true},
}

// KDV oran kırılımı (#780): her oran kendi 191 alt hesabına; gider net,
// ödeme toplam.
func TestExpenseEntryVatRates(t *testing.T) {
	e, err := ExpenseEntry(ExpenseInput{
		SourceID: "expense:x", Date: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), Total: d("820"), Tax: d("120"),
		Deductible: true, ExpenseAccount: "770.01", VATAccount: "191", PaymentAccount: "100.01", Plan: ratePlan,
		VatLines: []VatRate{{Rate: d("20"), Tax: d("100")}, {Rate: d("10"), Tax: d("20")}},
	})
	if err != nil || !e.Balanced() {
		t.Fatalf("%v %+v", err, e)
	}
	got := []string{}
	for _, l := range e.Lines {
		got = append(got, l.String())
	}
	want := "770.01 D700.00 C0.00|191.01 D100.00 C0.00|191.02 D20.00 C0.00|100.01 D0.00 C820.00"
	if strings.Join(got, "|") != want {
		t.Fatalf("lines %s", strings.Join(got, "|"))
	}
}

// Kırılım yok: tek KDV satırı; birden çok oran alt hesabı varken oran
// seçilmez, ana 191 kalır ve satır işaretlenir.
func TestExpenseEntryNoBreakdown(t *testing.T) {
	e, err := ExpenseEntry(ExpenseInput{
		Date: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), Total: d("120"), Tax: d("20"), Deductible: true,
		ExpenseAccount: "770.01", VATAccount: "191", PaymentAccount: "100.01", Plan: ratePlan, NoBreakdownNote: "(kırılım yok)",
	})
	if err != nil || len(e.Lines) != 3 {
		t.Fatalf("%v %+v", err, e)
	}
	v := e.Lines[1]
	if v.Account != "191" || !v.Debit.Equal(d("20")) || !strings.Contains(v.Description, "(kırılım yok)") {
		t.Fatalf("vat line %+v", v)
	}
}
