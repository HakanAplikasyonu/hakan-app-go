// Package handler serves the period management routes (issue #32):
// readiness, lock / unlock and change requests per engagement, and the
// office-wide engagement × period matrix.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/periods/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/apiquery"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

const maxBodyBytes = 16 << 10

// Handler serves the period routes.
type Handler struct{ svc *usecase.Service }

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func subject(r *http.Request) usecase.Subject {
	e := engagementctx.Must(r.Context())
	return usecase.Subject{OrganizationID: orgctx.Must(r.Context()).ID, EngagementID: e.ID, TaxpayerID: e.TaxpayerID}
}

func userID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

// GetReadiness: GET .../periods/{period}.
func (h *Handler) GetReadiness(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Readiness(r.Context(), subject(r), r.PathValue("period"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type lockInput struct {
	Note string `json:"note"`
}

type unlockInput struct {
	Reason string `json:"reason"`
}

// decode reads an optional JSON body; an empty body leaves v as is.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

// Lock: POST .../periods/{period}/lock.
func (h *Handler) Lock(w http.ResponseWriter, r *http.Request) {
	var in lockInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Lock(r.Context(), subject(r), r.PathValue("period"), in.Note, userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Unlock: POST .../periods/{period}/unlock. A reason is required.
func (h *Handler) Unlock(w http.ResponseWriter, r *http.Request) {
	var in unlockInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Unlock(r.Context(), subject(r), r.PathValue("period"), in.Reason, userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type eventsResponse struct {
	Items []usecase.LockEvent `json:"items"`
}

// ListLockEvents: GET .../periods/{period}/lock-events.
func (h *Handler) ListLockEvents(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.LockEvents(r.Context(), subject(r), r.PathValue("period"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, eventsResponse{Items: items})
}

func page(r *http.Request) (int32, int32) {
	q := r.URL.Query()
	limit, offset := apiquery.DefaultLimit, int32(0)
	limit = intconv.ParseInt32(q.Get("limit"), limit, 1, apiquery.MaxLimit)
	offset = intconv.ParseInt32(q.Get("offset"), offset, 0, 1_000_000)
	return limit, offset
}

// ListChangeRequests: GET .../change-requests. Open requests unless
// status says otherwise ("all" = any status).
func (h *Handler) ListChangeRequests(w http.ResponseWriter, r *http.Request) {
	limit, offset := page(r)
	f := usecase.ChangeRequestFilter{Period: r.URL.Query().Get("period"), Limit: limit, Offset: offset}
	switch st := r.URL.Query().Get("status"); st {
	case "":
		f.Status = usecase.StatusOpen
	case "all":
	default:
		f.Status = st
	}
	rows, total, err := h.svc.ListChangeRequests(r.Context(), subject(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(rows, total, limit, offset))
}

func changeRequestID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("changeRequestUuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "changeRequestUuid is invalid")
		return uuid.Nil, false
	}
	return id, true
}

// GetChangeRequest: GET .../change-requests/{changeRequestUuid}.
func (h *Handler) GetChangeRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := changeRequestID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.GetChangeRequest(r.Context(), subject(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ApplyChangeRequest: POST .../change-requests/{changeRequestUuid}/apply.
func (h *Handler) ApplyChangeRequest(w http.ResponseWriter, r *http.Request) { h.decide(w, r, true) }

// RejectChangeRequest: POST .../change-requests/{changeRequestUuid}/reject.
func (h *Handler) RejectChangeRequest(w http.ResponseWriter, r *http.Request) { h.decide(w, r, false) }

func (h *Handler) decide(w http.ResponseWriter, r *http.Request, apply bool) {
	id, ok := changeRequestID(w, r)
	if !ok {
		return
	}
	var (
		out usecase.ChangeRequest
		err error
	)
	if apply {
		out, err = h.svc.Apply(r.Context(), subject(r), id, userID(r))
	} else {
		out, err = h.svc.Reject(r.Context(), subject(r), id, userID(r))
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Matrix: GET /v1/tenant/periods/matrix. Without
// tenant.engagements.read_all only assigned engagements are listed.
func (h *Handler) Matrix(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	limit, offset := page(r)
	q := r.URL.Query()
	n := 0
	if v := q.Get("periods"); v != "" {
		var err error
		if n, err = strconv.Atoi(v); err != nil || n < 1 || n > usecase.MaxMatrixPeriods {
			response.BadRequest(w, r, response.CodeValidationError, "periods must be 1.."+strconv.Itoa(usecase.MaxMatrixPeriods))
			return
		}
	}
	out, err := h.svc.Matrix(r.Context(), usecase.MatrixFilter{
		OrganizationID: s.ID, AllEngagements: s.Has(rbac.PermTenantEngagementsReadAll), UserID: userID(r),
		Limit: limit, Offset: offset,
	}, q.Get("until"), n)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Change request not found")
	case errors.Is(err, usecase.ErrInvalid), errors.Is(err, usecase.ErrReasonRequired):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, usecase.ErrAlreadyLocked), errors.Is(err, usecase.ErrNotLocked),
		errors.Is(err, usecase.ErrDecided), errors.Is(err, usecase.ErrNotApplicable),
		errors.Is(err, usecase.ErrInvalidPayload):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
