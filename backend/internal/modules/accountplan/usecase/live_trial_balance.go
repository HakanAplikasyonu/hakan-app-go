package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// LiveTotal is one account code's debit / credit sum from the engagement's
// own journal entries (#669 E).
type LiveTotal struct {
	Code          string
	Debit, Credit decimal.Decimal
}

// Compare modes of LiveTrialBalance.
const (
	// CompareLuca compares with the uploaded Luca trial balance of the same
	// period: change = canlı bakiye − Luca bakiyesi.
	CompareLuca = "luca"
)

// LiveTrialBalanceReport is the trial balance built from the office's own
// journal entries: cumulative from January of the period's year to the
// period (#669 E).
type LiveTrialBalanceReport struct {
	Period        string `json:"period"`
	FromPeriod    string `json:"from_period"`
	IncludeDrafts bool   `json:"include_drafts"`
	// ComparedTo is the uploaded Luca trial balance the lines are compared
	// with (compare=luca); null otherwise or when that period has none.
	ComparedTo *TrialBalance       `json:"compared_to"`
	Lines      []TrialBalanceLine  `json:"lines"`
	Totals     LiveTrialBalanceSum `json:"totals"`
}

// LiveTrialBalanceSum is the total of every posted amount.
type LiveTrialBalanceSum struct {
	Debit  string `json:"debit"`
	Credit string `json:"credit"`
}

// LiveTrialBalance builds the period's trial balance from the engagement's
// journal entries: approved and exported ones (drafts too when asked),
// January to period. Each posted code is a leaf; its parents ("120",
// "120.01") carry the sums of their children. Names come from the active
// Luca plan.
func (s *Service) LiveTrialBalance(ctx context.Context, sub Subject, period string, includeDrafts bool, compare string, maxLevel int) (LiveTrialBalanceReport, error) {
	if !ValidPeriod(period) {
		return LiveTrialBalanceReport{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	if maxLevel < 0 || maxLevel > 10 {
		return LiveTrialBalanceReport{}, fmt.Errorf("%w: level", ErrInvalid)
	}
	if compare != "" && compare != CompareNone && compare != CompareLuca {
		return LiveTrialBalanceReport{}, fmt.Errorf("%w: compare must be none or luca", ErrInvalid)
	}
	from := period[:4] + "-01"
	totals, err := s.store.LiveTotals(ctx, sub, from, period, includeDrafts)
	if err != nil {
		return LiveTrialBalanceReport{}, err
	}
	accounts := LiveAccounts(totals)
	codes := make([]string, 0, len(accounts))
	for _, a := range accounts {
		codes = append(codes, a.Code)
	}
	names, err := s.store.AccountNames(ctx, sub, DefaultTarget, codes)
	if err != nil {
		return LiveTrialBalanceReport{}, err
	}
	// Totals are every posted amount (a line on a parent code like "120"
	// next to "120.01" counts too), so they match the journal's own sums.
	debit, credit := decimal.Zero, decimal.Zero
	for _, t := range totals {
		if NormalizeCode(t.Code) != "" {
			debit, credit = debit.Add(t.Debit), credit.Add(t.Credit)
		}
	}
	sum := LiveTrialBalanceSum{Debit: debit.StringFixed(2), Credit: credit.StringFixed(2)}
	kept := accounts[:0]
	for _, a := range accounts {
		a.Name = names[a.Code]
		if maxLevel == 0 || a.Level <= maxLevel {
			kept = append(kept, a)
		}
	}

	out := LiveTrialBalanceReport{Period: period, FromPeriod: from, IncludeDrafts: includeDrafts, Totals: sum}
	var before map[string]decimal.Decimal
	if compare == CompareLuca {
		all, err := s.store.TrialBalances(ctx, sub, MaxTrialBalances)
		if err != nil {
			return LiveTrialBalanceReport{}, err
		}
		for i := range all { // newest upload first: the latest of the period
			if all[i].Period != nil && *all[i].Period == period {
				tb := all[i]
				out.ComparedTo = &tb
				break
			}
		}
		if out.ComparedTo != nil {
			lines, err := s.store.TrialBalanceLines(ctx, sub, out.ComparedTo.ID, maxLevel)
			if err != nil {
				return LiveTrialBalanceReport{}, err
			}
			before = make(map[string]decimal.Decimal, len(lines))
			for _, l := range lines {
				before[l.Code] = l.Balance
			}
		}
	}
	out.Lines = compareLines(kept, before)
	return out, nil
}

// LiveAccounts turns per-code sums into a trial balance tree: every code
// is normalized ("120 01" → "120.01"), its parents are added with the
// children's sums, and the result is sorted by code. Balance is debit −
// credit (positive: borç bakiyesi).
func LiveAccounts(totals []LiveTotal) []Account {
	byCode := map[string]*Account{}
	posted := map[string]bool{}
	add := func(code string, debit, credit decimal.Decimal) {
		a, ok := byCode[code]
		if !ok {
			parts := strings.Split(code, ".")
			a = &Account{Code: code, MainCode: parts[0], Level: len(parts)}
			byCode[code] = a
		}
		a.Debit = a.Debit.Add(debit)
		a.Credit = a.Credit.Add(credit)
	}
	for _, t := range totals {
		code := NormalizeCode(t.Code)
		if code == "" {
			continue
		}
		posted[code] = true
		parts := strings.Split(code, ".")
		for i := 1; i <= len(parts); i++ {
			add(strings.Join(parts[:i], "."), t.Debit, t.Credit)
		}
	}
	out := make([]Account, 0, len(byCode))
	for code, a := range byCode {
		// A posted code with children (a line written to "120" and another
		// to "120.01") is not a leaf: its own amounts are in the parent sum.
		a.IsLeaf = posted[code] && !hasChild(byCode, code)
		a.Balance = a.Debit.Sub(a.Credit)
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return codeLess(out[i].Code, out[j].Code) })
	return out
}

func hasChild(byCode map[string]*Account, code string) bool {
	prefix := code + "."
	for c := range byCode {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// codeLess orders codes segment by segment ("120.2" before "120.10" when
// both are numeric, else text order).
func codeLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] == pb[i] {
			continue
		}
		if len(pa[i]) != len(pb[i]) && isDigits(pa[i]) && isDigits(pb[i]) {
			return len(pa[i]) < len(pb[i])
		}
		return pa[i] < pb[i]
	}
	return len(pa) < len(pb)
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
