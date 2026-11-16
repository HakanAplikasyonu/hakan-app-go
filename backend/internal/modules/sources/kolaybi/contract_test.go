package kolaybi

// Contract test (#196): the KolayBi client against a fake built from the
// documented requests (docs/kaynak 11): api_key → access token, Bearer +
// Channel headers, {data, meta{current_page,last_page}} pages, 401 → one
// new token, 429 → backoff. CONTRACT_LIVE=1 with CONTRACT_KOLAYBI_API_KEY,
// _CHANNEL (and optional _ENDPOINT) runs the read-only listing against the
// sandbox. See docs/entegrator-sozlesme-testleri.md.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testutil/contract"
)

type fakeKolayBi struct {
	mu        sync.Mutex
	rows      int
	perPage   int
	logins    int
	expireOne bool // the first data call answers 401 (token revoked)
}

func (f *fakeKolayBi) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Channel") != "test-channel" {
		http.Error(w, "channel", http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/kolaybi/v1/access_token" {
		var in struct {
			APIKey string `json:"api_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.APIKey != "good-key" {
			http.Error(w, `{"message":"invalid api key"}`, http.StatusUnauthorized)
			return
		}
		f.logins++
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"access_token": "tok" + strconv.Itoa(f.logins)}})
		return
	}
	if r.Header.Get("Authorization") != "Bearer tok"+strconv.Itoa(f.logins) {
		http.Error(w, "token", http.StatusUnauthorized)
		return
	}
	if f.expireOne {
		f.expireOne = false
		f.logins += 100 // the current token stops working
		http.Error(w, "expired", http.StatusUnauthorized)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	last := (f.rows + f.perPage - 1) / f.perPage
	var data []map[string]any
	for i := (page - 1) * f.perPage; i < min(page*f.perPage, f.rows); i++ {
		data = append(data, map[string]any{"id": strconv.Itoa(i + 1)})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "meta": map[string]any{"current_page": page, "last_page": last}})
}

var dataCalls = func(r *http.Request) bool { return r.URL.Path != "/kolaybi/v1/access_token" }

func testClient(url, key string) *Client {
	c := NewClient(App{BaseURL: url, Channel: "test-channel"}, nil, key, "1")
	c.Interval, c.Backoff = 0, time.Millisecond
	return c
}

func collect(ctx context.Context, c *Client) ([]string, error) {
	var ids []string
	err := c.pages(ctx, "/documents", url.Values{}, func(o obj) error {
		ids = append(ids, o.pick("id"))
		return nil
	})
	return ids, err
}

func TestKolayBiContract(t *testing.T) {
	ctx := context.Background()
	if env, ok := contract.Live("KOLAYBI", "API_KEY", "CHANNEL"); ok {
		t.Run("live", func(t *testing.T) {
			c := NewClient(App{BaseURL: contract.Optional("KOLAYBI", "ENDPOINT"), Channel: env["CHANNEL"]}, nil, env["API_KEY"], "")
			cos, err := c.Companies(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("kolaybi live: %d companies", len(cos))
		})
	}
	serve := func(t *testing.T, h http.Handler) string {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		return srv.URL
	}

	t.Run("pagination", func(t *testing.T) {
		ids, err := collect(ctx, testClient(serve(t, &fakeKolayBi{rows: 5, perPage: 2}), "good-key"))
		if err != nil || len(ids) != 5 {
			t.Fatalf("ids %v, %v; want 5 over 3 pages", ids, err)
		}
	})
	t.Run("auth_refused", func(t *testing.T) {
		_, err := collect(ctx, testClient(serve(t, &fakeKolayBi{rows: 1, perPage: 2}), "bad-key"))
		if !errors.Is(err, ErrAuth) {
			t.Fatalf("err = %v, want ErrAuth", err)
		}
	})
	t.Run("revoked_token_renewed_once", func(t *testing.T) {
		f := &fakeKolayBi{rows: 1, perPage: 2, expireOne: true}
		ids, err := collect(ctx, testClient(serve(t, f), "good-key"))
		if err != nil || len(ids) != 1 {
			t.Fatalf("ids %v, %v", ids, err)
		}
	})
	t.Run("rate_limit_backoff", func(t *testing.T) {
		f := &contract.Fault{Next: &fakeKolayBi{rows: 1, perPage: 2}, Status: http.StatusTooManyRequests, Times: 2, Match: dataCalls}
		ids, err := collect(ctx, testClient(serve(t, f), "good-key"))
		if err != nil || len(ids) != 1 {
			t.Fatalf("ids %v, %v after 429", ids, err)
		}
	})
	t.Run("server_error_reported", func(t *testing.T) {
		f := &contract.Fault{Next: &fakeKolayBi{rows: 1, perPage: 2}, Status: http.StatusBadGateway, Times: -1, Match: dataCalls}
		if _, err := collect(ctx, testClient(serve(t, f), "good-key")); err == nil || errors.Is(err, ErrAuth) {
			t.Fatalf("err = %v, want an HTTP error", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		f := &contract.Fault{Next: &fakeKolayBi{rows: 1, perPage: 2}, Delay: time.Second, Times: -1, Match: dataCalls}
		cctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		start := time.Now()
		if _, err := collect(cctx, testClient(serve(t, f), "good-key")); err == nil {
			t.Fatal("hung server returned no error")
		}
		if d := time.Since(start); d > time.Second {
			t.Fatalf("returned after %s", d)
		}
	})
}
