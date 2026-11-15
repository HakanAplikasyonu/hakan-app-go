// Package ai is the AI module (docs/01 G11, issue #24): provider layer
// (Anthropic primary, configurable fallback), masking of VKN / TCKN / IBAN /
// names, a tool registry, and a per-office / per-taxpayer cost ledger.
// Ported from otopoly-go internal/modules/ai.
package ai

import (
	aihandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the AI tenant routes.
type Module struct{ H *aihandler.Handler }

// Name implements module.Module.
func (Module) Name() string { return "ai" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	var h aihandler.Handler
	if m.H != nil {
		h = *m.H
	}
	r.Tenant("GET /v1/tenant/ai/usage", rbac.PermTenantAIUsageRead, h.Usage)
	r.Tenant("GET /v1/tenant/ai/privacy", rbac.PermTenantSettingsRead, h.Privacy)
}
