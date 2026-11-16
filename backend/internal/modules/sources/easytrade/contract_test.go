package easytrade

// Contract test (#196): the Easytrade client against a fake built from the
// documented requests (docs/kaynak 09): Bearer token + X-Company-ID, page /
// per_page / meta.last_page lists, 401 → /auth/refresh once, 403
// LICENSE_EXPIRED → ErrLicense. CONTRACT_LIVE=1 with
// CONTRACT_EASYTRADE_EMAIL, _PASSWORD (and optional _ENDPOINT) runs the
// read-only sign-in and branch listing against the sandbox.
// See docs/entegrator-sozlesme-testleri.md.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testutil/contract"
)

type fakeContractET struct {
	mu        sync.Mutex
	rows      int
	token     string
	refreshes int
	license   bool
}

func (f *fakeContractET) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	js := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	if r.Header.Get("Authorization") != "Bearer "+f.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/auth/refresh" {
		f.refreshes++
		f.token = "tok" + strconv.Itoa(f.refreshes+1)
		js(map[string]any{"data": map[string]any{"token": f.token, "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)}})
		return
	}
	if !f.license {
		w.WriteHeader(http.StatusForbidden)
		js(map[string]any{"error_code": "LICENSE_EXPIRED"})
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	last := max((f.rows+per-1)/per, 1)
	var data []map[string]any
	for i := (page - 1) * per; i < min(page*per, f.rows); i++ {
		data = append(data, map[string]any{"id": i + 1, "issue_date": "2026-10-15"})
	}
	js(map[string]any{"data": data, "meta": map[string]any{"current_page": page, "last_page": last}})
}

func contractET(url string, tok string) *Client {
	c := NewClient(url, nil, "42", Token{Value: tok, ExpiresAt: time.Now().Add(time.Hour)})
	c.Interval = 0
	return c
}

var (
	octFrom = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	octTo   = time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
)

func TestEasytradeContract(t *testing.T) {
	ctx := context.Background()
	if env, ok := contract.Live("EASYTRADE", "EMAIL", "PASSWORD"); ok {
		t.Run("live", func(t *testing.T) {
			base := contract.Optional("EASYTRADE", "ENDPOINT")
			tok, err := Login(ctx, base, nil, env["EMAIL"], env["PASSWORD"])
			if err != nil {
				t.Fatal(err)
			}
			br, err := NewClient(base, nil, contract.Optional("EASYTRADE", "COMPANY_ID"), tok).Branches(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("easytrade live: %d branches", len(br))
		})
	}
	serve := func(t *testing.T, h http.Handler) string {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		return srv.URL
	}
	list := func(ctx context.Context, c *Client) ([]string, error) {
		return c.listIDs(ctx, "/company/invoices/sales", "issue_date", octFrom, octTo)
	}

	t.Run("pagination", func(t *testing.T) {
		ids, err := list(ctx, contractET(serve(t, &fakeContractET{rows: 2*PerPage + 5, token: "tok1", license: true}), "tok1"))
		if err != nil || len(ids) != 2*PerPage+5 {
			t.Fatalf("%d ids, %v; want %d over 3 pages", len(ids), err, 2*PerPage+5)
		}
	})
	t.Run("expired_token_refreshed_once", func(t *testing.T) {
		f := &fakeContractET{rows: 3, token: "tok1", license: true}
		f.token = "rotated" // the stored token no longer works; refresh with it fails too
		if _, err := list(ctx, contractET(serve(t, f), "tok1")); !errors.Is(err, ErrAuth) {
			t.Fatalf("err = %v, want ErrAuth when refresh is refused", err)
		}
		g := &fakeContractET{rows: 3, token: "tok1", license: true}
		c := contractET(serve(t, &contract.Fault{Next: g, Status: http.StatusUnauthorized, Times: 1, Match: contract.PathPrefix("/company/")}), "tok1")
		ids, err := list(ctx, c)
		if err != nil || len(ids) != 3 || g.refreshes != 1 {
			t.Fatalf("%d ids, %v, %d refreshes; want one refresh then success", len(ids), err, g.refreshes)
		}
	})
	t.Run("license_expired", func(t *testing.T) {
		if _, err := list(ctx, contractET(serve(t, &fakeContractET{rows: 1, token: "tok1"}), "tok1")); !errors.Is(err, ErrLicense) {
			t.Fatalf("err = %v, want ErrLicense", err)
		}
	})
	t.Run("rate_limit_and_server_error_reported", func(t *testing.T) {
		for _, st := range []int{http.StatusTooManyRequests, http.StatusInternalServerError} {
			f := &contract.Fault{Next: &fakeContractET{rows: 1, token: "tok1", license: true}, Status: st, Times: -1}
			_, err := list(ctx, contractET(serve(t, f), "tok1"))
			if err == nil || errors.Is(err, ErrAuth) {
				t.Fatalf("HTTP %d: err = %v", st, err)
			}
			if f.Seen() != 1 {
				t.Fatalf("HTTP %d: %d requests; the sync run retries, not the client", st, f.Seen())
			}
		}
	})
	t.Run("timeout", func(t *testing.T) {
		f := &contract.Fault{Next: &fakeContractET{rows: 1, token: "tok1", license: true}, Delay: time.Second, Times: -1}
		cctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		start := time.Now()
		if _, err := list(cctx, contractET(serve(t, f), "tok1")); err == nil {
			t.Fatal("hung server returned no error")
		}
		if d := time.Since(start); d > time.Second {
			t.Fatalf("returned after %s", d)
		}
	})
}
