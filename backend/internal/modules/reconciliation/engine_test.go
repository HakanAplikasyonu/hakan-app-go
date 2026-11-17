package reconciliation

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func leaf(code, debit, credit string) LedgerLine {
	return LedgerLine{Code: code, MainCode: code[:3], Level: strings.Count(code, ".") + 1, IsLeaf: true, Debit: d(debit), Credit: d(credit)}
}

func byMain(items []Item) map[string]Item {
	m := map[string]Item{}
	for _, it := range items {
		m[it.MainCode] = it
	}
	return m
}

func TestCompareMovementAndResults(t *testing.T) {
	prev := []LedgerLine{
		{Code: "102", MainCode: "102", Level: 1, Name: "BANKALAR"},
		leaf("102.01", "1000", "200"),
		leaf("600.01", "0", "5000"),
		leaf("500", "0", "10000"), // not covered
	}
	cur := []LedgerLine{
		{Code: "102", MainCode: "102", Level: 1, Name: "BANKALAR"},
		leaf("102.01", "3000", "700"), // movement 2000 / 500
		leaf("600.01", "0", "9000"),   // movement 0 / 4000
		leaf("500", "0", "10000"),
		leaf("335.01", "0", "800"), // no source
		leaf("770.01", "100", "0"), // matches source
	}
	in := Input{
		Current: cur, Previous: prev, Tolerance: d("1"),
		Sources: []SourceLine{
			{Account: "102.01", Kind: KindBank, Debit: d("2000"), Credit: d("500"), Lines: 3},
			{Account: "600.01", Kind: KindInvoice, Credit: d("3000"), Lines: 2},
			{Account: "600.02", Kind: KindZReport, Credit: d("500"), Lines: 1, Draft: true},
			{Account: "770.01", Kind: KindExpense, Debit: d("100.40"), Lines: 1},
			{Account: "153.01", Kind: KindInvoice, Debit: d("250"), Lines: 1},
		},
	}
	got := byMain(Compare(in))
	if _, ok := got["500"]; ok {
		t.Fatal("uncovered account without sources compared")
	}
	if it := got["102"]; it.Result != ResultMatched || !it.Difference.IsZero() || it.AccountName != "BANKALAR" || it.LedgerDebit.String() != "2000" {
		t.Fatalf("102 %+v", it)
	}
	// 600: ledger −4000, sources −3500 → −500, not the draft (−500 draft would explain +500).
	it := got["600"]
	if it.Result != ResultDifference || it.Difference.String() != "-500" || it.AccountName != "Yurt içi satışlar" {
		t.Fatalf("600 %+v", it)
	}
	if len(it.Sources) != 2 || it.Sources[0].Kind != KindInvoice || it.Sources[1].DraftNet.String() != "-500" {
		t.Fatalf("600 sources %+v", it.Sources)
	}
	if len(it.SubAccounts) != 2 || it.SubAccounts[1].Code != "600.02" || it.SubAccounts[1].LedgerNet.String() != "0" {
		t.Fatalf("600 subs %+v", it.SubAccounts)
	}
	if it.SuggestCode != SuggestTiming || !strings.Contains(it.Suggestion, "500,00 TL eksik") {
		t.Fatalf("600 suggestion %q %q", it.SuggestCode, it.Suggestion)
	}
	if it := got["335"]; it.Result != ResultMissingInSources || it.SuggestCode != SuggestMissingSource || !strings.Contains(it.Suggestion, "bordro") {
		t.Fatalf("335 %+v", it)
	}
	if it := got["770"]; it.Result != ResultMatched || it.Suggestion != "" {
		t.Fatalf("770 within tolerance %+v", it)
	}
	if it := got["153"]; it.Result != ResultMissingInLedger || it.SuggestCode != SuggestNotExported {
		t.Fatalf("153 %+v", it)
	}
}

func TestExplanations(t *testing.T) {
	cur := []LedgerLine{
		leaf("391.01", "0", "1200"),
		leaf("102.01", "5000", "0"),
		leaf("770.01", "345", "0"),
		leaf("120.01", "1003", "0"),
		leaf("191.01", "900", "0"),
	}
	in := Input{
		Current: cur, Previous: []LedgerLine{}, Tolerance: d("0.5"),
		Sources: []SourceLine{
			// 391: sources −1400 of which −200 draft → ledger lacks the draft.
			{Account: "391.01", Kind: KindInvoice, Credit: d("1200")},
			{Account: "391.01", Kind: KindZReport, Credit: d("200"), Draft: true},
			{Account: "102.01", Kind: KindBank, Debit: d("3000")},
			{Account: "120.01", Kind: KindInvoice, Debit: d("1000")},
			{Account: "191.01", Kind: KindInvoice, Debit: d("100")},
		},
		Unposted: []Unposted{
			{Kind: KindBank, ID: "1", Date: "2026-10-03", Label: "X", Amount: d("1500"), Effects: map[string]decimal.Decimal{"102": d("1500")}},
			{Kind: KindBank, ID: "2", Date: "2026-10-04", Label: "Y", Amount: d("500"), Effects: map[string]decimal.Decimal{"102": d("500")}},
			{Kind: KindExpense, ID: "e1", Date: "2026-10-05", Label: "Akaryakıt", Amount: d("414"), Effects: map[string]decimal.Decimal{"770": d("345"), "191": d("69")}},
		},
	}
	got := byMain(Compare(in))
	if it := got["391"]; it.SuggestCode != SuggestDraftNotExported || !strings.Contains(it.Suggestion, "200,00 TL") {
		t.Fatalf("391 %+v", it)
	}
	if it := got["102"]; it.SuggestCode != SuggestUnposted || it.UnpostedCount != 2 || !strings.Contains(it.Suggestion, "2 kaydın") {
		t.Fatalf("102 %+v", it)
	}
	// 770: no source at all, the one expense equals the difference.
	if it := got["770"]; it.Result != ResultMissingInSources || it.SuggestCode != SuggestUnposted || len(it.Unposted) != 1 || it.Unposted[0].Effect.String() != "345" {
		t.Fatalf("770 %+v", it)
	}
	if it := got["120"]; it.SuggestCode != SuggestRounding {
		t.Fatalf("120 %+v", it)
	}
	// 191: 800 off, one of the unposted (69) does not explain it.
	if it := got["191"]; it.SuggestCode != SuggestTiming {
		t.Fatalf("191 %+v", it)
	}
}

func TestCumulativeOpeningAndSingle(t *testing.T) {
	cur := []LedgerLine{leaf("102.01", "10000", "2000"), leaf("770.01", "750", "0")}
	in := Input{
		Current: cur, Tolerance: d("1"),
		Sources: []SourceLine{{Account: "102.01", Kind: KindBank, Debit: d("3000"), Credit: d("2000")}, {Account: "770.01", Kind: KindPayroll, Debit: d("500")}},
		Unposted: []Unposted{
			{Kind: KindExpense, ID: "a", Date: "2026-10-01", Label: "Kırtasiye", Amount: d("250"), Effects: map[string]decimal.Decimal{"770": d("250")}},
			{Kind: KindExpense, ID: "b", Date: "2026-10-02", Label: "Yemek", Amount: d("90"), Effects: map[string]decimal.Decimal{"770": d("90")}},
		},
	}
	got := byMain(Compare(in))
	if it := got["102"]; it.SuggestCode != SuggestOpeningBalance {
		t.Fatalf("102 cumulative %+v", it)
	}
	if it := got["770"]; it.SuggestCode != SuggestSingleUnposted || !strings.Contains(it.Suggestion, "Kırtasiye") {
		t.Fatalf("770 %+v", it)
	}
}

func TestHelpers(t *testing.T) {
	for in, want := range map[string]string{"102.01": "102", "600": "600", "7701": "770", "10": "", "AB1": "", "102-1": ""} {
		if got := MainCodeOf(in); got != want {
			t.Errorf("MainCodeOf(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"1234567.5": "1.234.567,50 TL", "-12.345": "-12,35 TL", "0": "0,00 TL", "999": "999,00 TL"} {
		if got := Money(d(in)); got != want {
			t.Errorf("Money(%s) = %q, want %q", in, got, want)
		}
	}
	if SourceKindOf("receipt", true) != KindZReport || SourceKindOf("invoice_item", false) != KindInvoice || SourceKindOf("", false) != KindManual {
		t.Fatal("SourceKindOf")
	}
	if !ValidDecision(StatusResolved) || ValidDecision(StatusInReview) {
		t.Fatal("ValidDecision")
	}
}
