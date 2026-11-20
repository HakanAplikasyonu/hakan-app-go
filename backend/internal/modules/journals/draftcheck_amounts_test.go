package journals

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestValidLineAmounts(t *testing.T) {
	d := decimal.RequireFromString
	ok := []struct{ debit, credit string }{
		{"100", "0"},
		{"0", "1250.50"},
		{"0.01", "0"},
	}
	for _, c := range ok {
		if err := ValidLineAmounts(d(c.debit), d(c.credit)); err != nil {
			t.Errorf("%s / %s: %v", c.debit, c.credit, err)
		}
	}
	bad := []struct{ debit, credit string }{
		{"0", "0"},      // no side
		{"10", "10"},    // both sides
		{"-5", "0"},     // negative
		{"0", "-1"},     // negative credit
		{"10.005", "0"}, // more than two decimals
		{"0", "1.2345"}, // more than two decimals
	}
	for _, c := range bad {
		if err := ValidLineAmounts(d(c.debit), d(c.credit)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s / %s: want ErrInvalid, got %v", c.debit, c.credit, err)
		}
	}
}
