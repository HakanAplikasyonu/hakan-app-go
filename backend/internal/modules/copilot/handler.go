package copilot

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the co-pilot routes.
type Handler struct {
	svc   *Service
	plans *Plans
}

// NewHandler builds the handler; planner may be nil (rules only).
func NewHandler(svc *Service, plans *Plans) *Handler { return &Handler{svc: svc, plans: plans} }

func viewer(r *http.Request) Viewer {
	org, _ := orgctx.From(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	return Viewer{OrganizationID: org.ID, UserID: p.UserInternal, ReadAll: org.Has(rbac.PermTenantEngagementsReadAll), Has: org.Has}
}

// Workflow: GET /v1/tenant/copilot/workflow?period=&q=&step=&summary=&limit=&offset=.
func (h *Handler) Workflow(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	summary := strings.TrimSpace(q.Get("summary"))
	page, err := h.svc.Workflow(r.Context(), viewer(r), Filter{
		Period: strings.TrimSpace(q.Get("period")), Search: q.Get("q"),
		Step: strings.TrimSpace(q.Get("step")), Summary: summary == "true" || summary == "1",
		Limit: intconv.ParseInt32(q.Get("limit"), 50, 1, 200), Offset: intconv.ParseInt32(q.Get("offset"), 0, 0, 1<<30),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

// EngagementWorkflow: GET /v1/tenant/engagements/{engagementUuid}/copilot/workflow?period=.
func (h *Handler) EngagementWorkflow(w http.ResponseWriter, r *http.Request) {
	sc := engagementctx.Must(r.Context())
	e, err := h.svc.q.GetCopilotEngagement(r.Context(), db.GetCopilotEngagementParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID})
	// Mükellef görünmüyorsa (kapsam dışı) 500 değil 404: kart sessizce kalkar.
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "Engagement not found")
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	row, err := h.svc.One(r.Context(), sc.OrganizationID, EngagementRef{ID: e.ID, UUID: e.Uuid, TaxpayerUUID: e.TaxpayerUuid,
		TaxpayerTitle: e.TaxpayerTitle, TaxpayerTaxID: e.TaxpayerTaxID}, strings.TrimSpace(r.URL.Query().Get("period")))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, row)
}

type createInput struct {
	Command string `json:"command"`
	Period  string `json:"period"`
}

// CreatePlan: POST /v1/tenant/copilot/plans — plan a command. The plan is
// a proposal; nothing runs.
func (h *Handler) CreatePlan(w http.ResponseWriter, r *http.Request) {
	var in createInput
	if !decode(w, r, &in) {
		return
	}
	pl, err := h.plans.Create(r.Context(), viewer(r), in.Command, strings.TrimSpace(in.Period))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, pl)
}

// ListPlans: GET /v1/tenant/copilot/plans — the caller's latest plans.
func (h *Handler) ListPlans(w http.ResponseWriter, r *http.Request) {
	items, err := h.plans.List(r.Context(), viewer(r), intconv.ParseInt32(r.URL.Query().Get("limit"), 10, 1, 50))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func planID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("planUuid"))
	if err != nil {
		response.NotFound(w, r, "Plan not found")
		return uuid.Nil, false
	}
	return id, true
}

// GetPlan: GET /v1/tenant/copilot/plans/{planUuid}.
func (h *Handler) GetPlan(w http.ResponseWriter, r *http.Request) {
	id, ok := planID(w, r)
	if !ok {
		return
	}
	pl, err := h.plans.Get(r.Context(), viewer(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, pl)
}

// RecordStep: POST /v1/tenant/copilot/plans/{planUuid}/steps/{stepIndex} —
// the outcome of a step the client ran (or skipped) after approval.
func (h *Handler) RecordStep(w http.ResponseWriter, r *http.Request) {
	id, ok := planID(w, r)
	if !ok {
		return
	}
	idx, err := strconv.Atoi(r.PathValue("stepIndex"))
	if err != nil || idx < 0 {
		response.NotFound(w, r, "Step not found")
		return
	}
	var in StepResult
	if !decode(w, r, &in) {
		return
	}
	pl, err := h.plans.RecordStep(r.Context(), viewer(r), id, idx, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, pl)
}

// CancelPlan: POST /v1/tenant/copilot/plans/{planUuid}/cancel.
func (h *Handler) CancelPlan(w http.ResponseWriter, r *http.Request) {
	id, ok := planID(w, r)
	if !ok {
		return
	}
	pl, err := h.plans.Cancel(r.Context(), viewer(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, pl)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
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
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, strings.TrimPrefix(err.Error(), "copilot: invalid input: "))
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "Plan or step not found")
	case errors.Is(err, ErrState):
		response.Conflict(w, r, "PLAN_STATE", "The plan or step is already settled")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
