package egress

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Store keeps the last check result in Redis so the API (another process)
// can show it. Redis is not the alarm: a failed check fails its task, which
// is logged and reported.
type Store struct {
	rdb *redis.Client
	key string
}

// NewStore builds the store. rdb may be nil (Save/Load become no-ops).
func NewStore(rdb *redis.Client, appEnv string) *Store {
	env := strings.TrimSpace(appEnv)
	if env == "" {
		env = "dev"
	}
	return &Store{rdb: rdb, key: "app:" + env + ":egress:status"}
}

// Save records st for 7 days.
func (s *Store) Save(ctx context.Context, st Status) error {
	if s == nil || s.rdb == nil {
		return nil
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, s.key, raw, 7*24*time.Hour).Err()
}

// Load returns the last result; ok is false when there is none.
func (s *Store) Load(ctx context.Context) (Status, bool, error) {
	if s == nil || s.rdb == nil {
		return Status{}, false, nil
	}
	raw, err := s.rdb.Get(ctx, s.key).Bytes()
	if errors.Is(err, redis.Nil) {
		return Status{}, false, nil
	}
	if err != nil {
		return Status{}, false, err
	}
	var st Status
	if err := json.Unmarshal(raw, &st); err != nil {
		return Status{}, false, err
	}
	return st, true, nil
}

// Checker runs Check with the fixed-egress client and stores the result.
type Checker struct {
	cfg   Config
	hc    Doer
	store *Store
	now   func() time.Time
}

// NewChecker builds the checker for the fixed-egress worker.
func NewChecker(cfg Config, hc Doer, store *Store) *Checker {
	return &Checker{cfg: cfg.Normalize(), hc: hc, store: store, now: time.Now}
}

// Run checks once. A mismatch or a failed check returns an error so the
// queue reports it (alarm); the result is stored either way.
func (c *Checker) Run(ctx context.Context) error {
	st, err := Check(ctx, c.hc, c.cfg, c.now())
	if serr := c.store.Save(ctx, st); serr != nil && err == nil {
		return serr
	}
	return err
}

// View is the API shape of the fixed-egress status.
type View struct {
	Configured  bool      `json:"configured"`
	ViaProxy    bool      `json:"via_proxy"`
	ExpectedIPs []string  `json:"expected_ips"`
	Last        *Status   `json:"last"`
	Stale       bool      `json:"stale"`
	Interval    string    `json:"check_interval"`
	Now         time.Time `json:"now"`
}

// Describe builds the status view: configuration plus the last stored check.
// The status is stale when no check ran within two intervals.
func Describe(ctx context.Context, cfg Config, store *Store, now time.Time) (View, error) {
	cfg = cfg.Normalize()
	v := View{
		Configured: len(cfg.ExpectedIPs) > 0, ViaProxy: strings.TrimSpace(cfg.ProxyURL) != "",
		ExpectedIPs: cfg.ExpectedIPs, Interval: cfg.CheckInterval.String(), Now: now.UTC(),
	}
	if v.ExpectedIPs == nil {
		v.ExpectedIPs = []string{}
	}
	st, ok, err := store.Load(ctx)
	if err != nil {
		return v, err
	}
	if ok {
		v.Last = &st
		v.Stale = now.Sub(st.CheckedAt) > 2*cfg.CheckInterval
	} else {
		v.Stale = true
	}
	return v, nil
}
