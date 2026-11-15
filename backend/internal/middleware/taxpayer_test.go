package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

type taxpayerResolverFunc func(ctx context.Context, id uuid.UUID, userID int64) (taxpayerctx.Scope, error)

func (f taxpayerResolverFunc) ResolveTaxpayer(ctx context.Context, id uuid.UUID, userID int64) (taxpayerctx.Scope, error) {
	return f(ctx, id, userID)
}

func TestRequireTaxpayer(t *testing.T) {
	t.Parallel()
	const member = int64(1)
	known := uuid.New()
	resolver := taxpayerResolverFunc(func(_ context.Context, id uuid.UUID, userID int64) (taxpayerctx.Scope, error) {
		if id == known && userID == member {
			return taxpayerctx.Scope{ID: 5, UUID: id, Role: taxpayerctx.RoleOwner}, nil
		}
		return taxpayerctx.Scope{}, taxpayerctx.ErrNotMember
	})
	cases := []struct {
		name     string
		header   string
		user     int64
		noAuth   bool
		wantCode int
		wantErr  string
	}{
		{"member", known.String(), member, false, http.StatusOK, ""},
		{"no header", "", member, false, http.StatusBadRequest, response.CodeTaxpayerRequired},
		{"malformed", "not-a-uuid", member, false, http.StatusForbidden, response.CodeNoTaxpayerMembership},
		{"unknown taxpayer", uuid.NewString(), member, false, http.StatusForbidden, response.CodeNoTaxpayerMembership},
		{"not a member", known.String(), 2, false, http.StatusForbidden, response.CodeNoTaxpayerMembership},
		{"unauthenticated", known.String(), 0, true, http.StatusUnauthorized, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got taxpayerctx.Scope
			h := RequireTaxpayer(resolver)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = taxpayerctx.Must(r.Context())
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, "/v1/taxpayer/context", nil)
			if tc.header != "" {
				req.Header.Set(taxpayerctx.HeaderName, tc.header)
			}
			if !tc.noAuth {
				req = withPrincipal(req, authctx.Principal{UserInternal: tc.user})
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.wantCode, rec.Body.String())
			}
			if tc.wantCode == http.StatusOK && got.ID != 5 {
				t.Fatalf("scope = %+v", got)
			}
			if tc.wantErr != "" {
				var env struct {
					Error struct {
						Code string `json:"code"`
					} `json:"error"`
				}
				_ = json.Unmarshal(rec.Body.Bytes(), &env)
				if env.Error.Code != tc.wantErr {
					t.Fatalf("code = %q, want %q (%s)", env.Error.Code, tc.wantErr, rec.Body.String())
				}
			}
		})
	}
}
