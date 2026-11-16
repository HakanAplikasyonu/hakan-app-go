package parasut

// Contract test (#196): the Paraşüt client against the JSON:API fake
// (docs/kaynak 10): OAuth refresh token → access token, page[number] /
// page[size] pages, 401 → one refresh, 429 → backoff. CONTRACT_LIVE=1 with
// CONTRACT_PARASUT_CLIENT_ID, _CLIENT_SECRET, _REFRESH_TOKEN, _COMPANY_ID
// (and optional _ENDPOINT) runs the read-only listing against the sandbox.
// See docs/entegrator-sozlesme-testleri.md.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testutil/contract"
)

func contractFake(n int) *fakeParasut {
	f := &fakeParasut{}
	for i := range n {
		f.add(&f.sales, resource{ID: fmt.Sprint(i + 1), Type: "sales_invoices",
			Attributes: attrs(map[string]any{"issue_date": fmt.Sprintf("2026-10-%02d", i%28+1)})})
	}
	return f
}

func contractClient(url, secret string) *Client {
	c := NewClient(App{BaseURL: url, ClientID: "cid", ClientSecret: secret}, nil, "777", Tokens{RefreshToken: "ref0"})
	c.Interval, c.Backoff = 0, time.Millisecond
	return c
}

func walk(ctx context.Context, c *Client) (int, error) {
	n := 0
	err := c.pages(ctx, "/sales_invoices", url.Values{"sort": {"-issue_date"}}, func(resource, index) (bool, error) {
		n++
		return false, nil
	})
	return n, err
}

var listCalls = contract.PathPrefix("/v4/777/")

func TestParasutContract(t *testing.T) {
	ctx := context.Background()
	if env, ok := contract.Live("PARASUT", "CLIENT_ID", "CLIENT_SECRET", "REFRESH_TOKEN", "COMPANY_ID"); ok {
		t.Run("live", func(t *testing.T) {
			app := App{BaseURL: contract.Optional("PARASUT", "ENDPOINT"), ClientID: env["CLIENT_ID"], ClientSecret: env["CLIENT_SECRET"]}
			c := NewClient(app, nil, env["COMPANY_ID"], Tokens{RefreshToken: env["REFRESH_TOKEN"]})
			n, err := walk(ctx, c)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("parasut live: %d sales invoices", n)
		})
	}
	serve := func(t *testing.T, h http.Handler) string {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		return srv.URL
	}

	t.Run("pagination", func(t *testing.T) {
		n, err := walk(ctx, contractClient(serve(t, contractFake(2*PageSize+3).handler()), "sec"))
		if err != nil || n != 2*PageSize+3 {
			t.Fatalf("walked %d, %v; want %d over 3 pages", n, err, 2*PageSize+3)
		}
	})
	t.Run("auth_refused", func(t *testing.T) {
		if _, err := walk(ctx, contractClient(serve(t, contractFake(1).handler()), "wrong")); !errors.Is(err, ErrAuth) {
			t.Fatalf("err = %v, want ErrAuth", err)
		}
	})
	t.Run("revoked_token_refreshed_once", func(t *testing.T) {
		f := contractFake(1)
		fault := &contract.Fault{Next: f.handler(), Status: http.StatusUnauthorized, Times: 1, Match: listCalls}
		n, err := walk(ctx, contractClient(serve(t, fault), "sec"))
		if err != nil || n != 1 || f.refreshs != 2 {
			t.Fatalf("walked %d, %v, refreshes %d; want one extra refresh", n, err, f.refreshs)
		}
	})
	t.Run("rate_limit_backoff", func(t *testing.T) {
		fault := &contract.Fault{Next: contractFake(1).handler(), Status: http.StatusTooManyRequests, Times: 2, Match: listCalls}
		if n, err := walk(ctx, contractClient(serve(t, fault), "sec")); err != nil || n != 1 {
			t.Fatalf("walked %d, %v after 429", n, err)
		}
	})
	t.Run("server_error_reported", func(t *testing.T) {
		fault := &contract.Fault{Next: contractFake(1).handler(), Status: http.StatusInternalServerError, Times: -1, Match: listCalls}
		if _, err := walk(ctx, contractClient(serve(t, fault), "sec")); err == nil || errors.Is(err, ErrAuth) {
			t.Fatalf("err = %v, want an HTTP error", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		fault := &contract.Fault{Next: contractFake(1).handler(), Delay: time.Second, Times: -1, Match: listCalls}
		cctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		start := time.Now()
		if _, err := walk(cctx, contractClient(serve(t, fault), "sec")); err == nil {
			t.Fatal("hung server returned no error")
		}
		if d := time.Since(start); d > time.Second {
			t.Fatalf("returned after %s", d)
		}
	})
}
