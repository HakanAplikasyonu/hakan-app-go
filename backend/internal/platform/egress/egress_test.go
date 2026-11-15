package egress

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func ipServer(answer string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, answer)
	}))
}

func TestCheckComparesWithExpectedIPs(t *testing.T) {
	srv := ipServer("203.0.113.7")
	defer srv.Close()
	ctx, now := context.Background(), time.Now()

	st, err := Check(ctx, srv.Client(), Config{CheckURL: srv.URL, ExpectedIPs: []string{"198.51.100.1", " 203.0.113.7 "}}, now)
	if err != nil || !st.OK || st.ObservedIP != "203.0.113.7" {
		t.Fatalf("match: %+v %v", st, err)
	}
	st, err = Check(ctx, srv.Client(), Config{CheckURL: srv.URL, ExpectedIPs: []string{"198.51.100.1"}}, now)
	if !errors.Is(err, ErrIPMismatch) || st.OK || st.ObservedIP != "203.0.113.7" || st.Error == "" {
		t.Fatalf("mismatch: %+v %v", st, err)
	}
	st, err = Check(ctx, srv.Client(), Config{CheckURL: srv.URL}, now)
	if err != nil || !st.OK {
		t.Fatalf("no expectation records the IP: %+v %v", st, err)
	}

	bad := ipServer("<html>not an ip</html>")
	defer bad.Close()
	if _, err := Check(ctx, bad.Client(), Config{CheckURL: bad.URL, ExpectedIPs: []string{"203.0.113.7"}}, now); !errors.Is(err, ErrCheck) {
		t.Fatalf("garbage answer: %v", err)
	}
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(502) }))
	defer down.Close()
	if _, err := Check(ctx, down.Client(), Config{CheckURL: down.URL}, now); !errors.Is(err, ErrCheck) {
		t.Fatalf("502: %v", err)
	}
}

// TestHTTPClientLeavesThroughProxy proves the fixed-egress client takes the
// configured route: every request (the IP check and integrator calls alike)
// reaches the proxy, and the IP the check sees is the proxy's answer.
func TestHTTPClientLeavesThroughProxy(t *testing.T) {
	var proxied atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A forward proxy receives the absolute target URL.
		if !strings.HasPrefix(r.RequestURI, "http://ip-check.invalid/") {
			http.Error(w, "unexpected target "+r.RequestURI, http.StatusBadGateway)
			return
		}
		proxied.Add(1)
		_, _ = fmt.Fprint(w, "192.0.2.10")
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1") // must be ignored

	cfg := Config{ProxyURL: proxy.URL, CheckURL: "http://ip-check.invalid/", ExpectedIPs: []string{"192.0.2.10"}}
	hc, err := cfg.HTTPClient(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	st, err := Check(context.Background(), hc, cfg, time.Now())
	if err != nil || !st.OK || !st.ViaProxy || proxied.Load() != 1 {
		t.Fatalf("via proxy: %+v %v (proxied=%d)", st, err, proxied.Load())
	}

	// Without a proxy the environment's HTTP_PROXY is not used either: the
	// request goes direct (and fails to resolve the .invalid host).
	direct, _ := Config{}.HTTPClient(2 * time.Second)
	if _, err := Check(context.Background(), direct, Config{CheckURL: "http://ip-check.invalid/"}, time.Now()); !errors.Is(err, ErrCheck) {
		t.Fatalf("direct: %v", err)
	}
	if proxied.Load() != 1 {
		t.Fatal("direct client used a proxy")
	}
}

func TestValidate(t *testing.T) {
	for _, c := range []Config{
		{ProxyURL: "ftp://proxy:21"},
		{ProxyURL: "not a url"},
		{ExpectedIPs: []string{"300.1.1.1"}},
	} {
		if err := c.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil", c)
		}
	}
	for _, c := range []Config{
		{},
		{ProxyURL: "socks5://10.0.0.5:1080", ExpectedIPs: []string{"203.0.113.7", "2001:db8::1"}},
		{ProxyURL: "http://user:pass@proxy.internal:3128"},
	} {
		if err := c.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v", c, err)
		}
	}
}

// TestCheckerStoresAndAlarms: a changed IP fails the run (the queue logs and
// reports the failed task) and the status endpoint shows it.
func TestCheckerStoresAndAlarms(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewStore(rdb, "test")
	answer := atomic.Value{}
	answer.Store("203.0.113.7")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, answer.Load().(string))
	}))
	defer srv.Close()
	cfg := Config{CheckURL: srv.URL, ExpectedIPs: []string{"203.0.113.7"}, CheckInterval: time.Minute}
	c := NewChecker(cfg, srv.Client(), store)
	ctx := context.Background()

	if err := c.Run(ctx); err != nil {
		t.Fatalf("first run: %v", err)
	}
	v, err := Describe(ctx, cfg, store, time.Now())
	if err != nil || v.Last == nil || !v.Last.OK || v.Stale || !v.Configured {
		t.Fatalf("after ok run: %+v %v", v, err)
	}

	answer.Store("198.51.100.99") // the IP changed
	if err := c.Run(ctx); !errors.Is(err, ErrIPMismatch) {
		t.Fatalf("changed IP: %v, want ErrIPMismatch", err)
	}
	v, _ = Describe(ctx, cfg, store, time.Now())
	if v.Last == nil || v.Last.OK || v.Last.ObservedIP != "198.51.100.99" {
		t.Fatalf("after mismatch: %+v", v.Last)
	}
	if v, _ := Describe(ctx, cfg, store, time.Now().Add(3*time.Minute)); !v.Stale {
		t.Fatal("status older than two intervals is not stale")
	}
	if v, _ := Describe(ctx, cfg, NewStore(rdb, "other"), time.Now()); v.Last != nil || !v.Stale {
		t.Fatalf("no check yet: %+v", v)
	}
}

func TestIsIPNotAllowlisted(t *testing.T) {
	for _, msg := range []string{
		"izibiz: GetInvoice: ip-not-allowlisted: 203.0.113.7",
		"IP not allowlisted",
		"Erişim reddedildi: IP adresi yetkili değil",
	} {
		if !IsIPNotAllowlisted(errors.New(msg)) {
			t.Errorf("%q not recognized", msg)
		}
	}
	for _, err := range []error{nil, errors.New("auth: bad password"), errors.New("timeout")} {
		if IsIPNotAllowlisted(err) {
			t.Errorf("%v recognized", err)
		}
	}
}
