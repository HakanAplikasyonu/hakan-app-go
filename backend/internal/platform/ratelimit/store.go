package ratelimit

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Store keeps the counters and markers behind the limiter. Two
// implementations exist: Redis (shared by every API instance) and an
// in-process map (single instance, or a fallback when Redis is not
// configured). Errors make the limiter fail open.
type Store interface {
	// Incr adds one to key and returns the new value. The first increment
	// starts a window: the key expires after window.
	Incr(ctx context.Context, key string, window time.Duration) (int64, error)
	// TTL is how long key lives on; 0 when it does not exist.
	TTL(ctx context.Context, key string) (time.Duration, error)
	// Set writes a marker that expires after ttl (overwriting any earlier one).
	Set(ctx context.Context, key string, ttl time.Duration) error
	// SetNX writes the marker only when it is absent and reports whether it did.
	SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error)
	// Del removes keys.
	Del(ctx context.Context, keys ...string) error
}

// RedisStore is the shared store (INCR + EXPIRE).
type RedisStore struct{ rdb *redis.Client }

// NewRedisStore wraps a client; nil returns nil.
func NewRedisStore(rdb *redis.Client) Store {
	if rdb == nil {
		return nil
	}
	return &RedisStore{rdb: rdb}
}

func (s *RedisStore) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	n, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 {
		_ = s.rdb.Expire(ctx, key, window).Err()
	}
	return n, nil
}

func (s *RedisStore) TTL(ctx context.Context, key string) (time.Duration, error) {
	ttl, err := s.rdb.PTTL(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if ttl < 0 {
		// -2: missing. -1: no expiry (should not happen; treat as missing).
		return 0, nil
	}
	return ttl, nil
}

func (s *RedisStore) Set(ctx context.Context, key string, ttl time.Duration) error {
	return s.rdb.Set(ctx, key, "1", ttl).Err()
}

func (s *RedisStore) SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return s.rdb.SetNX(ctx, key, "1", ttl).Result()
}

func (s *RedisStore) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return s.rdb.Del(ctx, keys...).Err()
}

// MemoryStore is an in-process store. Counters are per API instance, so with
// several replicas each one enforces the limit on its own share of traffic.
type MemoryStore struct {
	mu      sync.Mutex
	items   map[string]memItem
	max     int
	now     func() time.Time
	lastGC  time.Time
	gcEvery time.Duration
}

type memItem struct {
	n   int64
	exp time.Time
}

// defaultMemoryEntries bounds memory use: past it, expired entries are swept
// and, if the map is still full, new keys are not tracked (fail open).
const defaultMemoryEntries = 100_000

// NewMemoryStore returns an empty in-process store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: map[string]memItem{}, max: defaultMemoryEntries, now: time.Now, gcEvery: time.Minute}
}

// get returns the live entry for key, dropping it when expired. mu held.
func (s *MemoryStore) get(key string, now time.Time) (memItem, bool) {
	it, ok := s.items[key]
	if !ok {
		return memItem{}, false
	}
	if !now.Before(it.exp) {
		delete(s.items, key)
		return memItem{}, false
	}
	return it, true
}

// room reports whether a new key may be added, sweeping when needed. mu held.
func (s *MemoryStore) room(now time.Time) bool {
	if now.Sub(s.lastGC) >= s.gcEvery || len(s.items) >= s.max {
		for k, it := range s.items {
			if !now.Before(it.exp) {
				delete(s.items, k)
			}
		}
		s.lastGC = now
	}
	return len(s.items) < s.max
}

func (s *MemoryStore) Incr(_ context.Context, key string, window time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	it, ok := s.get(key, now)
	if !ok {
		if !s.room(now) {
			return 0, errStoreFull
		}
		it = memItem{exp: now.Add(window)}
	}
	it.n++
	s.items[key] = it
	return it.n, nil
}

func (s *MemoryStore) TTL(_ context.Context, key string) (time.Duration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	it, ok := s.get(key, now)
	if !ok {
		return 0, nil
	}
	return it.exp.Sub(now), nil
}

func (s *MemoryStore) Set(_ context.Context, key string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if _, ok := s.get(key, now); !ok && !s.room(now) {
		return errStoreFull
	}
	s.items[key] = memItem{n: 1, exp: now.Add(ttl)}
	return nil
}

func (s *MemoryStore) SetNX(_ context.Context, key string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if _, ok := s.get(key, now); ok {
		return false, nil
	}
	if !s.room(now) {
		return false, errStoreFull
	}
	s.items[key] = memItem{n: 1, exp: now.Add(ttl)}
	return true, nil
}

func (s *MemoryStore) Del(_ context.Context, keys ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range keys {
		delete(s.items, k)
	}
	return nil
}

// Len is the number of tracked keys (tests and metrics).
func (s *MemoryStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

type storeError string

func (e storeError) Error() string { return string(e) }

const errStoreFull = storeError("ratelimit: memory store is full")
