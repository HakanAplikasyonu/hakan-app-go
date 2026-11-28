package whatsapp

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestBadgeState(t *testing.T) {
	cases := []struct {
		target, receipt, approval, want string
	}{
		{TargetExpense, "draft", "pending", BadgePending},
		{TargetExpense, "draft", "approved", BadgeApproved},
		{TargetExpense, "approved", "", BadgeApproved},
		{TargetExpense, "draft", "rejected", BadgeRejected},
		{TargetExpense, "rejected", "", BadgeRejected},
		{TargetExpense, "failed", "", BadgeFailed},
		{TargetIncome, "duplicate", "", BadgeDuplicate},
		{TargetDocument, "", "", BadgeSaved},
		{TargetForeignInvoice, "", "", BadgeSaved},
	}
	for _, c := range cases {
		if got := BadgeState(c.target, c.receipt, c.approval); got != c.want {
			t.Errorf("BadgeState(%q,%q,%q) = %q, want %q", c.target, c.receipt, c.approval, got, c.want)
		}
	}
}

func TestParseShaList(t *testing.T) {
	a := strings.Repeat("a", 64)
	b := strings.Repeat("B", 64)
	got, err := ParseShaList(" " + a + "," + b + "," + a + ",")
	if err != nil || len(got) != 2 || got[1] != strings.ToLower(b) {
		t.Fatalf("got %v, %v", got, err)
	}
	many := make([]string, MaxStatusFiles+1)
	for i := range many {
		many[i] = fmt.Sprintf("%064x", i)
	}
	for _, bad := range []string{"", "xyz", strings.Repeat("g", 64), strings.Join(many, ",")} {
		if _, err := ParseShaList(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseShaList(%.20q…) err = %v, want ErrInvalid", bad, err)
		}
	}
}
