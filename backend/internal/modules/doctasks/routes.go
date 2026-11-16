package doctasks

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Module mounts the document task routes.
//
//   - Office side, behind the active-engagement chain:
//     /v1/tenant/engagements/{engagementUuid}/document-tasks/*
//     (tenant.doc_tasks.read / write).
//   - Mükellef side, behind X-Taxpayer membership (issue #4):
//     /v1/taxpayer/document-tasks/*. Office membership gives no access here;
//     every taxpayer member (owner or employee) may see and upload.
type Module struct {
	H           *Handler
	Engagements engagementctx.Resolver
	Taxpayers   taxpayerctx.Resolver
}

// Name implements module.Module.
func (Module) Name() string { return "doctasks" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	m.taxpayerRoutes(r, h)
	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	read, write := rbac.PermTenantDocTasksRead, rbac.PermTenantDocTasksWrite
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/document-tasks"
	r.Tenant("GET "+base, read, h.List, eng...)
	r.Tenant("POST "+base, write, h.CreateRequest, eng...)
	r.Tenant("POST "+base+"/detect", write, h.Detect, eng...)
	r.Tenant("POST "+base+"/generate", write, h.Generate, eng...)
	r.Tenant("GET "+base+"/{taskUuid}", read, h.Get, eng...)
	r.Tenant("POST "+base+"/{taskUuid}/cancel", write, h.Cancel, eng...)
	r.Tenant("POST "+base+"/{taskUuid}/complete", write, h.Complete, eng...)
	r.Tenant("POST "+base+"/{taskUuid}/rerequest", write, h.Rerequest, eng...)
	r.Tenant("GET "+base+"/{taskUuid}/uploads/{uploadUuid}/file", read, h.OfficeFile, eng...)
}

// taxpayerRoutes mounts /v1/taxpayer/document-tasks. Without a taxpayer
// resolver (tenancy off) the routes answer 404, like the rest of the
// mükellef shell, but stay in the route table.
func (m Module) taxpayerRoutes(r *module.Router, h *Handler) {
	tp := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			response.NotFound(w, req, "")
		})
	}
	if m.Taxpayers != nil {
		tp = middleware.RequireTaxpayer(m.Taxpayers)
	}
	base := "/v1/taxpayer/document-tasks"
	r.SignedIn("GET "+base, h.ListMine, tp)
	r.SignedIn("GET "+base+"/{taskUuid}", h.GetMine, tp)
	r.SignedIn("POST "+base+"/{taskUuid}/uploads", h.UploadMine, tp)
	r.SignedIn("GET "+base+"/{taskUuid}/uploads/{uploadUuid}/file", h.MyFile, tp)
}
