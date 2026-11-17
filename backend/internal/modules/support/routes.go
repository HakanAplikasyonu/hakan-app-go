package support

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Module mounts the routes.
type Module struct {
	H         *Handler
	Taxpayers taxpayerctx.Resolver
}

// Name implements module.Module.
func (Module) Name() string { return "support" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	// Ayar her üyede okunur (tenant.settings.read üye rolünde de var);
	// yazmak ofis sahibi / yöneticisinin.
	r.Tenant("GET /v1/tenant/support-settings", rbac.PermTenantSettingsRead, h.GetSettings)
	r.Tenant("PUT /v1/tenant/support-settings", rbac.PermTenantSettingsWrite, h.PutSettings)
	r.Tenant("GET /v1/tenant/support/clickfix-identity", rbac.PermTenantSettingsRead, h.ClickFixIdentity)

	tp := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			response.NotFound(w, req, "")
		})
	}
	if m.Taxpayers != nil {
		tp = middleware.RequireTaxpayer(m.Taxpayers)
	}
	r.SignedIn("GET /v1/taxpayer/support", h.TaxpayerSupport, tp)
}
