// Package accountrules is the account code rule engine module (issue #25):
// versioned office template and engagement rules, suggestions into the
// review queue (#14) and rule candidates learnt from decisions.
package accountrules

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the routes.
type Module struct {
	H           *handler.Handler
	Engagements engagementctx.Resolver
}

// Name implements module.Module.
func (Module) Name() string { return "accountrules" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &handler.Handler{}
	}
	read, write := rbac.PermTenantAccountRulesRead, rbac.PermTenantAccountRulesWrite
	office := "/v1/tenant/account-rules"
	r.Tenant("GET "+office, read, h.ListRules)
	r.Tenant("POST "+office, write, h.CreateRule)
	r.Tenant("PUT "+office+"/{ruleKey}", write, h.UpdateRule)
	r.Tenant("DELETE "+office+"/{ruleKey}", write, h.RetireRule)
	r.Tenant("GET "+office+"/{ruleKey}/versions", read, h.Versions)
	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	base := "/v1/tenant/engagements/{engagementUuid}"
	r.Tenant("GET "+base+"/account-rules", read, h.ListRules, eng...)
	r.Tenant("POST "+base+"/account-rules", write, h.CreateRule, eng...)
	r.Tenant("POST "+base+"/account-rules/apply-template", write, h.ApplyTemplate, eng...)
	r.Tenant("PUT "+base+"/account-rules/{ruleKey}", write, h.UpdateRule, eng...)
	r.Tenant("DELETE "+base+"/account-rules/{ruleKey}", write, h.RetireRule, eng...)
	r.Tenant("GET "+base+"/account-rules/{ruleKey}/versions", read, h.Versions, eng...)
	r.Tenant("GET "+base+"/account-rule-candidates", read, h.Candidates, eng...)
	r.Tenant("POST "+base+"/account-rule-candidates/promote", write, h.Promote, eng...)
	r.Tenant("POST "+base+"/invoices/{invoiceUuid}/account-suggestions", rbac.PermTenantReviewDecide, h.Suggest, eng...)
}
