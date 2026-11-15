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

// RegisterOnboardingRoutes mounts the add-mükellef flow (issue #7):
//
//   - office side: /v1/tenant/taxpayer-requests (one answer for every tax id);
//   - consent link: /v1/taxpayer-consents/preview, /approve, /reject;
//   - owners in the mükellef panel: /v1/taxpayer/onboarding-requests.
func RegisterOnboardingRoutes(
	mux *http.ServeMux,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver,
	tpResolver taxpayerctx.Resolver,
	h *OnboardingHandler,
) {
	if orgResolver == nil || tpResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(orgResolver)
	perm := middleware.RequireTenantPermission
	tp := middleware.RequireTaxpayer(tpResolver)

	mux.Handle("GET /v1/tenant/taxpayer-requests", middleware.Chain(
		http.HandlerFunc(h.listRequests), authn, org, perm(rbac.PermTenantEngagementsRead),
	))
	mux.Handle("POST /v1/tenant/taxpayer-requests", middleware.Chain(
		http.HandlerFunc(h.createRequest), authn, org, perm(rbac.PermTenantEngagementsWrite),
	))
	mux.Handle("DELETE /v1/tenant/taxpayer-requests/{uuid}", middleware.Chain(
		http.HandlerFunc(h.cancelRequest), authn, org, perm(rbac.PermTenantEngagementsWrite),
	))

	mux.Handle("POST /v1/taxpayer-consents/preview", http.HandlerFunc(h.previewConsent))
	mux.Handle("POST /v1/taxpayer-consents/approve", middleware.Chain(http.HandlerFunc(h.approveConsent), authn))
	mux.Handle("POST /v1/taxpayer-consents/reject", middleware.Chain(http.HandlerFunc(h.rejectConsent), authn))

	mux.Handle("GET /v1/taxpayer/onboarding-requests", middleware.Chain(http.HandlerFunc(h.ownerPending), authn, tp))
	mux.Handle("POST /v1/taxpayer/onboarding-requests/{uuid}/approve", middleware.Chain(http.HandlerFunc(h.ownerApprove), authn, tp))
	mux.Handle("POST /v1/taxpayer/onboarding-requests/{uuid}/reject", middleware.Chain(http.HandlerFunc(h.ownerReject), authn, tp))
}

// RegisterHistoryRoutes mounts the office-change routes (issue #8, D2):
//
//   - mükellef owners: /v1/taxpayer/history-shares (share / list / revoke);
//   - office: documents the engagement may see, and the shares it holds.
//     Documents go through RequireEngagementArchive: an ended engagement
//     reads its own period while its archive window is open.
func RegisterHistoryRoutes(
	mux *http.ServeMux,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver,
	engResolver engagementctx.Resolver,
	tpResolver taxpayerctx.Resolver,
	h *HistoryHandler,
) {
	if orgResolver == nil || engResolver == nil || tpResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(orgResolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead)
	archive := middleware.RequireEngagementArchive(engResolver)
	eng := middleware.RequireEngagementAllowEnded(engResolver)
	tp := middleware.RequireTaxpayer(tpResolver)

	mux.Handle("GET /v1/taxpayer/history-shares", middleware.Chain(http.HandlerFunc(h.list), authn, tp))
	mux.Handle("POST /v1/taxpayer/history-shares", middleware.Chain(http.HandlerFunc(h.share), authn, tp))
	mux.Handle("DELETE /v1/taxpayer/history-shares/{uuid}", middleware.Chain(http.HandlerFunc(h.revoke), authn, tp))

	mux.Handle("GET /v1/tenant/engagements/{engagementUuid}/documents", middleware.Chain(
		http.HandlerFunc(h.officeDocuments), authn, org, read, archive,
	))
	mux.Handle("GET /v1/tenant/engagements/{engagementUuid}/history-shares", middleware.Chain(
		http.HandlerFunc(h.officeShares), authn, org, read, eng,
	))
}
