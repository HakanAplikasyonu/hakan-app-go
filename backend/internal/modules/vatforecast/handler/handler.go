// Package handler serves the VAT forecast routes (issue #33).
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/notice"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/reportrules"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/watch"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/activity"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/apiquery"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
	"github.com/shopspring/decimal"
)

const maxBodyBytes = 8 << 10

// Handler serves /v1/tenant/vat-forecast and the engagement routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	notices  *notice.Service
	watch    *watch.Watcher
	rules    *reportrules.Service
	// portal: the mükellef panel route (#780), see WithPortal.
	portal PortalQueries
}

// WithRules enables the VAT report rules routes (#780); rs may be nil.
func (h *Handler) WithRules(rs *reportrules.Service) *Handler { h.rules = rs; return h }

// WithWatch enables the VAT watch routes (#780); wt may be nil.
func (h *Handler) WithWatch(wt *watch.Watcher) *Handler { h.watch = wt; return h }

// WithNotices enables the taxpayer VAT notices (#780); n may be nil.
func (h *Handler) WithNotices(n *notice.Service) *Handler { h.notices = n; return h }

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

type noticesResponse struct {
	Items []notice.Notice `json:"items"`
}

// ListNotices: GET .../vat-forecast/notices (#780): what the taxpayer was
// told, newest first.
func (h *Handler) ListNotices(w http.ResponseWriter, r *http.Request) {
	if h.notices == nil {
		response.JSON(w, r, http.StatusOK, noticesResponse{Items: []notice.Notice{}})
		return
	}
	items, err := h.notices.List(r.Context(), subject(r), 20)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, noticesResponse{Items: items})
}

// SendNotice: POST .../vat-forecast/notices {"period"?, "message"?} (#780).
func (h *Handler) SendNotice(w http.ResponseWriter, r *http.Request) {
	if h.notices == nil {
		writeError(w, r, notice.ErrNoRecipients)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in notice.SendInput
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	n, err := h.notices.Send(r.Context(), subject(r), userID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, n)
}

// Tracking: GET /v1/tenant/vat-tracking-settings (#780).
func (h *Handler) Tracking(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Tracking(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

type trackingBody struct {
	ReportEnabled *bool `json:"report_enabled"`
	ReportDay     *int  `json:"report_day"`
	// WatchEnabled is optional; omitted keeps the current value.
	WatchEnabled *bool `json:"watch_enabled"`
	// WatchEmail is optional; omitted keeps the current value.
	WatchEmail *bool `json:"watch_email"`
	// PayableLimit is optional: omitted keeps the threshold, null clears
	// it, a decimal string (or number) sets it (warning_rule_configs).
	PayableLimit json.RawMessage `json:"payable_limit"`
}

// parseLimit reads payable_limit: set false when omitted; nil when null.
func parseLimit(raw json.RawMessage) (limit *decimal.Decimal, set bool, err error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	if string(raw) == "null" {
		return nil, true, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		var n json.Number
		if err := json.Unmarshal(raw, &n); err != nil {
			return nil, true, fmt.Errorf("%w: payable_limit must be a decimal string or null", usecase.ErrInvalid)
		}
		s = n.String()
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return nil, true, fmt.Errorf("%w: payable_limit must be a decimal string or null", usecase.ErrInvalid)
	}
	return &d, true, nil
}

// PutTracking: PUT /v1/tenant/vat-tracking-settings.
func (h *Handler) PutTracking(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var in trackingBody
	if err := dec.Decode(&in); err != nil || in.ReportEnabled == nil || in.ReportDay == nil {
		response.BadRequest(w, r, response.CodeValidationError, "report_enabled and report_day are required")
		return
	}
	limit, limitSet, err := parseLimit(in.PayableLimit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	org := orgctx.Must(r.Context()).ID
	cur, err := h.svc.Tracking(r.Context(), org)
	if err != nil {
		writeError(w, r, err)
		return
	}
	watchOn, watchEmail := cur.WatchEnabled, cur.WatchEmail
	if in.WatchEnabled != nil {
		watchOn = *in.WatchEnabled
	}
	if in.WatchEmail != nil {
		watchEmail = *in.WatchEmail
	}
	if limitSet {
		if err := h.svc.SavePayableLimit(r.Context(), org, userID(r), limit); err != nil {
			writeError(w, r, err)
			return
		}
	}
	st, err := h.svc.SaveTracking(r.Context(), org, userID(r), usecase.TrackingSettings{
		ReportEnabled: *in.ReportEnabled, ReportDay: *in.ReportDay, WatchEnabled: watchOn, WatchEmail: watchEmail,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

type officeWatchResponse struct {
	Items []watch.OfficeEvent `json:"items"`
}

// OfficeWatchEvents: GET /v1/tenant/vat-forecast/watch-events (#780 B):
// the office's latest VAT watch events ("Son KDV uyarıları"), newest
// first, at most 50. Callers without tenant.engagements.read_all see only
// their assigned taxpayers.
func (h *Handler) OfficeWatchEvents(w http.ResponseWriter, r *http.Request) {
	if h.watch == nil {
		response.JSON(w, r, http.StatusOK, officeWatchResponse{Items: []watch.OfficeEvent{}})
		return
	}
	s := orgctx.Must(r.Context())
	limit := int32(20)
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n < 1 {
			response.BadRequest(w, r, response.CodeValidationError, "limit must be a positive number")
			return
		}
		limit = int32(min(n, 50))
	}
	items, err := h.watch.ListOffice(r.Context(), watch.OfficeFilter{
		OrganizationID: s.ID, AllEngagements: s.Has(rbac.PermTenantEngagementsReadAll), UserID: userID(r), Limit: limit,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, officeWatchResponse{Items: items})
}

type watchResponse struct {
	Raised int           `json:"raised"`
	Items  []watch.Event `json:"items"`
}

// ListWatchEvents: GET .../vat-forecast/watch-events (#780): the VAT watch
// events raised for the engagement, newest first.
func (h *Handler) ListWatchEvents(w http.ResponseWriter, r *http.Request) {
	if h.watch == nil {
		response.JSON(w, r, http.StatusOK, watchResponse{Items: []watch.Event{}})
		return
	}
	items, err := h.watch.List(r.Context(), subject(r), 20)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, watchResponse{Items: items})
}

// RunWatch: POST .../vat-forecast/watch (#780): runs the VAT watch for the
// engagement now ("Şimdi kontrol et") and returns the events.
func (h *Handler) RunWatch(w http.ResponseWriter, r *http.Request) {
	if h.watch == nil {
		writeError(w, r, watch.ErrDisabled)
		return
	}
	sub := subject(r)
	n, err := h.watch.CheckNow(r.Context(), sub)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := h.watch.List(r.Context(), sub, 20)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, watchResponse{Raised: n, Items: items})
}

// ReportRules: GET /v1/tenant/vat-report-rules (#780).
func (h *Handler) ReportRules(w http.ResponseWriter, r *http.Request) {
	if h.rules == nil {
		response.JSON(w, r, http.StatusOK, reportrules.List{Items: []reportrules.Rule{}})
		return
	}
	out, err := h.rules.List(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type rulesBody struct {
	Items []reportrules.Rule `json:"items"`
}

// PutReportRules: PUT /v1/tenant/vat-report-rules {"items": [...]} replaces
// the office's rules.
func (h *Handler) PutReportRules(w http.ResponseWriter, r *http.Request) {
	if h.rules == nil {
		writeError(w, r, reportrules.ErrNotFound)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var in rulesBody
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	out, err := h.rules.Save(r.Context(), orgctx.Must(r.Context()).ID, userID(r), in.Items)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// RunReportRule: POST /v1/tenant/vat-report-rules/{rule}/run runs one rule
// now ("Şimdi çalıştır"; {rule} is its uuid or "default"). Once a day.
func (h *Handler) RunReportRule(w http.ResponseWriter, r *http.Request) {
	if h.rules == nil {
		writeError(w, r, reportrules.ErrNotFound)
		return
	}
	res, err := h.rules.RunNow(r.Context(), orgctx.Must(r.Context()).ID, r.PathValue("rule"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
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
	case errors.Is(err, notice.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, notice.ErrNoRecipients):
		response.Error(w, r, http.StatusUnprocessableEntity, "NO_TAXPAYER_USER", "The taxpayer has no panel user to notify")
	case errors.Is(err, reportrules.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, reportrules.ErrNotFound):
		response.NotFound(w, r, "VAT report rule not found")
	case errors.Is(err, watch.ErrDisabled):
		response.Error(w, r, http.StatusConflict, "VAT_WATCH_DISABLED", "The VAT watch is switched off in the office settings")
	case errors.Is(err, usecase.ErrOpeningNotFound):
		response.NotFound(w, r, "Opening carry-forward not found")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
