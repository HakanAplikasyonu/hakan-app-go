package statement

import (
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

var tolerance = decimal.RequireFromString("0.01")

// Gap is a break in the running balance (20 §7.4, BNK_GAP).
type Gap struct {
	// Sequence of the line after which the balance does not add up; 0 for a
	// block-level opening/closing mismatch.
	AfterSequence int
	Expected      decimal.Decimal
	Actual        decimal.Decimal
	Reason        string
}

func signed(l Line) decimal.Decimal {
	a := decimal.RequireFromString(l.Amount)
	if l.Direction == banking.DirOut {
		return a.Neg()
	}
	return a
}

// CheckBlock: opening + Σ credits − Σ debits = closing (23 §3.4).
func CheckBlock(s Statement) *Gap {
	if s.Opening == nil || s.Closing == nil {
		return nil
	}
	sum := s.Opening.Amount
	for _, l := range s.Lines {
		sum = sum.Add(signed(l))
	}
	if sum.Sub(s.Closing.Amount).Abs().GreaterThan(tolerance) {
		return &Gap{Expected: sum, Actual: s.Closing.Amount,
			Reason: fmt.Sprintf("açılış + hareketler = %s, kapanış %s", sum.StringFixed(2), s.Closing.Amount.StringFixed(2))}
	}
	return nil
}

// CheckRunning walks lines that carry balance_after: previous balance ±
// amount must equal the next balance (±0,01). It returns the first break.
func CheckRunning(lines []Line) *Gap {
	var prev *decimal.Decimal
	for i, l := range lines {
		if l.BalanceAfter == nil {
			prev = nil
			continue
		}
		cur := decimal.RequireFromString(*l.BalanceAfter)
		if prev != nil {
			exp := prev.Add(signed(l))
			if exp.Sub(cur).Abs().GreaterThan(tolerance) {
				return &Gap{AfterSequence: i, Expected: exp, Actual: cur,
					Reason: fmt.Sprintf("satır %d: önceki bakiye ± tutar = %s, bakiye %s", i+1, exp.StringFixed(2), cur.StringFixed(2))}
			}
		}
		prev = &cur
	}
	return nil
}
