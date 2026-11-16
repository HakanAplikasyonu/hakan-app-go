package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
)

// TestTenantScopeWrapsAfterOrganization (issue #166): the scope set with
// SetTenantScope wraps everything after RequireOrganization and sees the
// resolved organization; a refused request never reaches it. Not parallel:
// it sets the package-level hook.
func TestTenantScopeWrapsAfterOrganization(t *testing.T) {
	var sawOrg int64
	calls := 0
	SetTenantScope(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if s, ok := orgctx.From(r.Context()); ok {
				sawOrg = s.ID
			}
			next.ServeHTTP(w, r)
		})
	})
	defer SetTenantScope(nil)

	res := resolverFunc(func(_ context.Context, ref string, _ int64) (orgctx.Scope, error) {
		if ref != "acme" {
			return orgctx.Scope{}, orgctx.ErrOrganizationNotFound
		}
		return orgctx.Scope{ID: 7, Slug: "acme", Status: orgctx.StatusActive}, nil
	})
	reached := false
	h := RequireOrganization(res)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	}))
	do := func(ref string) int {
		r := httptest.NewRequest(http.MethodGet, "/v1/tenant/x", nil)
		r.Header.Set(orgctx.HeaderName, ref)
		r = r.WithContext(authctx.WithPrincipal(r.Context(), authctx.Principal{UserInternal: 1}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if code := do("acme"); code != http.StatusNoContent || !reached || calls != 1 || sawOrg != 7 {
		t.Fatalf("member: %d reached=%v calls=%d org=%d", code, reached, calls, sawOrg)
	}
	if code := do("other"); code != http.StatusForbidden || calls != 1 {
		t.Fatalf("refused request: %d, scope calls %d (want no new scope)", code, calls)
	}
}
