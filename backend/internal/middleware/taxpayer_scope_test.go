package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
)

// TestTaxpayerScopeWrapsAfterTaxpayer (issue #166): the scope set with
// SetTaxpayerScope wraps everything after RequireTaxpayer and sees the
// resolved taxpayer; a refused request never reaches it. Not parallel: it
// sets the package-level hook.
func TestTaxpayerScopeWrapsAfterTaxpayer(t *testing.T) {
	member := uuid.New()
	var saw int64
	calls := 0
	SetTaxpayerScope(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if s, ok := taxpayerctx.From(r.Context()); ok {
				saw = s.ID
			}
			next.ServeHTTP(w, r)
		})
	})
	defer SetTaxpayerScope(nil)
	res := taxpayerResolverFunc(func(_ context.Context, id uuid.UUID, _ int64) (taxpayerctx.Scope, error) {
		if id != member {
			return taxpayerctx.Scope{}, taxpayerctx.ErrNotMember
		}
		return taxpayerctx.Scope{ID: 42, UUID: id}, nil
	})
	h := RequireTaxpayer(res)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	do := func(ref string) int {
		r := httptest.NewRequest(http.MethodGet, "/v1/taxpayer/x", nil)
		r.Header.Set(taxpayerctx.HeaderName, ref)
		r = r.WithContext(authctx.WithPrincipal(r.Context(), authctx.Principal{UserInternal: 1}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if code := do(member.String()); code != http.StatusNoContent || calls != 1 || saw != 42 {
		t.Fatalf("member: %d calls=%d taxpayer=%d", code, calls, saw)
	}
	if code := do(uuid.NewString()); code != http.StatusForbidden || calls != 1 {
		t.Fatalf("not a member: %d, scope calls %d", code, calls)
	}
}
