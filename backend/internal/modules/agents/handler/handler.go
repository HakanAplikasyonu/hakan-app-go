// Package handler serves the desktop agent routes (issue #51).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/agents/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the routes.
type Handler struct{ svc *usecase.Service }

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// Register: POST /v1/tenant/agents.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var in usecase.RegisterInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.Register(r.Context(), orgctx.Must(r.Context()).ID, userID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// List: GET /v1/tenant/agents.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Get: GET /v1/tenant/agents/{agentUuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Get(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Heartbeat: POST /v1/tenant/agents/{agentUuid}/heartbeat.
func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in usecase.HeartbeatInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.Heartbeat(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Revoke: DELETE /v1/tenant/agents/{agentUuid}.
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Revoke(r.Context(), orgctx.Must(r.Context()).ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Agent not found")
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Agent request failed")
	}
}

func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("agentUuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "agentUuid is invalid")
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
