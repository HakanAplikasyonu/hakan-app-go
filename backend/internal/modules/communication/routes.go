package communication

import (
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the routes.
type Module struct{ H *Handler }

// Name implements module.Module.
func (Module) Name() string { return "communication" }

// Routes implements module.Module. Reading is every member's (settings
// read); changing the account and sending a test SMS is the owner's /
// manager's (settings write).
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	r.Tenant("GET /v1/tenant/sms-settings", rbac.PermTenantSettingsRead, h.GetSettings)
	r.Tenant("PUT /v1/tenant/sms-settings", rbac.PermTenantSettingsWrite, h.PutSettings)
	r.Tenant("POST /v1/tenant/sms-settings/test", rbac.PermTenantSettingsWrite, h.TestConnection)
	r.Tenant("GET /v1/tenant/sms-messages", rbac.PermTenantSettingsRead, h.ListMessages)
	r.Tenant("POST /v1/tenant/sms-messages/test", rbac.PermTenantSettingsWrite, h.TestMessage)

	// İletişim sayfası (#780 B): gruplar ve elle toplu gönderim. Görmek
	// mükellef okuma, grup değiştirmek ve göndermek mükellef yazma izniyle.
	r.Tenant("GET /v1/tenant/communication/groups", rbac.PermTenantEngagementsRead, h.ListGroups)
	r.Tenant("POST /v1/tenant/communication/groups", rbac.PermTenantEngagementsWrite, h.CreateGroup)
	r.Tenant("GET /v1/tenant/communication/groups/{groupUuid}", rbac.PermTenantEngagementsRead, h.GetGroup)
	r.Tenant("PUT /v1/tenant/communication/groups/{groupUuid}", rbac.PermTenantEngagementsWrite, h.UpdateGroup)
	r.Tenant("DELETE /v1/tenant/communication/groups/{groupUuid}", rbac.PermTenantEngagementsWrite, h.DeleteGroup)
	r.Tenant("GET /v1/tenant/communication/filters", rbac.PermTenantEngagementsRead, h.Filters)
	r.Tenant("POST /v1/tenant/communication/bulk-sends/preview", rbac.PermTenantEngagementsWrite, h.PreviewBulk)
	r.Tenant("POST /v1/tenant/communication/bulk-sends", rbac.PermTenantEngagementsWrite, h.StartBulk)
	r.Tenant("GET /v1/tenant/communication/bulk-sends", rbac.PermTenantEngagementsRead, h.ListBulk)
	r.Tenant("GET /v1/tenant/communication/bulk-sends/{bulkUuid}", rbac.PermTenantEngagementsRead, h.GetBulk)
}
