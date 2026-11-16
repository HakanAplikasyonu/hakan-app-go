// Package handler serves the KVKK legal text routes (issue #57).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/legal/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/clientip"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the routes.
type Handler struct{ svc *usecase.Service }

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func language(r *http.Request) string {
	if l := r.URL.Query().Get("lang"); l != "" {
		return l
	}
	return r.Header.Get("Accept-Language")
}

// List: GET /v1/legal/documents (public).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListCurrent(r.Context(), language(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Get: GET /v1/legal/documents/{kind}?version= (public).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	var (
		d   usecase.Document
		err error
	)
	if v := r.URL.Query().Get("version"); v != "" {
		n, perr := strconv.Atoi(v)
		if perr != nil {
			response.BadRequest(w, r, response.CodeValidationError, "version is invalid")
			return
		}
		d, err = h.svc.Version(r.Context(), kind, language(r), n)
	} else {
		d, err = h.svc.Current(r.Context(), kind, language(r))
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

type acceptBody struct {
	Kind     string `json:"kind"`
	Version  int    `json:"version"`
	Language string `json:"language"`
}

// selfKinds can be acknowledged by the user for themselves.
var selfKinds = map[string]bool{usecase.KindPrivacyNotice: true, usecase.KindRetention: true}

// AcceptMine: POST /v1/me/legal-acceptances (signed in): the user
// acknowledges the privacy notice.
func (h *Handler) AcceptMine(w http.ResponseWriter, r *http.Request) {
	var in acceptBody
	if !decode(w, r, &in) {
		return
	}
	if !selfKinds[in.Kind] {
		response.BadRequest(w, r, response.CodeValidationError, "kind must be privacy_notice or retention_policy")
		return
	}
	a, err := h.svc.Accept(r.Context(), usecase.AcceptInput{
		Kind: in.Kind, Version: in.Version, Language: in.Language, UserID: userID(r), Context: usecase.ContextSelf,
		IP: clientip.From(r), UserAgent: r.UserAgent(),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, a)
}

// Mine: GET /v1/me/legal-acceptances.
func (h *Handler) Mine(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Mine(r.Context(), userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// DPA: GET /v1/tenant/legal/dpa.
func (h *Handler) DPA(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.DPA(r.Context(), orgctx.Must(r.Context()).ID, language(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// AcceptDPA: POST /v1/tenant/legal/dpa/accept {"version": n}.
func (h *Handler) AcceptDPA(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version  int    `json:"version"`
		Language string `json:"language"`
	}
	if !decode(w, r, &in) {
		return
	}
	org := orgctx.Must(r.Context()).ID
	if _, err := h.svc.Accept(r.Context(), usecase.AcceptInput{
		Kind: usecase.KindDPA, Version: in.Version, Language: in.Language, UserID: userID(r),
		Context: usecase.ContextOfficeDPA, OrganizationID: org, IP: clientip.From(r), UserAgent: r.UserAgent(),
	}); err != nil {
		writeError(w, r, err)
		return
	}
	st, err := h.svc.DPA(r.Context(), org, in.Language)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// AdminList: GET /v1/platform/legal/documents.
func (h *Handler) AdminList(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.All(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// AdminCreate: POST /v1/platform/legal/documents (new draft version).
func (h *Handler) AdminCreate(w http.ResponseWriter, r *http.Request) {
	var in usecase.DraftInput
	if !decodeLimit(w, r, &in, 256<<10) {
		return
	}
	d, err := h.svc.CreateDraft(r.Context(), userID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, d)
}

// AdminPublish: POST /v1/platform/legal/documents/{documentUuid}/publish.
func (h *Handler) AdminPublish(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("documentUuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "documentUuid is invalid")
		return
	}
	var in struct {
		LegalReviewed bool `json:"legal_reviewed"`
	}
	if !decode(w, r, &in) {
		return
	}
	d, err := h.svc.Publish(r.Context(), id, in.LegalReviewed)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Legal document not found")
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, usecase.ErrOutdated):
		response.Conflict(w, r, "LEGAL_VERSION_OUTDATED", err.Error())
	case errors.Is(err, usecase.ErrState):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	default:
		response.InternalErr(w, r, err, "Legal request failed")
	}
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeLimit(w, r, dst, 64<<10)
}

func decodeLimit(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
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
