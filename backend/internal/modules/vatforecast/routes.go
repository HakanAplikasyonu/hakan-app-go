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
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Module mounts the VAT forecast routes.
type Module struct {
	H           *vathandler.Handler
	Engagements engagementctx.Resolver
	// Scope runs each request in one row level security transaction
	// (rls.Middleware, issue #166): vat_carry_forwards is enforced.
	Scope func(http.Handler) http.Handler
	// Taxpayers resolves X-Taxpayer for the mükellef panel route (#780).
	Taxpayers taxpayerctx.Resolver
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
	var office []func(http.Handler) http.Handler
	if m.Scope != nil {
		office = append(office, m.Scope)
	}
	r.Tenant("GET /v1/tenant/vat-forecast", read, h.Office, office...)
	r.Tenant("GET /v1/tenant/vat-forecast/watch-events", read, h.OfficeWatchEvents, office...)
	r.Tenant("GET /v1/tenant/vat-tracking-settings", read, h.Tracking, office...)
	r.Tenant("PUT /v1/tenant/vat-tracking-settings", rbac.PermTenantSettingsWrite, h.PutTracking, office...)
	r.Tenant("GET /v1/tenant/vat-report-rules", read, h.ReportRules, office...)
	r.Tenant("PUT /v1/tenant/vat-report-rules", rbac.PermTenantSettingsWrite, h.PutReportRules, office...)
	r.Tenant("POST /v1/tenant/vat-report-rules/{rule}/run", rbac.PermTenantSettingsWrite, h.RunReportRule, office...)
	// Mükellef paneli (#780): anlık KDV ve satış raporu, salt okunur.
	tp := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { response.NotFound(w, req, "") })
	}
	if m.Taxpayers != nil {
		tp = middleware.RequireTaxpayer(m.Taxpayers)
	}
	r.SignedIn("GET /v1/taxpayer/vat-sales", h.Taxpayer, tp)
	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	if m.Scope != nil {
		eng = append(eng, m.Scope)
	}
	base := "/v1/tenant/engagements/{engagementUuid}/vat-forecast"
	r.Tenant("GET "+base, read, h.Get, eng...)
	r.Tenant("POST "+base+"/evaluate", write, h.Evaluate, eng...)
	r.Tenant("GET "+base+"/openings", read, h.ListOpenings, eng...)
	r.Tenant("GET "+base+"/notices", read, h.ListNotices, eng...)
	r.Tenant("POST "+base+"/notices", write, h.SendNotice, eng...)
	r.Tenant("GET "+base+"/watch-events", read, h.ListWatchEvents, eng...)
	r.Tenant("POST "+base+"/watch", write, h.RunWatch, eng...)
	r.Tenant("PUT "+base+"/openings/{period}", write, h.PutOpening, eng...)
	r.Tenant("DELETE "+base+"/openings/{period}", write, h.DeleteOpening, eng...)
}
