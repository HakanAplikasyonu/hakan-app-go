package posting

import (
	"strings"
	"testing"
	"time"
)

// Kaynak tutarlar dört ondalıklı gelebilir (NUMERIC(18,4)). Satır satır
// yuvarlamak fişi bir kuruş bozuyordu: 100,00 toplam, 8,335 KDV → 91,67 +
// 8,34 = 100,01. Önce toplam ve KDV kuruşa yuvarlanır, net kalandır.
// Uydurma tutarlar.
func TestExpenseEntryRoundsToKurus(t *testing.T) {
	day := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		in   ExpenseInput
		want string
	}{
		{"tek KDV satırı", ExpenseInput{Date: day, Total: d("100.0000"), Tax: d("8.3350"), Deductible: true,
			ExpenseAccount: "770.01", VATAccount: "191.01", PaymentAccount: "100.01"},
			"770.01 D91.66 C0.00|191.01 D8.34 C0.00|100.01 D0.00 C100.00"},
		{"oran kırılımı", ExpenseInput{Date: day, Total: d("150.0000"), Tax: d("12.7900"), Deductible: true,
			ExpenseAccount: "770.01", VATAccount: "191", PaymentAccount: "100.01", Plan: ratePlan,
			VatLines: []VatRate{{Rate: d("20"), Tax: d("8.3350")}, {Rate: d("10"), Tax: d("4.4550")}}},
			"770.01 D137.20 C0.00|191.01 D8.34 C0.00|191.02 D4.46 C0.00|100.01 D0.00 C150.00"},
		{"dört ondalıklı toplam", ExpenseInput{Date: day, Total: d("59.9950"), Tax: d("9.9992"), Deductible: true,
			ExpenseAccount: "770.01", VATAccount: "191.01", PaymentAccount: "100.01"},
			"770.01 D50.00 C0.00|191.01 D10.00 C0.00|100.01 D0.00 C60.00"},
	}
	for _, c := range cases {
		e, err := ExpenseEntry(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !e.Balanced() {
			t.Errorf("%s: unbalanced %v", c.name, e.Lines)
		}
		got := make([]string, len(e.Lines))
		for i, l := range e.Lines {
			got[i] = l.String()
		}
		if strings.Join(got, "|") != c.want {
			t.Errorf("%s: lines %s, want %s", c.name, strings.Join(got, "|"), c.want)
		}
	}
}

// Z raporu: matrah ve KDV dört ondalıklı; fark kuruş olarak en büyük
// matraha gider, yuvarlanmış satırlar dengede kalır. Uydurma tutarlar.
func TestZEntryRoundsToKurus(t *testing.T) {
	card := d("40.0050")
	e, err := ZEntry(ZInput{Date: day("2026-10-15"), ZNumber: "0042", Total: d("100.0000"), Card: &card, Lines: []ZVatLine{
		{Rate: d("20"), Base: d("45.4550"), Tax: d("4.5450")},
		{Rate: d("10"), Base: d("45.4550"), Tax: d("4.5450")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !e.Balanced() {
		t.Fatalf("unbalanced %v", e.Lines)
	}
	for _, l := range e.Lines {
		if !l.Debit.Equal(l.Debit.Round(2)) || !l.Credit.Equal(l.Credit.Round(2)) {
			t.Errorf("not kuruş: %v", l)
		}
	}
	if e.Lines[0].String() != "100 D59.99 C0.00" || e.Lines[1].String() != "108 D40.01 C0.00" {
		t.Errorf("payments %v", e.Lines[:2])
	}
}
