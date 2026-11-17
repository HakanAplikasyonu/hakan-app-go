package usecase

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func tb(id int64, period string) TrialBalance {
	t := TrialBalance{ID: id, UUID: uuid.New()}
	if period != "" {
		t.Period = &period
	}
	return t
}

func TestPreviousTrialBalance(t *testing.T) {
	t.Parallel()
	// Newest upload first: Ekim re-uploaded (4), Eylül uploaded late (3),
	// Ekim (2), Ağustos (1).
	all := []TrialBalance{tb(4, "2026-10"), tb(3, "2026-09"), tb(2, "2026-10"), tb(1, "2026-08")}
	if p := previousTrialBalance(all, all[0]); p == nil || p.ID != 3 {
		t.Fatalf("previous of Ekim = %+v, want Eylül (3)", p)
	}
	if p := previousTrialBalance(all, all[1]); p == nil || p.ID != 1 {
		t.Fatalf("previous of Eylül = %+v, want Ağustos (1)", p)
	}
	if p := previousTrialBalance(all, all[3]); p != nil {
		t.Fatalf("previous of the first = %+v", p)
	}
	// Without periods: the upload before it.
	np := []TrialBalance{tb(9, ""), tb(7, ""), tb(5, "")}
	if p := previousTrialBalance(np, np[0]); p == nil || p.ID != 7 {
		t.Fatalf("previous without period = %+v", p)
	}
}

func TestPickTrialBalance(t *testing.T) {
	t.Parallel()
	all := []TrialBalance{tb(2, "2026-10"), tb(1, "2026-09")}
	if got, err := pickTrialBalance(all, "latest"); err != nil || got.ID != 2 {
		t.Fatalf("latest = %+v %v", got, err)
	}
	if got, err := pickTrialBalance(all, all[1].UUID.String()); err != nil || got.ID != 1 {
		t.Fatalf("by uuid = %+v %v", got, err)
	}
	if _, err := pickTrialBalance(all, uuid.NewString()); err != ErrNotFound {
		t.Fatalf("unknown uuid: %v", err)
	}
	if _, err := pickTrialBalance(all, "x"); err == nil || err == ErrNotFound {
		t.Fatalf("bad uuid: %v", err)
	}
	if _, err := pickTrialBalance(nil, ""); err != ErrNotFound {
		t.Fatalf("none: %v", err)
	}
}

func TestCompareLines(t *testing.T) {
	t.Parallel()
	d := decimal.RequireFromString
	lines := []Account{
		{Code: "100", Level: 1, Balance: d("7150")},
		{Code: "320", Level: 1, Balance: d("-35500.5")},
	}
	out := compareLines(lines, map[string]decimal.Decimal{"100": d("5000")})
	if *out[0].PreviousBalance != "5000.00" || *out[0].Change != "2150.00" {
		t.Fatalf("100 = %+v", out[0])
	}
	if out[1].PreviousBalance != nil || *out[1].Change != "-35500.50" {
		t.Fatalf("320 (new account) = %+v", out[1])
	}
	if out := compareLines(lines, nil); out[0].Change != nil || out[0].PreviousBalance != nil {
		t.Fatalf("no comparison = %+v", out[0])
	}
}
