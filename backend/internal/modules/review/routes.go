// Package review is the review queue module (issue #14, docs/01 §3 and
// G12): suggestions with confidence, rationale and provenance; below the
// office / engagement threshold they wait for the accountant, above it they
// are applied automatically and can be reverted.
package review

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	reviewhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the review routes.
type Module struct {
	H           *reviewhandler.Handler
	Engagements engagementctx.Resolver
}

// Name implements module.Module.
func (Module) Name() string { return "review" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &reviewhandler.Handler{}
	}
	read, decide, settings := rbac.PermTenantReviewRead, rbac.PermTenantReviewDecide, rbac.PermTenantReviewSettingsWrite
	r.Tenant("GET /v1/tenant/review/suggestions", read, h.ListOffice)
	r.Tenant("GET /v1/tenant/review/settings", read, h.GetOfficeSettings)
	r.Tenant("GET /v1/tenant/review/threshold-impact", read, h.ThresholdImpact)
	r.Tenant("PUT /v1/tenant/review/settings", settings, h.PutOfficeSetting)
	r.Tenant("DELETE /v1/tenant/review/settings/{kind}", settings, h.DeleteOfficeSetting)
	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	base := "/v1/tenant/engagements/{engagementUuid}"
	r.Tenant("GET "+base+"/suggestions", read, h.ListEngagement, eng...)
	r.Tenant("GET "+base+"/suggestions/history", read, h.CounterpartyHistory, eng...)
	r.Tenant("GET "+base+"/suggestions/{suggestionUuid}", read, h.Get, eng...)
	r.Tenant("POST "+base+"/suggestions/{suggestionUuid}/accept", decide, h.Accept, eng...)
	r.Tenant("POST "+base+"/suggestions/{suggestionUuid}/reject", decide, h.Reject, eng...)
	r.Tenant("POST "+base+"/suggestions/{suggestionUuid}/correct", decide, h.Correct, eng...)
	r.Tenant("POST "+base+"/suggestions/{suggestionUuid}/revert", decide, h.Revert, eng...)
	r.Tenant("GET "+base+"/review-settings", read, h.GetEngagementSettings, eng...)
	r.Tenant("PUT "+base+"/review-settings", settings, h.PutEngagementSetting, eng...)
	r.Tenant("DELETE "+base+"/review-settings/{kind}", settings, h.DeleteEngagementSetting, eng...)
}
