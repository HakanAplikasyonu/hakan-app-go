// Package agents is the desktop agent registry (issue #51, 02 §6): the
// desktop app registers with the office user's session, reports a heartbeat
// every 5 minutes, and an agent silent for 30 minutes goes offline with an
// AGENT_OFFLINE notification.
package agents

import (
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/agents/documents"
	agentshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/agents/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the routes.
type Module struct {
	H *agentshandler.Handler
	// Docs: files the agent picks up from the watched folders (#780 e).
	Docs *documents.Handler
}

// Name implements module.Module.
func (Module) Name() string { return "agents" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &agentshandler.Handler{}
	}
	base := "/v1/tenant/agents"
	read, write := rbac.PermTenantAgentsRead, rbac.PermTenantAgentsWrite
	r.Tenant("POST "+base, write, h.Register)
	r.Tenant("GET "+base, read, h.List)
	r.Tenant("GET "+base+"/{agentUuid}", read, h.Get)
	r.Tenant("POST "+base+"/{agentUuid}/heartbeat", write, h.Heartbeat)
	r.Tenant("DELETE "+base+"/{agentUuid}", write, h.Revoke)
	// Luca bordro dökümü, mizan ve yabancı fatura PDF'i (#780 e): ajan
	// yükler; eşleşmeyenleri ofis eşleştirir.
	d := m.Docs
	if d == nil {
		d = &documents.Handler{}
	}
	r.Tenant("POST "+base+"/{agentUuid}/documents", write, d.Upload)
	docs := "/v1/tenant/agent-documents"
	r.Tenant("GET "+docs, read, d.List)
	r.Tenant("POST "+docs+"/{documentUuid}/resolve", write, d.Resolve)
	r.Tenant("POST "+docs+"/{documentUuid}/dismiss", write, d.Dismiss)
}
