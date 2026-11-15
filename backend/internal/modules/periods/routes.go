// Package periods is the period management module (issue #32): a period is
// "ready" for the declaration when it has no open blocker; a locked period
// does not change, data arriving for it becomes a change request with a
// LOCKED_PERIOD_CHANGE warning (02-adaptor-mimarisi.md §3.4). The office
// sees every engagement × period in one matrix (docs/01 §3 "command center").
//
// The invoice write path calls repository.Guard (invoices/repository.PeriodGuard).
package periods

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	periodshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/periods/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the period routes.
type Module struct {
	H           *periodshandler.Handler
	Engagements engagementctx.Resolver
}

// Name implements module.Module.
func (Module) Name() string { return "periods" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &periodshandler.Handler{}
	}
	read, lock, decide := rbac.PermTenantPeriodsRead, rbac.PermTenantPeriodsLock, rbac.PermTenantChangeRequestsDecide
	r.Tenant("GET /v1/tenant/periods/matrix", read, h.Matrix)
	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	base := "/v1/tenant/engagements/{engagementUuid}"
	r.Tenant("GET "+base+"/periods/{period}", read, h.GetReadiness, eng...)
	r.Tenant("POST "+base+"/periods/{period}/lock", lock, h.Lock, eng...)
	r.Tenant("POST "+base+"/periods/{period}/unlock", lock, h.Unlock, eng...)
	r.Tenant("GET "+base+"/periods/{period}/lock-events", read, h.ListLockEvents, eng...)
	r.Tenant("GET "+base+"/change-requests", read, h.ListChangeRequests, eng...)
	r.Tenant("GET "+base+"/change-requests/{changeRequestUuid}", read, h.GetChangeRequest, eng...)
	r.Tenant("POST "+base+"/change-requests/{changeRequestUuid}/apply", decide, h.ApplyChangeRequest, eng...)
	r.Tenant("POST "+base+"/change-requests/{changeRequestUuid}/reject", decide, h.RejectChangeRequest, eng...)
}
