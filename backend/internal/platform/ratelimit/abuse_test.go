package ratelimit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ratelimit"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func memLimiter(rules map[string]ratelimit.Rule, b ratelimit.Backoff) *ratelimit.Limiter {
	return ratelimit.NewWithStore(ratelimit.NewMemoryStore(), "test", rules, b)
}

func TestMemoryStoreWindow(t *testing.T) {
	t.Parallel()
	lim := memLimiter(map[string]ratelimit.Rule{ratelimit.ActionLogin: {Limit: 2, Window: 50 * time.Millisecond}}, ratelimit.Backoff{})
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if ok, _ := lim.AllowLogin(ctx, "1.1.1.1", "a@x.com"); !ok {
			t.Fatalf("attempt %d should pass", i+1)
		}
	}
	ok, retry := lim.AllowLogin(ctx, "1.1.1.1", "A@X.com ")
	if ok || retry <= 0 || retry > 50*time.Millisecond {
		t.Fatalf("third attempt (same email, other case) must be limited: ok=%v retry=%v", ok, retry)
	}
	time.Sleep(60 * time.Millisecond)
	if ok, _ := lim.AllowLogin(ctx, "1.1.1.1", "a@x.com"); !ok {
		t.Fatal("window must reset")
	}
}

func TestConfiguredRuleOverridesDefault(t *testing.T) {
	t.Parallel()
	lim := memLimiter(map[string]ratelimit.Rule{ratelimit.ActionReset: {Limit: 1, Window: time.Minute}}, ratelimit.Backoff{})
	ctx := context.Background()
	if ok, _ := lim.AllowResetPassword(ctx, "9.9.9.9"); !ok {
		t.Fatal("first reset must pass")
	}
	if ok, _ := lim.AllowResetPassword(ctx, "9.9.9.9"); ok {
		t.Fatal("configured limit 1 must apply")
	}
	if got := lim.Rule(ratelimit.ActionLogin); got.Limit != 10 {
		t.Fatalf("unconfigured actions keep defaults, got %+v", got)
	}
}

func TestNewWithConfigStores(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rule := map[string]ratelimit.Rule{ratelimit.ActionRegister: {Limit: 1, Window: time.Minute}}

	// auto without Redis falls back to memory: limits apply.
	auto := ratelimit.NewWithConfig(nil, "test", ratelimit.Config{Store: "auto", Rules: rule})
	auto.AllowRegister(ctx, "1.2.3.4")
	if ok, _ := auto.AllowRegister(ctx, "1.2.3.4"); ok {
		t.Fatal("auto without redis must use the memory store")
	}
	// off: nothing is limited.
	off := ratelimit.NewWithConfig(nil, "test", ratelimit.Config{Store: "off", Rules: rule})
	off.AllowRegister(ctx, "1.2.3.4")
	if ok, _ := off.AllowRegister(ctx, "1.2.3.4"); !ok {
		t.Fatal("off must not limit")
	}
	// redis selected but not configured: fail open.
	rOnly := ratelimit.NewWithConfig(nil, "test", ratelimit.Config{Store: "redis", Rules: rule})
	rOnly.AllowRegister(ctx, "1.2.3.4")
	if ok, _ := rOnly.AllowRegister(ctx, "1.2.3.4"); !ok {
		t.Fatal("redis without a client must fail open")
	}
	// auto with Redis uses it.
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	withRedis := ratelimit.NewWithConfig(rdb, "test", ratelimit.Config{Rules: rule})
	withRedis.AllowRegister(ctx, "1.2.3.4")
	if ok, _ := withRedis.AllowRegister(ctx, "1.2.3.4"); ok {
		t.Fatal("auto with redis must limit")
	}
	if len(mr.Keys()) == 0 {
		t.Fatal("expected the counter in redis")
	}
}

func TestBackoffDelay(t *testing.T) {
	t.Parallel()
	b := ratelimit.Backoff{Base: 30 * time.Second, Max: 5 * time.Minute}
	want := map[int64]time.Duration{
		4: 0, 5: 30 * time.Second, 6: time.Minute, 7: 2 * time.Minute, 8: 4 * time.Minute, 9: 5 * time.Minute, 50: 5 * time.Minute,
	}
	for n, d := range want {
		if got := b.Delay(n, 5); got != d {
			t.Errorf("Delay(%d) = %v, want %v", n, got, d)
		}
	}
}

func testLoginBackoff(t *testing.T, lim *ratelimit.Limiter) {
	t.Helper()
	ctx := context.Background()
	for i := 1; i <= 2; i++ {
		if lock := lim.LoginFailed(ctx, "5.5.5.5", "u@x.com"); lock != 0 {
			t.Fatalf("failure %d below threshold must not lock, got %v", i, lock)
		}
	}
	if locked, _ := lim.LoginLocked(ctx, "5.5.5.5", "u@x.com"); locked {
		t.Fatal("not locked yet")
	}
	if lock := lim.LoginFailed(ctx, "5.5.5.5", "u@x.com"); lock != time.Second {
		t.Fatalf("third failure locks for base, got %v", lock)
	}
	if lock := lim.LoginFailed(ctx, "5.5.5.5", "u@x.com"); lock != 2*time.Second {
		t.Fatalf("fourth failure doubles, got %v", lock)
	}
	// The account is locked from any IP; the IP is locked for any account?
	// Only after IPThreshold failures (10 here), so not yet.
	if locked, wait := lim.LoginLocked(ctx, "6.6.6.6", "U@x.com"); !locked || wait <= 0 {
		t.Fatalf("account lock must apply from another IP: %v %v", locked, wait)
	}
	if locked, _ := lim.LoginLocked(ctx, "5.5.5.5", "other@x.com"); locked {
		t.Fatal("IP is below its own threshold; other accounts are not locked")
	}
	lim.LoginSucceeded(ctx, "u@x.com")
	if locked, _ := lim.LoginLocked(ctx, "5.5.5.5", "u@x.com"); locked {
		t.Fatal("success clears the account lock")
	}
	if lock := lim.LoginFailed(ctx, "5.5.5.5", "u@x.com"); lock != 0 {
		t.Fatalf("counter restarts after success, got %v", lock)
	}
}

func TestLoginBackoffMemory(t *testing.T) {
	t.Parallel()
	testLoginBackoff(t, memLimiter(nil, ratelimit.Backoff{Threshold: 3, IPThreshold: 10, Base: time.Second, Max: time.Minute, Window: time.Hour}))
}

func TestLoginBackoffRedis(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	lim := ratelimit.NewWithStore(ratelimit.NewRedisStore(rdb), "test", nil,
		ratelimit.Backoff{Threshold: 3, IPThreshold: 10, Base: time.Second, Max: time.Minute, Window: time.Hour})
	testLoginBackoff(t, lim)
}

func TestLoginBackoffPerIP(t *testing.T) {
	t.Parallel()
	lim := memLimiter(nil, ratelimit.Backoff{Threshold: 100, IPThreshold: 3, Base: time.Second, Max: time.Minute, Window: time.Hour})
	ctx := context.Background()
	// A spray: one IP, a different account each time.
	for i, email := range []string{"a@x.com", "b@x.com", "c@x.com"} {
		lock := lim.LoginFailed(ctx, "7.7.7.7", email)
		if (i < 2 && lock != 0) || (i == 2 && lock != time.Second) {
			t.Fatalf("failure %d: lock %v", i+1, lock)
		}
	}
	if locked, _ := lim.LoginLocked(ctx, "7.7.7.7", "fresh@x.com"); !locked {
		t.Fatal("the spraying IP must be locked for every account")
	}
	lim.LoginSucceeded(ctx, "fresh@x.com")
	if locked, _ := lim.LoginLocked(ctx, "7.7.7.7", "fresh@x.com"); !locked {
		t.Fatal("a success must not clear the IP lock")
	}
	if locked, _ := lim.LoginLocked(ctx, "8.8.8.8", "fresh@x.com"); locked {
		t.Fatal("other IPs are not affected")
	}
}

type hookLog struct {
	mu     sync.Mutex
	events []ratelimit.Event
}

func (h *hookLog) add(_ *http.Request, ev ratelimit.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, ev)
}

func (h *hookLog) len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.events)
}

func TestReportDedupesAndHashes(t *testing.T) {
	t.Parallel()
	lim := memLimiter(nil, ratelimit.Backoff{})
	var log hookLog
	lim.SetAbuseHook(log.add)
	r := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	for i := 0; i < 5; i++ {
		lim.Report(r, ratelimit.Event{Kind: "rate_limited", Action: "login", Retry: time.Minute}, "login|1.1.1.1|secret@x.com")
	}
	if log.len() != 1 {
		t.Fatalf("expected one audit event per window, got %d", log.len())
	}
	ev := log.events[0]
	if ev.Subject == "" || strings.Contains(ev.Subject, "secret") || len(ev.Subject) != 16 {
		t.Fatalf("subject must be a short hash, got %q", ev.Subject)
	}
	lim.Report(r, ratelimit.Event{Kind: "login_locked", Action: "login", Retry: time.Minute}, "login|1.1.1.1|secret@x.com")
	lim.Report(r, ratelimit.Event{Kind: "rate_limited", Action: "login", Retry: time.Minute}, "login|2.2.2.2|secret@x.com")
	if log.len() != 3 {
		t.Fatalf("other kind / subject must be reported, got %d", log.len())
	}
}

func TestMaskEmail(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Furkan@Example.com": "f***@example.com",
		"ş@x.com.tr":         "ş***@x.com.tr",
		"not-an-email":       "***",
		"":                   "",
	}
	for in, want := range cases {
		if got := ratelimit.MaskEmail(in); got != want {
			t.Errorf("MaskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsUpload(t *testing.T) {
	t.Parallel()
	cases := []struct {
		method, path, ctype string
		want                bool
	}{
		{http.MethodPost, "/v1/platform/storage/uploads", "application/json", true},
		{http.MethodPost, "/v1/tenant/engagements/x/ubl-uploads", "multipart/form-data; boundary=a", true},
		{http.MethodPost, "/v1/taxpayer/receipts", "multipart/form-data; boundary=a", true},
		{http.MethodPut, "/v1/platform/settings/logo", "multipart/form-data; boundary=a", true},
		{http.MethodPost, "/v1/platform/users/import", "text/csv", true},
		{http.MethodGet, "/v1/platform/storage/uploads", "", false},
		{http.MethodPost, "/v1/auth/login", "application/json", false},
		{http.MethodPost, "/v1/uploader-settings", "application/json", false},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.ctype != "" {
			r.Header.Set("Content-Type", tc.ctype)
		}
		if got := ratelimit.IsUpload(r); got != tc.want {
			t.Errorf("%s %s (%s) = %v, want %v", tc.method, tc.path, tc.ctype, got, tc.want)
		}
	}
}

func TestUploadGuard(t *testing.T) {
	t.Parallel()
	lim := memLimiter(map[string]ratelimit.Rule{
		ratelimit.ActionUploadUser: {Limit: 2, Window: time.Minute},
		ratelimit.ActionUploadIP:   {Limit: 3, Window: time.Minute},
	}, ratelimit.Backoff{})
	var log hookLog
	lim.SetAbuseHook(log.add)
	reached := 0
	h := lim.UploadGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached++
		w.WriteHeader(http.StatusCreated)
	}))
	do := func(method, path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader("x"))
		r.RemoteAddr = "203.0.113.9:5555"
		r.Header.Set("Content-Type", "multipart/form-data; boundary=b")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	for i := 0; i < 2; i++ {
		if rec := do(http.MethodPost, "/v1/taxpayer/uploads", "tok-a"); rec.Code != http.StatusCreated {
			t.Fatalf("upload %d: %d", i+1, rec.Code)
		}
	}
	rec := do(http.MethodPost, "/v1/taxpayer/uploads", "tok-a")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("third upload for the token: %d (Retry-After %q)", rec.Code, rec.Header().Get("Retry-After"))
	}
	// Another session from the same IP still has the IP budget (3; two used
	// by tok-a, the blocked third did not reach the IP check).
	if rec := do(http.MethodPost, "/v1/taxpayer/uploads", "tok-b"); rec.Code != http.StatusCreated {
		t.Fatalf("other token: %d", rec.Code)
	}
	if rec := do(http.MethodPost, "/v1/taxpayer/uploads", "tok-c"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("IP budget exhausted: %d", rec.Code)
	}
	// Non-upload requests are never counted.
	for i := 0; i < 5; i++ {
		r := httptest.NewRequest(http.MethodGet, "/v1/taxpayer/uploads", nil)
		r.RemoteAddr = "203.0.113.9:5555"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		if rr.Code != http.StatusCreated {
			t.Fatalf("GET must pass through: %d", rr.Code)
		}
	}
	if reached != 3+5 {
		t.Fatalf("handler reached %d times, want 8", reached)
	}
	if log.len() != 2 {
		t.Fatalf("expected one audit event per limited key (user, ip), got %d", log.len())
	}
}

func TestMemoryStoreBasics(t *testing.T) {
	t.Parallel()
	s := ratelimit.NewMemoryStore()
	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		if _, err := s.Incr(ctx, "k"+string(rune('a'+i%26))+time.Duration(i).String(), time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := s.Incr(ctx, "after", time.Minute); err != nil {
		t.Fatal(err)
	}
	if ttl, _ := s.TTL(ctx, "missing"); ttl != 0 {
		t.Fatalf("missing key ttl = %v", ttl)
	}
	if ok, _ := s.SetNX(ctx, "m", time.Minute); !ok {
		t.Fatal("first SetNX must set")
	}
	if ok, _ := s.SetNX(ctx, "m", time.Minute); ok {
		t.Fatal("second SetNX must not set")
	}
}
