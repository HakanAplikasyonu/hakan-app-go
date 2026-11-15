// Package receipts is receipt capture (issue #23, docs/00 K8): expense /
// fuel receipt and Z report photos read by the AI module into drafts that
// an accountant approves, the taxpayer's vehicle list and the plate check.
package receipts

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	receiptshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the routes (all engagement-scoped).
type Module struct {
	H           *receiptshandler.Handler
	Engagements engagementctx.Resolver
}

// Name implements module.Module.
func (Module) Name() string { return "receipts" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	if m.Engagements == nil {
		return
	}
	h := m.H
	if h == nil {
		h = &receiptshandler.Handler{}
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	read, write, approve := rbac.PermTenantReceiptsRead, rbac.PermTenantReceiptsWrite, rbac.PermTenantReceiptsApprove
	base := "/v1/tenant/engagements/{engagementUuid}"
	r.Tenant("POST "+base+"/receipts", write, h.Capture, eng...)
	r.Tenant("GET "+base+"/receipts", read, h.List, eng...)
	r.Tenant("GET "+base+"/receipts/{receiptUuid}", read, h.Get, eng...)
	r.Tenant("POST "+base+"/receipts/{receiptUuid}/approve", approve, h.Approve, eng...)
	r.Tenant("POST "+base+"/receipts/{receiptUuid}/reject", approve, h.Reject, eng...)
	r.Tenant("GET "+base+"/vehicles", read, h.Vehicles, eng...)
	r.Tenant("POST "+base+"/vehicles", write, h.AddVehicle, eng...)
	r.Tenant("DELETE "+base+"/vehicles/{vehicleUuid}", write, h.DeleteVehicle, eng...)
}
