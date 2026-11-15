package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

type engagementResolverFunc func(ctx context.Context, orgID int64, id uuid.UUID, userID int64) (engagementctx.Access, error)

func (f engagementResolverFunc) ResolveEngagement(ctx context.Context, orgID int64, id uuid.UUID, userID int64) (engagementctx.Access, error) {
	return f(ctx, orgID, id, userID)
}

func TestRequireEngagement(t *testing.T) {
	t.Parallel()
	const orgID, assignedUser, otherUser = int64(7), int64(1), int64(2)
	active := uuid.New()
	ended := uuid.New()
	foreign := uuid.New()
	resolver := engagementResolverFunc(func(_ context.Context, org int64, id uuid.UUID, userID int64) (engagementctx.Access, error) {
		if org != orgID {
			return engagementctx.Access{}, engagementctx.ErrNotFound
		}
		switch id {
		case active:
			return engagementctx.Access{ID: 10, UUID: id, OrganizationID: orgID, Status: engagementctx.StatusActive, Assigned: userID == assignedUser}, nil
		case ended:
			return engagementctx.Access{ID: 11, UUID: id, OrganizationID: orgID, Status: engagementctx.StatusEnded, Assigned: userID == assignedUser}, nil
		default:
			return engagementctx.Access{}, engagementctx.ErrNotFound
		}
	})
	memberPerms := []string{rbac.PermTenantEngagementsRead}
	readAllPerms := []string{rbac.PermTenantEngagementsRead, rbac.PermTenantEngagementsReadAll}

	cases := []struct {
		name        string
		ref         string
		user        int64
		perms       []string
		allowEnded  bool
		wantStatus  int
		wantCode    string
		wantReadAll bool
	}{
		{name: "assigned member", ref: active.String(), user: assignedUser, perms: memberPerms, wantStatus: http.StatusOK},
		{name: "unassigned member", ref: active.String(), user: otherUser, perms: memberPerms, wantStatus: http.StatusForbidden, wantCode: response.CodeNoEngagementAccess},
		{name: "unassigned with read_all", ref: active.String(), user: otherUser, perms: readAllPerms, wantStatus: http.StatusOK, wantReadAll: true},
		{name: "unknown engagement", ref: foreign.String(), user: assignedUser, perms: readAllPerms, wantStatus: http.StatusForbidden, wantCode: response.CodeNoEngagementAccess},
		{name: "malformed uuid", ref: "not-a-uuid", user: assignedUser, perms: readAllPerms, wantStatus: http.StatusForbidden, wantCode: response.CodeNoEngagementAccess},
		{name: "ended, assigned", ref: ended.String(), user: assignedUser, perms: memberPerms, wantStatus: http.StatusForbidden, wantCode: response.CodeEngagementEnded},
		{name: "ended, unassigned does not learn it ended", ref: ended.String(), user: otherUser, perms: memberPerms, wantStatus: http.StatusForbidden, wantCode: response.CodeNoEngagementAccess},
		{name: "ended allowed on management route", ref: ended.String(), user: assignedUser, perms: memberPerms, allowEnded: true, wantStatus: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got engagementctx.Scope
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = engagementctx.Must(r.Context())
				w.WriteHeader(http.StatusOK)
			})
			mw := RequireEngagement(resolver)
			if tc.allowEnded {
				mw = RequireEngagementAllowEnded(resolver)
			}
			mux := http.NewServeMux()
			mux.Handle("GET /e/{engagementUuid}", mw(next))

			req := httptest.NewRequest(http.MethodGet, "/e/"+tc.ref, nil)
			req = withPrincipal(req, authctx.Principal{UserInternal: tc.user})
			req = req.WithContext(orgctx.WithScope(req.Context(), orgctx.Scope{ID: orgID, Permissions: tc.perms}))
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.wantCode != "" {
				if code := errorCode(t, rr); code != tc.wantCode {
					t.Fatalf("code = %q, want %q", code, tc.wantCode)
				}
			}
			if tc.wantStatus == http.StatusOK && got.ViaReadAll != tc.wantReadAll {
				t.Fatalf("ViaReadAll = %v, want %v", got.ViaReadAll, tc.wantReadAll)
			}
		})
	}
}

func TestRequireEngagementWithoutOrganization(t *testing.T) {
	t.Parallel()
	mw := RequireEngagement(engagementResolverFunc(func(context.Context, int64, uuid.UUID, int64) (engagementctx.Access, error) {
		t.Fatal("resolver must not run")
		return engagementctx.Access{}, nil
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = withPrincipal(req, authctx.Principal{UserInternal: 1})
	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}
