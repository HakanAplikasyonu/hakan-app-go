package automation

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/communication"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves Ayarlar > Otomasyonlar (#780 C).
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func actor(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "Kural bulunamadı.")
	case errors.Is(err, ErrInvalid):
		msg := err.Error()
		if i := strings.Index(msg, ": "); i >= 0 {
			msg = msg[i+2:]
		}
		response.BadRequest(w, r, response.CodeValidationError, msg)
	case errors.Is(err, communication.ErrCampaignText):
		response.BadRequest(w, r, response.CodeValidationError, communication.UserMessage(err))
	default:
		response.InternalErr(w, r, err, "Otomasyon isteği işlenemedi.")
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func ruleID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("automationUuid"))
	if err != nil {
		response.NotFound(w, r, "Kural bulunamadı.")
		return uuid.Nil, false
	}
	return id, true
}

// List: GET /v1/tenant/automations.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Overview(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type enabledBody struct {
	Enabled bool `json:"enabled"`
}

// PutSettings: PUT /v1/tenant/automation-settings (the master switch).
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var in enabledBody
	if !decode(w, r, &in) {
		return
	}
	on, err := h.svc.SetEnabled(r.Context(), orgctx.Must(r.Context()).ID, actor(r), in.Enabled)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, enabledBody{Enabled: on})
}

// Create: POST /v1/tenant/automations.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in Input
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.Create(r.Context(), orgctx.Must(r.Context()).ID, actor(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

// Update: PUT /v1/tenant/automations/{automationUuid}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := ruleID(w, r)
	if !ok {
		return
	}
	var in Input
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.Update(r.Context(), orgctx.Must(r.Context()).ID, actor(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type activeBody struct {
	Active bool `json:"active"`
}

// SetActive: PUT /v1/tenant/automations/{automationUuid}/active.
func (h *Handler) SetActive(w http.ResponseWriter, r *http.Request) {
	id, ok := ruleID(w, r)
	if !ok {
		return
	}
	var in activeBody
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.SetActive(r.Context(), orgctx.Must(r.Context()).ID, actor(r), id, in.Active)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Delete: DELETE /v1/tenant/automations/{automationUuid}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := ruleID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), orgctx.Must(r.Context()).ID, actor(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// History: GET /v1/tenant/automations/{automationUuid}/history.
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	id, ok := ruleID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.RuleHistory(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Runs: GET /v1/tenant/automation-runs?status=draft&limit=100.
func (h *Handler) Runs(w http.ResponseWriter, r *http.Request) {
	limit, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 32)
	if err != nil || limit < 1 || limit > 500 {
		limit = 100
	}
	items, err := h.svc.Runs(r.Context(), orgctx.Must(r.Context()).ID, RunFilter{
		Status: strings.TrimSpace(r.URL.Query().Get("status")), Limit: int32(limit)})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type decideBody struct {
	UUIDs   []string `json:"uuids"`
	Approve bool     `json:"approve"`
}

// Decide: POST /v1/tenant/automation-runs/decide (approve or reject drafts).
func (h *Handler) Decide(w http.ResponseWriter, r *http.Request) {
	var in decideBody
	if !decode(w, r, &in) {
		return
	}
	ids := make([]uuid.UUID, 0, len(in.UUIDs))
	for _, s := range in.UUIDs {
		id, err := uuid.Parse(strings.TrimSpace(s))
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "Taslak kimliği geçersiz.")
			return
		}
		ids = append(ids, id)
	}
	res, err := h.svc.Decide(r.Context(), orgctx.Must(r.Context()).ID, actor(r), ids, in.Approve)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}
