package foreigninvoices

import (
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/exchangerates"
)

// TestKeepStoredRate (#780): a rate entered by hand always wins; an
// automatic TCMB rate is looked up again on every change.
func TestKeepStoredRate(t *testing.T) {
	cases := []struct {
		name               string
		patchSets, hasRate bool
		source             string
		want               bool
	}{
		{"manual rate stays on an unrelated change", false, true, exchangerates.SourceManual, true},
		{"older rate without a label stays", false, true, "", true},
		{"automatic rate is recomputed", false, true, exchangerates.SourceTCMB, false},
		{"missing rate is looked up", false, false, "", false},
		{"the patch sets the rate by hand", true, true, exchangerates.SourceTCMB, false},
		{"the patch clears a manual rate", true, true, exchangerates.SourceManual, false},
	}
	for _, c := range cases {
		if got := keepStoredRate(c.patchSets, c.hasRate, c.source); got != c.want {
			t.Errorf("%s: keep = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestNormalizeMarksManualRate: a rate in the input is the office's.
func TestNormalizeMarksManualRate(t *testing.T) {
	k, _ := DefaultOf(KindCloud)
	rate := "34,5"
	n, reason := normalize(Input{Period: "2026-11", Vendor: "AWS", Country: "US", Amount: "10", Currency: "USD",
		ServiceKind: KindCloud, ExchangeRate: &rate}, k)
	if reason != "" {
		t.Fatal(reason)
	}
	if n.RateSource != exchangerates.SourceManual || n.Rate == nil || n.Rate.String() != "34.5" {
		t.Fatalf("got %+v", n)
	}
	n, _ = normalize(Input{Period: "2026-11", Vendor: "X", Country: "TR", Amount: "10", Currency: "TRY", ServiceKind: KindCloud}, k)
	if n.RateSource != "" {
		t.Fatalf("TL has no rate label, got %q", n.RateSource)
	}
}
