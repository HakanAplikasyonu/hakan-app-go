// Package handler serves the receipt capture and vehicle list routes.
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/periodq"
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
	parts, ok := ReadImages(w, r)
	if !ok {
		return
	}
	v, err := h.svc.CaptureParts(r.Context(), engagementctx.Must(r.Context()), userID(r), parts, r.FormValue("kind"), r.Header.Get("Idempotency-Key"))
	WriteCapture(w, r, v, err)
}

// partFields are the multipart fields of a document photographed in parts,
// in order: every "image" file, then image_2 … image_5 (Oğuz'un demo geri
// bildirimi: a long Z report as parça 1/2/3).
var partFields = []string{"image", "image_2", "image_3", "image_4", "image_5"}

// ReadImages reads the photos of one document (1 to usecase.MaxParts, 10 MB
// each) in order. On failure it has written the 400 and returns ok=false.
// Shared with the taxpayer panel.
func ReadImages(w http.ResponseWriter, r *http.Request) ([]usecase.Part, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxParts*(usecase.MaxImageBytes+1<<20))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "multipart form with an image is required (max 10 MB per photo)")
		return nil, false
	}
	var parts []usecase.Part
	for _, field := range partFields {
		for _, hdr := range r.MultipartForm.File[field] {
			if len(parts) == usecase.MaxParts {
				response.BadRequest(w, r, response.CodeValidationError, fmt.Sprintf("at most %d photos per document", usecase.MaxParts))
				return nil, false
			}
			f, err := hdr.Open()
			if err != nil {
				response.BadRequest(w, r, response.CodeValidationError, "image could not be read")
				return nil, false
			}
			data, err := io.ReadAll(io.LimitReader(f, usecase.MaxImageBytes+1))
			_ = f.Close()
			if err != nil {
				response.BadRequest(w, r, response.CodeValidationError, "image could not be read")
				return nil, false
			}
			parts = append(parts, usecase.Part{FileName: hdr.Filename, Data: data})
		}
	}
	if len(parts) == 0 {
		response.BadRequest(w, r, response.CodeValidationError, "image is required")
		return nil, false
	}
	return parts, true
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
	period, ok := periodq.Parse(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.List(r.Context(), engagementctx.Must(r.Context()), r.URL.Query().Get("status"), period, limit, offset)
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
	case errors.Is(err, usecase.ErrPartsOff):
		response.Error(w, r, http.StatusConflict, "Z_REPORT_PARTS_OFF", "capturing a document in parts is turned off for this office")
	case incomplete(err):
		response.Error(w, r, http.StatusUnprocessableEntity, "RECORD_INCOMPLETE", err.Error())
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, usecase.ErrState), errors.Is(err, usecase.ErrConflict):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	case errors.Is(err, usecase.ErrNoWarnings):
		response.Error(w, r, http.StatusServiceUnavailable, "WARNINGS_UNAVAILABLE", "the warning engine is not available")
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

// PartsSettings: GET /v1/tenant/z-report-parts-settings (parçalı Z raporu).
func (h *Handler) PartsSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.PartsSettings(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		response.InternalErr(w, r, err, "could not load z report parts settings")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// SavePartsSettings: PUT /v1/tenant/z-report-parts-settings {"enabled": bool}.
func (h *Handler) SavePartsSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Enabled == nil {
		response.BadRequest(w, r, "INVALID_BODY", "enabled is required")
		return
	}
	st, err := h.svc.SavePartsSettings(r.Context(), orgctx.Must(r.Context()).ID, userID(r), *in.Enabled)
	if err != nil {
		response.InternalErr(w, r, err, "could not save z report parts settings")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}
