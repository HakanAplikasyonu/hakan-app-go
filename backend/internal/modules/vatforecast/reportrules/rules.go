// Package reportrules runs the office's VAT report rules (#780 B).
//
// The month-end VAT report is not fixed to the 29th: the office keeps a
// list of days (for example the 15th as a mid-month report, the 25th, the
// 29th) or "X days before the month end". Each rule says who gets it (the
// office only, the taxpayer after the office's approval, the taxpayer
// directly) and which taxpayers (all, a group, selected ones, only those
// with a payment due). When a rule fires, "vat.report_ready" is raised once
// per taxpayer (vat_watch_events) and enqueued to the outbox, with the same
// payload as the VAT watch events, so the communication centre's
// automation rules (#780 A) can trigger on it.
package reportrules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/notice"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/watch"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// Trigger kinds, audiences and scopes.
const (
	TriggerDay            = "day"
	TriggerBeforeMonthEnd = "before_month_end"

	AudienceOffice           = "office"
	AudienceTaxpayerApproval = "taxpayer_approval"
	AudienceTaxpayerDirect   = "taxpayer_direct"

	ScopeAll         = "all"
	ScopeGroup       = "group"
	ScopeSelected    = "selected"
	ScopePayableOnly = "payable_only"

	maxRules    = 10
	maxSelected = 500
	maxLabel    = 80
)

var (
	// ErrInvalid: a rule failed validation.
	ErrInvalid = errors.New("invalid vat report rule")
	// ErrNotFound: no such rule in the office.
	ErrNotFound = errors.New("vat report rule not found")
)

// Rule is one VAT report rule (API view and input).
type Rule struct {
	UUID            *uuid.UUID  `json:"uuid,omitempty"`
	TriggerKind     string      `json:"trigger_kind"`
	Day             *int        `json:"day"`
	DaysBefore      *int        `json:"days_before"`
	Audience        string      `json:"audience"`
	Scope           string      `json:"scope"`
	GroupRef        *string     `json:"group_ref"`
	EngagementUUIDs []uuid.UUID `json:"engagement_uuids"`
	Label           *string     `json:"label"`
	Enabled         bool        `json:"enabled"`
	// NextRunOn is the next date the rule fires (YYYY-MM-DD, Istanbul);
	// computed, ignored on save.
	NextRunOn string `json:"next_run_on,omitempty"`
}

// List is the office's rules. Default: the office never saved a list and
// gets the single default rule (the report day, office only, all).
type List struct {
	Items   []Rule `json:"items"`
	Default bool   `json:"default"`
}

// DefaultRule is what an office without rules gets: its report day (29).
func DefaultRule(reportDay int) Rule {
	d := reportDay
	return Rule{TriggerKind: TriggerDay, Day: &d, Audience: AudienceOffice, Scope: ScopeAll, EngagementUUIDs: []uuid.UUID{}, Enabled: true}
}

// Validate checks a rule.
func (r Rule) Validate() error {
	switch r.TriggerKind {
	case TriggerDay:
		if r.Day == nil || *r.Day < 1 || *r.Day > 31 || r.DaysBefore != nil {
			return fmt.Errorf("%w: day must be 1-31", ErrInvalid)
		}
	case TriggerBeforeMonthEnd:
		if r.DaysBefore == nil || *r.DaysBefore < 0 || *r.DaysBefore > 27 || r.Day != nil {
			return fmt.Errorf("%w: days_before must be 0-27", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: trigger_kind must be day or before_month_end", ErrInvalid)
	}
	switch r.Audience {
	case AudienceOffice, AudienceTaxpayerApproval, AudienceTaxpayerDirect:
	default:
		return fmt.Errorf("%w: audience must be office, taxpayer_approval or taxpayer_direct", ErrInvalid)
	}
	switch r.Scope {
	case ScopeAll, ScopePayableOnly:
	case ScopeGroup:
		if r.GroupRef == nil || strings.TrimSpace(*r.GroupRef) == "" {
			return fmt.Errorf("%w: group_ref is required for scope group", ErrInvalid)
		}
	case ScopeSelected:
		if len(r.EngagementUUIDs) == 0 || len(r.EngagementUUIDs) > maxSelected {
			return fmt.Errorf("%w: engagement_uuids must list 1-%d taxpayers", ErrInvalid, maxSelected)
		}
	default:
		return fmt.Errorf("%w: scope must be all, group, selected or payable_only", ErrInvalid)
	}
	if r.Label != nil && utf8.RuneCountInString(strings.TrimSpace(*r.Label)) > maxLabel {
		return fmt.Errorf("%w: label is too long", ErrInvalid)
	}
	return nil
}

// FireDate is the day of t's month the rule fires on (Istanbul): day (a
// shorter month uses its last day) or the month end minus days_before.
func (r Rule) FireDate(t time.Time) time.Time {
	t = t.In(usecase.Istanbul)
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, usecase.Istanbul)
	last := first.AddDate(0, 1, -1)
	if r.TriggerKind == TriggerBeforeMonthEnd && r.DaysBefore != nil {
		return last.AddDate(0, 0, -*r.DaysBefore)
	}
	day := 1
	if r.Day != nil {
		day = *r.Day
	}
	if day > last.Day() {
		day = last.Day()
	}
	return first.AddDate(0, 0, day-1)
}

// NextRun is the first fire date on or after t's day.
func (r Rule) NextRun(t time.Time) time.Time {
	t = t.In(usecase.Istanbul)
	today := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, usecase.Istanbul)
	if d := r.FireDate(today); !d.Before(today) {
		return d
	}
	return r.FireDate(time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, usecase.Istanbul))
}

// Key identifies the rule in dedupe keys ("default" for the default rule).
func (r Rule) Key() string {
	if r.UUID == nil {
		return "default"
	}
	return r.UUID.String()
}

// Trigger is the payload trigger: "day:15" or "before_month_end:3".
func (r Rule) Trigger() string {
	if r.TriggerKind == TriggerBeforeMonthEnd && r.DaysBefore != nil {
		return fmt.Sprintf("%s:%d", TriggerBeforeMonthEnd, *r.DaysBefore)
	}
	if r.Day != nil {
		return fmt.Sprintf("%s:%d", TriggerDay, *r.Day)
	}
	return TriggerDay
}

// Target is one taxpayer file a rule may cover.
type Target struct {
	EngagementID  int64
	Engagement    uuid.UUID
	TaxpayerID    int64
	TaxpayerUUID  uuid.UUID
	TaxpayerTitle string
}

// InScope reports whether t is in the rule's static scope (payable_only is
// decided after the forecast; group has no members until #780 A).
func (r Rule) InScope(t Target) bool {
	switch r.Scope {
	case ScopeSelected:
		for _, u := range r.EngagementUUIDs {
			if u == t.Engagement {
				return true
			}
		}
		return false
	case ScopeGroup:
		return false
	default:
		return true
	}
}

// Queries is the db subset used here.
type Queries interface {
	ListVATReportRules(ctx context.Context, organizationID int64) ([]db.VatReportRule, error)
	InsertVATReportRule(ctx context.Context, arg db.InsertVATReportRuleParams) (db.VatReportRule, error)
	DeleteVATReportRules(ctx context.Context, organizationID int64) error
	ListActiveVATReportRules(ctx context.Context) ([]db.ListActiveVATReportRulesRow, error)
	ListVATReportDefaultOrgs(ctx context.Context) ([]db.ListVATReportDefaultOrgsRow, error)
	ListVATReportTargets(ctx context.Context, organizationID int64) ([]db.ListVATReportTargetsRow, error)
	InsertVATWatchEvent(ctx context.Context, arg db.InsertVATWatchEventParams) (db.InsertVATWatchEventRow, error)
	ListEngagementAlertRecipients(ctx context.Context, arg db.ListEngagementAlertRecipientsParams) ([]int64, error)
	GetDocumentTaskNotifyNames(ctx context.Context, arg db.GetDocumentTaskNotifyNamesParams) (db.GetDocumentTaskNotifyNamesRow, error)
}

// VAT is the vatforecast usecase subset used here.
type VAT interface {
	CurrentPeriod() string
	Forecast(ctx context.Context, sub usecase.Subject, period string) (usecase.Forecast, error)
	Tracking(ctx context.Context, orgID int64) (usecase.TrackingSettings, error)
}

// Sender sends the taxpayer the VAT notice (notice.Service).
type Sender interface {
	Send(ctx context.Context, sub usecase.Subject, userID int64, in notice.SendInput) (notice.Notice, error)
}

// Service manages and runs the rules.
type Service struct {
	Q           Queries
	VAT         VAT
	Notices     Sender
	Outbox      watch.Outbox
	Notifier    watch.Notifier
	Tx          watch.TxFunc
	FrontendURL string
	Log         *slog.Logger
	Now         func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// List returns the office's rules (or the default rule).
func (s *Service) List(ctx context.Context, orgID int64) (List, error) {
	rows, err := s.Q.ListVATReportRules(ctx, orgID)
	if err != nil {
		return List{}, fmt.Errorf("list vat report rules: %w", err)
	}
	now := s.now()
	if len(rows) == 0 {
		st, err := s.VAT.Tracking(ctx, orgID)
		if err != nil {
			return List{}, err
		}
		r := DefaultRule(st.ReportDay)
		r.NextRunOn = r.NextRun(now).Format(time.DateOnly)
		return List{Items: []Rule{r}, Default: true}, nil
	}
	out := make([]Rule, 0, len(rows))
	for _, row := range rows {
		r := ruleOf(row)
		r.NextRunOn = r.NextRun(now).Format(time.DateOnly)
		out = append(out, r)
	}
	return List{Items: out}, nil
}

// Save replaces the office's rules (up to 10). An empty list removes them
// all, so the office is back on the single default rule (report day, office,
// all taxpayers); to stop the report switch it off instead.
func (s *Service) Save(ctx context.Context, orgID, userID int64, rules []Rule) (List, error) {
	if len(rules) > maxRules {
		return List{}, fmt.Errorf("%w: keep at most %d rules", ErrInvalid, maxRules)
	}
	for i, r := range rules {
		if err := r.Validate(); err != nil {
			return List{}, fmt.Errorf("rule %d: %w", i+1, err)
		}
	}
	if err := s.Q.DeleteVATReportRules(ctx, orgID); err != nil {
		return List{}, fmt.Errorf("delete vat report rules: %w", err)
	}
	var by pgtype.Int8
	if userID > 0 {
		by = pgtype.Int8{Int64: userID, Valid: true}
	}
	for _, r := range rules {
		p := db.InsertVATReportRuleParams{
			OrganizationID: orgID, TriggerKind: r.TriggerKind, Audience: r.Audience, Scope: r.Scope,
			EngagementUuids: []uuid.UUID{}, Enabled: r.Enabled, UpdatedBy: by,
		}
		if r.Day != nil {
			p.Day = pgtype.Int4{Int32: int32(*r.Day), Valid: true} //nolint:gosec // 1-31
		}
		if r.DaysBefore != nil {
			p.DaysBefore = pgtype.Int4{Int32: int32(*r.DaysBefore), Valid: true} //nolint:gosec // 0-27
		}
		if r.Scope == ScopeGroup && r.GroupRef != nil {
			p.GroupRef = pgtype.Text{String: strings.TrimSpace(*r.GroupRef), Valid: true}
		}
		if r.Scope == ScopeSelected {
			p.EngagementUuids = r.EngagementUUIDs
		}
		if r.Label != nil && strings.TrimSpace(*r.Label) != "" {
			p.Label = pgtype.Text{String: strings.TrimSpace(*r.Label), Valid: true}
		}
		if _, err := s.Q.InsertVATReportRule(ctx, p); err != nil {
			return List{}, fmt.Errorf("insert vat report rule: %w", err)
		}
	}
	return s.List(ctx, orgID)
}

// Result is what one rule run did.
type Result struct {
	Taxpayers int `json:"taxpayers"`
	Raised    int `json:"raised"`
	Payable   int `json:"payable"`
	Notified  int `json:"notified"`
	Skipped   int `json:"skipped"`
}

// Run fires today's rules of every office (daily sweep) and returns how
// many events it raised.
func (s *Service) Run(ctx context.Context) (int, error) {
	type job struct {
		org  int64
		slug string
		rule Rule
	}
	var jobs []job
	today := s.now()
	if err := s.Tx(ctx, func(ctx context.Context) error {
		rows, err := s.Q.ListActiveVATReportRules(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			r := ruleOf(db.VatReportRule{
				ID: row.ID, Uuid: row.Uuid, OrganizationID: row.OrganizationID, TriggerKind: row.TriggerKind,
				Day: row.Day, DaysBefore: row.DaysBefore, Audience: row.Audience, Scope: row.Scope,
				GroupRef: row.GroupRef, EngagementUuids: row.EngagementUuids, Label: row.Label, Enabled: row.Enabled,
			})
			if sameDay(r.FireDate(today), today) {
				jobs = append(jobs, job{row.OrganizationID, row.OrganizationSlug, r})
			}
		}
		defs, err := s.Q.ListVATReportDefaultOrgs(ctx)
		if err != nil {
			return err
		}
		for _, d := range defs {
			r := DefaultRule(int(d.ReportDay))
			if sameDay(r.FireDate(today), today) {
				jobs = append(jobs, job{d.OrganizationID, d.OrganizationSlug, r})
			}
		}
		return nil
	}); err != nil {
		return 0, fmt.Errorf("vat report rules: %w", err)
	}
	total := 0
	for _, j := range jobs {
		var res Result
		err := s.Tx(ctx, func(ctx context.Context) error {
			var err error
			res, err = s.Fire(ctx, j.org, j.slug, j.rule, today)
			return err
		})
		if err != nil {
			s.log().Error("vat_report_rule_failed", "organization_id", j.org, "rule", j.rule.Key(), "error", err)
			continue
		}
		total += res.Raised
	}
	return total, nil
}

// RunNow fires one of the office's rules today on request ("Şimdi
// çalıştır"); a rule runs once a day.
func (s *Service) RunNow(ctx context.Context, orgID int64, ruleKey string) (Result, error) {
	list, err := s.List(ctx, orgID)
	if err != nil {
		return Result{}, err
	}
	st, err := s.VAT.Tracking(ctx, orgID)
	if err != nil {
		return Result{}, err
	}
	if !st.ReportEnabled {
		return Result{}, fmt.Errorf("%w: the VAT report is switched off", ErrInvalid)
	}
	for _, r := range list.Items {
		if r.Key() == ruleKey {
			names, err := s.slug(ctx, orgID)
			if err != nil {
				return Result{}, err
			}
			return s.Fire(ctx, orgID, names, r, s.now())
		}
	}
	return Result{}, ErrNotFound
}

func (s *Service) slug(ctx context.Context, orgID int64) (string, error) {
	targets, err := s.Q.ListVATReportTargets(ctx, orgID)
	if err != nil {
		return "", fmt.Errorf("vat report targets: %w", err)
	}
	if len(targets) == 0 {
		return "", nil
	}
	n, err := s.Q.GetDocumentTaskNotifyNames(ctx, db.GetDocumentTaskNotifyNamesParams{OrganizationID: orgID, TaxpayerID: targets[0].TaxpayerID})
	if err != nil {
		return "", fmt.Errorf("vat report names: %w", err)
	}
	return n.OrganizationSlug, nil
}

// Fire runs one rule for the office on day: one "vat.report_ready" per
// covered taxpayer (once a day per rule), the taxpayer notice when the
// audience is taxpayer_direct and one summary for the office.
func (s *Service) Fire(ctx context.Context, orgID int64, slug string, r Rule, day time.Time) (Result, error) {
	var res Result
	rows, err := s.Q.ListVATReportTargets(ctx, orgID)
	if err != nil {
		return res, fmt.Errorf("vat report targets: %w", err)
	}
	period := s.VAT.CurrentPeriod()
	date := day.In(usecase.Istanbul).Format(time.DateOnly)
	recipients := map[int64]bool{}
	var payableTotal decimal.Decimal
	for _, row := range rows {
		t := Target{EngagementID: row.EngagementID, Engagement: row.EngagementUuid, TaxpayerID: row.TaxpayerID,
			TaxpayerUUID: row.TaxpayerUuid, TaxpayerTitle: row.TaxpayerTitle}
		if !r.InScope(t) {
			continue
		}
		sub := usecase.Subject{OrganizationID: orgID, EngagementID: t.EngagementID, EngagementUUID: t.Engagement,
			TaxpayerID: t.TaxpayerID, TaxpayerUUID: t.TaxpayerUUID}
		f, err := s.VAT.Forecast(ctx, sub, period)
		if err != nil {
			return res, err
		}
		payable := f.Totals.Payable.Dec().IsPositive()
		if r.Scope == ScopePayableOnly && !payable {
			continue
		}
		res.Taxpayers++
		if payable {
			res.Payable++
			payableTotal = payableTotal.Add(f.Totals.Payable.Dec())
		}
		dedupe := fmt.Sprintf("%s:%s:%s:%s:%s", events.VATReportReady, r.Key(), t.Engagement, period, date)
		payload := watch.Payload(db.ListVATWatchTargetsRow{
			OrganizationID: orgID, EngagementID: t.EngagementID, EngagementUuid: t.Engagement,
			TaxpayerID: t.TaxpayerID, TaxpayerUuid: t.TaxpayerUUID, TaxpayerTitle: t.TaxpayerTitle,
		}, f, dedupe)
		payload["rule_uuid"] = r.Key()
		payload["trigger"] = r.Trigger()
		payload["audience"] = r.Audience
		payload["scope"] = r.Scope
		raw, err := json.Marshal(payload)
		if err != nil {
			return res, err
		}
		ev, err := s.Q.InsertVATWatchEvent(ctx, db.InsertVATWatchEventParams{
			OrganizationID: orgID, EngagementID: t.EngagementID, Period: period,
			Event: events.VATReportReady, DedupeKey: dedupe, Payload: raw,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue // this rule already ran for the taxpayer today
		}
		if err != nil {
			return res, fmt.Errorf("insert vat report event: %w", err)
		}
		res.Raised++
		if s.Outbox != nil {
			eng := t.Engagement
			e := events.New(events.VATReportReady).WithOrganization(orgID).
				WithEntity("engagement", &t.EngagementID, &eng).WithPayload(payload)
			e.EventID = ev.Uuid
			if err := s.Outbox.Enqueue(ctx, nil, e); err != nil {
				return res, fmt.Errorf("enqueue vat report event: %w", err)
			}
		}
		if r.Audience == AudienceTaxpayerDirect && s.Notices != nil {
			if _, err := s.Notices.Send(ctx, sub, 0, notice.SendInput{Period: period}); err != nil {
				if !errors.Is(err, notice.ErrNoRecipients) {
					return res, err
				}
				res.Skipped++
			} else {
				res.Notified++
			}
		}
		users, err := s.Q.ListEngagementAlertRecipients(ctx, db.ListEngagementAlertRecipientsParams{OrganizationID: orgID, EngagementID: t.EngagementID})
		if err != nil {
			return res, fmt.Errorf("vat report recipients: %w", err)
		}
		for _, u := range users {
			recipients[u] = true
		}
	}
	if res.Raised == 0 || s.Notifier == nil {
		return res, nil
	}
	title, body := Summary(r, period, res, payableTotal)
	link := fmt.Sprintf("%s/t/%s/reports", strings.TrimRight(s.FrontendURL, "/"), slug)
	for u := range recipients {
		org, user := orgID, u
		if _, err := s.Notifier.Enqueue(ctx, notifmodel.EnqueueInput{
			OrganizationID: &org, UserID: &user, Channels: []string{notifmodel.ChannelInapp},
			Title: title, Body: body, ActionURL: &link, SourceEvent: events.VATReportReady,
			Payload: map[string]any{"rule_uuid": r.Key(), "period": period, "trigger": r.Trigger(), "audience": r.Audience, "scope": r.Scope},
		}); err != nil {
			return res, fmt.Errorf("notify vat report: %w", err)
		}
	}
	return res, nil
}

// Summary is the office notification of a rule run.
func Summary(r Rule, period string, res Result, payable decimal.Decimal) (string, string) {
	title := "KDV raporu hazır: " + notice.MonthName(period)
	if r.Label != nil && strings.TrimSpace(*r.Label) != "" {
		title += " (" + strings.TrimSpace(*r.Label) + ")"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d mükellefin bu ayki KDV durumu hazır.", res.Taxpayers)
	if res.Payable > 0 {
		fmt.Fprintf(&b, " %d mükellefte ödeme çıkıyor (toplam tahmini %s TL).", res.Payable, notice.TL(usecase.M(payable)))
	} else {
		b.WriteString(" Hiçbirinde ödeme çıkmıyor.")
	}
	switch r.Audience {
	case AudienceTaxpayerApproval:
		fmt.Fprintf(&b, " %d mükellefe gönderim onayınızı bekliyor.", res.Taxpayers)
	case AudienceTaxpayerDirect:
		fmt.Fprintf(&b, " %d mükellefe bildirim gönderildi.", res.Notified)
		if res.Skipped > 0 {
			fmt.Fprintf(&b, " %d mükellefin panel kullanıcısı olmadığı için gönderilemedi.", res.Skipped)
		}
	}
	return title, b.String()
}

func ruleOf(row db.VatReportRule) Rule {
	id := row.Uuid
	r := Rule{UUID: &id, TriggerKind: row.TriggerKind, Audience: row.Audience, Scope: row.Scope,
		EngagementUUIDs: row.EngagementUuids, Enabled: row.Enabled}
	if r.EngagementUUIDs == nil {
		r.EngagementUUIDs = []uuid.UUID{}
	}
	if row.Day.Valid {
		d := int(row.Day.Int32)
		r.Day = &d
	}
	if row.DaysBefore.Valid {
		d := int(row.DaysBefore.Int32)
		r.DaysBefore = &d
	}
	if row.GroupRef.Valid {
		g := row.GroupRef.String
		r.GroupRef = &g
	}
	if row.Label.Valid {
		l := row.Label.String
		r.Label = &l
	}
	return r
}

func sameDay(a, b time.Time) bool {
	a, b = a.In(usecase.Istanbul), b.In(usecase.Istanbul)
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

func (s *Service) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}
