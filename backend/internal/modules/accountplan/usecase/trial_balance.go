package usecase

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// MaxTrialBalances is how many trial balance imports the report looks at
// (ten years of monthly uploads).
const MaxTrialBalances = 120

// TrialBalance is one trial balance (mizan) import with amounts (#393).
type TrialBalance struct {
	ID           int64     `json:"-"`
	UUID         uuid.UUID `json:"uuid"`
	Period       *string   `json:"period"`
	FileName     string    `json:"file_name"`
	SourceFormat string    `json:"source_format"`
	AccountCount int       `json:"account_count"`
	// Totals over the leaf accounts (group accounts are not counted twice).
	DebitTotal  string    `json:"debit_total"`
	CreditTotal string    `json:"credit_total"`
	CreatedAt   time.Time `json:"created_at"`
}

// TrialBalanceLine is one account of a trial balance, with the balance of
// the compared trial balance when there is one.
type TrialBalanceLine struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	MainCode string `json:"main_code"`
	Level    int    `json:"level"`
	IsLeaf   bool   `json:"is_leaf"`
	Debit    string `json:"debit"`
	Credit   string `json:"credit"`
	// Balance is signed: positive = borç bakiyesi, negative = alacak bakiyesi.
	Balance string `json:"balance"`
	// PreviousBalance is the account's balance in the compared trial balance
	// (null when not compared or the account was not there).
	PreviousBalance *string `json:"previous_balance"`
	// Change = balance − previous balance (null when not compared; an
	// account missing from the compared file counts as 0).
	Change *string `json:"change"`
}

// TrialBalanceReport is a trial balance with an optional comparison.
type TrialBalanceReport struct {
	TrialBalance TrialBalance       `json:"trial_balance"`
	ComparedTo   *TrialBalance      `json:"compared_to"`
	Lines        []TrialBalanceLine `json:"lines"`
}

var periodRe = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

// ValidPeriod reports whether p is "YYYY-MM".
func ValidPeriod(p string) bool { return periodRe.MatchString(p) }

// TrialBalances lists the engagement's trial balances with amounts, newest
// upload first.
func (s *Service) TrialBalances(ctx context.Context, sub Subject) ([]TrialBalance, error) {
	out, err := s.store.TrialBalances(ctx, sub, MaxTrialBalances)
	if out == nil {
		out = []TrialBalance{}
	}
	return out, err
}

// Compare modes of TrialBalanceReport.
const (
	CompareNone     = "none"
	ComparePrevious = "previous"
)

// TrialBalanceReport returns one trial balance ("latest" or an import uuid)
// down to maxLevel (0: every level), compared with another one: compare is
// "previous" (default: the latest earlier period, else the upload before
// it), "none" or an import uuid.
func (s *Service) TrialBalanceReport(ctx context.Context, sub Subject, which, compare string, maxLevel int) (TrialBalanceReport, error) {
	if maxLevel < 0 || maxLevel > 10 {
		return TrialBalanceReport{}, fmt.Errorf("%w: level", ErrInvalid)
	}
	all, err := s.store.TrialBalances(ctx, sub, MaxTrialBalances)
	if err != nil {
		return TrialBalanceReport{}, err
	}
	cur, err := pickTrialBalance(all, which)
	if err != nil {
		return TrialBalanceReport{}, err
	}
	var prev *TrialBalance
	switch compare {
	case CompareNone:
	case "", ComparePrevious:
		prev = previousTrialBalance(all, cur)
	default:
		p, err := pickTrialBalance(all, compare)
		if err != nil {
			return TrialBalanceReport{}, err
		}
		if p.ID != cur.ID {
			prev = &p
		}
	}
	lines, err := s.store.TrialBalanceLines(ctx, sub, cur.ID, maxLevel)
	if err != nil {
		return TrialBalanceReport{}, err
	}
	var before map[string]decimal.Decimal
	if prev != nil {
		pl, err := s.store.TrialBalanceLines(ctx, sub, prev.ID, maxLevel)
		if err != nil {
			return TrialBalanceReport{}, err
		}
		before = make(map[string]decimal.Decimal, len(pl))
		for _, l := range pl {
			before[l.Code] = l.Balance
		}
	}
	return TrialBalanceReport{TrialBalance: cur, ComparedTo: prev, Lines: compareLines(lines, before)}, nil
}

func pickTrialBalance(all []TrialBalance, which string) (TrialBalance, error) {
	if which == "" || which == "latest" {
		if len(all) == 0 {
			return TrialBalance{}, ErrNotFound
		}
		return all[0], nil
	}
	id, err := uuid.Parse(which)
	if err != nil {
		return TrialBalance{}, fmt.Errorf("%w: trial balance id", ErrInvalid)
	}
	for _, t := range all {
		if t.UUID == id {
			return t, nil
		}
	}
	return TrialBalance{}, ErrNotFound
}

// previousTrialBalance: with a known period, the latest trial balance of an
// earlier period (the newest upload of that period); otherwise the upload
// right before cur. all is newest upload first.
func previousTrialBalance(all []TrialBalance, cur TrialBalance) *TrialBalance {
	if cur.Period != nil {
		var best *TrialBalance
		for i := range all {
			t := &all[i]
			if t.Period == nil || *t.Period >= *cur.Period {
				continue
			}
			if best == nil || *t.Period > *best.Period {
				best = t
			}
		}
		if best != nil {
			b := *best
			return &b
		}
	}
	for i := range all {
		if all[i].ID < cur.ID {
			if cur.Period != nil && all[i].Period != nil && *all[i].Period == *cur.Period {
				continue // same period re-uploaded: not a comparison
			}
			b := all[i]
			return &b
		}
	}
	return nil
}

func compareLines(lines []Account, before map[string]decimal.Decimal) []TrialBalanceLine {
	out := make([]TrialBalanceLine, 0, len(lines))
	for _, l := range lines {
		row := TrialBalanceLine{
			Code: l.Code, Name: l.Name, MainCode: l.MainCode, Level: l.Level, IsLeaf: l.IsLeaf,
			Debit: l.Debit.StringFixed(2), Credit: l.Credit.StringFixed(2), Balance: l.Balance.StringFixed(2),
		}
		if before != nil {
			prev, ok := before[l.Code]
			if ok {
				p := prev.StringFixed(2)
				row.PreviousBalance = &p
			}
			c := l.Balance.Sub(prev).StringFixed(2)
			row.Change = &c
		}
		out = append(out, row)
	}
	return out
}
