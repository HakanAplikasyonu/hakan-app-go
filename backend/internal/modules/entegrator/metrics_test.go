package entegrator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Entegratör hata oranı metriği hata türüne göre ayrılır (#179).
func TestOutcomeLabels(t *testing.T) {
	cases := map[string]error{
		"ok":         nil,
		"auth":       fmt.Errorf("wrap: %w", &provider.Error{Kind: provider.KindAuth, Provider: "izibiz"}),
		"rate-limit": &provider.Error{Kind: provider.KindRateLimit},
		"timeout":    context.DeadlineExceeded,
		"canceled":   context.Canceled,
		"error":      errors.New("boom"),
	}
	for want, err := range cases {
		if got := outcome(err); got != want {
			t.Errorf("outcome(%v) = %q, want %q", err, got, want)
		}
	}
}
