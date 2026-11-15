// Package summaries is the AI period summary (issue #35, 06 T15): a draft
// financial note for the taxpayer whose numbers all come from portal data;
// the accountant edits and approves; nothing is sent automatically.
package summaries

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	summarieshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/summaries/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the routes.
type Module struct {
	H           *summarieshandler.Handler
	Engagements engagementctx.Resolver
}

// Name implements module.Module.
func (Module) Name() string { return "summaries" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	if m.Engagements == nil {
		return
	}
	h := m.H
	if h == nil {
		h = &summarieshandler.Handler{}
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	base := "/v1/tenant/engagements/{engagementUuid}/period-summaries"
	read, write, approve := rbac.PermTenantSummariesRead, rbac.PermTenantSummariesWrite, rbac.PermTenantSummariesApprove
	r.Tenant("POST "+base, write, h.Generate, eng...)
	r.Tenant("GET "+base, read, h.List, eng...)
	r.Tenant("GET "+base+"/{summaryUuid}", read, h.Get, eng...)
	r.Tenant("PUT "+base+"/{summaryUuid}", write, h.Edit, eng...)
	r.Tenant("POST "+base+"/{summaryUuid}/approve", approve, h.Approve, eng...)
}
