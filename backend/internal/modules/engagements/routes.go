package engagements

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/tenant/engagements. A nil orgResolver (tenancy
// off) registers nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver,
	engResolver engagementctx.Resolver,
	h *Handler,
) {
	if orgResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(orgResolver)
	perm := middleware.RequireTenantPermission
	eng := middleware.RequireEngagement(engResolver)
	engAny := middleware.RequireEngagementAllowEnded(engResolver)
	read := perm(rbac.PermTenantEngagementsRead)
	write := perm(rbac.PermTenantEngagementsWrite)

	mux.Handle("GET /v1/tenant/engagements", middleware.Chain(http.HandlerFunc(h.list), authn, org, read))
	mux.Handle("POST /v1/tenant/engagements", middleware.Chain(http.HandlerFunc(h.create), authn, org, write))

	mux.Handle("GET /v1/tenant/engagements/{engagementUuid}", middleware.Chain(http.HandlerFunc(h.get), authn, org, read, engAny))
	// Mükellef data routes follow this chain: active engagement only.
	mux.Handle("GET /v1/tenant/engagements/{engagementUuid}/context", middleware.Chain(http.HandlerFunc(h.context), authn, org, read, eng))
	mux.Handle("POST /v1/tenant/engagements/{engagementUuid}/end", middleware.Chain(http.HandlerFunc(h.end), authn, org, write, engAny))

	mux.Handle("GET /v1/tenant/engagements/{engagementUuid}/assignments", middleware.Chain(http.HandlerFunc(h.listAssignments), authn, org, read, engAny))
	mux.Handle("PUT /v1/tenant/engagements/{engagementUuid}/assignments/{userUuid}", middleware.Chain(http.HandlerFunc(h.assign), authn, org, write, eng))
	mux.Handle("DELETE /v1/tenant/engagements/{engagementUuid}/assignments/{userUuid}", middleware.Chain(http.HandlerFunc(h.unassign), authn, org, write, engAny))
}
