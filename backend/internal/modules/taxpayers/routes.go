package taxpayers

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
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
