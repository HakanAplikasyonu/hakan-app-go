// Package legal serves the KVKK texts (issue #57): versioned privacy notice,
// explicit consent for sharing past data, data processing agreement and
// retention policy, and the acceptance records that tie them to flows.
package legal

import (
	legalhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/legal/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the routes.
type Module struct {
	H *legalhandler.Handler
}

// Name implements module.Module.
func (Module) Name() string { return "legal" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &legalhandler.Handler{}
	}
	r.Public("GET /v1/legal/documents", h.List)
	r.Public("GET /v1/legal/documents/{kind}", h.Get)
	r.Session("GET /v1/me/legal-acceptances", h.Mine)
	r.Session("POST /v1/me/legal-acceptances", h.AcceptMine)
	r.Tenant("GET /v1/tenant/legal/dpa", rbac.PermTenantSettingsRead, h.DPA)
	r.Tenant("POST /v1/tenant/legal/dpa/accept", rbac.PermTenantLegalAccept, h.AcceptDPA)
	r.Platform("GET /v1/platform/legal/documents", rbac.PermPlatformLegalManage, h.AdminList)
	r.Platform("POST /v1/platform/legal/documents", rbac.PermPlatformLegalManage, h.AdminCreate)
	r.Platform("POST /v1/platform/legal/documents/{documentUuid}/publish", rbac.PermPlatformLegalManage, h.AdminPublish)
}
