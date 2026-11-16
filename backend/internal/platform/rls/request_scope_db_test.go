package rls_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

func requestFor(f fixture, user int64) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/v1/tenant/x", nil)
	ctx := authctx.WithPrincipal(r.Context(), authctx.Principal{UserInternal: user})
	ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: f.orgA})
	return r.WithContext(ctx)
}

// TestNarrowAndStreaming (issue #166): a GET answer streams while the scope
// is open; Narrow switches the scope's taxpayers to one; a second
// Middleware inside a scope adds nothing.
func TestNarrowAndStreaming(t *testing.T) {
	f := setup(t)
	mw := rls.Middleware(f.d.Pool, func(context.Context, int64, int64, bool) ([]int64, error) {
		return []int64{f.tpA, f.tpB}, nil
	}, nil)
	var before, after string
	var inner bool
	h := mw(mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tx, ok := dbtx.Tx(r.Context())
		if !ok {
			t.Fatal("no scope")
		}
		_ = tx.QueryRow(r.Context(), `SELECT current_setting('app.taxpayer_ids', true)`).Scan(&before)
		if err := rls.Narrow(r.Context(), f.tpA); err != nil {
			t.Fatal(err)
		}
		_ = tx.QueryRow(r.Context(), `SELECT current_setting('app.taxpayer_ids', true)`).Scan(&after)
		var n int
		_ = tx.QueryRow(r.Context(), `SELECT count(*) FROM pg_stat_activity WHERE pid = pg_backend_pid()`).Scan(&n)
		inner = n == 1
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-stream"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, requestFor(f, f.ownerA))
	if w.Code != http.StatusOK || w.Body.String() != "%PDF-stream" || !w.Flushed {
		t.Fatalf("streamed answer: %d %q flushed=%v", w.Code, w.Body.String(), w.Flushed)
	}
	wantBefore := "{" + strconv.FormatInt(f.tpA, 10) + "," + strconv.FormatInt(f.tpB, 10) + "}"
	if before != wantBefore || after != "{"+strconv.FormatInt(f.tpA, 10)+"}" || !inner {
		t.Fatalf("taxpayer_ids before %q after %q (inner=%v)", before, after, inner)
	}
	// Narrow without a scope is a no-op.
	if err := rls.Narrow(context.Background(), f.tpA); err != nil {
		t.Fatal(err)
	}
}

// TestLegalTextsInScope: published legal texts are readable inside a tenant
// scope, drafts are not; an acceptance is written only for the scope's own
// user and office (migration 000077).
func TestLegalTextsInScope(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var pub, draft int64
	err := rls.System(ctx, f.d.Pool, "test fixture", func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO legal_documents (kind, version, title, body, status, published_at, content_sha256)
			VALUES ('retention_policy', 901, 'R', 'x', 'published', NOW(), repeat('a', 64)) RETURNING id`).Scan(&pub); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO legal_documents (kind, version, title, body, status, content_sha256)
			VALUES ('retention_policy', 902, 'R2', 'y', 'draft', repeat('b', 64)) RETURNING id`).Scan(&draft)
	})
	if err != nil {
		t.Fatal(err)
	}
	scopeA := rls.Scope{OrganizationID: f.orgA, UserID: f.ownerA}
	err = rls.WithScope(ctx, f.d.Pool, scopeA, func(ctx context.Context, tx pgx.Tx) error {
		var ids []int64
		rows, err := tx.Query(ctx, `SELECT id FROM legal_documents ORDER BY id`)
		if err != nil {
			return err
		}
		ids, err = pgx.CollectRows(rows, pgx.RowTo[int64])
		if err != nil {
			return err
		}
		seen := map[int64]bool{}
		for _, id := range ids {
			seen[id] = true
		}
		if !seen[pub] || seen[draft] {
			t.Errorf("scope sees legal documents %v: published %d visible=%v, draft %d visible=%v", ids, pub, seen[pub], draft, seen[draft])
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO legal_acceptances (document_id, user_id, organization_id, context, content_sha256)
			VALUES ($1, $2, $3, 'office_dpa', repeat('a', 64))`, pub, f.ownerA, f.orgA)
		return err
	})
	if err != nil {
		t.Fatalf("own office acceptance: %v", err)
	}
	// Taxpayer scope (mükellef panel): a history share consent names the
	// receiving office; allowed for its own taxpayer only (migration 000080).
	history := func(scope rls.Scope, user, org, tp int64) error {
		return rls.WithScope(ctx, f.d.Pool, scope, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO legal_acceptances (document_id, user_id, organization_id, taxpayer_id, context, content_sha256)
				VALUES ($1, $2, $3, $4, 'history_share', repeat('a', 64))`, pub, user, org, tp)
			return err
		})
	}
	tpScope := rls.Scope{UserID: f.ownerA, TaxpayerIDs: []int64{f.tpA}}
	if err := history(tpScope, f.ownerA, f.orgB, f.tpA); err != nil {
		t.Errorf("taxpayer A's history consent naming office B: %v", err)
	}
	if err := history(tpScope, f.ownerA, f.orgB, f.tpB); !isRLSError(err) {
		t.Errorf("history consent for another taxpayer: %v, want a row level security violation", err)
	}
	for name, args := range map[string][]any{
		"another office": {pub, f.ownerA, f.orgB},
		"another user":   {pub, f.ownerB, f.orgA},
	} {
		err = rls.WithScope(ctx, f.d.Pool, scopeA, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO legal_acceptances (document_id, user_id, organization_id, context, content_sha256)
				VALUES ($1, $2, $3, 'office_dpa', repeat('a', 64))`, args...)
			return err
		})
		if !isRLSError(err) {
			t.Errorf("%s acceptance: %v, want a row level security violation", name, err)
		}
	}
}

func isRLSError(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "42501"
}
