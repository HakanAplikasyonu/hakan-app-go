package usecase

import (
	"fmt"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
)

// modelPrice is milli-USD per million tokens (so 0.20 USD/MTok = 200), which
// keeps the arithmetic in integers. Cache writes are billed at 1.25x input
// (5-minute TTL). Ported from otopoly-go usecase/pricing.go.
type modelPrice struct {
	prefix    string
	input     int64
	output    int64
	cacheRead int64
}

// Ordered longest-prefix first. Estimates only: the provider invoice is
// authoritative.
var modelPrices = []modelPrice{
	{"claude-fable-5-1", 10_000, 50_000, 250},
	{"claude-mythos-5-1", 10_000, 50_000, 250},
	{"claude-fable-5", 10_000, 50_000, 1_000},
	{"claude-mythos-5", 10_000, 50_000, 1_000},
	{"claude-opus-5-5", 4_000, 20_000, 200},
	{"claude-opus-5", 5_000, 25_000, 500},
	{"claude-opus-4-8", 5_000, 25_000, 500},
	{"claude-opus-4-7", 5_000, 25_000, 500},
	{"claude-opus-4-6", 5_000, 25_000, 500},
	{"claude-opus-4-5", 5_000, 25_000, 500},
	{"claude-opus-4-1", 15_000, 75_000, 1_500},
	{"claude-opus-4", 15_000, 75_000, 1_500},
	{"claude-sonnet-5", 2_000, 10_000, 200},
	{"claude-sonnet-4", 3_000, 15_000, 300},
	{"claude-haiku-4-5", 1_000, 5_000, 100},
}

// EstimateCostNanoUSD returns the estimated cost in nano-USD (1e-9 USD);
// false when the model has no known price (e.g. a self-hosted fallback).
func EstimateCostNanoUSD(model string, u provider.Usage) (int64, bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	for _, p := range modelPrices {
		if strings.HasPrefix(m, p.prefix) {
			// tokens x milli-USD/MTok = nano-USD. Cache write = 1.25 x input.
			return u.InputTokens*p.input +
				u.OutputTokens*p.output +
				u.CacheWriteTokens*p.input*5/4 +
				u.CacheReadTokens*p.cacheRead, true
		}
	}
	return 0, false
}

// FormatUSD renders nano-USD as a decimal string with 6 fractional digits
// (rounded half up), the scale of ai_usage_events.cost_usd.
func FormatUSD(nano int64) string {
	sign := ""
	if nano < 0 {
		sign, nano = "-", -nano
	}
	micro := (nano + 500) / 1000
	return fmt.Sprintf("%s%d.%06d", sign, micro/1_000_000, micro%1_000_000)
}
