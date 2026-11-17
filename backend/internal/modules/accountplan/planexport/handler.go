package planexport

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the plan file routes under .../account-plan.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func principal(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

// ListSubaccounts: GET .../account-plan/subaccounts?limit=&offset=.
func (h *Handler) ListSubaccounts(w http.ResponseWriter, r *http.Request) {
	limit, offset := paging(r)
	items, err := h.svc.List(r.Context(), engagementctx.Must(r.Context()))
	if err != nil {
		writeError(w, r, err)
		return
	}
	total := len(items)
	start := min(int(offset), total)
	end := min(start+int(limit), total)
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items[start:end], "total": total, "limit": limit, "offset": offset})
}

func paging(r *http.Request) (limit, offset int32) {
	limit = 200
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = int32(v) //nolint:gosec // bounded
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 && v <= 1_000_000 {
		offset = int32(v) //nolint:gosec // bounded
	}
	return limit, offset
}

// allocateRequest is the API body: the cari card only. A VKN / TCKN is
// never an API input here (D1b, TestNoTaxIDExistenceEndpoint); the card
// carries it.
type allocateRequest struct {
	MainCode     string     `json:"main_code"`
	CustomerUUID *uuid.UUID `json:"customer_uuid"`
}

// Allocate: POST .../account-plan/subaccounts (201 new code, 200 existing).
func (h *Handler) Allocate(w http.ResponseWriter, r *http.Request) {
	var in allocateRequest
	if !decode(w, r, &in) {
		return
	}
	if in.CustomerUUID == nil {
		response.BadRequest(w, r, response.CodeValidationError, "customer_uuid is required")
		return
	}
	sub, created, err := h.svc.Allocate(r.Context(), engagementctx.Must(r.Context()), principal(r), AllocateInput{MainCode: in.MainCode, CustomerUUID: in.CustomerUUID})
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	response.JSON(w, r, status, map[string]any{"subaccount": sub, "created": created})
}

// Generate: POST .../account-plan/subaccounts/generate.
func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	var in GenerateInput
	if !decode(w, r, &in) {
		return
	}
	res, err := h.svc.Generate(r.Context(), engagementctx.Must(r.Context()), principal(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

// BuildFile: POST .../account-plan/luca-files → the CSV (recorded in the
// history; X-Export-UUID names the record).
func (h *Handler) BuildFile(w http.ResponseWriter, r *http.Request) {
	var in FileInput
	if !decode(w, r, &in) {
		return
	}
	f, err := h.svc.BuildFile(r.Context(), engagementctx.Must(r.Context()), principal(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", f.FileName))
	w.Header().Set("Content-Length", strconv.Itoa(len(f.Data)))
	w.Header().Set("X-Export-UUID", f.UUID.String())
	w.Header().Set("X-Template-Verified", strconv.FormatBool(f.TemplateVerified))
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(f.Data)
}

// ListFiles: GET .../account-plan/luca-files (download history).
func (h *Handler) ListFiles(w http.ResponseWriter, r *http.Request) {
	limit, offset := int32(50), int32(0)
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = int32(v) //nolint:gosec // bounded
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 && v <= 1_000_000 {
		offset = int32(v) //nolint:gosec // bounded
	}
	items, err := h.svc.Exports(r.Context(), engagementctx.Must(r.Context()), limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items, "limit": limit, "offset": offset})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var fe *ae.FieldError
	switch {
	case errors.As(err, &fe):
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, "REQUIRED_FIELD_MISSING", "a required column is empty",
			[]response.Detail{{Field: fmt.Sprintf("row:%d/%s", fe.Row, fe.Column), Code: "required", Message: fe.Error()}})
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "not found")
	case errors.Is(err, ErrNothingNew):
		response.Error(w, r, http.StatusUnprocessableEntity, "NO_NEW_ACCOUNTS", "Luca already has every account")
	case errors.Is(err, ErrNoFreeCode):
		response.Error(w, r, http.StatusUnprocessableEntity, "NO_FREE_CODE", "every sub account code under the main account is taken")
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ae.ErrTemplate):
		response.BadRequest(w, r, "UNKNOWN_TEMPLATE", err.Error())
	default:
		response.InternalErr(w, r, err, "account plan file failed")
	}
}
