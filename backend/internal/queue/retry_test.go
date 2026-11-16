package queue

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestRetryDelayIsShortForRetrySoon(t *testing.T) {
	err := fmt.Errorf("deliver 14: %w", ErrRetrySoon)
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second}
	for n, w := range want {
		if got := RetryDelay(n, err, nil); got != w {
			t.Errorf("RetryDelay(%d) = %v, want %v", n, got, w)
		}
	}
}

func TestRetryDelayDefaultForOtherErrors(t *testing.T) {
	// asynq's default: n^4 + 15 s + jitter, so at least 15 s.
	if got := RetryDelay(0, errors.New("smtp down"), nil); got < 15*time.Second {
		t.Fatalf("default retry delay %v, want >= 15s", got)
	}
}
