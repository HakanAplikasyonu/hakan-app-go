package communication

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/clientip"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// RateLimiter is a fixed-window limiter (satisfied by *ratelimit.Limiter).
type RateLimiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// SetRateLimiter caps the public tracking routes per client address.
func (h *Handler) SetRateLimiter(l RateLimiter) { h.limiter = l }

// Tracking hits per client address and minute. Over the cap the answer is
// the same (pixel / redirect); only nothing is written.
const (
	trackLimit  = 120
	trackWindow = time.Minute
)

func (h *Handler) trackAllowed(r *http.Request) bool {
	if h.limiter == nil {
		return true
	}
	ok, _ := h.limiter.Allow(r.Context(), "mail_tracking", clientip.From(r), trackLimit, trackWindow)
	return ok
}

// History: GET /v1/tenant/engagements/{engagementUuid}/communications
// ?channel=sms|email|push&limit=200. The taxpayer's SMS, e-mail and in-app
// notification records, newest first.
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		response.NotFound(w, r, "not found")
		return
	}
	channel := r.URL.Query().Get("channel")
	if channel != "" && channel != ChannelSMS && channel != ChannelEmail && channel != ChannelPush {
		response.BadRequest(w, r, response.CodeValidationError, "Kanal sms, email ya da push olmalı.")
		return
	}
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 32)
	eng := engagementctx.Must(r.Context())
	items, err := h.svc.History(r.Context(), orgctx.Must(r.Context()).ID, eng.ID, channel, int32(limit))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// transparentGIF is a 1x1 transparent GIF.
var transparentGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xff, 0xff, 0xff, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00,
	0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b,
}

// MailOpen: GET /v1/public/mail/{token}/open, the open pixel. Always the
// same image, whatever the token: a bad or unknown token writes nothing
// and tells the caller nothing.
func (h *Handler) MailOpen(w http.ResponseWriter, r *http.Request) {
	if token, err := uuid.Parse(r.PathValue("token")); err == nil && h.svc != nil && h.trackAllowed(r) {
		if _, err := h.svc.MarkOpened(r.Context(), token); err != nil {
			slog.Default().Warn("mail_open_tracking_failed", "error", err)
		}
	}
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Content-Length", strconv.Itoa(len(transparentGIF)))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(transparentGIF)
}

// MailLink: GET /v1/public/mail/{token}/link?to=<url>, an attachment or
// document link. It writes downloaded_at, then redirects. Only the app's
// own frontend and storage origins are redirect targets (no open
// redirect); any other target answers 404.
func (h *Handler) MailLink(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		response.NotFound(w, r, "not found")
		return
	}
	target := h.svc.RedirectTarget(r.URL.Query().Get("to"))
	if target == "" {
		response.NotFound(w, r, "not found")
		return
	}
	if token, err := uuid.Parse(r.PathValue("token")); err == nil && h.trackAllowed(r) {
		if _, err := h.svc.MarkDownloaded(r.Context(), token); err != nil {
			slog.Default().Warn("mail_link_tracking_failed", "error", err)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, target, http.StatusFound)
}
