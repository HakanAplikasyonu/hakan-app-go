package automation

import (
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the routes.
type Module struct{ H *Handler }

// Name implements module.Module.
func (Module) Name() string { return "automation" }

// Routes implements module.Module. Reading is every member's (settings
// read); changing rules, the master switch and approving drafts is the
// owner's / manager's (settings write).
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	r.Tenant("GET /v1/tenant/automations", rbac.PermTenantSettingsRead, h.List)
	r.Tenant("POST /v1/tenant/automations", rbac.PermTenantSettingsWrite, h.Create)
	r.Tenant("PUT /v1/tenant/automations/{automationUuid}", rbac.PermTenantSettingsWrite, h.Update)
	r.Tenant("DELETE /v1/tenant/automations/{automationUuid}", rbac.PermTenantSettingsWrite, h.Delete)
	r.Tenant("PUT /v1/tenant/automations/{automationUuid}/active", rbac.PermTenantSettingsWrite, h.SetActive)
	r.Tenant("GET /v1/tenant/automations/{automationUuid}/history", rbac.PermTenantSettingsRead, h.History)
	r.Tenant("PUT /v1/tenant/automation-settings", rbac.PermTenantSettingsWrite, h.PutSettings)
	r.Tenant("GET /v1/tenant/automation-runs", rbac.PermTenantSettingsRead, h.Runs)
	r.Tenant("POST /v1/tenant/automation-runs/decide", rbac.PermTenantSettingsWrite, h.Decide)
}
