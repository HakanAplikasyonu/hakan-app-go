package notifications

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
)

// RegisterRoutes mounts notification routes.
func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	requirePerm := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}

	mux.Handle("GET /v1/notifications/vapid-public-key", middleware.Chain(
		http.HandlerFunc(h.VAPIDPublicKey), authn,
	))
	mux.Handle("POST /v1/notifications/push-subscriptions", middleware.Chain(
		http.HandlerFunc(h.UpsertPushSubscription), authn,
	))
	mux.Handle("DELETE /v1/notifications/push-subscriptions", middleware.Chain(
		http.HandlerFunc(h.DeletePushSubscription), authn,
	))
	// Mobile (Expo) push devices: registered at login, removed at logout.
	mux.Handle("POST /v1/push-devices", middleware.Chain(
		http.HandlerFunc(h.RegisterPushDevice), authn,
	))
	mux.Handle("DELETE /v1/push-devices/{token}", middleware.Chain(
		http.HandlerFunc(h.DeletePushDevice), authn,
	))

	// One-click unsubscribe (List-Unsubscribe-Post): public, the link is signed.
	mux.Handle("POST /v1/notifications/unsubscribe", http.HandlerFunc(h.Unsubscribe))
	mux.Handle("GET /v1/notifications/unsubscribe", http.HandlerFunc(h.UnsubscribeLanding))

	mux.Handle("GET /v1/notifications/meta", middleware.Chain(
		http.HandlerFunc(h.Meta), authn, requirePerm(rbac.PermNotificationsRead),
	))
	mux.Handle("GET /v1/notifications/unread-count", middleware.Chain(
		http.HandlerFunc(h.UnreadCount), authn, requirePerm(rbac.PermNotificationsRead),
	))
	mux.Handle("POST /v1/notifications/read-all", middleware.Chain(
		http.HandlerFunc(h.MarkAllRead), authn, requirePerm(rbac.PermNotificationsRead),
	))
	mux.Handle("POST /v1/notifications/test", middleware.Chain(
		http.HandlerFunc(h.Test), authn,
	))
	mux.Handle("POST /v1/notifications/send", middleware.Chain(
		http.HandlerFunc(h.Send), authn, requirePerm(rbac.PermNotificationsManage),
	))
	mux.Handle("GET /v1/notifications", middleware.Chain(
		http.HandlerFunc(h.List), authn, requirePerm(rbac.PermNotificationsRead),
	))
	mux.Handle("GET /v1/notifications/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Get), authn, requirePerm(rbac.PermNotificationsRead),
	))
	mux.Handle("GET /v1/notifications/{uuid}/action", middleware.Chain(
		http.HandlerFunc(h.FollowAction), authn, requirePerm(rbac.PermNotificationsRead),
	))
	mux.Handle("POST /v1/notifications/{uuid}/read", middleware.Chain(
		http.HandlerFunc(h.MarkRead), authn, requirePerm(rbac.PermNotificationsRead),
	))

	mux.Handle("GET /v1/notification-preferences", middleware.Chain(
		http.HandlerFunc(h.GetPreferences), authn,
	))
	mux.Handle("PUT /v1/notification-preferences", middleware.Chain(
		http.HandlerFunc(h.PutPreferences), authn,
	))

	mux.Handle("GET /v1/platform/notifications/meta", middleware.Chain(
		http.HandlerFunc(h.PlatformMeta), authn, requirePerm(rbac.PermPlatformNotificationsRead),
	))
	mux.Handle("GET /v1/platform/notifications", middleware.Chain(
		http.HandlerFunc(h.ListPlatform), authn, requirePerm(rbac.PermPlatformNotificationsRead),
	))
}

// RegisterScopedRoutes mounts the office and mükellef panel inboxes (issue
// #333). Without a resolver (tenancy off) that side is not mounted.
func RegisterScopedRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver,
	tpResolver taxpayerctx.Resolver,
) {
	authn := middleware.Authenticate(tokens, loader)
	if orgResolver != nil {
		org := middleware.RequireOrganization(orgResolver)
		mux.Handle("GET /v1/tenant/notifications", middleware.Chain(http.HandlerFunc(h.TenantInbox), authn, org))
		mux.Handle("GET /v1/tenant/notifications/unread-count", middleware.Chain(http.HandlerFunc(h.TenantUnread), authn, org))
		mux.Handle("POST /v1/tenant/notifications/read-all", middleware.Chain(http.HandlerFunc(h.TenantReadAll), authn, org))
		mux.Handle("POST /v1/tenant/notifications/{uuid}/read", middleware.Chain(http.HandlerFunc(h.TenantMarkRead), authn, org))
	}
	if tpResolver != nil {
		tp := middleware.RequireTaxpayer(tpResolver)
		mux.Handle("GET /v1/taxpayer/notifications", middleware.Chain(http.HandlerFunc(h.TaxpayerInbox), authn, tp))
		mux.Handle("GET /v1/taxpayer/notifications/unread-count", middleware.Chain(http.HandlerFunc(h.TaxpayerUnread), authn, tp))
		mux.Handle("POST /v1/taxpayer/notifications/read-all", middleware.Chain(http.HandlerFunc(h.TaxpayerReadAll), authn, tp))
		mux.Handle("POST /v1/taxpayer/notifications/{uuid}/read", middleware.Chain(http.HandlerFunc(h.TaxpayerMarkRead), authn, tp))
	}
}
