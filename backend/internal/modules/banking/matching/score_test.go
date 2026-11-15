package matching

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }
func on(day int) time.Time       { return time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC) }

var opt = Options{FeeTolerance: d("25")}

func TestScoreTable(t *testing.T) {
	due := on(12)
	cases := []struct {
		name  string
		m     Movement
		c     Candidate
		total int
		parts []string
	}{
		{"exact + vkn + same day", Movement{Date: on(10), Direction: banking.DirIn, Amount: d("12000"), TaxID: "9100000021"},
			Candidate{IssueDate: on(10), Open: d("12000"), TaxID: "9100000021"}, 80, []string{"amount_exact", "tax_id", "date"}},
		{"fee deducted incoming", Movement{Date: on(10), Direction: banking.DirIn, Amount: d("11995"), TaxID: "9100000021"},
			Candidate{IssueDate: on(8), Open: d("12000"), TaxID: "9100000021"}, 61, []string{"amount_less_fee", "tax_id", "date"}},
		{"fee rule never for outgoing", Movement{Date: on(10), Direction: banking.DirOut, Amount: d("11995")},
			Candidate{IssueDate: on(10), Open: d("12000")}, 10, []string{"date"}},
		{"iban with vkn counts once", Movement{Date: on(10), Direction: banking.DirOut, Amount: d("5"), TaxID: "9200000035", IBAN: "TR320010009999901234567890"},
			Candidate{IssueDate: on(30), Open: d("100"), TaxID: "9200000035", IBANs: []string{"TR32 0010 0099 9990 1234 5678 90"}}, 30, []string{"tax_id", "iban"}},
		{"iban alone", Movement{Date: on(10), Direction: banking.DirOut, Amount: d("5"), IBAN: "TR320010009999901234567890"},
			Candidate{IssueDate: on(30), Open: d("100"), IBANs: []string{"TR320010009999901234567890"}}, 30, []string{"iban"}},
		{"invoice number in description", Movement{Date: on(20), Direction: banking.DirIn, Amount: d("1"), Description: "fatura ATL2026000000101 odemesi"},
			Candidate{Number: "ATL2026000000101", IssueDate: on(1), Open: d("500")}, 30, []string{"invoice_number"}},
		{"due date beats issue date", Movement{Date: on(13), Direction: banking.DirOut, Amount: d("1")},
			Candidate{IssueDate: on(1), DueDate: &due, Open: d("9")}, 8, []string{"date"}},
		{"name similarity", Movement{Date: on(1), Direction: banking.DirIn, Amount: d("1"), Name: "YILDIZ MARKET LTD STI"},
			Candidate{IssueDate: on(30), Open: d("900"), Name: "Yıldız Market Ltd. Şti."}, 10, []string{"name"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := ScoreCandidate(tc.m, tc.c, opt)
			if s.Total != tc.total {
				t.Errorf("total %d, want %d (%v)", s.Total, tc.total, s.Parts)
			}
			for _, p := range tc.parts {
				if _, ok := s.Parts[p]; !ok {
					t.Errorf("missing part %s in %v", p, s.Parts)
				}
			}
		})
	}
}

func TestDecide(t *testing.T) {
	m := Movement{Date: on(10), Direction: banking.DirIn, Amount: d("12000"), TaxID: "9100000021"}
	one := Candidate{ID: 1, IssueDate: on(10), Open: d("12000"), TaxID: "9100000021"}
	if dd := Decide(m, []Candidate{one}, opt); dd.Outcome != OutcomeAuto || dd.Picks[0].Candidate.ID != 1 {
		t.Fatalf("single strong → auto, got %s", dd.Outcome)
	}
	// Two equally strong candidates: never auto (second within 20).
	twin := one
	twin.ID = 2
	if dd := Decide(m, []Candidate{one, twin}, opt); dd.Outcome != OutcomeSuggest {
		t.Fatalf("two strong → suggest, got %s", dd.Outcome)
	}
	// 50–79 → suggest.
	weak := Candidate{ID: 3, IssueDate: on(8), Open: d("12000"), TaxID: "9100000021"}
	if dd := Decide(m, []Candidate{weak}, opt); dd.Outcome != OutcomeSuggest || dd.Picks[0].Total != 76 {
		t.Fatalf("76 → suggest, got %s %v", dd.Outcome, dd.Picks)
	}
	// < 50 → none.
	far := Candidate{ID: 4, IssueDate: on(1), Open: d("99")}
	if dd := Decide(m, []Candidate{far}, opt); dd.Outcome != OutcomeNone {
		t.Fatalf("weak → none, got %s", dd.Outcome)
	}
	// Fully paid invoices are not candidates.
	paid := one
	paid.Open = decimal.Zero
	if dd := Decide(m, []Candidate{paid}, opt); dd.Outcome != OutcomeNone {
		t.Fatalf("paid → none, got %s", dd.Outcome)
	}
}

func TestCombination(t *testing.T) {
	// 5 000 TL from Yıldız closes two invoices of 3 000 + 2 000 (21 §6.2 toplu ödeme).
	m := Movement{Date: on(25), Direction: banking.DirIn, Amount: d("5000"), TaxID: "9100000021"}
	cands := []Candidate{
		{ID: 1, Number: "A1", IssueDate: on(1), Open: d("3000"), TaxID: "9100000021"},
		{ID: 2, Number: "A2", IssueDate: on(2), Open: d("2000"), TaxID: "9100000021"},
		{ID: 3, Number: "A3", IssueDate: on(3), Open: d("4500"), TaxID: "9100000021"},
		{ID: 4, Number: "B1", IssueDate: on(3), Open: d("5000"), TaxID: "9999999999"}, // another party, exact amount
	}
	dd := Decide(m, cands, opt)
	// B1 has the exact amount (40) but nothing else: 40 < 50, not a suggestion;
	// the combination of the movement's own counterparty wins.
	if dd.Outcome != OutcomeSuggest || !dd.Combination || len(dd.Picks) != 2 {
		t.Fatalf("combination: %s combo=%v picks=%d", dd.Outcome, dd.Combination, len(dd.Picks))
	}
	v := valueOf(m, dd.Picks, true)
	if v.Invoices[0].Amount != "3000.00" || v.Invoices[1].Amount != "2000.00" {
		t.Fatalf("amounts: %+v", v.Invoices)
	}
}

func TestInvoiceNumbers(t *testing.T) {
	got := InvoiceNumbers("EFT ATL2026000000101 ve atl2026000000102 / GIB20260000000012")
	if len(got) != 2 || got[0] != "ATL2026000000101" || got[1] != "ATL2026000000102" {
		t.Fatalf("%v", got)
	}
}

func TestControls(t *testing.T) {
	rows := []Row{
		{UUID: "c1", Direction: banking.DirIn, Amount: d("900"), Currency: "TRY", Detail: banking.DetailCustomerCollection, Counterparty: "MAVI AJANS"},
		{UUID: "c2", Direction: banking.DirIn, Amount: d("100"), Currency: "TRY", Detail: banking.DetailCustomerCollection, Counterparty: "MAVI AJANS"},
		{UUID: "c3", Direction: banking.DirIn, Amount: d("12000"), Currency: "TRY", Detail: banking.DetailCustomerCollection, Matched: true},
		{UUID: "p1", Direction: banking.DirOut, Amount: d("7000"), Currency: "TRY", Detail: banking.DetailSupplierPayment, TaxNumber: "9300000049"},
		{UUID: "i1", Direction: banking.DirIn, Amount: d("150.25"), Currency: "TRY", Detail: banking.DetailInterestIncome},
		{UUID: "w1", Direction: banking.DirOut, Amount: d("22.54"), Currency: "TRY", Detail: banking.DetailWithholdingTax},
		{UUID: "s1", Direction: banking.DirOut, Amount: d("1500"), Currency: "TRY", Detail: banking.DetailSGKPayment},
		{UUID: "r1", Direction: banking.DirOut, Amount: d("22000"), Currency: "TRY", Detail: banking.DetailRentPayment, IdentityNumber: "10000000146"},
		{UUID: "r2", Direction: banking.DirOut, Amount: d("9000"), Currency: "TRY", Detail: banking.DetailRentPayment, TaxNumber: "9600000070"},
		{UUID: "n1", Direction: banking.DirOut, Amount: d("50"), Currency: "TRY", Detail: banking.DetailPrivate},
		{UUID: "u1", Direction: banking.DirOut, Amount: d("75"), Currency: "TRY", Detail: banking.DetailUnknown},
	}
	got := map[string]map[string]any{}
	for _, f := range Controls(rows) {
		got[f.Code] = f.Params
	}
	want := []string{CodeUninvoicedCollection, CodeUndocumentedPayment, CodeInterest, CodeTaxPaid, CodeRentWithholding, CodeNonBusiness, CodeUnclassified}
	if len(got) != len(want) {
		t.Fatalf("codes: %v", got)
	}
	for _, c := range want {
		if _, ok := got[c]; !ok {
			t.Errorf("missing %s", c)
		}
	}
	if got[CodeUninvoicedCollection]["total"] != "1000.00" {
		t.Errorf("uninvoiced total: %v", got[CodeUninvoicedCollection])
	}
	if gs := got[CodeUninvoicedCollection]["groups"].([]group); len(gs) != 1 || gs[0].Count != 2 {
		t.Errorf("groups by counterparty: %+v", gs)
	}
	if got[CodeInterest]["interest_income"] != "150.25" || got[CodeInterest]["withholding_tax"] != "22.54" {
		t.Errorf("interest: %v", got[CodeInterest])
	}
	// Rent to a company (VKN, no M05) carries no withholding duty.
	if tx := got[CodeRentWithholding]["transactions"].([]string); len(tx) != 1 || tx[0] != "r1" {
		t.Errorf("rent withholding: %v", tx)
	}
	if len(Controls(nil)) != 0 {
		t.Error("no rows, no findings")
	}
}
