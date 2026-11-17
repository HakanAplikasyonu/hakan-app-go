package reconciliation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mailtpl"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestBABSResult(t *testing.T) {
	fa := func(a string, n int) FormAmount { return FormAmount{Amount: d(a), Count: n} }
	p := func(f FormAmount) *FormAmount { return &f }
	cases := []struct {
		computed FormAmount
		declared *FormAmount
		imported bool
		want     string
	}{
		{fa("4999.99", 3), nil, true, BABSNotRequired},
		{fa("4999.99", 3), p(fa("5200", 3)), true, BABSExtraInForm},
		{fa("5000", 1), nil, true, BABSMissingInForm},
		{fa("5000", 1), nil, false, BABSNoDeclaration},
		{fa("12500.80", 2), p(fa("12500", 2)), true, BABSOK},
		{fa("12500.80", 2), p(fa("12000", 2)), true, BABSAmountMismatch},
		{fa("12500.80", 2), p(fa("12500", 3)), true, BABSCountMismatch},
	}
	for i, c := range cases {
		if got := BABSResult(c.computed, c.declared, c.imported); got != c.want {
			t.Errorf("%d: %s, want %s", i, got, c.want)
		}
	}
	if !BABSIssue(BABSMissingInForm) || BABSIssue(BABSNoDeclaration) || BABSIssue(BABSOK) {
		t.Fatal("BABSIssue")
	}
}

func TestCompareCounterparties(t *testing.T) {
	rows := CompareCounterparties(CounterpartyInput{
		Invoices: map[string]CounterpartyInvoices{
			"1111111111":  {Name: "Alıcı A.Ş.", Balance: d("11800"), BS: FormAmount{Amount: d("10000"), Count: 1}},
			"2222222222":  {Name: "Satıcı Ltd.", Balance: d("-6000"), BA: FormAmount{Amount: d("5000"), Count: 2}},
			"33333333333": {Name: "Küçük Esnaf", Balance: d("-1200"), BA: FormAmount{Amount: d("1000"), Count: 1}},
		},
		Paid:         map[string]decimal.Decimal{"1111111111": d("-1800")},
		Accounts:     map[string][]string{"1111111111": {"120.01"}, "2222222222": {"320.05"}, "4444444444": {"320.09"}},
		AccountNames: map[string]string{"320.09": "ESKİ TEDARİKÇİ"},
		Ledger:       map[string]decimal.Decimal{"120.01": d("10000"), "320.05": d("-5500"), "320.09": d("-750")},
		DeclaredBA:   map[string]FormAmount{"2222222222": {Amount: d("5000"), Count: 2}},
		Tolerance:    d("1"),
	})
	if len(rows) != 4 {
		t.Fatalf("rows %+v", rows)
	}
	by := map[string]CounterpartyRow{}
	for _, r := range rows {
		by[r.TaxID] = r
	}
	a := by["1111111111"]
	if a.Result != CPMatched || a.Status != StatusResolved || a.SourceBalance.String() != "10000" || a.BSResult != BABSNoDeclaration || a.BAResult != BABSNotRequired {
		t.Fatalf("A %+v", a)
	}
	s := by["2222222222"]
	if s.Result != CPDifference || s.Difference.String() != "500" || s.BAResult != BABSOK || *s.DeclaredBACount != 2 {
		t.Fatalf("S %+v", s)
	}
	if k := by["33333333333"]; k.Result != CPNoLedgerAccount || k.AccountCode != nil || k.BAResult != BABSNotRequired {
		t.Fatalf("K %+v", k)
	}
	if e := by["4444444444"]; e.Name != "ESKİ TEDARİKÇİ" || e.Result != CPDifference || e.Difference.String() != "-750" {
		t.Fatalf("E %+v", e)
	}
	if rows[0].Name != "Alıcı A.Ş." {
		t.Fatalf("order %s", rows[0].Name)
	}
}

func TestLetterHelpers(t *testing.T) {
	id := uuid.MustParse("6f9f1c1e-3b1f-4c55-9a3e-1d2f3a4b5c6d")
	tok := LetterToken("gizli", id)
	if len(tok) != 64 || !validToken(tok) || tok == LetterToken("baska", id) || validToken(strings.ToUpper(tok)) || validToken("x") {
		t.Fatalf("token %q", tok)
	}
	if TokenDigest(tok) == tok || len(TokenDigest(tok)) != 64 {
		t.Fatal("digest")
	}
	if LetterLink("https://app.test/", tok) != "https://app.test/mutabakat/"+tok {
		t.Fatal("link")
	}
	snap := LetterSnapshot{TaxpayerTitle: "Atlas Gıda A.Ş.", TaxpayerTaxID: "4730074556", CounterpartyName: "Öz Şeker Ltd. Şti.",
		CounterpartyTaxID: "1111111111", AsOfDate: "2026-10-31", Balance: "-12345.60", Office: "Demo Ofis", Note: "Ekstre ektedir."}
	if got := BalanceSentence(snap); !strings.Contains(got, "31.10.2026") || !strings.Contains(got, "12.345,60 TL alacak") {
		t.Fatalf("sentence %q", got)
	}
	snap.Balance = "0"
	if !strings.Contains(BalanceSentence(snap), "bakiye vermemektedir") {
		t.Fatal("zero")
	}
	snap.Balance = "500"
	pdf, err := RenderLetterPDF(snap, "https://app.test/mutabakat/"+tok)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("pdf %v", err)
	}
	if LetterFileName(snap) != "mutabakat-2026-10-31-oz-seker-ltd-sti.pdf" {
		t.Fatal(LetterFileName(snap))
	}
	link := "https://app.test/mutabakat/" + tok
	m, err := LetterEmail(snap, link, mailtpl.New("https://app.test"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Subject, "Atlas Gıda") || !strings.Contains(strings.ReplaceAll(m.Body, "\n", " "), "500,00 TL borç") || !strings.Contains(m.Body, "\n"+link+"\n") {
		t.Fatalf("mail %+v", m)
	}
	if !strings.Contains(m.HTML, `href="`+link+`"`) || !strings.Contains(m.HTML, "500,00 TL borç") {
		t.Fatal("html part lacks the answer link or the balance")
	}
	for in, want := range map[string]string{"2026-10": "2026-10", "10/2026": "2026-10", "2026/9": "2026-09", "Ekim 2026": "2026-10", "x": "x"} {
		if got := normPeriod(in); got != want {
			t.Errorf("normPeriod(%q) = %q", in, got)
		}
	}
	if _, err := ValidEmail("a@b"); err == nil {
		t.Fatal("email without domain dot")
	}
	if e, err := ValidEmail(" muhasebe@ornek.com.tr "); err != nil || e != "muhasebe@ornek.com.tr" {
		t.Fatal(e, err)
	}
}
