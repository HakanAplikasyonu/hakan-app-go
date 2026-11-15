// Package handler serves the VAT forecast routes (issue #33).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/activity"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/apiquery"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

const maxBodyBytes = 8 << 10

// Handler serves /v1/tenant/vat-forecast and the engagement routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler; rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func subject(r *http.Request) usecase.Subject {
	e := engagementctx.Must(r.Context())
	return usecase.Subject{
		OrganizationID: e.OrganizationID, EngagementID: e.ID, EngagementUUID: e.UUID,
		TaxpayerID: e.TaxpayerID, TaxpayerUUID: e.TaxpayerUUID,
	}
}

func (h *Handler) period(r *http.Request) string {
	if p := r.URL.Query().Get("period"); p != "" {
		return p
	}
	return h.svc.CurrentPeriod()
}

// Get: GET /v1/tenant/engagements/{engagementUuid}/vat-forecast.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	f, err := h.svc.Forecast(r.Context(), subject(r), h.period(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, f)
}

type evaluateResponse struct {
	Items []usecase.Evaluation `json:"items"`
}

// Evaluate: POST /v1/tenant/engagements/{engagementUuid}/vat-forecast/evaluate.
// Without ?period= it evaluates last month and this month.
func (h *Handler) Evaluate(w http.ResponseWriter, r *http.Request) {
	periods := h.svc.DefaultEvaluationPeriods()
	if p := r.URL.Query().Get("period"); p != "" {
		periods = []string{p}
	}
	items, err := h.svc.Evaluate(r.Context(), subject(r), periods)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, evaluateResponse{Items: items})
}

// Office: GET /v1/tenant/vat-forecast. Callers without
// tenant.engagements.read_all see only engagements assigned to them.
func (h *Handler) Office(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	q := r.URL.Query()
	limit, offset := apiquery.DefaultLimit, int32(0)
	if v, err := strconv.ParseInt(q.Get("limit"), 10, 32); err == nil && v > 0 && v <= int64(apiquery.MaxLimit) {
		limit = int32(v)
	}
	if v, err := strconv.ParseInt(q.Get("offset"), 10, 32); err == nil && v >= 0 {
		offset = int32(v)
	}
	items, total, err := h.svc.Office(r.Context(), usecase.OfficeFilter{
		OrganizationID: s.ID, AllEngagements: s.Has(rbac.PermTenantEngagementsReadAll), UserID: userID(r),
		Limit: limit, Offset: offset,
	}, h.period(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, limit, offset))
}

type openingsResponse struct {
	Items []usecase.Opening `json:"items"`
}

// ListOpenings: GET .../vat-forecast/openings.
func (h *Handler) ListOpenings(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListOpenings(r.Context(), subject(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, openingsResponse{Items: items})
}

type openingBody struct {
	Amount string  `json:"amount"`
	Note   *string `json:"note"`
}

// PutOpening: PUT .../vat-forecast/openings/{period}.
func (h *Handler) PutOpening(w http.ResponseWriter, r *http.Request) {
	var body openingBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	sub := subject(r)
	uid := userID(r)
	o, err := h.svc.SetOpening(r.Context(), sub, usecase.OpeningInput{
		Period: r.PathValue("period"), Amount: body.Amount, Note: body.Note,
	}, uid)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), &uid, "vat.opening.set", "engagement", &sub.EngagementUUID,
			map[string]any{"period": o.Period, "amount": o.Amount.Dec().StringFixed(2)}, r)
	}
	response.JSON(w, r, http.StatusOK, o)
}

// DeleteOpening: DELETE .../vat-forecast/openings/{period}.
func (h *Handler) DeleteOpening(w http.ResponseWriter, r *http.Request) {
	sub := subject(r)
	period := r.PathValue("period")
	if err := h.svc.DeleteOpening(r.Context(), sub, period); err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		uid := userID(r)
		h.activity.Record(r.Context(), &uid, "vat.opening.delete", "engagement", &sub.EngagementUUID,
			map[string]any{"period": period}, r)
	}
	w.WriteHeader(http.StatusNoContent)
}

func userID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, usecase.ErrInvalidPeriod):
		response.BadRequest(w, r, response.CodeValidationError, "period must be YYYY-MM")
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, usecase.ErrOpeningNotFound):
		response.NotFound(w, r, "Opening carry-forward not found")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
