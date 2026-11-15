// Package warnings is the warning engine module (issue #13,
// docs/kaynak/musavir-entegrasyon-paketi/05-uyari-kodlari.md): the code
// catalog, the engine that raises / dedupes / auto-resolves data warnings,
// controls that feed it, and the office / engagement rule configuration API.
package warnings

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	warningshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the warning routes. Engagement routes need Engagements (the
// engagement resolver); without it only the office routes are mounted.
type Module struct {
	H           *warningshandler.Handler
	Engagements engagementctx.Resolver
}

// Name implements module.Module.
func (Module) Name() string { return "warnings" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &warningshandler.Handler{}
	}
	read, write := rbac.PermTenantWarningsRead, rbac.PermTenantWarningRulesWrite
	r.Tenant("GET /v1/tenant/warning-rules", read, h.ListOfficeRules)
	r.Tenant("PUT /v1/tenant/warning-rules/{code}", write, h.SetOfficeRule)
	r.Tenant("DELETE /v1/tenant/warning-rules/{code}", write, h.ResetOfficeRule)
	r.Tenant("GET /v1/tenant/warnings", read, h.ListOfficeWarnings)

	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	r.Tenant("GET /v1/tenant/engagements/{engagementUuid}/warning-rules", read, h.ListEngagementRules, eng...)
	r.Tenant("PUT /v1/tenant/engagements/{engagementUuid}/warning-rules/{code}", write, h.SetEngagementRule, eng...)
	r.Tenant("DELETE /v1/tenant/engagements/{engagementUuid}/warning-rules/{code}", write, h.ResetEngagementRule, eng...)
	decide := rbac.PermTenantWarningsWrite
	r.Tenant("GET /v1/tenant/engagements/{engagementUuid}/warnings", read, h.ListEngagementWarnings, eng...)
	r.Tenant("POST /v1/tenant/engagements/{engagementUuid}/warnings/{warningUuid}/acknowledge", decide, h.AcknowledgeWarning, eng...)
	r.Tenant("POST /v1/tenant/engagements/{engagementUuid}/warnings/{warningUuid}/resolve", decide, h.ResolveWarning, eng...)
	r.Tenant("POST /v1/tenant/engagements/{engagementUuid}/controls/run", decide, h.RunControls, eng...)
}
