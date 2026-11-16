package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultWindow = 15 * time.Minute

// Action names of the built-in rules. Config.Rules overrides them by name.
const (
	ActionLogin      = "login"
	ActionRegister   = "register"
	ActionForgot     = "forgot"
	ActionReset      = "reset"
	ActionUploadUser = "upload_user"
	ActionUploadIP   = "upload_ip"
)

// Rule is a fixed window: at most Limit hits per Window.
type Rule struct {
	Limit  int
	Window time.Duration
}

// DefaultRules are the limits used when Config.Rules does not name an action.
func DefaultRules() map[string]Rule {
	return map[string]Rule{
		ActionLogin:    {Limit: 10, Window: defaultWindow},
		ActionRegister: {Limit: 5, Window: defaultWindow},
		ActionForgot:   {Limit: 5, Window: defaultWindow},
		ActionReset:    {Limit: 10, Window: defaultWindow},
		// Uploads: generous enough for a bulk UBL drop that the browser sends
		// file by file; per session token and per IP.
		ActionUploadUser: {Limit: 300, Window: 10 * time.Minute},
		ActionUploadIP:   {Limit: 600, Window: 10 * time.Minute},
	}
}

// Backoff is the progressive wait after failed password logins. Failures are
// counted per account (email) and per IP within Window; from Threshold
// failures on, each further failure locks that key for Base·2^(n-Threshold),
// capped at Max. A successful login clears the account's counter.
type Backoff struct {
	Threshold   int
	IPThreshold int
	Base        time.Duration
	Max         time.Duration
	Window      time.Duration
}

// DefaultBackoff: 5 failures per account (20 per IP) → 30 s, 1 m, 2 m … 15 m.
func DefaultBackoff() Backoff {
	return Backoff{Threshold: 5, IPThreshold: 20, Base: 30 * time.Second, Max: 15 * time.Minute, Window: time.Hour}
}

// Delay is the lock for the n-th failure (0 below the threshold).
func (b Backoff) Delay(n int64, threshold int) time.Duration {
	if threshold <= 0 || b.Base <= 0 || n < int64(threshold) {
		return 0
	}
	d := b.Base
	for i := int64(threshold); i < n; i++ {
		d *= 2
		if b.Max > 0 && d >= b.Max {
			return b.Max
		}
	}
	if b.Max > 0 && d > b.Max {
		return b.Max
	}
	return d
}

// Store kinds accepted by Config.Store.
const (
	StoreAuto   = "auto"   // Redis when configured, otherwise memory
	StoreRedis  = "redis"  // Redis only; without Redis nothing is limited
	StoreMemory = "memory" // in-process, per instance
	StoreOff    = "off"    // no limits
)

// Config selects the store and the limits.
type Config struct {
	Store   string
	Rules   map[string]Rule
	Backoff Backoff
}

// Event is an abuse signal reported to the audit hook.
type Event struct {
	// Kind: "rate_limited" or "login_locked".
	Kind   string
	Action string
	// Subject is a short hash of the limited key (no personal data).
	Subject string
	// Email is the masked account for login events ("f***@example.com").
	Email string
	IP    string
	Retry time.Duration
}

// Limiter is a fixed-window rate limiter with progressive login backoff.
// Store errors fail open.
type Limiter struct {
	store   Store
	env     string
	rules   map[string]Rule
	backoff Backoff
	hook    func(r *http.Request, ev Event)
}

// New creates a Redis-backed limiter with the default rules. rdb may be nil
// (all requests allowed). Kept for callers and tests that do not configure
// limits; the server uses NewWithConfig.
func New(rdb *redis.Client, appEnv string) *Limiter {
	return newLimiter(NewRedisStore(rdb), appEnv, nil, DefaultBackoff())
}

// NewWithConfig builds the limiter from configuration.
func NewWithConfig(rdb *redis.Client, appEnv string, cfg Config) *Limiter {
	var store Store
	switch strings.ToLower(strings.TrimSpace(cfg.Store)) {
	case StoreOff:
		store = nil
	case StoreRedis:
		store = NewRedisStore(rdb)
	case StoreMemory:
		store = NewMemoryStore()
	default: // auto
		if rdb != nil {
			store = NewRedisStore(rdb)
		} else {
			store = NewMemoryStore()
		}
	}
	return newLimiter(store, appEnv, cfg.Rules, cfg.Backoff)
}

// NewWithStore is for tests and custom stores.
func NewWithStore(store Store, appEnv string, rules map[string]Rule, backoff Backoff) *Limiter {
	return newLimiter(store, appEnv, rules, backoff)
}

func newLimiter(store Store, appEnv string, rules map[string]Rule, backoff Backoff) *Limiter {
	env := strings.TrimSpace(appEnv)
	if env == "" {
		env = "dev"
	}
	merged := DefaultRules()
	for k, v := range rules {
		merged[k] = v
	}
	if backoff == (Backoff{}) {
		backoff = DefaultBackoff()
	}
	if backoff.Window <= 0 {
		backoff.Window = time.Hour
	}
	return &Limiter{store: store, env: env, rules: merged, backoff: backoff}
}

// SetAbuseHook installs the audit callback (called at most once per key and
// window, see Report).
func (l *Limiter) SetAbuseHook(fn func(r *http.Request, ev Event)) {
	if l != nil {
		l.hook = fn
	}
}

// Rule returns the configured rule for action.
func (l *Limiter) Rule(action string) Rule {
	if l == nil {
		return Rule{}
	}
	return l.rules[action]
}

// AllowLogin limits password login attempts per IP + email.
func (l *Limiter) AllowLogin(ctx context.Context, ip, email string) (bool, time.Duration) {
	return l.allowRule(ctx, ActionLogin, ip+"|"+normEmail(email))
}

// AllowRegister limits self-registration per IP.
func (l *Limiter) AllowRegister(ctx context.Context, ip string) (bool, time.Duration) {
	return l.allowRule(ctx, ActionRegister, ip)
}

// AllowForgotPassword limits reset emails per IP + email.
func (l *Limiter) AllowForgotPassword(ctx context.Context, ip, email string) (bool, time.Duration) {
	return l.allowRule(ctx, ActionForgot, ip+"|"+normEmail(email))
}

// AllowResetPassword limits reset submissions per IP.
func (l *Limiter) AllowResetPassword(ctx context.Context, ip string) (bool, time.Duration) {
	return l.allowRule(ctx, ActionReset, ip)
}

// Allow is a generic fixed-window limit: at most limit hits per window for
// (action, subject). A rule configured for action overrides limit/window.
// Like the auth limits it fails open when the store is unavailable.
func (l *Limiter) Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration) {
	if l != nil {
		if r, ok := l.rules[action]; ok {
			limit, window = r.Limit, r.Window
		}
	}
	if limit <= 0 || window <= 0 {
		return true, 0
	}
	return l.allowWindow(ctx, action, subject, limit, window)
}

func (l *Limiter) allowRule(ctx context.Context, action, subject string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	r := l.rules[action]
	if r.Limit <= 0 || r.Window <= 0 {
		return true, 0
	}
	return l.allowWindow(ctx, action, subject, r.Limit, r.Window)
}

func (l *Limiter) key(parts ...string) string {
	return fmt.Sprintf("app:%s:rl:%s", l.env, strings.Join(parts, ":"))
}

func (l *Limiter) allowWindow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration) {
	if l == nil || l.store == nil || strings.TrimSpace(subject) == "" || strings.TrimSpace(subject) == "|" {
		return true, 0
	}
	key := l.key(action, subject)
	n, err := l.store.Incr(ctx, key, window)
	if err != nil {
		return true, 0
	}
	if n > int64(limit) {
		ttl, ttlErr := l.store.TTL(ctx, key)
		if ttlErr != nil || ttl <= 0 {
			ttl = window
		}
		return false, ttl
	}
	return true, 0
}

// LoginLocked reports whether a password login for (ip, email) is in a
// backoff lock and for how long.
func (l *Limiter) LoginLocked(ctx context.Context, ip, email string) (bool, time.Duration) {
	if l == nil || l.store == nil {
		return false, 0
	}
	var wait time.Duration
	for _, k := range l.lockKeys(ip, email) {
		if ttl, err := l.store.TTL(ctx, k); err == nil && ttl > wait {
			wait = ttl
		}
	}
	return wait > 0, wait
}

// LoginFailed records a failed password login and returns the lock it
// started (0 when below the threshold).
func (l *Limiter) LoginFailed(ctx context.Context, ip, email string) time.Duration {
	if l == nil || l.store == nil {
		return 0
	}
	var lock time.Duration
	email = normEmail(email)
	type part struct {
		kind, subject string
		threshold     int
	}
	for _, p := range []part{{"email", email, l.backoff.Threshold}, {"ip", strings.TrimSpace(ip), l.backoff.IPThreshold}} {
		if p.subject == "" || p.threshold <= 0 {
			continue
		}
		n, err := l.store.Incr(ctx, l.key("loginfail", p.kind, p.subject), l.backoff.Window)
		if err != nil {
			continue
		}
		if d := l.backoff.Delay(n, p.threshold); d > 0 {
			if l.store.Set(ctx, l.key("loginlock", p.kind, p.subject), d) == nil && d > lock {
				lock = d
			}
		}
	}
	return lock
}

// LoginSucceeded clears the account's failure counter and lock. The IP
// counter is left to expire so one valid account cannot reset a spray.
func (l *Limiter) LoginSucceeded(ctx context.Context, email string) {
	if l == nil || l.store == nil {
		return
	}
	email = normEmail(email)
	if email == "" {
		return
	}
	_ = l.store.Del(ctx, l.key("loginfail", "email", email), l.key("loginlock", "email", email))
}

func (l *Limiter) lockKeys(ip, email string) []string {
	var keys []string
	if e := normEmail(email); e != "" {
		keys = append(keys, l.key("loginlock", "email", e))
	}
	if ip = strings.TrimSpace(ip); ip != "" {
		keys = append(keys, l.key("loginlock", "ip", ip))
	}
	return keys
}

// Report forwards an abuse event to the audit hook, at most once per
// (kind, action, subject) for the lock/window so a flood of blocked requests
// does not become a flood of audit rows. subject is hashed before it is
// stored; only its hash reaches the hook.
func (l *Limiter) Report(r *http.Request, ev Event, subject string) {
	if l == nil || l.hook == nil {
		return
	}
	ev.Subject = HashSubject(subject)
	if l.store != nil && r != nil {
		ttl := ev.Retry
		if ttl < time.Minute {
			ttl = time.Minute
		}
		first, err := l.store.SetNX(r.Context(), l.key("audit", ev.Kind, ev.Action, ev.Subject), ttl)
		if err == nil && !first {
			return
		}
	}
	l.hook(r, ev)
}

// HashSubject is a short, stable, non-reversible id for a limiter key.
func HashSubject(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// MaskEmail keeps the first letter and the domain: "f***@example.com".
func MaskEmail(email string) string {
	email = normEmail(email)
	at := strings.LastIndexByte(email, '@')
	if at <= 0 {
		if email == "" {
			return ""
		}
		return "***"
	}
	first := []rune(email[:at])[0]
	return string(first) + "***" + email[at:]
}

func normEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
