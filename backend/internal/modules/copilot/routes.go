package copilot

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the co-pilot routes.
//
//   - /v1/tenant/copilot/workflow: the next-work list of every engagement
//     the caller reaches (engagement read).
//   - /v1/tenant/engagements/{engagementUuid}/copilot/workflow: one chain.
//   - /v1/tenant/copilot/plans: command bar plans of the caller (writing a
//     plan needs engagement write; each step still needs its own
//     permission, checked when the plan is made and again by the endpoint
//     the step calls).
type Module struct {
	H           *Handler
	Engagements engagementctx.Resolver
}

// Name implements module.Module.
func (Module) Name() string { return "copilot" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	read, write := rbac.PermTenantEngagementsRead, rbac.PermTenantEngagementsWrite
	r.Tenant("GET /v1/tenant/copilot/workflow", read, h.Workflow)
	r.Tenant("GET /v1/tenant/copilot/plans", read, h.ListPlans)
	r.Tenant("POST /v1/tenant/copilot/plans", write, h.CreatePlan)
	r.Tenant("GET /v1/tenant/copilot/plans/{planUuid}", read, h.GetPlan)
	r.Tenant("POST /v1/tenant/copilot/plans/{planUuid}/steps/{stepIndex}", write, h.RecordStep)
	r.Tenant("POST /v1/tenant/copilot/plans/{planUuid}/cancel", write, h.CancelPlan)
	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{middleware.RequireEngagement(m.Engagements)}
	r.Tenant("GET /v1/tenant/engagements/{"+engagementctx.PathParam+"}/copilot/workflow", read, h.EngagementWorkflow, eng...)
}
