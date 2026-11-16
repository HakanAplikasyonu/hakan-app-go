package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
)

type policyFunc func(r *http.Request) error

func (f policyFunc) Check(_ context.Context, _ orgctx.Scope, r *http.Request) error { return f(r) }

// Not parallel: the policy is process-wide (set once at start-up).
func TestOrganizationPolicy(t *testing.T) {
	SetOrganizationPolicy(policyFunc(func(r *http.Request) error {
		if r.Method == http.MethodPost {
			return errors.New("salt okunur")
		}
		return nil
	}))
	t.Cleanup(func() { SetOrganizationPolicy(nil) })
	resolver := resolverFunc(func(context.Context, string, int64) (orgctx.Scope, error) { return orgctx.Scope{ID: 7}, nil })
	h := RequireOrganization(resolver)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	for method, want := range map[string]int{http.MethodGet: http.StatusOK, http.MethodPost: http.StatusForbidden} {
		r := withPrincipal(httptest.NewRequest(method, "/v1/tenant/x", nil), authctx.Principal{UserInternal: 1})
		r.Header.Set(orgctx.HeaderName, "acme")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		if rr.Code != want {
			t.Fatalf("%s: %d", method, rr.Code)
		}
		if want == http.StatusForbidden && errorCode(t, rr) != CodeOrganizationReadOnly {
			t.Fatalf("code %s", errorCode(t, rr))
		}
	}
}
