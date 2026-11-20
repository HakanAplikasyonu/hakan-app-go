// Package handler serves the review queue routes.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/activity"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/periodq"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/apiquery"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

const maxBodyBytes = 64 << 10

// Handler serves /v1/tenant/review/* and the engagement suggestion routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler; rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// SuggestionDTO is a suggestion in the API.
type SuggestionDTO struct {
	UUID           uuid.UUID          `json:"uuid"`
	EngagementUUID *uuid.UUID         `json:"engagement_uuid,omitempty"`
	Kind           string             `json:"kind"`
	TargetType     string             `json:"target_type"`
	TargetID       *int64             `json:"target_id"`
	TargetField    string             `json:"target_field"`
	Period         *string            `json:"period"`
	ProposedValue  json.RawMessage    `json:"proposed_value"`
	Confidence     string             `json:"confidence"`
	Rationale      string             `json:"rationale"`
	Provenance     usecase.Provenance `json:"provenance"`
	Status         string             `json:"status"`
	AppliedValue   json.RawMessage    `json:"applied_value"`
	PreviousValue  json.RawMessage    `json:"previous_value"`
	RuleCandidate  bool               `json:"rule_candidate"`
	DecidedAt      *time.Time         `json:"decided_at"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// DecisionDTO is a decision-log entry.
type DecisionDTO struct {
	Action     string          `json:"action"`
	ActorUUID  *uuid.UUID      `json:"actor_uuid"`
	Reason     string          `json:"reason"`
	Value      json.RawMessage `json:"value"`
	Confidence string          `json:"confidence"`
	Threshold  *string         `json:"threshold"`
	CreatedAt  time.Time       `json:"created_at"`
}

func toDTO(s usecase.Suggestion) SuggestionDTO {
	d := SuggestionDTO{
		UUID: s.UUID, Kind: s.Input.Kind, TargetType: s.Input.TargetType, TargetID: s.Input.TargetID,
		TargetField: s.Input.TargetField, ProposedValue: s.Input.Value, Confidence: s.Input.Confidence.StringFixed(4),
		Rationale: engine.HumanizeRationale(s.Input.Rationale), Provenance: s.Input.Provenance, Status: string(s.Status),
		AppliedValue: nullJSON(s.AppliedValue), PreviousValue: nullJSON(s.PreviousValue),
		RuleCandidate: s.RuleCandidate, DecidedAt: s.DecidedAt, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
	if s.Input.Period != "" {
		p := s.Input.Period
		d.Period = &p
	}
	if d.Provenance.Inputs == nil {
		d.Provenance.Inputs = map[string]any{}
	}
	return d
}

// ListOffice: GET /v1/tenant/review/suggestions. Without
// tenant.engagements.read_all only assigned engagements are listed.
func (h *Handler) ListOffice(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	h.list(w, r, usecase.Filter{OrganizationID: s.ID, AllEngagements: s.Has(rbac.PermTenantEngagementsReadAll), UserID: userID(r)})
}

// ListEngagement: GET /v1/tenant/engagements/{engagementUuid}/suggestions.
func (h *Handler) ListEngagement(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	h.list(w, r, usecase.Filter{OrganizationID: s.ID, EngagementID: engagementctx.Must(r.Context()).ID, AllEngagements: true})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, f usecase.Filter) {
	q := r.URL.Query()
	var ok bool
	if f.Period, ok = periodq.Parse(w, r); !ok {
		return
	}
	f.Limit, f.Offset = apiquery.DefaultLimit, 0
	f.Limit = intconv.ParseInt32(q.Get("limit"), f.Limit, 1, apiquery.MaxLimit)
	f.Offset = intconv.ParseInt32(q.Get("offset"), f.Offset, 0, 1_000_000)
	// The queue view: pending unless the caller asks otherwise ("all" = any).
	switch st := q.Get("status"); st {
	case "":
		f.Status = string(usecase.StatusPending)
	case "all":
	default:
		f.Status = st
	}
	f.Kind = q.Get("kind")
	if v := q.Get("invoice"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "invoice must be a uuid")
			return
		}
		f.Invoice = id.String()
	}
	if v := q.Get("rule_candidate"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "rule_candidate must be true or false")
			return
		}
		f.RuleCandidate = &b
	}
	rows, total, err := h.svc.List(r.Context(), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items := make([]SuggestionDTO, 0, len(rows))
	for _, row := range rows {
		d := toDTO(row.Suggestion)
		eu := row.EngagementUUID
		if eu != uuid.Nil {
			d.EngagementUUID = &eu
		}
		items = append(items, d)
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

type detail struct {
	SuggestionDTO
	Decisions []DecisionDTO `json:"decisions"`
}

// Get: GET /v1/tenant/engagements/{engagementUuid}/suggestions/{suggestionUuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	sug, log, err := h.svc.Get(r.Context(), subject(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := detail{SuggestionDTO: toDTO(sug), Decisions: make([]DecisionDTO, 0, len(log))}
	for _, d := range log {
		dto := DecisionDTO{Action: string(d.Action), ActorUUID: d.ActorUUID, Reason: d.Reason,
			Value: nullJSON(d.Value), Confidence: d.Confidence.StringFixed(4), CreatedAt: d.CreatedAt}
		if d.Threshold != nil {
			t := d.Threshold.StringFixed(4)
			dto.Threshold = &t
		}
		out.Decisions = append(out.Decisions, dto)
	}
	response.JSON(w, r, http.StatusOK, out)
}

type decisionBody struct {
	Reason string          `json:"reason"`
	Value  json.RawMessage `json:"value"`
}

// Accept, Reject, Correct, Revert: POST .../suggestions/{suggestionUuid}/{action}.
func (h *Handler) Accept(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, usecase.ActionAccept)
}
func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, usecase.ActionReject)
}
func (h *Handler) Correct(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, usecase.ActionCorrect)
}
func (h *Handler) Revert(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, usecase.ActionRevert)
}

func (h *Handler) decide(w http.ResponseWriter, r *http.Request, action usecase.Action) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body decisionBody
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	ctx, sub, uid := r.Context(), subject(r), userID(r)
	var sug usecase.Suggestion
	var err error
	switch action {
	case usecase.ActionAccept:
		sug, err = h.svc.Accept(ctx, sub, id, uid, body.Reason)
	case usecase.ActionReject:
		sug, err = h.svc.Reject(ctx, sub, id, uid, body.Reason)
	case usecase.ActionCorrect:
		sug, err = h.svc.Correct(ctx, sub, id, uid, body.Value, body.Reason)
	case usecase.ActionRevert:
		sug, err = h.svc.Revert(ctx, sub, id, uid, body.Reason)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		actor := uid
		h.activity.Record(ctx, &actor, "review.suggestion."+string(action), "suggestion", &sug.UUID,
			map[string]any{"kind": sug.Input.Kind, "status": sug.Status, "reason": body.Reason}, r)
	}
	response.JSON(w, r, http.StatusOK, toDTO(sug))
}

type settingsDTO struct {
	Rows      []settingRowDTO           `json:"rows"`
	Default   usecase.Policy            `json:"default"`
	Effective map[string]usecase.Policy `json:"effective"`
}

type settingRowDTO struct {
	Level      string           `json:"level"`
	Kind       string           `json:"kind"`
	AutoAccept *bool            `json:"auto_accept"`
	Threshold  *decimal.Decimal `json:"threshold"`
}

// GetOfficeSettings / GetEngagementSettings return the thresholds.
func (h *Handler) GetOfficeSettings(w http.ResponseWriter, r *http.Request) { h.getSettings(w, r, 0) }
func (h *Handler) GetEngagementSettings(w http.ResponseWriter, r *http.Request) {
	h.getSettings(w, r, engagementctx.Must(r.Context()).ID)
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request, eng int64) {
	v, err := h.svc.Settings(r.Context(), orgctx.Must(r.Context()).ID, eng)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := settingsDTO{Rows: []settingRowDTO{}, Default: v.Default, Effective: v.Effective}
	for _, row := range v.Rows {
		level := "office"
		if row.EngagementID > 0 {
			level = "engagement"
		}
		out.Rows = append(out.Rows, settingRowDTO{Level: level, Kind: row.Kind, AutoAccept: row.AutoAccept, Threshold: row.Threshold})
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ThresholdImpact: GET /v1/tenant/review/threshold-impact?threshold=&months=&kind=
// (issue #333): what an office-wide threshold would have done. Without
// tenant.engagements.read_all only assigned engagements are counted.
func (h *Handler) ThresholdImpact(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	raw := q.Get("threshold")
	if raw == "" {
		response.BadRequest(w, r, response.CodeValidationError, "threshold is required")
		return
	}
	thr, err := decimal.NewFromString(raw)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "threshold must be a decimal")
		return
	}
	months := 0
	if v := q.Get("months"); v != "" {
		if months, err = strconv.Atoi(v); err != nil || months <= 0 {
			response.BadRequest(w, r, response.CodeValidationError, "months must be between 1 and 24")
			return
		}
	}
	s := orgctx.Must(r.Context())
	out, err := h.svc.ThresholdImpact(r.Context(), usecase.ImpactFilter{
		OrganizationID: s.ID, AllEngagements: s.Has(rbac.PermTenantEngagementsReadAll), UserID: userID(r),
		Threshold: thr, Months: months, Kind: q.Get("kind"),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type putSetting struct {
	Kind       string  `json:"kind"`
	AutoAccept *bool   `json:"auto_accept"`
	Threshold  *string `json:"threshold"`
}

// PutOfficeSetting / PutEngagementSetting write one kind's setting.
func (h *Handler) PutOfficeSetting(w http.ResponseWriter, r *http.Request) { h.putSetting(w, r, 0) }
func (h *Handler) PutEngagementSetting(w http.ResponseWriter, r *http.Request) {
	h.putSetting(w, r, engagementctx.Must(r.Context()).ID)
}

func (h *Handler) putSetting(w http.ResponseWriter, r *http.Request, eng int64) {
	var in putSetting
	if !decode(w, r, &in) {
		return
	}
	row := usecase.SettingRow{Kind: in.Kind, AutoAccept: in.AutoAccept}
	if in.Threshold != nil {
		t, err := decimal.NewFromString(*in.Threshold)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "threshold must be a decimal string")
			return
		}
		row.Threshold = &t
	}
	if err := h.svc.SetSetting(r.Context(), orgctx.Must(r.Context()).ID, eng, row, userID(r)); err != nil {
		writeError(w, r, err)
		return
	}
	h.getSettings(w, r, eng)
}

// DeleteOfficeSetting / DeleteEngagementSetting remove one kind's setting.
func (h *Handler) DeleteOfficeSetting(w http.ResponseWriter, r *http.Request) {
	h.deleteSetting(w, r, 0)
}
func (h *Handler) DeleteEngagementSetting(w http.ResponseWriter, r *http.Request) {
	h.deleteSetting(w, r, engagementctx.Must(r.Context()).ID)
}

func (h *Handler) deleteSetting(w http.ResponseWriter, r *http.Request, eng int64) {
	if err := h.svc.DeleteSetting(r.Context(), orgctx.Must(r.Context()).ID, eng, r.PathValue("kind")); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func subject(r *http.Request) usecase.Subject {
	e := engagementctx.Must(r.Context())
	return usecase.Subject{OrganizationID: orgctx.Must(r.Context()).ID, EngagementID: e.ID, TaxpayerID: e.TaxpayerID}
}

func userID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("suggestionUuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "suggestionUuid is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Suggestion or setting not found")
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, usecase.ErrState), errors.Is(err, usecase.ErrNoApplier):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

func nullJSON(v json.RawMessage) json.RawMessage {
	if len(v) == 0 {
		return json.RawMessage("null")
	}
	return v
}
