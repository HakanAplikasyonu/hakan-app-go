package matching

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

func TestFxDiff(t *testing.T) {
	d := decimal.RequireFromString
	cases := []struct {
		dir           banking.Direction
		amount, inv   string
		pay, want, ac string
	}{
		// Tahsilat, kur yükseldi: kâr (646).
		{banking.DirIn, "1199.88", "34.5678", "35", "518.59", FxGainAccount},
		// Tahsilat, kur düştü: zarar (656).
		{banking.DirIn, "1000", "35", "34.5", "-500.00", FxLossAccount},
		// Ödeme, kur yükseldi: zarar.
		{banking.DirOut, "1000", "35", "35.5", "-500.00", FxLossAccount},
		// Ödeme, kur düştü: kâr.
		{banking.DirOut, "1000", "35", "34.9", "100.00", FxGainAccount},
	}
	for _, c := range cases {
		got := FxDiff(c.dir, d(c.amount), d(c.inv), d(c.pay))
		if got.StringFixed(2) != c.want || FxAccount(got) != c.ac {
			t.Errorf("FxDiff(%v %s %s→%s) = %s %s, want %s %s", c.dir, c.amount, c.inv, c.pay, got.StringFixed(2), FxAccount(got), c.want, c.ac)
		}
	}
}
