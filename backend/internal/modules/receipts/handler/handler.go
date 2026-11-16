// Package handler serves the receipt capture and vehicle list routes.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/apiquery"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the routes.
type Handler struct{ svc *usecase.Service }

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// Capture: POST .../receipts (multipart: image, kind).
func (h *Handler) Capture(w http.ResponseWriter, r *http.Request) {
	name, data, ok := ReadImage(w, r)
	if !ok {
		return
	}
	v, err := h.svc.CaptureKeyed(r.Context(), engagementctx.Must(r.Context()), userID(r), name, data, r.FormValue("kind"), r.Header.Get("Idempotency-Key"))
	WriteCapture(w, r, v, err)
}

// ReadImage reads the multipart "image" field (max 10 MB). On failure it has
// written the 400 and returns ok=false. Shared with the taxpayer panel (#199).
func ReadImage(w http.ResponseWriter, r *http.Request) (string, []byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxImageBytes+1<<20)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "multipart form with an image is required (max 10 MB)")
		return "", nil, false
	}
	f, hdr, err := r.FormFile("image")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "image is required")
		return "", nil, false
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, usecase.MaxImageBytes+1))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "image could not be read")
		return "", nil, false
	}
	return hdr.Filename, data, true
}

// WriteCapture answers a capture: 201, or 200 when it was replayed from an
// earlier upload with the same Idempotency-Key.
func WriteCapture(w http.ResponseWriter, r *http.Request, v usecase.View, err error) {
	if err != nil {
		WriteError(w, r, err)
		return
	}
	status := http.StatusCreated
	if v.Replayed {
		status = http.StatusOK
	}
	response.JSON(w, r, status, v)
}

// List: GET .../receipts.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := apiquery.DefaultLimit, int32(0)
	limit = intconv.ParseInt32(r.URL.Query().Get("limit"), limit, 1, apiquery.MaxLimit)
	offset = intconv.ParseInt32(r.URL.Query().Get("offset"), offset, 0, 1_000_000)
	items, total, err := h.svc.List(r.Context(), engagementctx.Must(r.Context()), r.URL.Query().Get("status"), limit, offset)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, limit, offset))
}

// Get: GET .../receipts/{receiptUuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "receiptUuid")
	if !ok {
		return
	}
	v, err := h.svc.Get(r.Context(), engagementctx.Must(r.Context()), id)
	reply(w, r, v, err)
}

// Approve / Reject: POST .../receipts/{receiptUuid}/approve|reject.
func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) { h.decide(w, r, true) }
func (h *Handler) Reject(w http.ResponseWriter, r *http.Request)  { h.decide(w, r, false) }

func (h *Handler) decide(w http.ResponseWriter, r *http.Request, approve bool) {
	id, ok := pathID(w, r, "receiptUuid")
	if !ok {
		return
	}
	v, err := h.svc.Decide(r.Context(), engagementctx.Must(r.Context()), id, userID(r), approve)
	reply(w, r, v, err)
}

// Vehicles: GET .../vehicles.
func (h *Handler) Vehicles(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Vehicles(r.Context(), engagementctx.Must(r.Context()))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// AddVehicle: POST .../vehicles.
func (h *Handler) AddVehicle(w http.ResponseWriter, r *http.Request) {
	var in usecase.VehicleInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	v, err := h.svc.AddVehicle(r.Context(), engagementctx.Must(r.Context()), userID(r), in)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

// DeleteVehicle: DELETE .../vehicles/{vehicleUuid}.
func (h *Handler) DeleteVehicle(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "vehicleUuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteVehicle(r.Context(), engagementctx.Must(r.Context()), id); err != nil {
		WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func reply(w http.ResponseWriter, r *http.Request, v usecase.View, err error) {
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// WriteError maps service errors to responses.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Receipt or vehicle not found")
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, usecase.ErrState), errors.Is(err, usecase.ErrConflict):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	case errors.Is(err, usecase.ErrNoReviewQueue):
		response.Error(w, r, http.StatusServiceUnavailable, "REVIEW_UNAVAILABLE", "the review queue is not available")
	case errors.Is(err, extract.ErrAIUnavailable):
		response.Error(w, r, http.StatusServiceUnavailable, "AI_UNAVAILABLE", "AI is not configured")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, name+" is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func userID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}
