package entegrator

// Contract tests (#196): every e-Fatura integrator client against an
// httptest fake built from the integrator's documented requests and
// responses (the same fakes the *_db_test.go sync tests use). They need no
// database. Each integrator goes through the same scenarios:
//
//   - pagination: every page of a listing is reached through Page.Next,
//   - auth: a refused credential is provider.KindAuth and is not retried,
//   - error codes: 5xx is retried and then reported as retryable; 429 is
//     reported as a rate limit and not retried,
//   - retry: a passing 503 is retried and the call succeeds,
//   - timeout: a hung integrator returns within the caller's deadline,
//   - fetch: the UBL of a listed document downloads.
//
// Live sandbox: CONTRACT_LIVE=1 plus CONTRACT_<NAME>_* credentials run the
// read-only scenarios (pagination, fetch) against the real test system
// instead of the fake; the fault scenarios stay fake-only. Off by default.
// See docs/entegrator-sozlesme-testleri.md.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testutil/contract"
)

// contractCase describes one integrator for the shared scenarios.
type contractCase struct {
	name string
	// fake returns the seeded fake: n incoming e-Fatura documents created
	// within [created, created+n hours).
	fake func(t *testing.T, n int, created time.Time) http.Handler
	// open opens the client against url; bad=true uses a refused credential.
	open func(url string, bad bool) (*Client, error)
	// limit is the page size asked for (0: the integrator's own).
	limit int
	// minPages is the page count the fake must need for 3 documents.
	minPages int
	// live returns the live sandbox config, or nil when it is not set.
	live func() *provider.Config
	// refusingServer: the fake does not check credentials, so the auth
	// scenario answers 401 from the server side.
	refusingServer bool
}

func contractCases() []contractCase {
	created := func(t *testing.T, n int, base time.Time, add func(i int, ettn string, xml []byte, at time.Time)) {
		t.Helper()
		for i := range n {
			xml, ettn := invoiceXML(t, 5000+i, true, "")
			add(i, ettn, xml, base.Add(time.Duration(i)*time.Hour))
		}
	}
	return []contractCase{
		{
			name: "nilvera", limit: 2, minPages: 2,
			fake: func(t *testing.T, n int, base time.Time) http.Handler {
				f := &fakeNilvera{}
				created(t, n, base, func(i int, ettn string, xml []byte, at time.Time) {
					f.docs = append(f.docs, restDoc{uuid: ettn, number: fmt.Sprintf("TST2026%09d", 5000+i), profile: "TEMELFATURA", incoming: true, created: at, xml: xml})
				})
				return f
			},
			open: func(url string, bad bool) (*Client, error) {
				return Open("nilvera", provider.Config{APIKey: pick(bad, "wrong-key", "test-key"), Endpoint: url, VKN: ownVKN})
			},
			live: liveConfig("NILVERA", func(e func(string) string) provider.Config {
				return provider.Config{APIKey: e("API_KEY"), Test: true, VKN: e("VKN")}
			}),
		},
		{
			name: "nes", limit: 2, minPages: 2,
			fake: func(t *testing.T, n int, base time.Time) http.Handler {
				f := &fakeNes{}
				created(t, n, base, func(i int, ettn string, xml []byte, at time.Time) {
					f.docs = append(f.docs, nesDoc{uuid: ettn, number: fmt.Sprintf("TST2026%09d", 5000+i), answer: "Waiting", incoming: true, created: at, xml: xml})
				})
				return f
			},
			open: func(url string, bad bool) (*Client, error) {
				return Open("nes", provider.Config{APIKey: pick(bad, "wrong-key", "test-key"), Endpoint: url, VKN: ownVKN})
			},
			live: liveConfig("NES", func(e func(string) string) provider.Config {
				return provider.Config{APIKey: e("API_KEY"), Test: true, VKN: e("VKN")}
			}),
		},
		{
			name: "muhasebeizleme", limit: 2, minPages: 2,
			fake: func(t *testing.T, n int, base time.Time) http.Handler {
				f := &fakeMuhasebeizleme{}
				created(t, n, base, func(i int, ettn string, xml []byte, at time.Time) {
					f.docs = append(f.docs, miDoc{uuid: ettn, number: fmt.Sprintf("TST2026%09d", 5000+i), status: "Approved", incoming: true, created: at, xml: xml})
				})
				return f
			},
			open: func(url string, bad bool) (*Client, error) {
				k := pick(bad, "wrong-key", "test-key")
				return Open("muhasebeizleme", provider.Config{APIKey: k, Password: k, Endpoint: url, VKN: ownVKN})
			},
			live: liveConfig("MUHASEBEIZLEME", func(e func(string) string) provider.Config {
				return provider.Config{APIKey: e("API_KEY"), Password: e("API_KEY"), Test: true, VKN: e("VKN")}
			}),
		},
		{
			name: "turkcell", limit: 2, minPages: 2,
			fake: func(t *testing.T, n int, base time.Time) http.Handler {
				f := &fakeTurkcell{}
				created(t, n, base, func(i int, ettn string, xml []byte, at time.Time) {
					f.docs = append(f.docs, turkcellDoc{uuid: ettn, number: fmt.Sprintf("TST2026%09d", 5000+i), created: at, xml: xml})
				})
				return f
			},
			open: func(url string, bad bool) (*Client, error) {
				return Open("turkcell", provider.Config{APIKey: pick(bad, "wrong-key", "test-key"), Endpoint: url})
			},
			live: liveConfig("TURKCELL", func(e func(string) string) provider.Config {
				return provider.Config{APIKey: e("API_KEY"), Test: true}
			}),
		},
		{
			name: "sovos", minPages: 1, refusingServer: true,
			fake: func(t *testing.T, n int, base time.Time) http.Handler {
				f := &fakeSovos{}
				created(t, n, base, func(i int, ettn string, xml []byte, at time.Time) {
					f.docs = append(f.docs, sovosDoc{uuid: ettn, number: fmt.Sprintf("TST2026%09d", 5000+i), incoming: true, created: at, xml: xml})
				})
				return f
			},
			open: func(url string, bad bool) (*Client, error) {
				return Open("sovos", provider.Config{Username: "user", Password: pick(bad, "wrong", "pass"), VKN: ownVKN, Endpoint: url,
					Options: map[string]string{"postbox_alias": "urn:mail:defaultpk@example.invalid", "sender_alias": "urn:mail:defaultgb@example.invalid"}})
			},
			live: liveConfig("SOVOS", func(e func(string) string) provider.Config {
				return provider.Config{Username: e("USERNAME"), Password: e("PASSWORD"), Test: true, VKN: e("VKN"),
					Options: map[string]string{"postbox_alias": e("POSTBOX_ALIAS"), "sender_alias": e("SENDER_ALIAS")}}
			}),
		},
		{
			name: "trendyol", limit: 2, minPages: 2, refusingServer: true,
			fake: func(t *testing.T, n int, base time.Time) http.Handler {
				f := &fakeTrendyol{}
				created(t, n, base, func(i int, ettn string, xml []byte, at time.Time) {
					f.docs = append(f.docs, trendyolDoc{uuid: ettn, number: fmt.Sprintf("TST2026%09d", 5000+i), created: at, xml: xml})
				})
				// The download step answers a link on the fake's own address.
				var once sync.Once
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					once.Do(func() {
						f.mu.Lock()
						f.url = "http://" + r.Host
						f.mu.Unlock()
					})
					f.ServeHTTP(w, r)
				})
			},
			// The fake signs in any user; the auth scenario refuses server side.
			open: func(url string, _ bool) (*Client, error) {
				return Open("trendyol", provider.Config{Username: "api@example.invalid", Password: "test", Endpoint: url,
					Options: map[string]string{"user_id": "1", "company_id": "2"}})
			},
			live: liveConfig("TRENDYOL", func(e func(string) string) provider.Config {
				return provider.Config{Username: e("USERNAME"), Password: e("PASSWORD"), Test: true,
					Options: map[string]string{"user_id": e("USER_ID"), "company_id": e("COMPANY_ID")}}
			}),
		},
		{
			name: "isnet", limit: 2, minPages: 2, refusingServer: true,
			fake: func(t *testing.T, n int, base time.Time) http.Handler {
				f := &fakeIsnet{}
				created(t, n, base, func(i int, ettn string, xml []byte, at time.Time) {
					f.docs = append(f.docs, isnetDoc{ettn: strings.ToUpper(ettn), number: fmt.Sprintf("TST2026%09d", 5000+i), profile: "TEMELFATURA",
						status: "Basariyla_Tamamlandi", dir: "Incoming", created: at, xml: xml})
				})
				return f
			},
			// İşNet authenticates by the fixed egress IP and the VKN, not by a
			// credential (32 §2): the auth scenario uses a refusing server.
			open: func(url string, _ bool) (*Client, error) {
				return Open("isnet", provider.Config{VKN: ownVKN, Endpoint: url, Options: map[string]string{"addressbook_endpoint": url}})
			},
			live: liveConfig("ISNET", func(e func(string) string) provider.Config {
				return provider.Config{VKN: e("VKN"), Test: true}
			}),
		},
	}
}

func pick(bad bool, wrong, right string) string {
	if bad {
		return wrong
	}
	return right
}

// liveConfig builds the sandbox config from CONTRACT_<NAME>_* when
// CONTRACT_LIVE=1 (contract.Live); nil when off or incomplete.
func liveConfig(name string, build func(env func(string) string) provider.Config) func() *provider.Config {
	return func() *provider.Config {
		if os.Getenv(contract.LiveEnv) != "1" {
			return nil
		}
		missing := false
		cfg := build(func(k string) string {
			v := contract.Optional(name, k)
			if v == "" {
				missing = true
			}
			return v
		})
		if missing {
			return nil
		}
		if ep := contract.Optional(name, "ENDPOINT"); ep != "" {
			cfg.Endpoint = ep
		}
		return &cfg
	}
}

var contractBase = time.Date(2026, 11, 10, 9, 0, 0, 0, time.FixedZone("TRT", 3*3600))

func contractQuery(c contractCase) ListQuery {
	return ListQuery{Kind: provider.EInvoice, From: contractBase.Add(-time.Hour), To: contractBase.Add(6 * time.Hour), Limit: c.limit}
}

// listAll walks every page; it fails t past 20 pages (a cursor loop).
func listAll(ctx context.Context, t *testing.T, cl *Client, q ListQuery) ([]provider.DocumentInfo, int) {
	t.Helper()
	var out []provider.DocumentInfo
	for pages := 1; ; pages++ {
		p, err := cl.List(ctx, provider.Inbound, q)
		if err != nil {
			t.Fatalf("page %d: %v", pages, err)
		}
		out = append(out, p.Items...)
		if p.Next == "" {
			return out, pages
		}
		if pages > 20 {
			t.Fatal("pagination does not end")
		}
		q.Cursor = p.Next
	}
}

func fastRetry(c *Client) *Client {
	return c.WithRetry(RetryPolicy{Attempts: 3, Base: time.Millisecond})
}

func TestIntegratorContracts(t *testing.T) {
	for _, c := range contractCases() {
		t.Run(c.name, func(t *testing.T) {
			if live := c.live(); live != nil {
				t.Run("live", func(t *testing.T) { liveContract(t, c, *live) })
			}
			serve := func(t *testing.T, h http.Handler) string {
				t.Helper()
				srv := httptest.NewServer(h)
				t.Cleanup(srv.Close)
				return srv.URL
			}
			open := func(t *testing.T, url string, bad bool) *Client {
				t.Helper()
				cl, err := c.open(url, bad)
				if err != nil {
					t.Fatal(err)
				}
				return fastRetry(cl)
			}
			ctx := context.Background()

			t.Run("pagination_and_fetch", func(t *testing.T) {
				cl := open(t, serve(t, c.fake(t, 3, contractBase)), false)
				items, pages := listAll(ctx, t, cl, contractQuery(c))
				if len(items) != 3 || pages < c.minPages {
					t.Fatalf("listed %d documents in %d pages, want 3 in >= %d", len(items), pages, c.minPages)
				}
				res, err := cl.Fetch(ctx, provider.FetchQuery{Kind: provider.EInvoice, Direction: provider.Inbound, UUID: items[0].UUID, Format: provider.FormatUBL})
				if err != nil || len(res.Data) == 0 {
					t.Fatalf("fetch: %v", err)
				}
			})

			t.Run("auth_refused", func(t *testing.T) {
				h := c.fake(t, 1, contractBase)
				if c.refusingServer {
					h = &contract.Fault{Next: h, Status: http.StatusUnauthorized, Times: -1}
				}
				cl := open(t, serve(t, h), true)
				_, err := cl.List(ctx, provider.Inbound, contractQuery(c))
				if provider.KindOf(err) != provider.KindAuth {
					t.Fatalf("err = %v (kind %s), want auth", err, provider.KindOf(err))
				}
			})

			t.Run("server_error_retried_then_reported", func(t *testing.T) {
				f := &contract.Fault{Next: c.fake(t, 1, contractBase), Status: http.StatusInternalServerError, Times: -1}
				cl := open(t, serve(t, f), false)
				_, err := cl.List(ctx, provider.Inbound, contractQuery(c))
				if err == nil || !provider.IsRetryable(err) {
					t.Fatalf("err = %v, want a retryable error", err)
				}
				if n := f.Seen(); n < 3 {
					t.Fatalf("%d requests, want the call retried (3 attempts)", n)
				}
			})

			t.Run("transient_unavailable_recovers", func(t *testing.T) {
				f := &contract.Fault{Next: c.fake(t, 2, contractBase), Status: http.StatusServiceUnavailable, Times: 1}
				cl := open(t, serve(t, f), false)
				items, _ := listAll(ctx, t, cl, contractQuery(c))
				if len(items) != 2 {
					t.Fatalf("listed %d after a passing 503, want 2", len(items))
				}
			})

			t.Run("rate_limit_not_retried", func(t *testing.T) {
				f := &contract.Fault{Next: c.fake(t, 1, contractBase), Status: http.StatusTooManyRequests, Times: -1}
				cl := open(t, serve(t, f), false)
				_, err := cl.List(ctx, provider.Inbound, contractQuery(c))
				if provider.KindOf(err) != provider.KindRateLimit {
					t.Fatalf("err = %v (kind %s), want rate_limit", err, provider.KindOf(err))
				}
				if n := f.Seen(); n != 1 {
					t.Fatalf("%d requests after 429, want 1 (the next run waits instead)", n)
				}
			})

			t.Run("timeout", func(t *testing.T) {
				f := &contract.Fault{Next: c.fake(t, 1, contractBase), Delay: 2 * time.Second, Times: -1}
				cl := open(t, serve(t, f), false)
				cctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
				defer cancel()
				start := time.Now()
				_, err := cl.List(cctx, provider.Inbound, contractQuery(c))
				if err == nil {
					t.Fatal("hung integrator returned no error")
				}
				if d := time.Since(start); d > 2*time.Second {
					t.Fatalf("returned after %s, want within the 300ms deadline", d)
				}
			})
		})
	}
}

// liveContract runs the read-only scenarios against a real test system.
func liveContract(t *testing.T, c contractCase, cfg provider.Config) {
	cl, err := Open(c.name, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	to := time.Now()
	q := ListQuery{Kind: provider.EInvoice, From: to.Add(-7 * 24 * time.Hour), To: to, Limit: c.limit}
	items, pages := listAll(ctx, t, cl, q)
	t.Logf("%s live: %d documents in %d pages", c.name, len(items), pages)
	if len(items) == 0 {
		return
	}
	res, err := cl.Fetch(ctx, provider.FetchQuery{Kind: provider.EInvoice, Direction: provider.Inbound, UUID: items[0].UUID, Format: provider.FormatUBL})
	if err != nil || len(res.Data) == 0 {
		t.Fatalf("live fetch: %v", err)
	}
}
