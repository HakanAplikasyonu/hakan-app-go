package usecase

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestLiveAccounts(t *testing.T) {
	d := decimal.RequireFromString
	got := LiveAccounts([]LiveTotal{
		{Code: "320 01", Credit: d("118")},
		{Code: "153.10", Debit: d("50")},
		{Code: "153.2", Debit: d("100")},
		{Code: "191", Debit: d("18")},
		{Code: " ", Debit: d("1")},
	})
	want := []struct {
		code    string
		level   int
		leaf    bool
		balance string
	}{
		{"153", 1, false, "150.00"},
		{"153.2", 2, true, "100.00"},
		{"153.10", 2, true, "50.00"},
		{"191", 1, true, "18.00"},
		{"320", 1, false, "-118.00"},
		{"320.01", 2, true, "-118.00"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d accounts, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Code != w.code || g.Level != w.level || g.IsLeaf != w.leaf || g.Balance.StringFixed(2) != w.balance {
			t.Errorf("%d: got %s level %d leaf %v balance %s, want %+v", i, g.Code, g.Level, g.IsLeaf, g.Balance.StringFixed(2), w)
		}
	}
	if got[0].MainCode != "153" || got[5].MainCode != "320" {
		t.Errorf("main codes: %s %s", got[0].MainCode, got[5].MainCode)
	}
}

func TestLiveAccountsPostedParent(t *testing.T) {
	d := decimal.RequireFromString
	got := LiveAccounts([]LiveTotal{
		{Code: "120", Debit: d("10")},
		{Code: "120.01", Debit: d("5")},
	})
	if len(got) != 2 || got[0].Code != "120" || got[0].IsLeaf || got[0].Debit.StringFixed(2) != "15.00" || !got[1].IsLeaf {
		t.Fatalf("got %+v", got)
	}
}

func TestCodeLess(t *testing.T) {
	cases := []struct {
		a, b string
		less bool
	}{
		{"120.2", "120.10", true},
		{"120.10", "120.2", false},
		{"120", "120.01", true},
		{"153", "191", true},
		{"120.A", "120.B", true},
	}
	for _, c := range cases {
		if got := codeLess(c.a, c.b); got != c.less {
			t.Errorf("codeLess(%q, %q) = %v, want %v", c.a, c.b, got, c.less)
		}
	}
}
