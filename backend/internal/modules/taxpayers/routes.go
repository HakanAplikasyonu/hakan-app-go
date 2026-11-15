package taxpayers

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
)

// RegisterPlatformRoutes mounts the platform taxpayer catalog
// (/v1/platform/taxpayers). Only platform.taxpayers.* holders reach it.
func RegisterPlatformRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader, h *Handler) {
	if h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	read := middleware.RequirePermission(rbac.PermPlatformTaxpayersRead)
	write := middleware.RequirePermission(rbac.PermPlatformTaxpayersWrite)

	mux.Handle("GET /v1/platform/taxpayers", middleware.Chain(http.HandlerFunc(h.list), authn, read))
	mux.Handle("POST /v1/platform/taxpayers", middleware.Chain(http.HandlerFunc(h.create), authn, write))
	mux.Handle("GET /v1/platform/taxpayers/{uuid}", middleware.Chain(http.HandlerFunc(h.get), authn, read))
	mux.Handle("PATCH /v1/platform/taxpayers/{uuid}", middleware.Chain(http.HandlerFunc(h.patch), authn, write))
}

// RegisterTenantRoutes mounts the office-side taxpayer profile of an
// engagement. A nil orgResolver (tenancy off) registers nothing.
func RegisterTenantRoutes(
	mux *http.ServeMux,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver,
	engResolver engagementctx.Resolver,
	h *Handler,
) {
	if orgResolver == nil || engResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(orgResolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead)
	eng := middleware.RequireEngagement(engResolver)

	mux.Handle("GET /v1/tenant/engagements/{engagementUuid}/taxpayer", middleware.Chain(
		http.HandlerFunc(h.engagementTaxpayer), authn, org, read, eng,
	))
}

// RegisterMemberRoutes mounts the mükellef user routes (issue #4):
//
//   - office side: invitations of an active engagement
//     (/v1/tenant/engagements/{uuid}/invitations), D15;
//   - invitation links (/v1/taxpayer-invitations/preview, /accept);
//   - the mükellef shell (/v1/taxpayer/*), scoped by X-Taxpayer.
//
// /v1/taxpayer/* never accepts office membership, and /v1/tenant/* never
// accepts taxpayer membership: a mükellef user who is not an office member
// gets 403 NO_TENANT_MEMBERSHIP there.
func RegisterMemberRoutes(
	mux *http.ServeMux,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver,
	engResolver engagementctx.Resolver,
	tpResolver taxpayerctx.Resolver,
	h *MemberHandler,
) {
	if orgResolver == nil || engResolver == nil || tpResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(orgResolver)
	perm := middleware.RequireTenantPermission
	eng := middleware.RequireEngagement(engResolver)
	tp := middleware.RequireTaxpayer(tpResolver)

	mux.Handle("GET /v1/tenant/engagements/{engagementUuid}/invitations", middleware.Chain(
		http.HandlerFunc(h.listInvitations), authn, org, perm(rbac.PermTenantEngagementsRead), eng,
	))
	mux.Handle("POST /v1/tenant/engagements/{engagementUuid}/invitations", middleware.Chain(
		http.HandlerFunc(h.createInvitation), authn, org, perm(rbac.PermTenantEngagementsWrite), eng,
	))
	mux.Handle("DELETE /v1/tenant/engagements/{engagementUuid}/invitations/{uuid}", middleware.Chain(
		http.HandlerFunc(h.revokeInvitation), authn, org, perm(rbac.PermTenantEngagementsWrite), eng,
	))

	mux.Handle("POST /v1/taxpayer-invitations/preview", http.HandlerFunc(h.previewInvitation))
	mux.Handle("POST /v1/taxpayer-invitations/accept", middleware.Chain(http.HandlerFunc(h.acceptInvitation), authn))

	mux.Handle("GET /v1/taxpayer/context", middleware.Chain(http.HandlerFunc(h.taxpayerContext), authn, tp))
}
