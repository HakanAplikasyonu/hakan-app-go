package communication

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Module mounts the routes. The engagement history needs Engagements (the
// engagement resolver); without it that route is not mounted. Taxpayers
// resolves X-Taxpayer for the mükellef "Mesajlarım" list; without it
// (tenancy off) that route answers 404.
type Module struct {
	H           *Handler
	Engagements engagementctx.Resolver
	Taxpayers   taxpayerctx.Resolver
}

// Name implements module.Module.
func (Module) Name() string { return "communication" }

// Routes implements module.Module. Reading is every member's (settings
// read); changing the account and sending a test SMS is the owner's /
// manager's (settings write). The taxpayer's communication history is read
// with the engagement (engagements read). The e-mail tracking pixel and
// link are public: the unguessable track_token is the key.
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
	// E-posta izleme (#780 D).
	r.Public("GET /v1/public/mail/{token}/open", h.MailOpen)
	r.Public("GET /v1/public/mail/{token}/link", h.MailLink)

	// Mükellefin kendisine giden mesajlar (#780): her mükellef üyesi okur.
	tp := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { response.NotFound(w, req, "") })
	}
	if m.Taxpayers != nil {
		tp = middleware.RequireTaxpayer(m.Taxpayers)
	}
	r.SignedIn("GET /v1/taxpayer/messages", h.TaxpayerMessages, tp)

	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{middleware.RequireEngagement(m.Engagements)}
	// İletişim geçmişi (#780 D).
	r.Tenant("GET /v1/tenant/engagements/{engagementUuid}/communications", rbac.PermTenantEngagementsRead, h.History, eng...)
}
