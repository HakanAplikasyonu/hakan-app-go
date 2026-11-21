package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/communication"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
)

// Notifier enqueues in-app notifications (notifications usecase.Service).
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// Sender is the part of communication.Service the rules send through.
type Sender interface {
	SMSToEngagement(ctx context.Context, org int64, engagement uuid.UUID, in communication.SMSInput) (db.SmsMessage, error)
	MailToEngagement(ctx context.Context, org int64, engagement uuid.UUID, in communication.MailInput) (db.MailMessage, error)
}

// Service manages the rules and runs them.
type Service struct {
	pool        database.Pool
	q           *db.Queries
	send        Sender
	notify      Notifier
	frontendURL string
	log         *slog.Logger
	now         func() time.Time
}

// New builds the service. send and notify may be nil: the channel then
// fails with a note (tests, servers without the notification service).
func New(pool database.Pool, send Sender, notify Notifier, frontendURL string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{pool: pool, q: database.QueriesFor(pool), send: send, notify: notify,
		frontendURL: strings.TrimRight(frontendURL, "/"), log: log, now: time.Now}
}

// WithClock sets the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }

// Rule is a rule as the screen sees it.
type Rule struct {
	UUID               uuid.UUID  `json:"uuid"`
	TemplateCode       string     `json:"template_code"`
	Name               string     `json:"name"`
	TriggerKind        string     `json:"trigger_kind"`
	Trigger            Trigger    `json:"trigger"`
	TargetKind         string     `json:"target_kind"`
	TargetGroupUUID    *string    `json:"target_group_uuid"`
	TargetEngagements  []string   `json:"target_engagements"`
	OnlyDue            bool       `json:"only_due"`
	Channels           []string   `json:"channels"`
	SMSIfUnopenedHours *int       `json:"sms_if_unopened_hours"`
	Subject            string     `json:"subject"`
	Body               string     `json:"body"`
	ApprovalMode       string     `json:"approval_mode"`
	Active             bool       `json:"active"`
	SendCount          int        `json:"send_count"`
	LastRunAt          *time.Time `json:"last_run_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func parseTrigger(raw []byte) Trigger {
	var t Trigger
	_ = json.Unmarshal(raw, &t)
	if t.Days == nil {
		t.Days = []int{}
	}
	return t
}

func ruleOf(r db.AutomationRule) Rule {
	out := Rule{UUID: r.Uuid, TemplateCode: r.TemplateCode, Name: r.Name, TriggerKind: r.TriggerKind,
		Trigger: parseTrigger(r.TriggerConfig), TargetKind: r.TargetKind, TargetEngagements: []string{},
		OnlyDue: r.OnlyDue, Channels: r.Channels, Subject: r.Subject, Body: r.Body, ApprovalMode: r.ApprovalMode,
		Active: r.Active, SendCount: int(r.SendCount), LastRunAt: tsPtr(r.LastRunAt), CreatedAt: r.CreatedAt.Time,
		UpdatedAt: r.UpdatedAt.Time}
	for _, e := range r.TargetEngagements {
		out.TargetEngagements = append(out.TargetEngagements, e.String())
	}
	if r.TargetGroupUuid.Valid {
		g := uuid.UUID(r.TargetGroupUuid.Bytes).String()
		out.TargetGroupUUID = &g
	}
	if r.SmsIfUnopenedHours.Valid {
		h := int(r.SmsIfUnopenedHours.Int32)
		out.SMSIfUnopenedHours = &h
	}
	if out.Channels == nil {
		out.Channels = []string{}
	}
	return out
}

// Stats are the three numbers at the top of the list.
type Stats struct {
	Total  int64 `json:"total"`
	Active int64 `json:"active"`
	Sent   int64 `json:"sent"`
	Drafts int64 `json:"drafts"`
}

// Overview is GET automations: the master switch, the numbers, the rules
// and the event catalog.
type Overview struct {
	Enabled bool        `json:"enabled"`
	Stats   Stats       `json:"stats"`
	Items   []Rule      `json:"items"`
	Events  []EventInfo `json:"events"`
}

func actorID(id int64) pgtype.Int8 {
	if id <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: id, Valid: true}
}

// EnsureTemplates installs the ready-made rules the office does not have
// yet, all passive. A deleted ready-made rule does not come back.
func (s *Service) EnsureTemplates(ctx context.Context, org int64) error {
	for _, t := range Templates {
		raw, err := json.Marshal(t.Trigger)
		if err != nil {
			return err
		}
		mode := t.ApprovalMode
		if mode == "" {
			mode = ApprovalAuto
		}
		if _, err := s.q.InsertAutomationTemplate(ctx, db.InsertAutomationTemplateParams{
			OrganizationID: org, TemplateCode: t.Code, Name: t.Name, TriggerKind: t.TriggerKind, TriggerConfig: raw,
			OnlyDue: t.OnlyDue, Channels: t.Channels, Subject: t.Subject, Body: t.Body, ApprovalMode: mode,
		}); err != nil {
			return fmt.Errorf("install automation template %s: %w", t.Code, err)
		}
	}
	return nil
}

// Enabled reports the office's master switch (on when never saved).
func (s *Service) Enabled(ctx context.Context, org int64) (bool, error) {
	row, err := s.q.GetOfficeAutomationSettings(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("automation settings: %w", err)
	}
	return row.Enabled, nil
}

// SetEnabled saves the master switch.
func (s *Service) SetEnabled(ctx context.Context, org, actor int64, enabled bool) (bool, error) {
	row, err := s.q.UpsertOfficeAutomationSettings(ctx, db.UpsertOfficeAutomationSettingsParams{
		OrganizationID: org, Enabled: enabled, UpdatedBy: actorID(actor)})
	if err != nil {
		return false, fmt.Errorf("save automation settings: %w", err)
	}
	return row.Enabled, nil
}

// Overview lists the office's rules (installing the ready-made ones first).
func (s *Service) Overview(ctx context.Context, org int64) (Overview, error) {
	if err := s.EnsureTemplates(ctx, org); err != nil {
		return Overview{}, err
	}
	enabled, err := s.Enabled(ctx, org)
	if err != nil {
		return Overview{}, err
	}
	rows, err := s.q.ListAutomationRules(ctx, org)
	if err != nil {
		return Overview{}, fmt.Errorf("list automation rules: %w", err)
	}
	st, err := s.q.AutomationStats(ctx, org)
	if err != nil {
		return Overview{}, fmt.Errorf("automation stats: %w", err)
	}
	drafts, err := s.q.CountAutomationDrafts(ctx, org)
	if err != nil {
		return Overview{}, fmt.Errorf("automation drafts: %w", err)
	}
	out := Overview{Enabled: enabled, Stats: Stats{Total: st.Total, Active: st.Active, Sent: st.Sent, Drafts: drafts},
		Items: make([]Rule, 0, len(rows)), Events: Events}
	for _, r := range rows {
		out.Items = append(out.Items, ruleOf(r))
	}
	return out, nil
}

func (s *Service) record(ctx context.Context, r db.AutomationRule, action string, actor int64) error {
	snap, err := json.Marshal(ruleOf(r))
	if err != nil {
		return err
	}
	if err := s.q.InsertAutomationRuleHistory(ctx, db.InsertAutomationRuleHistoryParams{OrganizationID: r.OrganizationID,
		RuleID: r.ID, Action: action, ActorID: actorID(actor), Snapshot: snap}); err != nil {
		return fmt.Errorf("record automation history: %w", err)
	}
	return nil
}

type ruleParams struct {
	trigger  []byte
	group    pgtype.UUID
	targets  []uuid.UUID
	unopened pgtype.Int4
}

func (s *Service) params(ctx context.Context, org int64, in *Input) (ruleParams, error) {
	if err := Validate(in); err != nil {
		return ruleParams{}, err
	}
	raw, err := json.Marshal(in.Trigger)
	if err != nil {
		return ruleParams{}, err
	}
	p := ruleParams{trigger: raw, targets: []uuid.UUID{}}
	for _, e := range in.TargetEngagements {
		id, err := uuid.Parse(strings.TrimSpace(e))
		if err != nil {
			return ruleParams{}, invalid("seçili mükellef geçersiz")
		}
		p.targets = append(p.targets, id)
	}
	if len(p.targets) > 0 {
		ids, err := s.q.ListAutomationEngagementIDs(ctx, db.ListAutomationEngagementIDsParams{OrganizationID: org, Uuids: p.targets})
		if err != nil {
			return ruleParams{}, fmt.Errorf("check automation targets: %w", err)
		}
		if len(ids) != len(p.targets) {
			return ruleParams{}, invalid("seçili mükelleflerden biri bu ofiste değil")
		}
	}
	if in.SMSIfUnopenedHours != nil {
		h := *in.SMSIfUnopenedHours
		if h < 1 || h > 168 {
			return ruleParams{}, invalid("e-posta açılmadıysa SMS için bekleme 1 ile 168 saat arasında olmalı")
		}
		p.unopened = pgtype.Int4{Int32: int32(h), Valid: true}
	}
	return p, nil
}

// Create saves a new rule.
func (s *Service) Create(ctx context.Context, org, actor int64, in Input) (Rule, error) {
	p, err := s.params(ctx, org, &in)
	if err != nil {
		return Rule{}, err
	}
	row, err := s.q.InsertAutomationRule(ctx, db.InsertAutomationRuleParams{OrganizationID: org, Name: in.Name,
		TriggerKind: in.TriggerKind, TriggerConfig: p.trigger, TargetKind: in.TargetKind, TargetGroupUuid: p.group,
		TargetEngagements: p.targets, OnlyDue: in.OnlyDue, Channels: in.Channels, SmsIfUnopenedHours: p.unopened,
		Subject: in.Subject, Body: in.Body, ApprovalMode: in.ApprovalMode, Active: in.Active, Actor: actorID(actor)})
	if err != nil {
		return Rule{}, fmt.Errorf("create automation rule: %w", err)
	}
	if err := s.record(ctx, row, "created", actor); err != nil {
		return Rule{}, err
	}
	return ruleOf(row), nil
}

// Update replaces a rule.
func (s *Service) Update(ctx context.Context, org, actor int64, id uuid.UUID, in Input) (Rule, error) {
	p, err := s.params(ctx, org, &in)
	if err != nil {
		return Rule{}, err
	}
	row, err := s.q.UpdateAutomationRule(ctx, db.UpdateAutomationRuleParams{OrganizationID: org, Uuid: id, Name: in.Name,
		TriggerKind: in.TriggerKind, TriggerConfig: p.trigger, TargetKind: in.TargetKind, TargetGroupUuid: p.group,
		TargetEngagements: p.targets, OnlyDue: in.OnlyDue, Channels: in.Channels, SmsIfUnopenedHours: p.unopened,
		Subject: in.Subject, Body: in.Body, ApprovalMode: in.ApprovalMode, Active: in.Active, Actor: actorID(actor)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Rule{}, ErrNotFound
	} else if err != nil {
		return Rule{}, fmt.Errorf("update automation rule: %w", err)
	}
	if err := s.record(ctx, row, "updated", actor); err != nil {
		return Rule{}, err
	}
	return ruleOf(row), nil
}

// SetActive turns a rule on or off (one click on the list).
func (s *Service) SetActive(ctx context.Context, org, actor int64, id uuid.UUID, active bool) (Rule, error) {
	row, err := s.q.SetAutomationRuleActive(ctx, db.SetAutomationRuleActiveParams{OrganizationID: org, Uuid: id,
		Active: active, Actor: actorID(actor)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Rule{}, ErrNotFound
	} else if err != nil {
		return Rule{}, fmt.Errorf("toggle automation rule: %w", err)
	}
	action := "disabled"
	if active {
		action = "enabled"
	}
	if err := s.record(ctx, row, action, actor); err != nil {
		return Rule{}, err
	}
	return ruleOf(row), nil
}

// Delete hides a rule; its runs and history stay.
func (s *Service) Delete(ctx context.Context, org, actor int64, id uuid.UUID) error {
	row, err := s.q.SoftDeleteAutomationRule(ctx, db.SoftDeleteAutomationRuleParams{OrganizationID: org, Uuid: id, Actor: actorID(actor)})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("delete automation rule: %w", err)
	}
	return s.record(ctx, row, "deleted", actor)
}

// Run is one run of a rule for the screen.
type Run struct {
	UUID           uuid.UUID       `json:"uuid"`
	RuleUUID       uuid.UUID       `json:"rule_uuid"`
	RuleName       string          `json:"rule_name"`
	TaxpayerTitle  string          `json:"taxpayer_title"`
	EngagementUUID string          `json:"engagement_uuid"`
	Period         string          `json:"period"`
	Status         string          `json:"status"`
	ScheduledFor   time.Time       `json:"scheduled_for"`
	Subject        string          `json:"subject"`
	Body           string          `json:"body"`
	Channels       []string        `json:"channels"`
	Result         json.RawMessage `json:"result"`
	Note           string          `json:"note"`
	FollowupStatus string          `json:"sms_followup_status"`
	CreatedAt      time.Time       `json:"created_at"`
	SentAt         *time.Time      `json:"sent_at"`
	DecidedAt      *time.Time      `json:"decided_at"`
}

func runOf(r db.ListAutomationRunsRow) Run {
	out := Run{UUID: r.Uuid, RuleUUID: r.RuleUuid, RuleName: r.RuleName, TaxpayerTitle: r.TaxpayerTitle,
		EngagementUUID: r.EngagementUuid, Period: r.Period, Status: r.Status, ScheduledFor: r.ScheduledFor.Time,
		Subject: r.Subject, Body: r.Body, Channels: r.Channels, Result: json.RawMessage(r.Result), Note: r.Note,
		FollowupStatus: r.SmsFollowupStatus, CreatedAt: r.CreatedAt.Time, SentAt: tsPtr(r.SentAt), DecidedAt: tsPtr(r.DecidedAt)}
	if out.Channels == nil {
		out.Channels = []string{}
	}
	if len(out.Result) == 0 {
		out.Result = json.RawMessage(`{}`)
	}
	return out
}

// RunFilter narrows the run list.
type RunFilter struct {
	Rule   *uuid.UUID
	Status string
	Limit  int32
}

// Runs lists runs (the send history, a rule's history, the pending drafts).
func (s *Service) Runs(ctx context.Context, org int64, f RunFilter) ([]Run, error) {
	p := db.ListAutomationRunsParams{OrganizationID: org, Lim: f.Limit}
	if p.Lim <= 0 || p.Lim > 500 {
		p.Lim = 100
	}
	if f.Rule != nil {
		r, err := s.q.GetAutomationRule(ctx, db.GetAutomationRuleParams{OrganizationID: org, Uuid: *f.Rule})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		} else if err != nil {
			return nil, fmt.Errorf("automation rule: %w", err)
		}
		p.RuleID = pgtype.Int8{Int64: r.ID, Valid: true}
	}
	if f.Status != "" {
		switch f.Status {
		case RunScheduled, RunDraft, RunSent, RunSkipped, RunFailed, RunRejected:
		default:
			return nil, invalid("bilinmeyen durum")
		}
		p.Status = pgtype.Text{String: f.Status, Valid: true}
	}
	rows, err := s.q.ListAutomationRuns(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("list automation runs: %w", err)
	}
	out := make([]Run, 0, len(rows))
	for _, r := range rows {
		out = append(out, runOf(r))
	}
	return out, nil
}

// Edit is one entry of a rule's edit history.
type Edit struct {
	Action    string          `json:"action"`
	ActorName string          `json:"actor_name"`
	At        time.Time       `json:"at"`
	Snapshot  json.RawMessage `json:"snapshot"`
}

// History is a rule's runs and edits.
type History struct {
	Rule  Rule   `json:"rule"`
	Runs  []Run  `json:"runs"`
	Edits []Edit `json:"edits"`
}

// RuleHistory returns a rule's last runs and its edit history.
func (s *Service) RuleHistory(ctx context.Context, org int64, id uuid.UUID) (History, error) {
	r, err := s.q.GetAutomationRule(ctx, db.GetAutomationRuleParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return History{}, ErrNotFound
	} else if err != nil {
		return History{}, fmt.Errorf("automation rule: %w", err)
	}
	runs, err := s.Runs(ctx, org, RunFilter{Rule: &id, Limit: 100})
	if err != nil {
		return History{}, err
	}
	rows, err := s.q.ListAutomationRuleHistory(ctx, db.ListAutomationRuleHistoryParams{OrganizationID: org, RuleID: r.ID})
	if err != nil {
		return History{}, fmt.Errorf("automation history: %w", err)
	}
	h := History{Rule: ruleOf(r), Runs: runs, Edits: make([]Edit, 0, len(rows))}
	for _, e := range rows {
		h.Edits = append(h.Edits, Edit{Action: e.Action, ActorName: e.ActorName, At: e.CreatedAt.Time, Snapshot: json.RawMessage(e.Snapshot)})
	}
	return h, nil
}

// DecideResult says how many drafts were approved (sent) or rejected.
type DecideResult struct {
	Sent     int `json:"sent"`
	Failed   int `json:"failed"`
	Rejected int `json:"rejected"`
}

// Decide approves (sends) or rejects pending drafts, one or many.
func (s *Service) Decide(ctx context.Context, org, actor int64, ids []uuid.UUID, approve bool) (DecideResult, error) {
	if len(ids) == 0 {
		return DecideResult{}, invalid("en az bir taslak seçin")
	}
	if len(ids) > 500 {
		return DecideResult{}, invalid("tek seferde en fazla 500 taslak")
	}
	rowIDs, err := s.q.ListAutomationRunIDsByUUID(ctx, db.ListAutomationRunIDsByUUIDParams{OrganizationID: org, Uuids: ids})
	if err != nil {
		return DecideResult{}, fmt.Errorf("automation drafts: %w", err)
	}
	var res DecideResult
	for _, id := range rowIDs {
		status, err := s.decideOne(ctx, org, actor, id, approve)
		if err != nil {
			return res, err
		}
		switch status {
		case RunSent:
			res.Sent++
		case RunRejected:
			res.Rejected++
		case RunFailed:
			res.Failed++
		}
	}
	return res, nil
}

func (s *Service) decideOne(ctx context.Context, org, actor, id int64, approve bool) (string, error) {
	run, err := s.q.GetAutomationRunForUpdate(ctx, db.GetAutomationRunForUpdateParams{OrganizationID: org, ID: id})
	if err != nil {
		return "", fmt.Errorf("automation draft: %w", err)
	}
	if run.Status != RunDraft {
		return run.Status, nil
	}
	now := s.now()
	if !approve {
		_, err := s.q.FinishAutomationRun(ctx, db.FinishAutomationRunParams{OrganizationID: org, ID: id, Status: RunRejected,
			Subject: run.Subject, Body: run.Body, Channels: run.Channels, Result: run.Result, Note: "Ofis reddetti.",
			DecidedBy: actorID(actor), DecidedAt: pgtype.Timestamptz{Time: now, Valid: true}})
		if err != nil {
			return "", fmt.Errorf("reject automation draft: %w", err)
		}
		return RunRejected, nil
	}
	rule, err := s.q.GetAutomationRuleByID(ctx, db.GetAutomationRuleByIDParams{OrganizationID: org, ID: run.RuleID})
	if err != nil {
		return "", fmt.Errorf("automation rule: %w", err)
	}
	out, err := s.deliver(ctx, rule, run, run.Subject, run.Body, actor)
	if err != nil {
		return "", err
	}
	return out.Status, nil
}
