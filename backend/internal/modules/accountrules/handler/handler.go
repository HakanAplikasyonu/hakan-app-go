// Package handler serves the account code rule routes (issue #25).
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/usecase"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

const maxBodyBytes = 64 << 10

// Handler serves the routes.
type Handler struct{ svc *usecase.Service }

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

type items[T any] struct {
	Items []T `json:"items"`
}

func userID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

// level returns (org, engagement) — engagement 0 on office routes.
func level(r *http.Request) (int64, int64) {
	if s, ok := engagementctx.From(r.Context()); ok {
		return s.OrganizationID, s.ID
	}
	return orgctx.Must(r.Context()).ID, 0
}

func subject(r *http.Request) usecase.Subject {
	s := engagementctx.Must(r.Context())
	return usecase.Subject{OrganizationID: s.OrganizationID, EngagementID: s.ID, TaxpayerID: s.TaxpayerID}
}

// ListRules: GET .../account-rules?include_retired=true.
func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	org, eng := level(r)
	rules, err := h.svc.Rules(r.Context(), org, eng, r.URL.Query().Get("include_retired") == "true")
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, items[usecase.Rule]{Items: rules})
}

// CreateRule: POST .../account-rules.
func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	var in usecase.RuleInput
	if !decode(w, r, &in) {
		return
	}
	org, eng := level(r)
	rule, err := h.svc.CreateRule(r.Context(), org, eng, in, userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, rule)
}

// UpdateRule: PUT .../account-rules/{ruleKey} → new version.
func (h *Handler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	key, ok := ruleKey(w, r)
	if !ok {
		return
	}
	var in usecase.RuleInput
	if !decode(w, r, &in) {
		return
	}
	org, eng := level(r)
	rule, err := h.svc.UpdateRule(r.Context(), org, eng, key, in, userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rule)
}

// RetireRule: DELETE .../account-rules/{ruleKey}.
func (h *Handler) RetireRule(w http.ResponseWriter, r *http.Request) {
	key, ok := ruleKey(w, r)
	if !ok {
		return
	}
	org, eng := level(r)
	if err := h.svc.RetireRule(r.Context(), org, eng, key); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Versions: GET .../account-rules/{ruleKey}/versions.
func (h *Handler) Versions(w http.ResponseWriter, r *http.Request) {
	key, ok := ruleKey(w, r)
	if !ok {
		return
	}
	org, eng := level(r)
	v, err := h.svc.Versions(r.Context(), org, eng, key)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, items[usecase.Rule]{Items: v})
}

type copied struct {
	Copied int `json:"copied"`
}

// ApplyTemplate: POST .../account-rules/apply-template.
func (h *Handler) ApplyTemplate(w http.ResponseWriter, r *http.Request) {
	org, eng := level(r)
	n, err := h.svc.ApplyTemplate(r.Context(), org, eng)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, copied{Copied: n})
}

// Candidates: GET .../account-rule-candidates.
func (h *Handler) Candidates(w http.ResponseWriter, r *http.Request) {
	cs, err := h.svc.Candidates(r.Context(), subject(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, items[usecase.Candidate]{Items: cs})
}

// Promote: POST .../account-rule-candidates/promote.
func (h *Handler) Promote(w http.ResponseWriter, r *http.Request) {
	var in usecase.PromoteInput
	if !decode(w, r, &in) {
		return
	}
	rule, err := h.svc.Promote(r.Context(), subject(r), in, userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, rule)
}

// Suggest: POST .../invoices/{invoiceUuid}/account-suggestions.
func (h *Handler) Suggest(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("invoiceUuid"))
	if err != nil {
		response.NotFound(w, r, "invoice not found")
		return
	}
	out, err := h.svc.SuggestInvoice(r.Context(), subject(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, items[usecase.SuggestionView]{Items: out})
}

func ruleKey(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	k, err := uuid.Parse(r.PathValue("ruleKey"))
	if err != nil {
		response.NotFound(w, r, "rule not found")
		return uuid.Nil, false
	}
	return k, true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
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
	case errors.Is(err, usecase.ErrInvalid), errors.Is(err, reviewusecase.ErrInvalid), errors.Is(err, usecase.ErrInvalidSettings):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

// RuleSettings: GET /v1/tenant/account-rule-settings (#333).
func (h *Handler) RuleSettings(w http.ResponseWriter, r *http.Request) {
	set, err := h.svc.RuleSettings(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, set)
}

type ruleSettingsBody struct {
	MinCount  int    `json:"rule_candidate_min_count"`
	Decisions string `json:"rule_candidate_decisions"`
}

// PutRuleSettings: PUT /v1/tenant/account-rule-settings — owner / admin
// (tenant.settings.write).
func (h *Handler) PutRuleSettings(w http.ResponseWriter, r *http.Request) {
	var in ruleSettingsBody
	if !decode(w, r, &in) {
		return
	}
	set, err := h.svc.SaveRuleSettings(r.Context(), orgctx.Must(r.Context()).ID,
		usecase.RuleSettings{MinCount: in.MinCount, Decisions: in.Decisions}, userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, set)
}

// PreviewRuleSettings: GET /v1/tenant/account-rule-settings/preview
// ?rule_candidate_min_count=&rule_candidate_decisions= — the candidate
// count today and under the proposed setting; nothing is stored.
func (h *Handler) PreviewRuleSettings(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	cur, err := h.svc.RuleSettings(r.Context(), org)
	if err != nil {
		writeError(w, r, err)
		return
	}
	proposed := usecase.RuleSettings{MinCount: cur.MinCount, Decisions: cur.Decisions}
	q := r.URL.Query()
	if v := q.Get("rule_candidate_min_count"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, r, fmt.Errorf("%w: rule_candidate_min_count must be a number", usecase.ErrInvalidSettings))
			return
		}
		proposed.MinCount = n
	}
	if v := q.Get("rule_candidate_decisions"); v != "" {
		proposed.Decisions = v
	}
	out, err := h.svc.PreviewRuleSettings(r.Context(), org, proposed)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PreviewRule: POST /v1/tenant/account-rules/preview (office template, every
// active engagement) and POST .../engagements/{e}/account-rules/preview —
// what an unsaved rule would match in the last 6 months (#333).
func (h *Handler) PreviewRule(w http.ResponseWriter, r *http.Request) {
	var in usecase.RuleInput
	if !decode(w, r, &in) {
		return
	}
	org, eng := level(r)
	out, err := h.svc.PreviewRule(r.Context(), org, eng, in, time.Now())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
