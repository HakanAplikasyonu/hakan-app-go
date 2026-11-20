package matching

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestBalanceFindings(t *testing.T) {
	d := decimal.RequireFromString
	ok := Balance{Account: "Ziraat TL", Currency: "TRY", Opening: d("1000"), In: d("500"), Out: d("200"), Closing: d("1300"),
		HasOpening: true, HasClosing: true}
	if got := BalanceFindings([]Balance{ok}); got != nil {
		t.Fatalf("balanced statement raised %v", got)
	}
	// Excel / PDF statement without balances: no check, no warning.
	partial := Balance{Account: "Excel", Currency: "TRY", In: d("10"), HasClosing: true, Closing: d("5")}
	if got := BalanceFindings([]Balance{partial}); got != nil {
		t.Fatalf("statement without opening raised %v", got)
	}
	// A missing 250,00 outflow: closing is 250 below the expected balance.
	bad := ok
	bad.Account, bad.IBAN, bad.Closing = "Garanti TL", "TR12", d("1050")
	got := BalanceFindings([]Balance{ok, bad, partial})
	if len(got) != 1 || got[0].Code != CodeBalanceMismatch {
		t.Fatalf("want one %s, got %v", CodeBalanceMismatch, got)
	}
	accounts, _ := got[0].Params["accounts"].([]balanceAccount)
	if len(accounts) != 1 || accounts[0].Account != "Garanti TL" || accounts[0].Difference != "-250.00" ||
		accounts[0].Expected != "1300.00" || accounts[0].Closing != "1050.00" {
		t.Fatalf("params = %+v", accounts)
	}
}
