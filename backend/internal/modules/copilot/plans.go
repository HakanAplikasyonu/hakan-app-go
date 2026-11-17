package copilot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// Limits.
const (
	MaxCommandLen = 500
	MaxPlanSteps  = 50
	// maxPlanEngagements bounds the engagements a planner considers.
	maxPlanEngagements = 500
)

// Plan statuses (000111 chk_copilot_plans_status).
const (
	PlanProposed  = "proposed"
	PlanRunning   = "running"
	PlanDone      = "done"
	PlanCancelled = "cancelled"
)

// Plan step statuses.
const (
	StepPending = "pending"
	StepOK      = "done"
	StepFailed  = "failed"
	StepSkipped = "skipped"
)

// Planners.
const (
	PlannerAI    = "ai"
	PlannerRules = "rules"
)

// Note codes: what the planner could not do (shown to the user).
const (
	NoteEngagementNotFound = "ENGAGEMENT_NOT_FOUND"
	NoteAmbiguous          = "AMBIGUOUS_ENGAGEMENT"
	NoteNotUnderstood      = "ACTION_NOT_UNDERSTOOD"
	NoteNoPermission       = "NO_PERMISSION"
	NoteNothingToDo        = "NOTHING_TO_DO"
	NoteAlreadyDone        = "ALREADY_DONE"
	NotePrerequisite       = "PREREQUISITE_ADDED"
	NoteTooManySteps       = "TOO_MANY_STEPS"
	NoteAIUnavailable      = "AI_UNAVAILABLE"
	NoteAssistant          = "ASSISTANT_MESSAGE"
)

// PlanStep is one approved-or-not action of a plan.
type PlanStep struct {
	Index          int       `json:"index"`
	Action         string    `json:"action"`
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	TaxpayerTitle  string    `json:"taxpayer_title"`
	Period         string    `json:"period"`
	// Driver of export_transfer: luca | zirve; empty = the engagement's
	// export setting.
	Driver string `json:"driver,omitempty"`
	// Status: pending | done | failed | skipped.
	Status string `json:"status"`
	// ErrorCode is the API error code of a failed step.
	ErrorCode  string     `json:"error_code,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// Note is a remark of the planner.
type Note struct {
	Code          string `json:"code"`
	Action        string `json:"action,omitempty"`
	TaxpayerTitle string `json:"taxpayer_title,omitempty"`
	// Text is the assistant's own sentence (ASSISTANT_MESSAGE) or the part
	// of the command not understood.
	Text string `json:"text,omitempty"`
}

// Plan is the API view of a plan.
type Plan struct {
	UUID      uuid.UUID  `json:"uuid"`
	Command   string     `json:"command"`
	Period    *string    `json:"period"`
	Status    string     `json:"status"`
	Planner   string     `json:"planner"`
	Steps     []PlanStep `json:"steps"`
	Notes     []Note     `json:"notes"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Draft is what a planner proposes before validation.
type Draft struct {
	Period  string
	Steps   []PlanStep
	Notes   []Note
	Planner string
	Model   string
}

// PlanInput is what a planner gets.
type PlanInput struct {
	Viewer  Viewer
	Command string
	// Period is the period the user picked on the screen (may be empty).
	Period      string
	Now         time.Time
	Engagements []EngagementRef
	// Status returns the chains of the given engagements for a period.
	Status func(ctx context.Context, refs []EngagementRef, period string) (map[uuid.UUID]Row, error)
}

// Planner turns a command into a draft plan.
type Planner interface {
	Plan(ctx context.Context, in PlanInput) (Draft, error)
}

// Plans is the plan service.
type Plans struct {
	svc   *Service
	q     Queries
	ai    Planner
	rules Planner
	log   *slog.Logger
}

// NewPlans builds the plan service; ai may be nil (rules only).
func NewPlans(svc *Service, ai Planner, log *slog.Logger) *Plans {
	if log == nil {
		log = slog.Default()
	}
	return &Plans{svc: svc, q: svc.q, ai: ai, rules: RulesPlanner{}, log: log}
}

// Engagements lists every engagement the viewer reaches (up to 500).
func (s *Service) Engagements(ctx context.Context, v Viewer, search string) ([]EngagementRef, error) {
	rows, err := s.q.ListCopilotEngagements(ctx, db.ListCopilotEngagementsParams{
		OrganizationID: v.OrganizationID, AllEngagements: v.ReadAll, UserID: v.UserID, Search: searchParam(search),
		Lim: maxPlanEngagements, Off: 0,
	})
	if err != nil {
		return nil, err
	}
	out := make([]EngagementRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, EngagementRef{ID: r.ID, UUID: r.Uuid, TaxpayerUUID: r.TaxpayerUuid,
			TaxpayerTitle: r.TaxpayerTitle, TaxpayerTaxID: r.TaxpayerTaxID})
	}
	return out, nil
}

// Status returns the chains of refs for a period, by engagement uuid.
func (s *Service) Status(ctx context.Context, organizationID int64, refs []EngagementRef, period string) (map[uuid.UUID]Row, error) {
	rows := make([]Row, 0, len(refs))
	for _, e := range refs {
		rows = append(rows, Row{EngagementUUID: e.UUID, TaxpayerUUID: e.TaxpayerUUID, TaxpayerTitle: e.TaxpayerTitle,
			TaxpayerTaxID: e.TaxpayerTaxID, Period: period, id: e.ID})
	}
	if err := s.fillRows(ctx, organizationID, period, rows); err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]Row, len(rows))
	for _, r := range rows {
		out[r.EngagementUUID] = r
	}
	return out, nil
}

// Create plans a command and stores the proposal. Nothing runs: the user
// approves the steps and the client runs them.
func (p *Plans) Create(ctx context.Context, v Viewer, command, period string) (Plan, error) {
	command = strings.TrimSpace(command)
	if command == "" || utf8.RuneCountInString(command) > MaxCommandLen {
		return Plan{}, fmt.Errorf("%w: command must be 1-%d characters", ErrInvalid, MaxCommandLen)
	}
	if period != "" && !ValidPeriod(period) {
		return Plan{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	refs, err := p.svc.Engagements(ctx, v, "")
	if err != nil {
		return Plan{}, err
	}
	in := PlanInput{Viewer: v, Command: command, Period: period, Now: p.svc.now(), Engagements: refs,
		Status: func(ctx context.Context, r []EngagementRef, period string) (map[uuid.UUID]Row, error) {
			return p.svc.Status(ctx, v.OrganizationID, r, period)
		}}
	var draft Draft
	var aiNote []Note
	if p.ai != nil {
		draft, err = p.ai.Plan(ctx, in)
		if err != nil {
			if ctx.Err() != nil {
				return Plan{}, ctx.Err()
			}
			p.log.WarnContext(ctx, "copilot_ai_planner_failed", "err", err)
			aiNote = []Note{{Code: NoteAIUnavailable}}
		}
	}
	if p.ai == nil || err != nil {
		if draft, err = p.rules.Plan(ctx, in); err != nil {
			return Plan{}, err
		}
		draft.Notes = append(aiNote, draft.Notes...)
	}
	draft = Validate(draft, v, refs)
	return p.insert(ctx, v, command, draft)
}

// Validate keeps the steps the viewer may run: a known write action, an
// engagement the viewer reaches and the action's permissions. Everything
// dropped becomes a note.
func Validate(d Draft, v Viewer, refs []EngagementRef) Draft {
	byUUID := make(map[uuid.UUID]EngagementRef, len(refs))
	for _, e := range refs {
		byUUID[e.UUID] = e
	}
	if d.Period != "" && !ValidPeriod(d.Period) {
		d.Period = ""
	}
	has := v.Has
	if has == nil {
		has = func(string) bool { return false }
	}
	type key struct {
		action string
		eng    uuid.UUID
		period string
	}
	seen := map[key]bool{}
	deniedNoted := map[string]bool{}
	out := make([]PlanStep, 0, len(d.Steps))
	for _, s := range d.Steps {
		spec, ok := Spec(s.Action)
		if !ok || !spec.Write {
			d.Notes = append(d.Notes, Note{Code: NoteNotUnderstood, Text: s.Action})
			continue
		}
		e, ok := byUUID[s.EngagementUUID]
		if !ok {
			d.Notes = append(d.Notes, Note{Code: NoteEngagementNotFound})
			continue
		}
		if !spec.Allowed(has) {
			if !deniedNoted[spec.Name] {
				deniedNoted[spec.Name] = true
				d.Notes = append(d.Notes, Note{Code: NoteNoPermission, Action: spec.Name})
			}
			continue
		}
		if s.Period == "" {
			s.Period = d.Period
		}
		if !ValidPeriod(s.Period) {
			d.Notes = append(d.Notes, Note{Code: NoteNotUnderstood, Action: spec.Name, TaxpayerTitle: e.TaxpayerTitle})
			continue
		}
		if spec.Name != ActionExportTransfer || (s.Driver != "luca" && s.Driver != "zirve") {
			s.Driver = ""
		}
		k := key{spec.Name, e.UUID, s.Period}
		if seen[k] {
			continue
		}
		seen[k] = true
		if len(out) == MaxPlanSteps {
			d.Notes = append(d.Notes, Note{Code: NoteTooManySteps})
			break
		}
		out = append(out, PlanStep{Index: len(out), Action: spec.Name, EngagementUUID: e.UUID, TaxpayerTitle: e.TaxpayerTitle,
			Period: s.Period, Driver: s.Driver, Status: StepPending})
	}
	d.Steps = out
	if d.Notes == nil {
		d.Notes = []Note{}
	}
	return d
}

func (p *Plans) insert(ctx context.Context, v Viewer, command string, d Draft) (Plan, error) {
	steps, err := json.Marshal(d.Steps)
	if err != nil {
		return Plan{}, err
	}
	notes, err := json.Marshal(d.Notes)
	if err != nil {
		return Plan{}, err
	}
	var period, model pgtype.Text
	if d.Period != "" {
		period = pgtype.Text{String: d.Period, Valid: true}
	}
	if d.Model != "" {
		model = pgtype.Text{String: d.Model, Valid: true}
	}
	planner := d.Planner
	if planner != PlannerAI {
		planner = PlannerRules
	}
	status := PlanProposed
	if len(d.Steps) == 0 {
		status = PlanDone
	}
	r, err := p.q.InsertCopilotPlan(ctx, db.InsertCopilotPlanParams{OrganizationID: v.OrganizationID, CreatedBy: v.UserID,
		Command: command, Period: period, Planner: planner, Model: model, Steps: steps, Notes: notes})
	if err != nil {
		return Plan{}, err
	}
	if status != PlanProposed {
		if r, err = p.q.UpdateCopilotPlan(ctx, db.UpdateCopilotPlanParams{OrganizationID: v.OrganizationID,
			CreatedBy: v.UserID, ID: r.ID, Status: status, Steps: steps}); err != nil {
			return Plan{}, err
		}
	}
	return planView(r)
}

func planView(r db.CopilotPlan) (Plan, error) {
	out := Plan{UUID: r.Uuid, Command: r.Command, Status: r.Status, Planner: r.Planner,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time, Steps: []PlanStep{}, Notes: []Note{}}
	if r.Period.Valid {
		p := strings.TrimSpace(r.Period.String)
		out.Period = &p
	}
	if err := json.Unmarshal(r.Steps, &out.Steps); err != nil {
		return Plan{}, fmt.Errorf("copilot: plan steps: %w", err)
	}
	if err := json.Unmarshal(r.Notes, &out.Notes); err != nil {
		return Plan{}, fmt.Errorf("copilot: plan notes: %w", err)
	}
	return out, nil
}

// Get returns one of the viewer's plans.
func (p *Plans) Get(ctx context.Context, v Viewer, id uuid.UUID) (Plan, error) {
	r, err := p.q.GetCopilotPlan(ctx, db.GetCopilotPlanParams{OrganizationID: v.OrganizationID, CreatedBy: v.UserID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, err
	}
	return planView(r)
}

// List returns the viewer's latest plans.
func (p *Plans) List(ctx context.Context, v Viewer, limit int32) ([]Plan, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := p.q.ListCopilotPlans(ctx, db.ListCopilotPlansParams{OrganizationID: v.OrganizationID, CreatedBy: v.UserID, Lim: limit})
	if err != nil {
		return nil, err
	}
	out := make([]Plan, 0, len(rows))
	for _, r := range rows {
		pl, err := planView(r)
		if err != nil {
			return nil, err
		}
		out = append(out, pl)
	}
	return out, nil
}

// StepResult is what the client reports after running (or skipping) a step.
type StepResult struct {
	Status    string `json:"status"`
	ErrorCode string `json:"error_code"`
}

// RecordStep stores the outcome of one step. A plan whose every step is
// settled is done.
func (p *Plans) RecordStep(ctx context.Context, v Viewer, id uuid.UUID, index int, in StepResult) (Plan, error) {
	if in.Status != StepOK && in.Status != StepFailed && in.Status != StepSkipped {
		return Plan{}, fmt.Errorf("%w: status must be done, failed or skipped", ErrInvalid)
	}
	code := strings.TrimSpace(in.ErrorCode)
	if len(code) > 64 {
		return Plan{}, fmt.Errorf("%w: error_code is too long", ErrInvalid)
	}
	if in.Status != StepFailed {
		code = ""
	}
	return p.update(ctx, v, id, func(pl *Plan) error {
		if pl.Status != PlanProposed && pl.Status != PlanRunning {
			return ErrState
		}
		if index < 0 || index >= len(pl.Steps) {
			return ErrNotFound
		}
		st := &pl.Steps[index]
		if st.Status != StepPending {
			return ErrState
		}
		now := p.svc.now().UTC()
		st.Status, st.ErrorCode, st.FinishedAt = in.Status, code, &now
		pl.Status = PlanDone
		for _, s := range pl.Steps {
			if s.Status == StepPending {
				pl.Status = PlanRunning
				break
			}
		}
		return nil
	})
}

// Cancel drops the pending steps of a plan.
func (p *Plans) Cancel(ctx context.Context, v Viewer, id uuid.UUID) (Plan, error) {
	return p.update(ctx, v, id, func(pl *Plan) error {
		if pl.Status != PlanProposed && pl.Status != PlanRunning {
			return ErrState
		}
		now := p.svc.now().UTC()
		for i := range pl.Steps {
			if pl.Steps[i].Status == StepPending {
				pl.Steps[i].Status, pl.Steps[i].FinishedAt = StepSkipped, &now
			}
		}
		pl.Status = PlanCancelled
		return nil
	})
}

func (p *Plans) update(ctx context.Context, v Viewer, id uuid.UUID, fn func(*Plan) error) (Plan, error) {
	r, err := p.q.GetCopilotPlanForUpdate(ctx, db.GetCopilotPlanForUpdateParams{OrganizationID: v.OrganizationID, CreatedBy: v.UserID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, err
	}
	pl, err := planView(r)
	if err != nil {
		return Plan{}, err
	}
	if err := fn(&pl); err != nil {
		return Plan{}, err
	}
	steps, err := json.Marshal(pl.Steps)
	if err != nil {
		return Plan{}, err
	}
	r, err = p.q.UpdateCopilotPlan(ctx, db.UpdateCopilotPlanParams{OrganizationID: v.OrganizationID, CreatedBy: v.UserID,
		ID: r.ID, Status: pl.Status, Steps: steps})
	if err != nil {
		return Plan{}, err
	}
	return planView(r)
}
