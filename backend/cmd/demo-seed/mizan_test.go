package main

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// TestSeptemberMizan (#456): the September trial balance balances and its
// 335 / 360 / 361 balances are the open September obligations the October
// bank statement pays.
func TestSeptemberMizan(t *testing.T) {
	var debit, credit decimal.Decimal
	bal := map[string]decimal.Decimal{}
	for code, a := range septemberPostings() {
		debit, credit = debit.Add(a.debit), credit.Add(a.credit)
		bal[code[:3]] = bal[code[:3]].Add(a.credit.Sub(a.debit))
	}
	if !debit.Equal(credit) {
		t.Fatalf("borç %s ≠ alacak %s", debit, credit)
	}
	dec := decimal.RequireFromString
	for main, want := range map[string]decimal.Decimal{
		"335": dec(sepPayrollNet),
		"360": dec(sepKDVPayable).Add(dec(sepMUHSGKPayable)),
		"361": dec(sepSGKPayable),
	} {
		if !bal[main].Equal(want) {
			t.Errorf("%s alacak bakiyesi = %s, want %s", main, bal[main], want)
		}
	}
}

// TestMizanCSVPostsOnLeaves: an amount on a code with sub-accounts moves to
// a leaf, so no parent carries an amount of its own.
func TestMizanCSVPostsOnLeaves(t *testing.T) {
	p := map[string]mizanAmt{"320.01": {debit: decimal.NewFromInt(10)}}
	csv := demoMizanCSV("t", p, []string{"320.01.007"})
	for _, want := range []string{"320;SATICILAR;10,00", "320.01;YURTİÇİ SATICILAR;10,00", "320.01.001;ÇEŞİTLİ SATICILAR;10,00", "320.01.007;HESAP 320.01.007;;"} {
		if !strings.Contains(csv, want) {
			t.Errorf("csv lacks %q:\n%s", want, csv)
		}
	}
}
