package usecase

import (
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
)

func TestEstimateCost(t *testing.T) {
	u := provider.Usage{InputTokens: 1_000_000, OutputTokens: 100_000, CacheReadTokens: 1_000_000, CacheWriteTokens: 1_000_000}
	nano, ok := EstimateCostNanoUSD("claude-opus-5-5", u)
	if !ok {
		t.Fatal("price missing")
	}
	// 4 + 2 + 5 (1.25x4) + 0.2 = 11.2 USD
	if got := FormatUSD(nano); got != "11.200000" {
		t.Fatalf("cost = %s", got)
	}
	if _, ok := EstimateCostNanoUSD("qwen3-local", u); ok {
		t.Fatal("unknown model should have no price")
	}
	// Longest prefix wins: opus-5-5 is not priced as opus-5.
	a, _ := EstimateCostNanoUSD("claude-opus-5-5-20261001", provider.Usage{InputTokens: 1_000_000})
	b, _ := EstimateCostNanoUSD("claude-opus-5", provider.Usage{InputTokens: 1_000_000})
	if a == b {
		t.Fatalf("prefix ordering broken: %d == %d", a, b)
	}
}

func TestFormatUSD(t *testing.T) {
	cases := map[int64]string{0: "0.000000", 1499: "0.000001", 1500: "0.000002", 123_456_789: "0.123457", -2000: "-0.000002"}
	for in, want := range cases {
		if got := FormatUSD(in); got != want {
			t.Errorf("FormatUSD(%d) = %s, want %s", in, got, want)
		}
	}
}
