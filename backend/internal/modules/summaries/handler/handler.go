// Package handler serves the period summary routes.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/summaries/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the routes.
type Handler struct{ svc *usecase.Service }

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// Generate: POST .../period-summaries {"period":"YYYY-MM"}.
func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Period string `json:"period"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.Generate(r.Context(), engagementctx.Must(r.Context()), userID(r), in.Period)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

// List: GET .../period-summaries?period=.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context(), engagementctx.Must(r.Context()), r.URL.Query().Get("period"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Get: GET .../period-summaries/{summaryUuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Get(r.Context(), engagementctx.Must(r.Context()), id)
	reply(w, r, v, err)
}

// Edit: PUT .../period-summaries/{summaryUuid} {"body": "..."}.
func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.Edit(r.Context(), engagementctx.Must(r.Context()), id, in.Body)
	reply(w, r, v, err)
}

// Approve: POST .../period-summaries/{summaryUuid}/approve.
func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Approve(r.Context(), engagementctx.Must(r.Context()), id, userID(r))
	reply(w, r, v, err)
}

func reply(w http.ResponseWriter, r *http.Request, v usecase.View, err error) {
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var unmatched usecase.UnmatchedError
	switch {
	case errors.As(err, &unmatched):
		details := make([]response.Detail, 0, len(unmatched.Numbers))
		for _, n := range unmatched.Numbers {
			details = append(details, response.Detail{Field: "body", Message: n + " is not backed by portal data", Code: "unmatched_number"})
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, response.CodeValidationError, err.Error(), details)
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Summary not found")
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, usecase.ErrState):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("summaryUuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "summaryUuid is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
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
