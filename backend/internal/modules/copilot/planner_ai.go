package copilot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	aitools "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/tools"
	aiusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// AI is the subset of the AI service the planner uses.
type AI interface {
	Enabled() bool
	RunTools(ctx context.Context, call aiusecase.Call, reg *aitools.Registry, env aitools.Env, maxSteps int) (aiusecase.Result, error)
}

// AIPlanner plans with the office's AI provider through tool calling: the
// model reads the engagements and their chains with read tools and hands
// in the plan with propose_plan, which only records it. The plan is then
// validated like any other (Validate) and waits for the user's approval.
type AIPlanner struct {
	AI AI
	// Scope is the caller's organization scope (tool permissions).
	Scope func(ctx context.Context) (orgctx.Scope, bool)
}

// NewAIPlanner returns nil when the service has no provider.
func NewAIPlanner(ai AI) Planner {
	if ai == nil || !ai.Enabled() {
		return nil
	}
	return &AIPlanner{AI: ai, Scope: orgctx.From}
}

// ErrNoProposal: the model ended without calling propose_plan.
var ErrNoProposal = errors.New("copilot: the model proposed no plan")

const systemPrompt = `You are the co-pilot of a Turkish accounting office (mali müşavir). The user types a command in Turkish
about closing a month for one or more taxpayers (mükellef). Turn it into a plan of actions from the catalog
below and hand it in with the propose_plan tool. You never run anything: the user approves each step.

Rules:
- Find the taxpayers with list_engagements (search by a word of the title); never invent an engagement_uuid.
- Check engagement_status for the period before proposing: propose only steps that have work, in the
  order of the chain (bank suggest → bank approve → journal drafts → approve drafts → transfer → report).
- If a requested step needs an earlier one (for example approving drafts before a Luca transfer), add it first.
- period is YYYY-MM. Turkish month names: ocak=01 şubat=02 mart=03 nisan=04 mayıs=05 haziran=06 temmuz=07
  ağustos=08 eylül=09 ekim=10 kasım=11 aralık=12. Without a year, use the latest such month not in the future.
- message is one or two short Turkish sentences for the accountant, in plain professional language,
  without field names, JSON or uuids.
- If the command is not about these actions, call propose_plan with no steps and say so in message.

Actions:
`

func catalogPrompt() string {
	var sb strings.Builder
	for _, a := range Catalog {
		if !a.Write {
			continue
		}
		sb.WriteString("- ")
		sb.WriteString(a.Name)
		sb.WriteString(": ")
		sb.WriteString(a.Description)
		sb.WriteString("\n")
	}
	return sb.String()
}

type proposal struct {
	Period string `json:"period"`
	Steps  []struct {
		Action         string `json:"action"`
		EngagementUUID string `json:"engagement_uuid"`
		Period         string `json:"period"`
		Driver         string `json:"driver"`
	} `json:"steps"`
	Message string `json:"message"`
}

// Plan implements Planner.
func (p *AIPlanner) Plan(ctx context.Context, in PlanInput) (Draft, error) {
	scope, ok := p.Scope(ctx)
	if !ok {
		return Draft{}, errors.New("copilot: no organization scope")
	}
	var mu sync.Mutex
	var got *proposal
	reg := aitools.NewRegistry(
		&listEngagementsTool{refs: in.Engagements},
		&statusTool{in: in},
		&proposeTool{record: func(pr proposal) {
			mu.Lock()
			defer mu.Unlock()
			got = &pr
		}},
	)
	period := in.Period
	if period == "" {
		period = DefaultPeriod(in.Now)
	}
	user := "Today: " + in.Now.In(istanbul).Format("2006-01-02") + ". Period on screen: " + period + ".\nCommand: " + in.Command
	uid := in.Viewer.UserID
	res, err := p.AI.RunTools(ctx, aiusecase.Call{
		OrganizationID: in.Viewer.OrganizationID, UserID: &uid, Purpose: "copilot_plan",
		System:    []provider.SystemBlock{{Text: systemPrompt + catalogPrompt(), Cache: true}},
		Messages:  []provider.Message{{Role: provider.RoleUser, Content: []provider.Block{provider.TextBlock(user)}}},
		MaxTokens: 2048,
	}, reg, aitools.Env{Scope: scope, UserID: uid}, 6)
	if err != nil {
		return Draft{}, err
	}
	mu.Lock()
	pr := got
	mu.Unlock()
	if pr == nil {
		return Draft{}, ErrNoProposal
	}
	d := Draft{Planner: PlannerAI, Model: res.Model, Period: pr.Period, Notes: []Note{}}
	if d.Period == "" {
		d.Period = period
	}
	for _, s := range pr.Steps {
		id, err := uuid.Parse(strings.TrimSpace(s.EngagementUUID))
		if err != nil {
			d.Notes = append(d.Notes, Note{Code: NoteEngagementNotFound})
			continue
		}
		d.Steps = append(d.Steps, PlanStep{Action: strings.TrimSpace(s.Action), EngagementUUID: id,
			Period: strings.TrimSpace(s.Period), Driver: strings.ToLower(strings.TrimSpace(s.Driver))})
	}
	if msg := strings.TrimSpace(pr.Message); msg != "" {
		if r := []rune(msg); len(r) > 400 {
			msg = string(r[:400])
		}
		d.Notes = append(d.Notes, Note{Code: NoteAssistant, Text: msg})
	}
	return d, nil
}

// list_engagements: the engagements the user reaches, filtered by title.
type listEngagementsTool struct{ refs []EngagementRef }

func (t *listEngagementsTool) Spec() aitools.Spec {
	return aitools.Spec{
		Name:        "list_engagements",
		Description: "List the taxpayers (engagements) the user may work on. search matches a word of the title or the tax id; empty lists the first 30.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"search": map[string]any{"type": "string", "maxLength": 100},
		}},
		Permissions: []string{rbac.PermTenantEngagementsRead},
		Kind:        aitools.KindRead,
	}
}

func (t *listEngagementsTool) Run(_ context.Context, _ aitools.Env, input json.RawMessage) (aitools.Result, error) {
	var in struct {
		Search string `json:"search"`
	}
	_ = json.Unmarshal(input, &in)
	q := Fold(strings.TrimSpace(in.Search))
	type item struct {
		EngagementUUID string `json:"engagement_uuid"`
		Title          string `json:"title"`
		TaxID          string `json:"tax_id"`
	}
	out := []item{}
	for _, e := range t.refs {
		if q != "" && !strings.Contains(Fold(e.TaxpayerTitle), q) && e.TaxpayerTaxID != strings.TrimSpace(in.Search) {
			continue
		}
		out = append(out, item{EngagementUUID: e.UUID.String(), Title: e.TaxpayerTitle, TaxID: e.TaxpayerTaxID})
		if len(out) == 30 {
			break
		}
	}
	return aitools.JSONResult(out), nil
}

// engagement_status: the period close chain of one engagement.
type statusTool struct{ in PlanInput }

func (t *statusTool) Spec() aitools.Spec {
	return aitools.Spec{
		Name:        "engagement_status",
		Description: "The period close chain of one engagement: each step (documents, bank, review, journals, transfer, report) with its status (done, todo, waiting, blocked, na), a count and the action that runs it now.",
		InputSchema: map[string]any{"type": "object", "required": []any{"engagement_uuid", "period"}, "properties": map[string]any{
			"engagement_uuid": map[string]any{"type": "string"},
			"period":          map[string]any{"type": "string", "pattern": "^[0-9]{4}-(0[1-9]|1[0-2])$"},
		}},
		Permissions: []string{rbac.PermTenantEngagementsRead},
		Kind:        aitools.KindRead,
	}
}

func (t *statusTool) Run(ctx context.Context, _ aitools.Env, input json.RawMessage) (aitools.Result, error) {
	var in struct {
		EngagementUUID string `json:"engagement_uuid"`
		Period         string `json:"period"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return aitools.ErrorResult("invalid input"), nil
	}
	id, err := uuid.Parse(strings.TrimSpace(in.EngagementUUID))
	if err != nil {
		return aitools.ErrorResult("unknown engagement"), nil
	}
	for _, e := range t.in.Engagements {
		if e.UUID != id {
			continue
		}
		rows, err := t.in.Status(ctx, []EngagementRef{e}, in.Period)
		if err != nil {
			return aitools.Result{}, err
		}
		r := rows[id]
		return aitools.JSONResult(map[string]any{"title": e.TaxpayerTitle, "period": in.Period, "steps": r.Steps, "next_step": r.NextStep}), nil
	}
	return aitools.ErrorResult("unknown engagement"), nil
}

// propose_plan records the plan; it changes nothing.
type proposeTool struct{ record func(proposal) }

func (t *proposeTool) Spec() aitools.Spec {
	actions := make([]any, 0)
	for _, a := range WriteActions() {
		actions = append(actions, a)
	}
	return aitools.Spec{
		Name:        "propose_plan",
		Description: "Hand in the plan for the user's approval. Call it exactly once, at the end. Nothing runs until the user approves.",
		InputSchema: map[string]any{"type": "object", "required": []any{"steps", "message"}, "properties": map[string]any{
			"period": map[string]any{"type": "string", "pattern": "^[0-9]{4}-(0[1-9]|1[0-2])$"},
			"steps": map[string]any{"type": "array", "maxItems": MaxPlanSteps, "items": map[string]any{
				"type": "object", "required": []any{"action", "engagement_uuid"},
				"properties": map[string]any{
					"action":          map[string]any{"type": "string", "enum": actions},
					"engagement_uuid": map[string]any{"type": "string"},
					"period":          map[string]any{"type": "string"},
					"driver":          map[string]any{"type": "string", "enum": []any{"luca", "zirve"}},
				},
			}},
			"message": map[string]any{"type": "string", "maxLength": 400},
		}},
		Permissions: []string{rbac.PermTenantEngagementsRead},
		// Read: it only records the proposal in memory.
		Kind: aitools.KindRead,
	}
}

func (t *proposeTool) Run(_ context.Context, _ aitools.Env, input json.RawMessage) (aitools.Result, error) {
	var pr proposal
	if err := json.Unmarshal(input, &pr); err != nil {
		return aitools.ErrorResult("invalid plan"), nil
	}
	t.record(pr)
	return aitools.Result{Content: "Plan recorded; it is shown to the user for approval. Stop here."}, nil
}
