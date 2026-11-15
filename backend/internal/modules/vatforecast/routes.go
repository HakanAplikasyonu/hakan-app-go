// Package vatforecast is the VAT early-warning module (KDV erken uyarı,
// issue #33, docs/00 K9 / T10): the in-period estimate of hesaplanan −
// indirilecek KDV with devreden KDV, the KDV1 filing deadline and the days
// left, and two warning rules (VAT_PAYABLE_THRESHOLD, VAT_DEADLINE_NEAR)
// whose thresholds and filing day (due_day) are warning_rule_configs params,
// per office or engagement.
//
// The calculation rules are documented in usecase/types.go.
package vatforecast

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	vathandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the VAT forecast routes.
type Module struct {
	H           *vathandler.Handler
	Engagements engagementctx.Resolver
}

// Name implements module.Module.
func (Module) Name() string { return "vatforecast" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &vathandler.Handler{}
	}
	read, write := rbac.PermTenantVATRead, rbac.PermTenantVATWrite
	r.Tenant("GET /v1/tenant/vat-forecast", read, h.Office)
	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	base := "/v1/tenant/engagements/{engagementUuid}/vat-forecast"
	r.Tenant("GET "+base, read, h.Get, eng...)
	r.Tenant("POST "+base+"/evaluate", write, h.Evaluate, eng...)
	r.Tenant("GET "+base+"/openings", read, h.ListOpenings, eng...)
	r.Tenant("PUT "+base+"/openings/{period}", write, h.PutOpening, eng...)
	r.Tenant("DELETE "+base+"/openings/{period}", write, h.DeleteOpening, eng...)
}
