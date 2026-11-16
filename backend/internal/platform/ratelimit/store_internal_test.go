package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryStoreCapSweepsThenFailsOpen(t *testing.T) {
	s := NewMemoryStore()
	s.max = 3
	now := time.Unix(1000, 0)
	s.now = func() time.Time { return now }
	ctx := context.Background()
	for _, k := range []string{"a", "b", "c"} {
		if _, err := s.Incr(ctx, k, time.Second); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Incr(ctx, "d", time.Second); !errors.Is(err, errStoreFull) {
		t.Fatalf("full store must refuse new keys, got %v", err)
	}
	// Existing keys keep counting while full.
	if n, err := s.Incr(ctx, "a", time.Second); err != nil || n != 2 {
		t.Fatalf("existing key: n=%d err=%v", n, err)
	}
	now = now.Add(2 * time.Second)
	if _, err := s.Incr(ctx, "d", time.Second); err != nil {
		t.Fatalf("expired keys must be swept: %v", err)
	}
	if s.Len() != 1 {
		t.Fatalf("len = %d, want 1", s.Len())
	}

	// The limiter fails open on a full store.
	l := NewWithStore(s, "test", map[string]Rule{ActionRegister: {Limit: 1, Window: time.Minute}}, Backoff{})
	for _, k := range []string{"x", "y"} {
		_, _ = s.Incr(ctx, k, time.Minute)
	}
	for i := 0; i < 3; i++ {
		if ok, _ := l.AllowRegister(ctx, "10.0.0.1"); !ok {
			t.Fatal("full store must fail open")
		}
	}
}
