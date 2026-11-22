package foreigninvoices

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type wantLine struct{ account, debit, credit string }

func checkLines(t *testing.T, p JournalPlan, want []wantLine) {
	t.Helper()
	if len(p.Lines) != len(want) {
		t.Fatalf("lines = %d, want %d: %+v", len(p.Lines), len(want), p.Lines)
	}
	debit, credit := decimal.Zero, decimal.Zero
	for i, w := range want {
		l := p.Lines[i]
		if l.Account != w.account || l.Debit.StringFixed(2) != w.debit || l.Credit.StringFixed(2) != w.credit {
			t.Errorf("line %d = %s %s/%s, want %s %s/%s", i+1, l.Account, l.Debit.StringFixed(2), l.Credit.StringFixed(2), w.account, w.debit, w.credit)
		}
		if l.SourceKind != SourceKind || l.SourceID == "" {
			t.Errorf("line %d source = %q/%q", i+1, l.SourceKind, l.SourceID)
		}
		debit, credit = debit.Add(l.Debit), credit.Add(l.Credit)
	}
	if !debit.Equal(credit) {
		t.Fatalf("unbalanced: debit %s credit %s", debit, credit)
	}
}

// 100 USD net, kur 34,50, lisans %15 brütleştirme: brüt TL 4.058,93,
// KDV2 811,79, stopaj 608,84, satıcıya 3.450,00; 0,09 yuvarlama farkı 679.
func TestBuildJournalLicenceGrossUp(t *testing.T) {
	date := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	p, err := BuildJournal(JournalInvoice{
		UUID: uuid.New(), Period: "2026-10", Vendor: "JetBrains s.r.o.", InvoiceNo: "INV-1", InvoiceDate: &date,
		Currency: "USD", ExpenseAccount: "770",
		Terms: Terms{Amount: d("100"), Treatment: TreatmentKDV2Stopaj, StopajRate: d("15"), GrossUp: true, VATRate: d("20"), Rate: ptr("34.5")},
	}, Accounts{})
	if err != nil {
		t.Fatal(err)
	}
	checkLines(t, p, []wantLine{
		{"770", "4058.93", "0.00"},
		{"191", "811.79", "0.00"},
		{"360", "0.00", "811.79"},
		{"360", "0.00", "608.84"},
		{"320", "0.00", "3450.00"},
		{"679", "0.00", "0.09"},
	})
	if p.Rounding.StringFixed(2) != "0.09" || !p.Date.Equal(date) {
		t.Fatalf("rounding %s date %s", p.Rounding, p.Date)
	}
	if l := p.Lines[0]; l.Currency != "USD" || l.ForeignAmount == nil || l.ForeignAmount.StringFixed(2) != "117.65" {
		t.Fatalf("currency details = %+v", l)
	}
}

// Reklam yalnız KDV2, ofis hesapları: 760 / 191.01 / 360.02 / 102.
func TestBuildJournalAdvertisingOfficeAccounts(t *testing.T) {
	p, err := BuildJournal(JournalInvoice{
		UUID: uuid.New(), Period: "2026-10", Vendor: "Google Ireland", Currency: "USD", ExpenseAccount: DefaultExpenseAccount(KindAdvertising),
		Terms: Terms{Amount: d("250"), Treatment: TreatmentKDV2, VATRate: d("20"), Rate: ptr("34.25")},
	}, Accounts{KDV2: "360.02", DeductibleVAT: "191.01", Counter: "102"})
	if err != nil {
		t.Fatal(err)
	}
	checkLines(t, p, []wantLine{
		{"760", "8562.50", "0.00"},
		{"191.01", "1712.50", "0.00"},
		{"360.02", "0.00", "1712.50"},
		{"102", "0.00", "8562.50"},
	})
	if want := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC); !p.Date.Equal(want) {
		t.Fatalf("date = %s, want the last day of the period", p.Date)
	}
}

// Danışmanlık brütleştirmesiz %20 stopaj: satıcıya brüt − stopaj, fark yok.
func TestBuildJournalConsultingNoGrossUp(t *testing.T) {
	p, err := BuildJournal(JournalInvoice{
		UUID: uuid.New(), Period: "2026-10", Vendor: "Acme GmbH", Currency: "EUR", ExpenseAccount: "770",
		Terms: Terms{Amount: d("1000"), Treatment: TreatmentKDV2Stopaj, StopajRate: d("20"), VATRate: d("20"), Rate: ptr("37.123456")},
	}, Accounts{})
	if err != nil {
		t.Fatal(err)
	}
	checkLines(t, p, []wantLine{
		{"770", "37123.46", "0.00"},
		{"191", "7424.69", "0.00"},
		{"360", "0.00", "7424.69"},
		{"360", "0.00", "7424.69"},
		{"320", "0.00", "29698.77"},
	})
	if !p.Rounding.IsZero() {
		t.Fatalf("rounding = %s", p.Rounding)
	}
}

func TestBuildJournalMissingRate(t *testing.T) {
	if _, err := BuildJournal(JournalInvoice{UUID: uuid.New(), Period: "2026-10", Currency: "USD",
		Terms: Terms{Amount: d("10"), Treatment: TreatmentKDV2, VATRate: d("20")}}, Accounts{}); err == nil {
		t.Fatal("want an error without a kur")
	}
}

func TestValidAccount(t *testing.T) {
	for code, want := range map[string]bool{"770": true, "770.01": true, "360.02.001": true, "320-ABC": true, "77": false, "abc": false, "770.": false} {
		if got := ValidAccount(code); got != want {
			t.Errorf("ValidAccount(%q) = %v", code, got)
		}
	}
}
