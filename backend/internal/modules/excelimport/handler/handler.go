// Package handler serves the Excel import routes (04 §7, repo API rules).
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/apiquery"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the routes.
type Handler struct{ svc *usecase.Service }

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// Upload: POST .../excel-imports (multipart: file, data_type, source_system, period, sheet).
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, parse.MaxBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "multipart form with a file is required (max 20 MB)")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, parse.MaxBytes+1))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file could not be read")
		return
	}
	v, err := h.svc.Upload(r.Context(), engagementctx.Must(r.Context()), userID(r), hdr.Filename, data, usecase.UploadOptions{
		DataType: r.FormValue("data_type"), SourceSystem: r.FormValue("source_system"),
		Period: r.FormValue("period"), Sheet: r.FormValue("sheet"),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

// List: GET .../excel-imports.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := page(r)
	items, total, err := h.svc.List(r.Context(), engagementctx.Must(r.Context()), r.URL.Query().Get("status"), limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, limit, offset))
}

// Get: GET .../excel-imports/{importUuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := importID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Get(r.Context(), engagementctx.Must(r.Context()), id)
	h.reply(w, r, v, err)
}

// SetMapping: PUT .../excel-imports/{importUuid}/mapping.
func (h *Handler) SetMapping(w http.ResponseWriter, r *http.Request) {
	id, ok := importID(w, r)
	if !ok {
		return
	}
	var in usecase.MappingInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.SetMapping(r.Context(), engagementctx.Must(r.Context()), id, userID(r), in)
	h.reply(w, r, v, err)
}

// Validate: POST .../excel-imports/{importUuid}/validate.
func (h *Handler) Validate(w http.ResponseWriter, r *http.Request) {
	id, ok := importID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Revalidate(r.Context(), engagementctx.Must(r.Context()), id)
	h.reply(w, r, v, err)
}

// Rows: GET .../excel-imports/{importUuid}/rows?result=.
func (h *Handler) Rows(w http.ResponseWriter, r *http.Request) {
	id, ok := importID(w, r)
	if !ok {
		return
	}
	limit, offset := page(r)
	items, total, err := h.svc.Rows(r.Context(), engagementctx.Must(r.Context()), id, r.URL.Query().Get("result"), limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, limit, offset))
}

// PatchRow: PATCH .../excel-imports/{importUuid}/rows/{rowNo} {"skip": bool}.
func (h *Handler) PatchRow(w http.ResponseWriter, r *http.Request) {
	id, ok := importID(w, r)
	if !ok {
		return
	}
	n, err := strconv.ParseInt(r.PathValue("rowNo"), 10, 32)
	if err != nil || n <= 0 {
		response.BadRequest(w, r, response.CodeValidationError, "rowNo is invalid")
		return
	}
	var in struct {
		Skip *bool `json:"skip"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Skip == nil {
		response.BadRequest(w, r, response.CodeValidationError, "skip is required")
		return
	}
	v, err := h.svc.SkipRow(r.Context(), engagementctx.Must(r.Context()), id, int32(n), *in.Skip)
	h.reply(w, r, v, err)
}

// Commit: POST .../excel-imports/{importUuid}/commit (Idempotency-Key required).
func (h *Handler) Commit(w http.ResponseWriter, r *http.Request) {
	id, ok := importID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Commit(r.Context(), engagementctx.Must(r.Context()), id, userID(r), r.Header.Get("Idempotency-Key"))
	h.reply(w, r, v, err)
}

// Rollback: POST .../excel-imports/{importUuid}/rollback.
func (h *Handler) Rollback(w http.ResponseWriter, r *http.Request) {
	id, ok := importID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Rollback(r.Context(), engagementctx.Must(r.Context()), id, userID(r))
	h.reply(w, r, v, err)
}

// Templates: GET /v1/tenant/excel-import-templates.
func (h *Handler) Templates(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Templates(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// DataTypes: GET /v1/tenant/excel-import-data-types. The importable data
// types with their target fields (#395).
func (h *Handler) DataTypes(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]any{"items": h.svc.DataTypes()})
}

// DeleteTemplate: DELETE /v1/tenant/excel-import-templates/{templateUuid}.
func (h *Handler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("templateUuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "templateUuid is invalid")
		return
	}
	if err := h.svc.DeleteTemplate(r.Context(), orgctx.Must(r.Context()).ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request, v usecase.View, err error) {
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Import or template not found")
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, usecase.ErrState), errors.Is(err, usecase.ErrIdempotency), errors.Is(err, usecase.ErrNoRollback):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

func importID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("importUuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "importUuid is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func page(r *http.Request) (int32, int32) {
	limit, offset := apiquery.DefaultLimit, int32(0)
	limit = intconv.ParseInt32(r.URL.Query().Get("limit"), limit, 1, apiquery.MaxLimit)
	offset = intconv.ParseInt32(r.URL.Query().Get("offset"), offset, 0, 1_000_000)
	return limit, offset
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
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
