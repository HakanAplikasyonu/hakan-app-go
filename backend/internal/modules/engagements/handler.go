package engagements

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

const maxBodyBytes = 4 << 10

// Handler serves /v1/tenant/engagements.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

type page struct {
	Items  []Engagement `json:"items"`
	Total  int64        `json:"total"`
	Limit  int32        `json:"limit"`
	Offset int32        `json:"offset"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	limit, offset := pageParams(r)
	items, total, err := h.svc.List(r.Context(), s, p.UserInternal, strings.TrimSpace(r.URL.Query().Get("status")), limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page{Items: items, Total: total, Limit: limit, Offset: offset})
}

type createRequest struct {
	TaxpayerUUID uuid.UUID `json:"taxpayer_uuid"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	var req createRequest
	if !decode(w, r, &req) {
		return
	}
	e, err := h.svc.Create(r.Context(), s.ID, req.TaxpayerUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, e)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	e := engagementctx.Must(r.Context())
	out, err := h.svc.Get(r.Context(), e.OrganizationID, e.UUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type contextDTO struct {
	Engagement Engagement `json:"engagement"`
	ViaReadAll bool       `json:"via_read_all"`
}

// context is the first route behind the strict RequireEngagement: the
// shape every mükellef data route follows (active engagement, assigned or
// read_all).
func (h *Handler) context(w http.ResponseWriter, r *http.Request) {
	e := engagementctx.Must(r.Context())
	out, err := h.svc.Get(r.Context(), e.OrganizationID, e.UUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, contextDTO{Engagement: out, ViaReadAll: e.ViaReadAll})
}

func (h *Handler) end(w http.ResponseWriter, r *http.Request) {
	e := engagementctx.Must(r.Context())
	out, err := h.svc.End(r.Context(), e.OrganizationID, e.UUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type assignmentList struct {
	Items []Assignment `json:"items"`
}

func (h *Handler) listAssignments(w http.ResponseWriter, r *http.Request) {
	e := engagementctx.Must(r.Context())
	items, err := h.svc.ListAssignments(r.Context(), e.OrganizationID, e.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, assignmentList{Items: items})
}

func (h *Handler) assign(w http.ResponseWriter, r *http.Request) {
	e := engagementctx.Must(r.Context())
	userUUID, ok := pathUUID(w, r, "userUuid")
	if !ok {
		return
	}
	if err := h.svc.Assign(r.Context(), e.OrganizationID, e.ID, userUUID); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) unassign(w http.ResponseWriter, r *http.Request) {
	e := engagementctx.Must(r.Context())
	userUUID, ok := pathUUID(w, r, "userUuid")
	if !ok {
		return
	}
	if err := h.svc.Unassign(r.Context(), e.OrganizationID, e.ID, userUUID); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, ErrAlreadyActive):
		response.Conflict(w, r, response.CodeEngagementAlreadyActive, "Taxpayer already has an active engagement")
	case errors.Is(err, ErrEnded):
		response.Conflict(w, r, response.CodeEngagementEnded, "Engagement has ended")
	case errors.Is(err, ErrTaxpayerNotFound), errors.Is(err, ErrNotFound),
		errors.Is(err, ErrMemberNotFound), errors.Is(err, ErrAssignmentNotFound):
		response.NotFound(w, r, "Resource was not found")
	case errors.Is(err, ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

func pageParams(r *http.Request) (int32, int32) {
	limit, offset := int32(50), int32(0)
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 100 {
		limit = int32(v)
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = int32(v)
	}
	return limit, offset
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, name+" is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}
