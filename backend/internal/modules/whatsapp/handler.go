package whatsapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	authusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/doctasks"
	receiptshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/clientip"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ratelimit"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/apiquery"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

const (
	rateWindow      = 15 * time.Minute
	pairIPLimit     = 20
	createUserLimit = 20
)

// Handler serves the extension routes.
type Handler struct {
	svc      *Service
	pairings *Pairings
	limiter  *ratelimit.Limiter
}

// NewHandler builds the handler. limiter may be nil.
func NewHandler(svc *Service, pairings *Pairings, limiter *ratelimit.Limiter) *Handler {
	return &Handler{svc: svc, pairings: pairings, limiter: limiter}
}

// CreatePairing: POST /v1/tenant/whatsapp/pairings.
func (h *Handler) CreatePairing(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	if !h.allow(w, r, "whatsapp_pair_create", p.UserID.String(), createUserLimit) {
		return
	}
	org := orgctx.Must(r.Context())
	out, err := h.pairings.Create(r.Context(), p.UserID, p.ImpersonatorUserID != nil,
		Office{UUID: org.UUID.String(), Slug: org.Slug, Name: org.Name})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusCreated, out)
}

// Pair: POST /v1/auth/whatsapp/pair (public; the code is the credential).
func (h *Handler) Pair(w http.ResponseWriter, r *http.Request) {
	ip := clientip.From(r)
	if !h.allow(w, r, "whatsapp_pair_ip", ip, pairIPLimit) {
		return
	}
	var in struct {
		Code       string `json:"code"`
		DeviceName string `json:"device_name"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := h.pairings.Redeem(r.Context(), in.Code, in.DeviceName, ip)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, out)
}

// LookupChat: GET /v1/tenant/whatsapp/chat-lookup?chat_key=.
func (h *Handler) LookupChat(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.LookupChat(r.Context(), orgctx.Must(r.Context()).ID, r.URL.Query().Get("chat_key"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListLinks: GET /v1/tenant/whatsapp/chat-links.
func (h *Handler) ListLinks(w http.ResponseWriter, r *http.Request) {
	limit, offset := page(r)
	items, total, err := h.svc.ListLinks(r.Context(), orgctx.Must(r.Context()).ID, limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, limit, offset))
}

// DeleteLink: DELETE /v1/tenant/whatsapp/chat-links/{linkUuid}.
func (h *Handler) DeleteLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "linkUuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteLink(r.Context(), orgctx.Must(r.Context()).ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetLink: PUT /v1/tenant/engagements/{engagementUuid}/whatsapp/chat-link.
func (h *Handler) SetLink(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ChatKey string `json:"chat_key"`
		Label   string `json:"label"`
	}
	if !decode(w, r, &in) {
		return
	}
	sc := engagementctx.Must(r.Context())
	out, err := h.svc.SetLink(r.Context(), sc, userID(r), LinkInput{ChatKey: in.ChatKey, EngagementUUID: sc.UUID, Label: in.Label})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Capture: POST /v1/tenant/engagements/{engagementUuid}/whatsapp/captures
// (multipart: file, target, chat_key, task_uuid).
func (h *Handler) Capture(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileBytes+1<<20)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		response.Error(w, r, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", fmt.Sprintf("multipart form with a file is required (max %d MB)", MaxFileBytes>>20))
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file could not be read")
		return
	}
	in := CaptureInput{Target: r.FormValue("target"), ChatKey: r.FormValue("chat_key"), FileName: hdr.Filename, Data: data}
	if raw := strings.TrimSpace(r.FormValue("task_uuid")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "task_uuid is invalid")
			return
		}
		// Answering a document request is a document task write.
		if !orgctx.Must(r.Context()).Has(rbac.PermTenantDocTasksWrite) {
			response.Forbidden(w, r, "tenant.doc_tasks.write is required to answer a document task")
			return
		}
		in.TaskUUID = &id
	}
	v, err := h.svc.Capture(r.Context(), engagementctx.Must(r.Context()), userID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if v.Replayed {
		status = http.StatusOK
	}
	response.JSON(w, r, status, v)
}

// ListCaptures: GET /v1/tenant/whatsapp/captures.
func (h *Handler) ListCaptures(w http.ResponseWriter, r *http.Request) {
	limit, offset := page(r)
	items, total, err := h.svc.ListCaptures(r.Context(), orgctx.Must(r.Context()).ID, Filter{
		Target: r.URL.Query().Get("target"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, limit, offset))
}

// CaptureFile: GET /v1/tenant/engagements/{engagementUuid}/whatsapp/captures/{captureUuid}/file.
func (h *Handler) CaptureFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "captureUuid")
	if !ok {
		return
	}
	f, err := h.svc.CaptureFile(r.Context(), engagementctx.Must(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, f.Name)
	// Always a download: uploaded content is never rendered inline.
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safe, url.PathEscape(f.Name)))
	_, _ = io.Copy(w, f.Body)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, ErrPairingNotFound):
		response.Error(w, r, http.StatusNotFound, "PAIRING_CODE_INVALID", "This pairing code has expired or was already used")
	case errors.Is(err, ErrPairingDisabled):
		response.ServiceUnavailable(w, r, "PAIRING_UNAVAILABLE", "Pairing is not available")
	case errors.Is(err, ErrImpersonating):
		response.Forbidden(w, r, "Stop impersonating before pairing a device")
	case errors.Is(err, authusecase.ErrAccountDeactivated):
		response.Error(w, r, http.StatusForbidden, response.CodeAccountDeactivated, "This account has been deleted")
	case errors.Is(err, authusecase.ErrUserDisabled):
		response.Forbidden(w, r, "User is disabled")
	case errors.Is(err, authusecase.ErrInvalidCredentials):
		response.Unauthorized(w, r, "Account is no longer available")
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "Not found")
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrNeedsImage):
		response.Error(w, r, http.StatusUnsupportedMediaType, "IMAGE_REQUIRED", "expense and income are read from a photo; save a PDF as a document")
	case errors.Is(err, ErrLinkedElsewhere):
		response.Conflict(w, r, "CHAT_LINKED_ELSEWHERE", "This chat is linked to a taxpayer you cannot see")
	case errors.Is(err, doctasks.ErrNotFound):
		response.NotFound(w, r, "Document task not found")
	case errors.Is(err, doctasks.ErrClosed):
		response.Conflict(w, r, "TASK_CLOSED", "The task is no longer open")
	case errors.Is(err, doctasks.ErrEmpty), errors.Is(err, doctasks.ErrTooLarge), errors.Is(err, doctasks.ErrUnsupported):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		// Receipt capture errors (validation, AI unavailable, …).
		receiptshandler.WriteError(w, r, err)
	}
}

func (h *Handler) allow(w http.ResponseWriter, r *http.Request, action, subject string, limit int) bool {
	if h.limiter == nil || strings.TrimSpace(subject) == "" {
		return true
	}
	ok, retry := h.limiter.Allow(r.Context(), action, subject, limit, rateWindow)
	if ok {
		return true
	}
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
	}
	response.TooManyRequests(w, r, "")
	return false
}

func page(r *http.Request) (int32, int32) {
	limit := intconv.ParseInt32(r.URL.Query().Get("limit"), apiquery.DefaultLimit, 1, apiquery.MaxLimit)
	offset := intconv.ParseInt32(r.URL.Query().Get("offset"), 0, 0, 1_000_000)
	return limit, offset
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, name+" is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	return true
}

func userID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}
