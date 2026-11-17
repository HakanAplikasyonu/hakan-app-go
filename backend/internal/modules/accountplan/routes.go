// Package accountplan is the target account plan module (issue #30,
// docs/02 §2): the engagement's Luca hesap planı / mizan import and the
// pre-export ACCOUNT_NOT_IN_TARGET_PLAN check.
package accountplan

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/planexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the account plan routes.
type Module struct {
	H           *handler.Handler
	Engagements engagementctx.Resolver
	// Plan serves the Luca hesap planı file and cari sub accounts (#464).
	Plan *planexport.Handler
}

// Name implements module.Module.
func (Module) Name() string { return "accountplan" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	if m.Engagements == nil {
		return
	}
	h := m.H
	if h == nil {
		h = &handler.Handler{}
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	read, write := rbac.PermTenantAccountPlanRead, rbac.PermTenantAccountPlanWrite
	base := "/v1/tenant/engagements/{engagementUuid}/account-plan"
	r.Tenant("POST "+base+"/imports", write, h.Import, eng...)
	r.Tenant("GET "+base+"/imports", read, h.ListImports, eng...)
	r.Tenant("GET "+base+"/accounts", read, h.ListAccounts, eng...)
	r.Tenant("GET "+base+"/trial-balances", read, h.ListTrialBalances, eng...)
	r.Tenant("GET "+base+"/trial-balances/{trialBalance}", read, h.TrialBalance, eng...)
	r.Tenant("POST "+base+"/check", read, h.Check, eng...)
	r.Tenant("GET "+base+"/missing", read, h.Missing, eng...)
	r.Tenant("POST "+base+"/missing-template", read, h.MissingTemplate, eng...)
	// Luca hesap planı CSV and cari sub accounts (#464).
	if m.Plan != nil {
		r.Tenant("GET "+base+"/subaccounts", read, m.Plan.ListSubaccounts, eng...)
		r.Tenant("POST "+base+"/subaccounts", write, m.Plan.Allocate, eng...)
		r.Tenant("POST "+base+"/subaccounts/generate", write, m.Plan.Generate, eng...)
		r.Tenant("GET "+base+"/luca-files", read, m.Plan.ListFiles, eng...)
		r.Tenant("POST "+base+"/luca-files", read, m.Plan.BuildFile, eng...)
	}
}
