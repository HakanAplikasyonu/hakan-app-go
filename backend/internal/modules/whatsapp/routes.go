package whatsapp

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Module mounts the extension routes. Capturing reuses the receipt
// permissions: the extension is one more way to capture a receipt.
//
//   - POST /v1/auth/whatsapp/pair (public): pairing code → session.
//   - /v1/tenant/whatsapp/*: pairing code, chat lookup / links, capture list.
//   - /v1/tenant/engagements/{engagementUuid}/whatsapp/*: link a chat, save
//     a file, download a saved document.
type Module struct {
	H           *Handler
	Engagements engagementctx.Resolver
	// Scope runs each request in one row level security transaction (the
	// whatsapp_* tables are enforced, issue #166).
	Scope func(http.Handler) http.Handler
}

// Name implements module.Module.
func (Module) Name() string { return "whatsapp" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	read, write := rbac.PermTenantReceiptsRead, rbac.PermTenantReceiptsWrite
	r.Public("POST /v1/auth/whatsapp/pair", h.Pair)
	var office []func(http.Handler) http.Handler
	if m.Scope != nil {
		office = append(office, m.Scope)
	}
	base := "/v1/tenant/whatsapp"
	r.Tenant("POST "+base+"/pairings", write, h.CreatePairing)
	r.Tenant("GET "+base+"/chat-lookup", read, h.LookupChat, office...)
	r.Tenant("GET "+base+"/chat-links", read, h.ListLinks, office...)
	r.Tenant("DELETE "+base+"/chat-links/{linkUuid}", write, h.DeleteLink, office...)
	r.Tenant("GET "+base+"/captures", read, h.ListCaptures, office...)
	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	if m.Scope != nil {
		eng = append(eng, m.Scope)
	}
	ebase := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/whatsapp"
	r.Tenant("PUT "+ebase+"/chat-link", write, h.SetLink, eng...)
	r.Tenant("POST "+ebase+"/captures", write, h.Capture, eng...)
	r.Tenant("GET "+ebase+"/captures/{captureUuid}/file", read, h.CaptureFile, eng...)
	// Görevsiz kaydedilen belge sonradan bir belge isteğine bağlanır (#571).
	r.Tenant("PUT "+ebase+"/captures/{captureUuid}/task", write, h.LinkTask, eng...)
}
