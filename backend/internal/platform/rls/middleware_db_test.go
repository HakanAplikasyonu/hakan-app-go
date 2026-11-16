package rls_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// TestMiddlewareCommitsOnlySuccessfulAnswers (issue #166): the request runs
// in one scoped transaction; a 2xx commits, a 4xx rolls back and still
// reaches the client, and an office-wide route gets the office's taxpayers.
func TestMiddlewareCommitsOnlySuccessfulAnswers(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var lister []int64
	mw := rls.Middleware(f.d.Pool, func(_ context.Context, org, _ int64, readAll bool) ([]int64, error) {
		if org != f.orgA || !readAll {
			t.Errorf("lister called with org=%d readAll=%v", org, readAll)
		}
		return []int64{f.tpA}, nil
	}, nil)

	insertPetition := func(status int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			tx, ok := dbtx.Tx(r.Context())
			if !ok {
				t.Fatal("no ambient transaction in the handler")
			}
			if _, err := tx.Exec(r.Context(), `
				INSERT INTO petitions (organization_id, engagement_id, template_key, template_version, rendered, draft)
				VALUES ($1, $2, 'genel_dilekce', 1, '{}'::jsonb, true)`, f.orgA, f.engA); err != nil {
				t.Fatalf("insert in scope: %v", err)
			}
			w.Header().Set("X-Test", "yes")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}
	serve := func(h http.Handler) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/v1/tenant/x", nil)
		rctx := authctx.WithPrincipal(r.Context(), authctx.Principal{UserInternal: f.ownerA})
		rctx = orgctx.WithScope(rctx, orgctx.Scope{ID: f.orgA, Permissions: []string{rbac.PermTenantEngagementsReadAll}})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r.WithContext(rctx))
		return w
	}
	count := func() int {
		var n int
		err := rls.System(ctx, f.d.Pool, "test: count", func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM petitions WHERE organization_id = $1`, f.orgA).Scan(&n)
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	w := serve(mw(insertPetition(http.StatusCreated)))
	if w.Code != http.StatusCreated || w.Header().Get("X-Test") != "yes" || w.Body.String() != `{"ok":true}` {
		t.Fatalf("2xx answer: %d %v %q", w.Code, w.Header(), w.Body.String())
	}
	if n := count(); n != 1 {
		t.Fatalf("after 2xx: %d petitions, want 1 (committed)", n)
	}

	w = serve(mw(insertPetition(http.StatusUnprocessableEntity)))
	if w.Code != http.StatusUnprocessableEntity || w.Body.String() != `{"ok":true}` {
		t.Fatalf("4xx answer: %d %q", w.Code, w.Body.String())
	}
	if n := count(); n != 1 {
		t.Fatalf("after 4xx: %d petitions, want 1 (rolled back)", n)
	}

	// Office-wide route: the scope carries the office's taxpayers.
	w = serve(mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tx, _ := dbtx.Tx(r.Context())
		var got string
		if err := tx.QueryRow(r.Context(), `SELECT current_setting('app.taxpayer_ids', true)`).Scan(&got); err != nil {
			t.Fatal(err)
		}
		lister = append(lister, int64(len(got)))
		if want := "{" + strconv.FormatInt(f.tpA, 10) + "}"; got != want {
			t.Errorf("app.taxpayer_ids = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	if w.Code != http.StatusNoContent || len(lister) != 1 {
		t.Fatalf("office route: %d, lister %v", w.Code, lister)
	}
}
