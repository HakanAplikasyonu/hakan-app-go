package matching

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/statement"
)

// #780: a statement uploaded as Excel / CSV reaches the matcher with what
// the file gives (no tax id column in most banks). The collection from
// YILDIZ YAZILIM must still be suggested for its invoice and not for another
// company's invoice of the same amount; the payment to KALEM KIRTASIYE
// likewise. Built as the service builds a Movement from the stored row.
func TestStatementFixturesSuggestInvoices(t *testing.T) {
	dir := filepath.Join("..", "statement", "testdata", "banks")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	sales := []Candidate{
		{ID: 1, Number: "YLD2026000000118", IssueDate: day(1), Open: d("12500"), Name: "Yıldız Yazılım Teknolojileri A.Ş."},
		{ID: 2, Number: "BSK2026000000007", IssueDate: day(1), Open: d("12500"), Name: "Başka Ticaret Ltd. Şti."},
	}
	purchases := []Candidate{
		{ID: 3, Number: "KLM2026000000044", IssueDate: day(1), Open: d("3250.75"), Name: "Kalem Kırtasiye Ltd. Şti."},
		{ID: 4, Number: "XYZ2026000000001", IssueDate: day(2), Open: d("3250.75"), Name: "Xyz Elektrik A.Ş."},
	}
	n := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".csv") {
			continue
		}
		n++
		t.Run(e.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			sts, _, err := statement.Parse(e.Name(), data)
			if err != nil {
				t.Fatal(err)
			}
			for i, c := range []struct {
				cands []Candidate
				want  int64
			}{{sales, 1}, {purchases, 3}} {
				l := sts[0].Lines[i]
				m := Movement{
					Date: l.TransactionDate, Direction: l.Direction, Amount: decimal.RequireFromString(l.Amount),
					IBAN: l.CounterpartyIBAN, Name: l.CounterpartyName, TaxID: l.CounterpartyTaxID, Description: l.Description,
				}
				dec := Decide(m, c.cands, opt)
				if dec.Outcome == OutcomeNone || len(dec.Picks) != 1 || dec.Picks[0].Candidate.ID != c.want {
					t.Fatalf("line %d (%s %s %q): outcome %s picks %+v ranked %+v", i, l.Direction, l.Amount, l.CounterpartyName, dec.Outcome, dec.Picks, dec.Ranked)
				}
				if _, ok := dec.Picks[0].Parts["name"]; !ok && m.TaxID == "" && m.IBAN == "" {
					t.Fatalf("line %d: suggested without a counterparty part %v", i, dec.Picks[0].Parts)
				}
			}
			// Bank fees and SGK carry no counterparty: nothing suggested.
			fee := sts[0].Lines[2]
			m := Movement{Date: fee.TransactionDate, Direction: fee.Direction, Amount: decimal.RequireFromString(fee.Amount),
				Name: fee.CounterpartyName, Description: fee.Description}
			if dec := Decide(m, []Candidate{{ID: 9, IssueDate: day(3), Open: d("8.50"), Name: "Başka"}}, opt); dec.Outcome != OutcomeNone {
				t.Fatalf("fee suggested: %+v", dec)
			}
		})
	}
	if n < 12 {
		t.Fatalf("%d CSV fixtures", n)
	}
}
