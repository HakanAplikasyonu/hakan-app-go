package model

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{"TRL": "TRY", " tl ": "TRY", "YTL": "TRY", "usd": "USD", "CHF": "CHF"} {
		if got := NormalizeCurrency(in); got != want {
			t.Errorf("currency %q = %q", in, got)
		}
	}
	if v, tc, ok := ClassifyTaxID("128 833 1521"); !ok || v != "1288331521" || tc != "" {
		t.Errorf("vkn: %q %q %v", v, tc, ok)
	}
	if v, tc, ok := ClassifyTaxID("123-456-789-01"); !ok || v != "" || tc != "12345678901" {
		t.Errorf("tckn: %q %q %v", v, tc, ok)
	}
	if _, _, ok := ClassifyTaxID("12345"); ok {
		t.Error("short id accepted")
	}
	if got := NormalizeInvoiceNumber(" abc-2026 000000001 "); got != "ABC2026000000001" {
		t.Errorf("invoice no = %q", got)
	}
	if got := NormalizePercent(decimal.RequireFromString("0.20")); !got.Equal(decimal.NewFromInt(20)) {
		t.Errorf("percent = %s", got)
	}
	if got := NearestVATRate(decimal.RequireFromString("19.6")); !got.Equal(decimal.NewFromInt(20)) {
		t.Errorf("nearest = %s", got)
	}
}
