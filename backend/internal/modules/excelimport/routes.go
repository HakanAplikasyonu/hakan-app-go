// Package excelimport is the Excel / CSV import channel (issue #22,
// 04-excel-ice-aktarma.md) for taxpayers whose programs have no API:
// structure detection, template or AI column mapping on a masked sample,
// deterministic conversion and validation, preview, accountant commit and
// rollback.
package excelimport

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	excelhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the routes.
type Module struct {
	H           *excelhandler.Handler
	Engagements engagementctx.Resolver
}

// Name implements module.Module.
func (Module) Name() string { return "excelimport" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &excelhandler.Handler{}
	}
	read, write, commit := rbac.PermTenantExcelImportsRead, rbac.PermTenantExcelImportsWrite, rbac.PermTenantExcelImportsCommit
	r.Tenant("GET /v1/tenant/excel-import-templates", read, h.Templates)
	r.Tenant("GET /v1/tenant/excel-import-data-types", read, h.DataTypes)
	r.Tenant("DELETE /v1/tenant/excel-import-templates/{templateUuid}", write, h.DeleteTemplate)
	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	base := "/v1/tenant/engagements/{engagementUuid}/excel-imports"
	r.Tenant("POST "+base, write, h.Upload, eng...)
	r.Tenant("GET "+base, read, h.List, eng...)
	r.Tenant("GET "+base+"/{importUuid}", read, h.Get, eng...)
	r.Tenant("PUT "+base+"/{importUuid}/mapping", write, h.SetMapping, eng...)
	r.Tenant("POST "+base+"/{importUuid}/validate", write, h.Validate, eng...)
	r.Tenant("GET "+base+"/{importUuid}/rows", read, h.Rows, eng...)
	r.Tenant("PATCH "+base+"/{importUuid}/rows/{rowNo}", write, h.PatchRow, eng...)
	r.Tenant("POST "+base+"/{importUuid}/commit", commit, h.Commit, eng...)
	r.Tenant("POST "+base+"/{importUuid}/rollback", commit, h.Rollback, eng...)
}
