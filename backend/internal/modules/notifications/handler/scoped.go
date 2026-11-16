package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/periodq"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Scoped inboxes (issue #333): the office panel (/v1/tenant/notifications,
// X-Organization) and the mükellef panel (/v1/taxpayer/notifications,
// X-Taxpayer) list the caller's own in-app notifications of that office or
// company. Every member may read their own; no permission beyond the
// membership the route's middleware checks.

func officeScope(r *http.Request) usecase.InboxScope {
	return usecase.InboxScope{OrganizationID: orgctx.Must(r.Context()).ID}
}

func taxpayerScope(r *http.Request) usecase.InboxScope {
	id := taxpayerctx.Must(r.Context()).UUID
	return usecase.InboxScope{TaxpayerUUID: &id}
}

// TenantInbox: GET /v1/tenant/notifications.
func (h *Handler) TenantInbox(w http.ResponseWriter, r *http.Request) {
	h.scopedList(w, r, officeScope(r))
}

// TaxpayerInbox: GET /v1/taxpayer/notifications.
func (h *Handler) TaxpayerInbox(w http.ResponseWriter, r *http.Request) {
	h.scopedList(w, r, taxpayerScope(r))
}

// TenantUnread / TaxpayerUnread: GET .../notifications/unread-count.
func (h *Handler) TenantUnread(w http.ResponseWriter, r *http.Request) {
	h.scopedUnread(w, r, officeScope(r))
}

// TaxpayerUnread: GET /v1/taxpayer/notifications/unread-count.
func (h *Handler) TaxpayerUnread(w http.ResponseWriter, r *http.Request) {
	h.scopedUnread(w, r, taxpayerScope(r))
}

// TenantMarkRead / TaxpayerMarkRead: POST .../notifications/{uuid}/read.
func (h *Handler) TenantMarkRead(w http.ResponseWriter, r *http.Request) {
	h.scopedMarkRead(w, r, officeScope(r))
}

// TaxpayerMarkRead: POST /v1/taxpayer/notifications/{uuid}/read.
func (h *Handler) TaxpayerMarkRead(w http.ResponseWriter, r *http.Request) {
	h.scopedMarkRead(w, r, taxpayerScope(r))
}

// TenantReadAll / TaxpayerReadAll: POST .../notifications/read-all.
func (h *Handler) TenantReadAll(w http.ResponseWriter, r *http.Request) {
	h.scopedReadAll(w, r, officeScope(r))
}

// TaxpayerReadAll: POST /v1/taxpayer/notifications/read-all.
func (h *Handler) TaxpayerReadAll(w http.ResponseWriter, r *http.Request) {
	h.scopedReadAll(w, r, taxpayerScope(r))
}

func (h *Handler) scopedList(w http.ResponseWriter, r *http.Request, sc usecase.InboxScope) {
	p := authctx.MustPrincipal(r.Context())
	period, ok := periodq.Parse(w, r)
	if !ok {
		return
	}
	limit, offset := int32(20), int32(0)
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 100 {
		limit = int32(v) //nolint:gosec // bounded
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 && v <= 1_000_000 {
		offset = int32(v) //nolint:gosec // bounded
	}
	unread := false
	if raw := strings.TrimSpace(r.URL.Query().Get("unread")); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "unread", Message: "unread must be a boolean", Code: "invalid"}})
			return
		}
		unread = b
	}
	page, err := h.svc.ListScopedInbox(r.Context(), p.UserInternal, sc, unread, period, limit, offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

func (h *Handler) scopedUnread(w http.ResponseWriter, r *http.Request, sc usecase.InboxScope) {
	p := authctx.MustPrincipal(r.Context())
	n, err := h.svc.ScopedUnread(r.Context(), p.UserInternal, sc)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]int64{"count": n})
}

func (h *Handler) scopedMarkRead(w http.ResponseWriter, r *http.Request, sc usecase.InboxScope) {
	p := authctx.MustPrincipal(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Notification was not found")
		return
	}
	n, err := h.svc.MarkScopedRead(r.Context(), p.UserInternal, sc, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, n)
}

func (h *Handler) scopedReadAll(w http.ResponseWriter, r *http.Request, sc usecase.InboxScope) {
	p := authctx.MustPrincipal(r.Context())
	n, err := h.svc.MarkScopedAllRead(r.Context(), p.UserInternal, sc)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]int64{"marked": n})
}
