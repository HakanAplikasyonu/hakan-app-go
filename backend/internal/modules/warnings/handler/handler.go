// Package handler serves /v1/tenant/warning-rules (office) and
// /v1/tenant/engagements/{engagementUuid}/warning-rules (taxpayer).
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

const maxBodyBytes = 16 << 10

// Rules is what the handler needs from the rules service.
type Rules interface {
	List(ctx context.Context, organizationID, engagementID int64) ([]usecase.RuleView, error)
	Set(ctx context.Context, organizationID, engagementID int64, code string, in usecase.SetInput, userID int64) (usecase.RuleView, error)
	Reset(ctx context.Context, organizationID, engagementID int64, code string) error
}

// Handler serves the warning routes.
type Handler struct {
	rules    Rules
	warnings Warnings
	controls ControlRunner
}

// New builds the handler.
func New(rules Rules) *Handler { return &Handler{rules: rules} }

type listResponse struct {
	Items []usecase.RuleView `json:"items"`
}

// ListOfficeRules: GET /v1/tenant/warning-rules.
func (h *Handler) ListOfficeRules(w http.ResponseWriter, r *http.Request) { h.list(w, r, 0) }

// ListEngagementRules: GET /v1/tenant/engagements/{engagementUuid}/warning-rules.
func (h *Handler) ListEngagementRules(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, engagementctx.Must(r.Context()).ID)
}

// SetOfficeRule: PUT /v1/tenant/warning-rules/{code}.
func (h *Handler) SetOfficeRule(w http.ResponseWriter, r *http.Request) { h.set(w, r, 0) }

// SetEngagementRule: PUT /v1/tenant/engagements/{engagementUuid}/warning-rules/{code}.
func (h *Handler) SetEngagementRule(w http.ResponseWriter, r *http.Request) {
	h.set(w, r, engagementctx.Must(r.Context()).ID)
}

// ResetOfficeRule: DELETE /v1/tenant/warning-rules/{code}.
func (h *Handler) ResetOfficeRule(w http.ResponseWriter, r *http.Request) { h.reset(w, r, 0) }

// ResetEngagementRule: DELETE /v1/tenant/engagements/{engagementUuid}/warning-rules/{code}.
func (h *Handler) ResetEngagementRule(w http.ResponseWriter, r *http.Request) {
	h.reset(w, r, engagementctx.Must(r.Context()).ID)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, engagementID int64) {
	s := orgctx.Must(r.Context())
	items, err := h.rules.List(r.Context(), s.ID, engagementID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, listResponse{Items: items})
}

func (h *Handler) set(w http.ResponseWriter, r *http.Request, engagementID int64) {
	s := orgctx.Must(r.Context())
	var in usecase.SetInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	v, err := h.rules.Set(r.Context(), s.ID, engagementID, r.PathValue("code"), in, userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

func (h *Handler) reset(w http.ResponseWriter, r *http.Request, engagementID int64) {
	s := orgctx.Must(r.Context())
	if err := h.rules.Reset(r.Context(), s.ID, engagementID, r.PathValue("code")); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, usecase.ErrUnknownCode), errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, err.Error())
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
