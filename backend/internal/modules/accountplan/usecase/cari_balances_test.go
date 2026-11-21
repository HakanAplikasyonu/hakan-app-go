package usecase

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestCariGroups(t *testing.T) {
	d := decimal.RequireFromString
	posted := CariAccounts([]LiveTotal{
		{Code: "120.01", Debit: d("1000"), Credit: d("400")},
		{Code: "120 02", Debit: d("300")},
		{Code: "120.03", Credit: d("100")}, // customer paid in advance: reversed
		{Code: "120", Debit: d("50")},      // on the main account: unassigned
		{Code: "320.01", Credit: d("700")},
		{Code: "600.01", Credit: d("1250")}, // not a cari
	})
	groups, tot := cariGroups(posted, map[string]CariInfo{
		"120.01": {Name: "Atlas Gıda", TaxID: "1234567890"},
		"320":    {Name: "SATICILAR"},
	})
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	g := groups[0]
	// The group equals the 120 line of the trial balance.
	if g.MainCode != "120" || g.Name != "Alıcılar" || g.Debit != "1350.00" || g.Credit != "500.00" || g.Balance != "850.00" || g.Side != SideDebit {
		t.Fatalf("120 group = %+v", g)
	}
	if g.Count != 3 || g.Reverse != 1 || len(g.Accounts) != 4 {
		t.Fatalf("120 counts = %d / %d / %d", g.Count, g.Reverse, len(g.Accounts))
	}
	main, a1, a3 := g.Accounts[0], g.Accounts[1], g.Accounts[3]
	if !main.Unassigned || main.Name != "Alıcılar" || main.Reverse {
		t.Fatalf("unassigned = %+v", main)
	}
	if a1.Name != "Atlas Gıda" || a1.TaxID == nil || *a1.TaxID != "1234567890" || a1.Balance != "600.00" || a1.Share == nil || *a1.Share != "63.2" {
		t.Fatalf("120.01 = %+v share=%v", a1, a1.Share)
	}
	if a3.Code != "120.03" || !a3.Reverse || a3.Side != SideCredit || a3.Share != nil {
		t.Fatalf("120.03 = %+v", a3)
	}
	s := groups[1]
	if s.Name != "SATICILAR" || s.Balance != "-700.00" || s.Side != SideCredit || s.Accounts[0].Reverse || *s.Accounts[0].Share != "100.0" {
		t.Fatalf("320 group = %+v", s)
	}
	if tot.Debit != "1350.00" || tot.Credit != "1200.00" || tot.DebitBalance != "950.00" || tot.CreditBalance != "800.00" ||
		tot.Count != 4 || tot.Reverse != 1 || tot.Unassigned != 1 {
		t.Fatalf("totals = %+v", tot)
	}
}
