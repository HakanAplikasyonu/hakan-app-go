package privacy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/activity"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/apiquery"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the data subject request routes.
type Handler struct {
	svc   *Service
	audit *activity.Recorder
}

// NewHandler builds the handler; audit may be nil (tests).
func NewHandler(svc *Service, audit *activity.Recorder) *Handler {
	return &Handler{svc: svc, audit: audit}
}

// RegisterRoutes mounts the routes under the active-engagement chain.
func RegisterRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver, engResolver engagementctx.Resolver, h *Handler,
) {
	if orgResolver == nil || engResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(orgResolver)
	eng := middleware.RequireEngagement(engResolver)
	engaged := func(perm string, fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, middleware.RequireTenantPermission(perm), eng)
	}
	req, approve := rbac.PermTenantPrivacyRequest, rbac.PermTenantPrivacyApprove
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/privacy-requests"
	mux.Handle("GET "+base, engaged(req, h.list))
	mux.Handle("POST "+base, engaged(req, h.create))
	mux.Handle("GET "+base+"/{requestUuid}", engaged(req, h.get))
	mux.Handle("POST "+base+"/{requestUuid}/approve", engaged(approve, h.approve))
	mux.Handle("POST "+base+"/{requestUuid}/reject", engaged(approve, h.reject))
	mux.Handle("GET "+base+"/{requestUuid}/export", engaged(approve, h.export))
}

// View is the API shape of a request.
type View struct {
	UUID            uuid.UUID       `json:"uuid"`
	Kind            string          `json:"kind"`
	SubjectType     string          `json:"subject_type"`
	SubjectUserUUID *uuid.UUID      `json:"subject_user_uuid,omitempty"`
	Status          string          `json:"status"`
	Reason          string          `json:"reason"`
	DecisionNote    string          `json:"decision_note"`
	Summary         json.RawMessage `json:"summary"`
	CreatedAt       time.Time       `json:"created_at"`
	DecidedAt       *time.Time      `json:"decided_at,omitempty"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
	ExpiresAt       *time.Time      `json:"expires_at,omitempty"`
}

func (h *Handler) view(r *http.Request, p db.PrivacyRequest) View {
	v := View{
		UUID: p.Uuid, Kind: p.Kind, SubjectType: p.SubjectType, Status: p.Status, Reason: p.Reason,
		DecisionNote: p.DecisionNote, Summary: p.Summary, CreatedAt: p.CreatedAt.Time,
		DecidedAt: ts(p.DecidedAt), CompletedAt: ts(p.CompletedAt), ExpiresAt: ts(p.ExpiresAt),
	}
	if len(v.Summary) == 0 {
		v.Summary = json.RawMessage("{}")
	}
	if p.SubjectUserID.Valid {
		if u, err := h.svc.q.GetPrivacyUser(r.Context(), p.SubjectUserID.Int64); err == nil {
			v.SubjectUserUUID = &u.Uuid
		}
	}
	return v
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := parseInt32(q.Get("limit"), apiquery.DefaultLimit, 1, apiquery.MaxLimit)
	offset := parseInt32(q.Get("offset"), 0, 0, 1_000_000)
	rows, err := h.svc.List(r.Context(), engagementctx.Must(r.Context()), limit, offset)
	if err != nil {
		response.InternalErr(w, r, err, "could not list privacy requests")
		return
	}
	items := make([]View, 0, len(rows))
	for _, p := range rows {
		items = append(items, h.view(r, p))
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type createBody struct {
	Kind            string    `json:"kind"`
	SubjectType     string    `json:"subject_type"`
	SubjectUserUUID uuid.UUID `json:"subject_user_uuid"`
	Reason          string    `json:"reason"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in createBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "request body is not a valid privacy request")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	eng := engagementctx.Must(r.Context())
	req, err := h.svc.Create(r.Context(), eng, p.UserInternal, CreateInput{
		Kind: in.Kind, SubjectType: in.SubjectType, SubjectUser: in.SubjectUserUUID, Reason: in.Reason,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.record(r, p, "privacy.request.created", req)
	response.JSON(w, r, http.StatusCreated, h.view(r, req))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := requestID(w, r)
	if !ok {
		return
	}
	req, err := h.svc.Get(r.Context(), engagementctx.Must(r.Context()), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, h.view(r, req))
}

type decisionBody struct {
	Note string `json:"note"`
}

func (h *Handler) approve(w http.ResponseWriter, r *http.Request) { h.decide(w, r, true) }
func (h *Handler) reject(w http.ResponseWriter, r *http.Request)  { h.decide(w, r, false) }

func (h *Handler) decide(w http.ResponseWriter, r *http.Request, approve bool) {
	id, ok := requestID(w, r)
	if !ok {
		return
	}
	var in decisionBody
	if r.ContentLength != 0 {
		if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
			response.BadRequest(w, r, "INVALID_BODY", "request body is not a valid decision")
			return
		}
	}
	p := authctx.MustPrincipal(r.Context())
	eng := engagementctx.Must(r.Context())
	var (
		req db.PrivacyRequest
		err error
	)
	if approve {
		req, err = h.svc.Approve(r.Context(), eng, p.UserInternal, id, in.Note)
	} else {
		if strings.TrimSpace(in.Note) == "" {
			response.BadRequest(w, r, "NOTE_REQUIRED", "a rejection needs a note")
			return
		}
		req, err = h.svc.Reject(r.Context(), eng, p.UserInternal, id, in.Note)
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	action := "privacy.request.rejected"
	if approve {
		action = "privacy.request.completed"
	}
	h.record(r, p, action, req)
	response.JSON(w, r, http.StatusOK, h.view(r, req))
}

func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	id, ok := requestID(w, r)
	if !ok {
		return
	}
	eng := engagementctx.Must(r.Context())
	req, err := h.svc.Get(r.Context(), eng, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	// Built in memory first: a failure must not leave a half-written download.
	var buf bytes.Buffer
	if err := h.svc.WriteExport(r.Context(), &buf, eng, req); err != nil {
		h.fail(w, r, err)
		return
	}
	h.record(r, authctx.MustPrincipal(r.Context()), "privacy.export.downloaded", req)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="kvkk-`+req.Uuid.String()[:8]+`.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func (h *Handler) record(r *http.Request, p authctx.Principal, action string, req db.PrivacyRequest) {
	if h.audit == nil {
		return
	}
	actor := p.UserInternal
	h.audit.Record(r.Context(), &actor, action, "privacy_request", &req.Uuid, map[string]any{
		"kind": req.Kind, "subject_type": req.SubjectType, "status": req.Status,
		"engagement_uuid": engagementctx.Must(r.Context()).UUID,
	}, r)
}

func requestID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("requestUuid"))
	if err != nil {
		response.NotFound(w, r, "privacy request not found")
		return uuid.UUID{}, false
	}
	return id, true
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "privacy request not found")
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, "INVALID_PRIVACY_REQUEST", err.Error())
	case errors.Is(err, ErrSubjectUnknown):
		response.Error(w, r, http.StatusUnprocessableEntity, "PRIVACY_SUBJECT_UNKNOWN", "the user is not a user of this taxpayer")
	case errors.Is(err, ErrNotPending):
		response.Conflict(w, r, "PRIVACY_REQUEST_DECIDED", "the request has already been decided")
	case errors.Is(err, ErrNotExport):
		response.Conflict(w, r, "PRIVACY_EXPORT_NOT_READY", "only an approved export can be downloaded")
	case errors.Is(err, ErrExpired):
		response.Error(w, r, http.StatusGone, "PRIVACY_EXPORT_EXPIRED", "the export has expired; open a new request")
	default:
		response.InternalErr(w, r, err, "privacy request failed")
	}
}

// parseInt32 parses a 32-bit value within [lo, hi]; anything else is def.
func parseInt32(s string, def, lo, hi int32) int32 {
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil || v < int64(lo) || v > int64(hi) {
		return def
	}
	return int32(v)
}
