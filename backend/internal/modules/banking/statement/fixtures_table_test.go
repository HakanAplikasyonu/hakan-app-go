package statement

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// #780: every file in testdata/banks is read line by line: date, signed
// amount, running balance, description and the detected bank.

type wantMove struct {
	date    string // yyyy-mm-dd
	amount  string // signed: + in, - out
	balance string
	desc    string // the description must contain this
}

// The twelve CSV fixtures carry the same five movements in each bank's layout.
var fixtureMoves = []wantMove{
	{"2026-10-01", "12500", "62500", "YILDIZ YAZILIM TEKNOLOJILERI A.S. FATURA 2026-118"},
	{"2026-10-02", "-3250.75", "59249.25", "KALEM KIRTASIYE LTD STI"},
	{"2026-10-03", "-8.5", "59240.75", "EFT UCRETI"},
	{"2026-10-03", "-0.43", "59240.32", "BSMV"},
	{"2026-10-05", "-18420", "40820.32", "SGK PRIM ODEMESI 2026/09"},
}

func checkMoves(t *testing.T, st Statement, bank string, want []wantMove) {
	t.Helper()
	if st.BankID != bank {
		t.Fatalf("bank %q, want %q", st.BankID, bank)
	}
	if len(st.Errors) != 0 {
		t.Fatalf("row errors %v", st.Errors)
	}
	if len(st.Lines) != len(want) {
		t.Fatalf("%d lines, want %d", len(st.Lines), len(want))
	}
	for i, w := range want {
		l := st.Lines[i]
		if d := l.TransactionDate.Format("2006-01-02"); d != w.date {
			t.Errorf("line %d date %s, want %s", i, d, w.date)
		}
		a := decimal.RequireFromString(l.Amount)
		if l.Direction == banking.DirOut {
			a = a.Neg()
		}
		if !a.Equal(decimal.RequireFromString(w.amount)) {
			t.Errorf("line %d amount %s (%s), want %s", i, l.Amount, l.Direction, w.amount)
		}
		if l.BalanceAfter == nil || !decimal.RequireFromString(*l.BalanceAfter).Equal(decimal.RequireFromString(w.balance)) {
			t.Errorf("line %d balance %v, want %s", i, l.BalanceAfter, w.balance)
		}
		if !strings.Contains(l.Description, w.desc) {
			t.Errorf("line %d description %q, want %q", i, l.Description, w.desc)
		}
	}
	if g := CheckRunning(st.Lines); g != nil {
		t.Fatalf("running balance: %s", g.Reason)
	}
}

func TestEveryTestdataFileMovements(t *testing.T) {
	cases := map[string]struct {
		bank, format string
		moves        []wantMove
	}{
		"ziraat_html.xls": {"ziraat", "excel", []wantMove{
			{"2026-10-01", "4800", "54800", "ORNEK GIDA LTD STI"},
			{"2026-10-02", "-12000", "42800", "KIRA ODEMESI EKIM"},
			{"2026-10-04", "-6.25", "42793.75", "EFT UCRETI"},
		}},
		"yapikredi_xml.xls": {"yapikredi", "excel", []wantMove{
			{"2026-10-01", "7500", "27500", "DENIZ OTOMOTIV A.S. & ORTAKLARI"},
			{"2026-10-02", "-1840.35", "25659.65", "ELEKTRIK FATURASI"},
			{"2026-10-03", "-659.65", "25000", "NAKIT CEKIM"},
		}},
	}
	for file, b := range bankFixtures {
		cases[file+".csv"] = struct {
			bank, format string
			moves        []wantMove
		}{b.bank, "csv", fixtureMoves}
	}
	entries, err := os.ReadDir(filepath.Join("testdata", "banks"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(cases) {
		t.Fatalf("%d files in testdata/banks, %d cases: add the new file here", len(entries), len(cases))
	}
	for _, e := range entries {
		c, ok := cases[e.Name()]
		if !ok {
			t.Fatalf("no case for %s", e.Name())
		}
		t.Run(e.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "banks", e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			sts, format, err := Parse(e.Name(), data)
			if err != nil {
				t.Fatal(err)
			}
			if format != c.format || len(sts) != 1 {
				t.Fatalf("format %s, %d statements", format, len(sts))
			}
			if sts[0].AccountIBAN == "" {
				t.Fatal("no IBAN read")
			}
			checkMoves(t, sts[0], c.bank, c.moves)
		})
	}
}
