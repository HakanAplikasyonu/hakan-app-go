// Package handler serves /v1/tenant/engagements/{engagementUuid}/account-plan.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

const maxJSONBytes = 1 << 20

// Handler serves the account plan routes.
type Handler struct{ svc *usecase.Service }

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func subject(r *http.Request) usecase.Subject {
	s := engagementctx.Must(r.Context())
	return usecase.Subject{OrganizationID: s.OrganizationID, EngagementID: s.ID}
}

// Import: POST .../account-plan/imports (multipart "file", optional "target").
func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		response.Error(w, r, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", "file is too large or the form is invalid")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "FILE_REQUIRED", "multipart field \"file\" is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, usecase.MaxUploadBytes+1))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "FILE_UNREADABLE", "could not read the file")
		return
	}
	var uid int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		uid = p.UserInternal
	}
	res, err := h.svc.Import(r.Context(), subject(r), uid, r.FormValue("target"), header.Filename, r.FormValue("period"), data)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, res)
}

type importsResponse struct {
	Items []usecase.Import `json:"items"`
}

// ListImports: GET .../account-plan/imports.
func (h *Handler) ListImports(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Imports(r.Context(), subject(r), 50)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, importsResponse{Items: items})
}

// ListAccounts: GET .../account-plan/accounts?target=&q=&limit=&offset=.
func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := int32(100), int32(0)
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = int32(v) //nolint:gosec // bounded above
	}
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v >= 0 && v <= 1_000_000 {
		offset = int32(v) //nolint:gosec // bounded above
	}
	page, err := h.svc.Accounts(r.Context(), subject(r), q.Get("target"), q.Get("q"), limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

type trialBalancesResponse struct {
	Items []usecase.TrialBalance `json:"items"`
}

// ListTrialBalances: GET .../account-plan/trial-balances — mizan uploads
// with amounts, newest first (#393).
func (h *Handler) ListTrialBalances(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.TrialBalances(r.Context(), subject(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, trialBalancesResponse{Items: items})
}

// TrialBalance: GET .../account-plan/trial-balances/{trialBalance}?compare=&level=
// — one mizan ("latest" or an import uuid) with the period comparison (#393).
func (h *Handler) TrialBalance(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	level := 0
	if v := q.Get("level"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "level must be a number")
			return
		}
		level = n
	}
	res, err := h.svc.TrialBalanceReport(r.Context(), subject(r), r.PathValue("trialBalance"), q.Get("compare"), level)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

type checkRequest struct {
	Target string   `json:"target"`
	Codes  []string `json:"codes"`
}

// Check: POST .../account-plan/check. Read-only: raises no warning.
func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	var in checkRequest
	if !decode(w, r, &in) {
		return
	}
	res, err := h.svc.Check(r.Context(), subject(r), in.Target, in.Codes)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

type missingResponse struct {
	Items []usecase.MissingAccount `json:"items"`
}

// Missing: GET .../account-plan/missing?target= — applied codes the active
// plan lacks (#319), for the missing-account template.
func (h *Handler) Missing(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Missing(r.Context(), subject(r), r.URL.Query().Get("target"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, missingResponse{Items: items})
}

type templateRequest struct {
	Accounts []usecase.TemplateAccount `json:"accounts"`
}

// MissingTemplate: POST .../account-plan/missing-template → .xlsx.
func (h *Handler) MissingTemplate(w http.ResponseWriter, r *http.Request) {
	var in templateRequest
	if !decode(w, r, &in) {
		return
	}
	data, err := usecase.MissingTemplate(in.Accounts, time.Now())
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="eksik-hesaplar.xlsx"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, err.Error())
	case errors.Is(err, usecase.ErrEmpty):
		response.Error(w, r, http.StatusBadRequest, "FILE_EMPTY", "the file has no account codes")
	case errors.Is(err, usecase.ErrTooLarge):
		response.Error(w, r, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", err.Error())
	case errors.Is(err, usecase.ErrUnsupported):
		response.Error(w, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_FILE", err.Error())
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
