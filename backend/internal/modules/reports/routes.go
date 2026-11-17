package reports

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Module mounts the report routes.
//
//   - Office: /v1/tenant/engagements/{engagementUuid}/report (engagement
//     read).
//   - Mükellef: /v1/taxpayer/report behind X-Taxpayer membership.
type Module struct {
	H           *Handler
	Engagements engagementctx.Resolver
	Taxpayers   taxpayerctx.Resolver
}

// Name implements module.Module.
func (Module) Name() string { return "reports" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	tp := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { response.NotFound(w, req, "") })
	}
	if m.Taxpayers != nil {
		tp = middleware.RequireTaxpayer(m.Taxpayers)
	}
	r.SignedIn("GET /v1/taxpayer/report", h.Mine, tp)
	if m.Engagements == nil {
		return
	}
	read := rbac.PermTenantEngagementsRead
	eng := []func(http.Handler) http.Handler{middleware.RequireEngagement(m.Engagements)}
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}"
	r.Tenant("GET "+base+"/report", read, h.Office, eng...)
}
