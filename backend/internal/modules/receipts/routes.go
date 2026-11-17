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
	// Ofis masraf kategorisi şablonu (#384).
	r.Tenant("GET /v1/tenant/expense-categories", read, h.OfficeCategories)
	r.Tenant("PUT /v1/tenant/expense-categories", approve, h.SaveOfficeCategory)
	r.Tenant("DELETE /v1/tenant/expense-categories/{categoryUuid}", approve, h.DeleteOfficeCategory)
	base := "/v1/tenant/engagements/{engagementUuid}"
	r.Tenant("POST "+base+"/receipts", write, h.Capture, eng...)
	r.Tenant("GET "+base+"/receipts", read, h.List, eng...)
	r.Tenant("GET "+base+"/receipts/{receiptUuid}", read, h.Get, eng...)
	r.Tenant("POST "+base+"/receipts/{receiptUuid}/approve", approve, h.Approve, eng...)
	r.Tenant("POST "+base+"/receipts/{receiptUuid}/reject", approve, h.Reject, eng...)
	// Masraflar, kategoriler ve muhasebeleştirme (#384).
	r.Tenant("GET "+base+"/expenses", read, h.Expenses, eng...)
	r.Tenant("GET "+base+"/expenses/{expenseUuid}", read, h.Expense, eng...)
	r.Tenant("PATCH "+base+"/expenses/{expenseUuid}", write, h.SetExpensePosting, eng...)
	r.Tenant("POST "+base+"/expenses/{expenseUuid}/approve", approve, h.ApproveExpense, eng...)
	r.Tenant("POST "+base+"/expenses/{expenseUuid}/reject", approve, h.RejectExpense, eng...)
	r.Tenant("GET "+base+"/expense-categories", read, h.EngagementCategories, eng...)
	r.Tenant("PUT "+base+"/expense-categories", approve, h.SaveEngagementCategory, eng...)
	r.Tenant("DELETE "+base+"/expense-categories/{categoryUuid}", approve, h.DeleteEngagementCategory, eng...)
	// Z raporları: ÖKC bazında liste, kırılım, süreklilik, onay (#384).
	r.Tenant("GET "+base+"/retail-sales", read, h.RetailSales, eng...)
	r.Tenant("POST "+base+"/retail-sales/continuity", write, h.RunZContinuity, eng...)
	r.Tenant("GET "+base+"/retail-sales/{retailSaleUuid}", read, h.RetailSale, eng...)
	r.Tenant("PATCH "+base+"/retail-sales/{retailSaleUuid}", write, h.UpdateRetailSale, eng...)
	r.Tenant("POST "+base+"/retail-sales/{retailSaleUuid}/approve", approve, h.ApproveRetailSale, eng...)
	r.Tenant("POST "+base+"/retail-sales/{retailSaleUuid}/reject", approve, h.RejectRetailSale, eng...)
	r.Tenant("GET "+base+"/vehicles", read, h.Vehicles, eng...)
	r.Tenant("POST "+base+"/vehicles", write, h.AddVehicle, eng...)
	r.Tenant("DELETE "+base+"/vehicles/{vehicleUuid}", write, h.DeleteVehicle, eng...)
}
