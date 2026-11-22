package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/communication"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/events"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// eventWindow: the scheduler looks this far back for database events (and
// never before the rule was switched on).
const eventWindow = 7 * 24 * time.Hour

// batch caps the runs one tick processes.
const batch = 200

// Internal run variables (not placeholders of the text).
const (
	varOccurredAt = "_occurred_at"
	varReturnUUID = "_return_uuid"
	varPayable    = "_payable"
	varVAT        = "_vat"
)

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func numText(n pgtype.Numeric) string {
	if !n.Valid {
		return ""
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return ""
	}
	return strconv.FormatFloat(f.Float64, 'f', 2, 64)
}

func positive(s string) bool {
	f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", "."), 64)
	return err == nil && f > 0.004
}

// Tick is the scheduler's periodic step across offices: it plans the
// calendar, payment and database-event runs of every active rule, then
// sends (or drafts) the runs whose time has come, checks "e-posta
// açılmadıysa SMS" and "hâlâ ödenmediyse tekrar gönder". Each run is its
// own transaction, so one failure does not undo sends already made.
// Returns how many runs were processed.
func (s *Service) Tick(ctx context.Context, pool rls.Beginner) (int, error) {
	now := s.now()
	if err := rls.System(ctx, pool, "automation: plan rule runs across offices", func(ctx context.Context, _ pgx.Tx) error {
		orgs, err := s.q.ListAutomationActiveOrgs(ctx)
		if err != nil {
			return fmt.Errorf("automation orgs: %w", err)
		}
		for _, org := range orgs {
			if err := s.Plan(ctx, org, now); err != nil {
				s.log.Warn("automation_plan_failed", "organization_id", org, "error", err)
			}
		}
		return nil
	}); err != nil {
		return 0, err
	}
	type item struct{ org, id int64 }
	var due, followups, resends []item
	if err := rls.System(ctx, pool, "automation: due runs across offices", func(ctx context.Context, _ pgx.Tx) error {
		rows, err := s.q.ListDueAutomationRuns(ctx, db.ListDueAutomationRunsParams{Now: ts(now), Lim: batch})
		if err != nil {
			return err
		}
		for _, r := range rows {
			due = append(due, item{r.OrganizationID, r.ID})
		}
		fr, err := s.q.ListDueAutomationFollowups(ctx, db.ListDueAutomationFollowupsParams{Now: ts(now), Lim: batch})
		if err != nil {
			return err
		}
		for _, r := range fr {
			followups = append(followups, item{r.OrganizationID, r.ID})
		}
		rr, err := s.q.ListDueAutomationResends(ctx, db.ListDueAutomationResendsParams{Now: ts(now), Lim: batch})
		if err != nil {
			return err
		}
		for _, r := range rr {
			resends = append(resends, item{r.OrganizationID, r.ID})
		}
		return nil
	}); err != nil {
		return 0, fmt.Errorf("automation due runs: %w", err)
	}
	n := 0
	for _, it := range resends {
		if err := rls.System(ctx, pool, "automation: payment re-check", func(ctx context.Context, _ pgx.Tx) error {
			return s.processResend(ctx, it.org, it.id)
		}); err != nil {
			s.log.Warn("automation_resend_failed", "organization_id", it.org, "run_id", it.id, "error", err)
		}
	}
	for _, it := range due {
		if err := rls.System(ctx, pool, "automation: run a rule", func(ctx context.Context, _ pgx.Tx) error {
			return s.ProcessRun(ctx, it.org, it.id)
		}); err != nil {
			s.log.Warn("automation_run_failed", "organization_id", it.org, "run_id", it.id, "error", err)
			continue
		}
		n++
	}
	for _, it := range followups {
		if err := rls.System(ctx, pool, "automation: sms if e-mail unopened", func(ctx context.Context, _ pgx.Tx) error {
			return s.processFollowup(ctx, it.org, it.id)
		}); err != nil {
			s.log.Warn("automation_followup_failed", "organization_id", it.org, "run_id", it.id, "error", err)
		}
	}
	return n, nil
}

// targets is the set of engagements a rule reaches.
func (s *Service) targets(ctx context.Context, r db.AutomationRule) (map[int64]bool, error) {
	ids, err := s.q.ListAutomationTargets(ctx, db.ListAutomationTargetsParams{OrganizationID: r.OrganizationID,
		OnlySelected: r.TargetKind != TargetAll, Uuids: r.TargetEngagements})
	if err != nil {
		return nil, fmt.Errorf("automation targets: %w", err)
	}
	set := make(map[int64]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set, nil
}

func (s *Service) insertRun(ctx context.Context, r db.AutomationRule, eng int64, key, period string, at time.Time, vars map[string]string, ref string) (bool, error) {
	if vars == nil {
		vars = map[string]string{}
	}
	raw, err := json.Marshal(vars)
	if err != nil {
		return false, err
	}
	n, err := s.q.InsertAutomationRun(ctx, db.InsertAutomationRunParams{OrganizationID: r.OrganizationID, RuleID: r.ID,
		EngagementID: pgtype.Int8{Int64: eng, Valid: true}, RunKey: key, Period: period, Status: RunScheduled,
		ScheduledFor: ts(at), Vars: raw, SourceRef: ref})
	if err != nil {
		return false, fmt.Errorf("insert automation run: %w", err)
	}
	return n > 0, nil
}

// Plan writes the runs an office's active rules owe at now. Running it
// again writes nothing new (unique run key).
func (s *Service) Plan(ctx context.Context, org int64, now time.Time) error {
	if on, err := s.Enabled(ctx, org); err != nil || !on {
		return err
	}
	rules, err := s.q.ListActiveAutomationRules(ctx, org)
	if err != nil {
		return fmt.Errorf("active automation rules: %w", err)
	}
	for _, r := range rules {
		if off, err := s.gated(ctx, org, r.TemplateCode); err != nil {
			return err
		} else if off {
			continue
		}
		t := parseTrigger(r.TriggerConfig)
		var occ Occurrence
		var ok bool
		switch r.TriggerKind {
		case TriggerCalendar:
			occ, ok = MatchCalendar(t, now)
		case TriggerPayment:
			occ, ok = MatchPayment(t, now)
		case TriggerEvent:
			if err := s.scanEvents(ctx, r, t, now); err != nil {
				return err
			}
			continue
		}
		if !ok {
			continue
		}
		set, err := s.targets(ctx, r)
		if err != nil {
			return err
		}
		for eng := range set {
			if _, err := s.insertRun(ctx, r, eng, occ.RunKey, occ.Period, now, nil, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

// eventRun writes the run of one event for one engagement: after the rule's
// least delay, only for engagements in the target.
func (s *Service) eventRun(ctx context.Context, r db.AutomationRule, t Trigger, set map[int64]bool, eng int64, key, period string, occurred time.Time, vars map[string]string, ref string) error {
	if !set[eng] {
		return nil
	}
	if vars == nil {
		vars = map[string]string{}
	}
	vars[varOccurredAt] = occurred.UTC().Format(time.RFC3339)
	_, err := s.insertRun(ctx, r, eng, key, period, occurred.Add(time.Duration(t.DelayMinHours)*time.Hour), vars, ref)
	return err
}

// scanEvents finds the database events of an event rule (tahakkuk oluştu,
// beyanname onaylandı, belge 3 gündür gelmedi) since the rule was switched
// on. KDV events come through the bus instead (HandleEvent).
func (s *Service) scanEvents(ctx context.Context, r db.AutomationRule, t Trigger, now time.Time) error {
	since := now.Add(-eventWindow)
	if r.ActiveSince.Valid && r.ActiveSince.Time.After(since) {
		since = r.ActiveSince.Time
	}
	switch t.Event {
	case EventReturnFiled, EventReturnChecked, EventDocRequestLate:
	default:
		return nil
	}
	set, err := s.targets(ctx, r)
	if err != nil {
		return err
	}
	switch t.Event {
	case EventReturnFiled:
		rows, err := s.q.ListAutomationFiledReturns(ctx, db.ListAutomationFiledReturnsParams{OrganizationID: r.OrganizationID, Since: ts(since)})
		if err != nil {
			return fmt.Errorf("automation filed returns: %w", err)
		}
		for _, x := range rows {
			vars := returnVars(x.Kind, x.Period, numText(x.PayableAmount))
			vars[varReturnUUID] = x.Uuid.String()
			if err := s.eventRun(ctx, r, t, set, x.EngagementID, EventRunKey(t.Event, "", x.Uuid.String()), x.Period,
				x.FiledAt.Time, vars, x.Uuid.String()); err != nil {
				return err
			}
		}
	case EventReturnChecked:
		rows, err := s.q.ListAutomationCheckedReturns(ctx, db.ListAutomationCheckedReturnsParams{OrganizationID: r.OrganizationID, Since: ts(since)})
		if err != nil {
			return fmt.Errorf("automation checked returns: %w", err)
		}
		for _, x := range rows {
			vars := returnVars(x.Kind, x.Period, numText(x.PayableAmount))
			if err := s.eventRun(ctx, r, t, set, x.EngagementID, EventRunKey(t.Event, "", x.Uuid.String()), x.Period,
				x.CheckedAt.Time, vars, x.Uuid.String()); err != nil {
				return err
			}
		}
	case EventDocRequestLate:
		// A request becomes late 3 days after it was made; it counts when
		// that moment is after the rule was switched on.
		rows, err := s.q.ListAutomationOverdueDocTasks(ctx, db.ListAutomationOverdueDocTasksParams{OrganizationID: r.OrganizationID,
			OverdueBefore: ts(now.Add(-DocLateAfter)), Since: ts(since.Add(-DocLateAfter))})
		if err != nil {
			return fmt.Errorf("automation late documents: %w", err)
		}
		for _, x := range rows {
			vars := map[string]string{"donem": FormatPeriod("", x.Period)}
			if err := s.eventRun(ctx, r, t, set, x.EngagementID, EventRunKey(t.Event, "", x.Uuid.String()), x.Period,
				x.CreatedAt.Time.Add(DocLateAfter), vars, x.Uuid.String()); err != nil {
				return err
			}
		}
	}
	return nil
}

func returnVars(kind, period, payable string) map[string]string {
	tax := taxOfReturn(kind)
	v := map[string]string{"beyan": returnLabel(kind), "donem": FormatPeriod(tax, period), varPayable: payable}
	if positive(payable) {
		v["tutar"] = FormatAmount(payable)
	}
	if due, ok := DueDate(tax, period); ok {
		v["son_gun"] = FormatDate(due)
	}
	return v
}

// RegisterEventHandlers subscribes the KDV events (K1) on the worker's bus.
// Each event becomes a run of every active event rule of the office that
// listens to it; the payload's dedupe_key keeps one run per taxpayer and
// period.
func (s *Service) RegisterEventHandlers(bus events.Bus, pool rls.Beginner) {
	if bus == nil {
		return
	}
	bus.Subscribe("vat.*", func(ctx context.Context, e events.Event) error {
		return rls.System(ctx, pool, "automation: KDV event to rule runs", func(ctx context.Context, _ pgx.Tx) error {
			return s.HandleEvent(ctx, e)
		})
	})
}

func payloadText(p map[string]any, key string) string {
	switch v := p[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

// HandleEvent turns one bus event into runs of the office's rules.
func (s *Service) HandleEvent(ctx context.Context, e events.Event) error {
	if e.OrganizationID == nil {
		return nil
	}
	if _, ok := eventInfo(e.Name); !ok {
		return nil
	}
	org := *e.OrganizationID
	if on, err := s.Enabled(ctx, org); err != nil || !on {
		return err
	}
	rules, err := s.q.ListActiveAutomationRules(ctx, org)
	if err != nil {
		return fmt.Errorf("active automation rules: %w", err)
	}
	var engUUID uuid.UUID
	if raw := payloadText(e.Payload, "engagement_uuid"); raw != "" {
		engUUID, _ = uuid.Parse(raw)
	}
	if engUUID == uuid.Nil && e.EntityUUID != nil && e.EntityType == "engagement" {
		engUUID = *e.EntityUUID
	}
	if engUUID == uuid.Nil {
		return nil
	}
	eng, err := s.q.GetAutomationEngagementByUUID(ctx, db.GetAutomationEngagementByUUIDParams{OrganizationID: org, Uuid: engUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("automation event engagement: %w", err)
	}
	period := payloadText(e.Payload, "period")
	payable := payloadText(e.Payload, "payable")
	vars := map[string]string{"beyan": "KDV", "donem": FormatPeriod(TaxKDV, period), varPayable: payable, varVAT: "1"}
	if title := payloadText(e.Payload, "taxpayer_title"); title != "" {
		vars["mukellef"] = title
	}
	if positive(payable) {
		vars["tutar"] = FormatAmount(payable)
	}
	if d, err := time.ParseInLocation("2006-01-02", payloadText(e.Payload, "due_date"), Istanbul); err == nil {
		vars["son_gun"] = FormatDate(d)
	}
	occurred := e.OccurredAt
	if occurred.IsZero() {
		occurred = s.now()
	}
	key := EventRunKey(e.Name, payloadText(e.Payload, "dedupe_key"), e.EventID.String())
	for _, r := range rules {
		if r.TriggerKind != TriggerEvent {
			continue
		}
		t := parseTrigger(r.TriggerConfig)
		if t.Event != e.Name {
			continue
		}
		set, err := s.targets(ctx, r)
		if err != nil {
			return err
		}
		v := make(map[string]string, len(vars)+1)
		for k, x := range vars {
			v[k] = x
		}
		if err := s.eventRun(ctx, r, t, set, eng, key, period, occurred, v, e.EventID.String()); err != nil {
			return err
		}
	}
	return nil
}

func readVars(raw []byte) map[string]string {
	v := map[string]string{}
	_ = json.Unmarshal(raw, &v)
	return v
}

// varsFor fills the text variables of a run: the taxpayer, the office, the
// period and its due date, the payable of the period's return, the banks
// and the portal link; event values win.
func (s *Service) varsFor(ctx context.Context, r db.AutomationRule, t Trigger, run db.AutomationRun, rec db.GetAutomationRecipientRow) (map[string]string, error) {
	v := map[string]string{"mukellef": rec.Title, "ofis": rec.OrganizationName}
	if s.frontendURL != "" {
		v["link"] = s.frontendURL + "/m/" + rec.TaxpayerUuid.String()
	}
	banks, err := s.q.ListAutomationBankNames(ctx, rec.TaxpayerID)
	if err != nil {
		return nil, fmt.Errorf("automation banks: %w", err)
	}
	v["banka"] = strings.Join(banks, ", ")
	switch r.TriggerKind {
	case TriggerPayment:
		v["donem"] = FormatPeriod("", run.Period)
		if d, err := time.ParseInLocation("2006-01-02", t.PaymentDate, Istanbul); err == nil {
			v["son_gun"] = FormatDate(d)
		}
		v["tutar"] = FormatAmount(t.PaymentAmount)
	case TriggerCalendar:
		v["beyan"] = TaxLabel(t.Tax)
		v["donem"] = FormatPeriod(t.Tax, run.Period)
		if due, ok := DueDate(t.Tax, run.Period); ok {
			v["son_gun"] = FormatDate(due)
		}
		if kind := returnKind(t.Tax); kind != "" {
			ret, err := s.q.GetAutomationReturnDue(ctx, db.GetAutomationReturnDueParams{OrganizationID: r.OrganizationID,
				EngagementID: rec.EngagementID, Kind: kind, Period: run.Period})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("automation return: %w", err)
			}
			if err == nil && positive(numText(ret.PayableAmount)) {
				v["tutar"] = FormatAmount(numText(ret.PayableAmount))
			}
		}
	}
	for k, x := range readVars(run.Vars) {
		if x != "" {
			v[k] = x
		}
	}
	return v, nil
}

// hasDue answers the "yalnız ödemesi/eksiği olanlar" switch: an unpaid
// payable for the run's tax and period (event: the event's payable), else
// open document requests.
func (s *Service) hasDue(ctx context.Context, r db.AutomationRule, t Trigger, run db.AutomationRun, eng int64, vars map[string]string) (bool, error) {
	if p, ok := vars[varPayable]; ok {
		if !positive(p) {
			return false, nil
		}
		if id, err := uuid.Parse(vars[varReturnUUID]); err == nil {
			paid, err := s.q.GetAutomationReturnPaid(ctx, db.GetAutomationReturnPaidParams{OrganizationID: r.OrganizationID, EngagementID: eng, Uuid: id})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return false, err
			}
			return !paid, nil
		}
		return true, nil
	}
	if r.TriggerKind == TriggerCalendar {
		if kind := returnKind(t.Tax); kind != "" {
			ret, err := s.q.GetAutomationReturnDue(ctx, db.GetAutomationReturnDueParams{OrganizationID: r.OrganizationID,
				EngagementID: eng, Kind: kind, Period: run.Period})
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			} else if err != nil {
				return false, err
			}
			return ret.Status != "draft" && !ret.PaidOn.Valid && positive(numText(ret.PayableAmount)), nil
		}
	}
	if r.TriggerKind == TriggerPayment {
		return true, nil
	}
	n, err := s.q.CountAutomationOpenDocTasks(ctx, db.CountAutomationOpenDocTasksParams{OrganizationID: r.OrganizationID, EngagementID: eng})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Service) finish(ctx context.Context, run db.AutomationRun, status, note string) error {
	if run.Channels == nil {
		run.Channels = []string{}
	}
	_, err := s.q.FinishAutomationRun(ctx, db.FinishAutomationRunParams{OrganizationID: run.OrganizationID, ID: run.ID,
		Status: status, Subject: run.Subject, Body: run.Body, Channels: run.Channels, Result: nonEmpty(run.Result), Note: note})
	if err != nil {
		return fmt.Errorf("finish automation run: %w", err)
	}
	return nil
}

func nonEmpty(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte(`{}`)
	}
	return raw
}

// ProcessRun sends (or drafts) one scheduled run.
func (s *Service) ProcessRun(ctx context.Context, org, id int64) error {
	run, err := s.q.GetAutomationRunForUpdate(ctx, db.GetAutomationRunForUpdateParams{OrganizationID: org, ID: id})
	if err != nil {
		return fmt.Errorf("automation run: %w", err)
	}
	if run.Status != RunScheduled {
		return nil
	}
	r, err := s.q.GetAutomationRuleByID(ctx, db.GetAutomationRuleByIDParams{OrganizationID: org, ID: run.RuleID})
	if err != nil {
		return fmt.Errorf("automation rule: %w", err)
	}
	if r.DeletedAt.Valid || !r.Active {
		return s.finish(ctx, run, RunSkipped, "Kural kapalı.")
	}
	if on, err := s.Enabled(ctx, org); err != nil {
		return err
	} else if !on {
		return s.finish(ctx, run, RunSkipped, "Otomasyonlar ofiste kapalı.")
	}
	t := parseTrigger(r.TriggerConfig)
	vars := readVars(run.Vars)
	if r.TriggerKind == TriggerEvent && t.DelayMaxHours > 0 {
		if at, err := time.Parse(time.RFC3339, vars[varOccurredAt]); err == nil &&
			s.now().After(at.Add(time.Duration(t.DelayMaxHours)*time.Hour)) {
			return s.finish(ctx, run, RunSkipped, "En fazla gecikme süresi geçti.")
		}
	}
	if !run.EngagementID.Valid {
		return s.finish(ctx, run, RunFailed, "Mükellef dosyası yok.")
	}
	rec, err := s.q.GetAutomationRecipient(ctx, db.GetAutomationRecipientParams{OrganizationID: org, EngagementID: run.EngagementID.Int64})
	if errors.Is(err, pgx.ErrNoRows) {
		return s.finish(ctx, run, RunFailed, "Mükellef dosyası bulunamadı.")
	} else if err != nil {
		return fmt.Errorf("automation recipient: %w", err)
	}
	full, err := s.varsFor(ctx, r, t, run, rec)
	if err != nil {
		return err
	}
	if r.OnlyDue {
		ok, err := s.hasDue(ctx, r, t, run, rec.EngagementID, full)
		if err != nil {
			return fmt.Errorf("automation due check: %w", err)
		}
		if !ok {
			return s.finish(ctx, run, RunSkipped, "Ödemesi ya da eksiği yok.")
		}
	}
	subject, body := Render(r.Subject, full), Render(r.Body, full)
	if subject == "" {
		subject = r.Name
	}
	if w := communication.CampaignWords(subject + "\n" + body); len(w) > 0 {
		run.Subject, run.Body = subject, body
		return s.finish(ctx, run, RunFailed, "Kampanya içerikli metin gönderilmez ("+strings.Join(w, ", ")+").")
	}
	if r.ApprovalMode == ApprovalDraft {
		_, err := s.q.FinishAutomationRun(ctx, db.FinishAutomationRunParams{OrganizationID: org, ID: run.ID, Status: RunDraft,
			Subject: subject, Body: body, Channels: r.Channels, Result: []byte(`{}`), Note: "Ofis onayı bekliyor."})
		if err != nil {
			return fmt.Errorf("draft automation run: %w", err)
		}
		return nil
	}
	_, err = s.deliver(ctx, r, run, subject, body, 0)
	return err
}

type channelResult struct {
	Status string `json:"status"`
	UUID   string `json:"uuid,omitempty"`
	Error  string `json:"error,omitempty"`
	Count  int    `json:"count,omitempty"`
	Hours  int    `json:"hours,omitempty"`
}

func errText(err error) string {
	if msg := communication.UserMessage(err); msg != "" {
		return msg
	}
	return "Gönderim sırasında hata oluştu."
}

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func sourceRule(r db.AutomationRule) string {
	if r.TemplateCode != "" {
		return "automation:" + r.TemplateCode
	}
	return "automation:" + r.Uuid.String()
}

func (s *Service) sendSMS(ctx context.Context, r db.AutomationRule, run db.AutomationRun, eng uuid.UUID, body string, actor int64) channelResult {
	if s.send == nil {
		return channelResult{Status: communication.StatusFailed, Error: "SMS servisi yok."}
	}
	m, err := s.send.SMSToEngagement(ctx, r.OrganizationID, eng, communication.SMSInput{Text: body,
		SourceRule: sourceRule(r), SourceRef: run.Uuid.String(), SentBy: actor})
	if err != nil {
		return channelResult{Status: communication.StatusFailed, Error: errText(err)}
	}
	return channelResult{Status: m.Status, UUID: m.Uuid.String(), Error: m.ErrorMessage}
}

func okStatus(st string) bool { return st != communication.StatusFailed && st != "" }

// deliver sends a run over the rule's channels and writes the outcome.
// With "e-posta açılmadıysa SMS" the SMS waits for the mail check.
func (s *Service) deliver(ctx context.Context, r db.AutomationRule, run db.AutomationRun, subject, body string, actor int64) (db.AutomationRun, error) {
	now := s.now()
	p := db.FinishAutomationRunParams{OrganizationID: r.OrganizationID, ID: run.ID, Subject: subject, Body: body,
		Channels: r.Channels, Note: ""}
	if actor > 0 {
		p.DecidedBy, p.DecidedAt = actorID(actor), ts(now)
	}
	results := map[string]channelResult{}
	ok := 0
	rec, err := s.q.GetAutomationRecipient(ctx, db.GetAutomationRecipientParams{OrganizationID: r.OrganizationID, EngagementID: run.EngagementID.Int64})
	if err != nil {
		p.Status, p.Note, p.Result = RunFailed, "Mükellef dosyası bulunamadı.", []byte(`{}`)
		return s.q.FinishAutomationRun(ctx, p)
	}
	followup := r.SmsIfUnopenedHours.Valid && has(r.Channels, ChannelSMS) && has(r.Channels, ChannelEmail)
	mailOK := false
	if has(r.Channels, ChannelEmail) {
		res := channelResult{Status: communication.StatusFailed, Error: "E-posta servisi yok."}
		if s.send != nil {
			m, err := s.send.MailToEngagement(ctx, r.OrganizationID, rec.EngagementUuid, communication.MailInput{
				Subject: subject, Body: body, SourceRule: sourceRule(r), SourceRef: run.Uuid.String(), SentBy: actor})
			if err != nil {
				res = channelResult{Status: communication.StatusFailed, Error: errText(err)}
			} else {
				res = channelResult{Status: m.Status, UUID: m.Uuid.String(), Error: m.ErrorMessage}
				p.MailMessageID = pgtype.Int8{Int64: m.ID, Valid: true}
			}
		}
		results[ChannelEmail] = res
		if okStatus(res.Status) {
			ok++
			mailOK = true
		}
	}
	if has(r.Channels, ChannelSMS) {
		if followup && mailOK {
			h := int(r.SmsIfUnopenedHours.Int32)
			results[ChannelSMS] = channelResult{Status: "waiting", Hours: h}
			p.SmsFollowupAt = ts(now.Add(time.Duration(h) * time.Hour))
		} else {
			res := s.sendSMS(ctx, r, run, rec.EngagementUuid, body, actor)
			results[ChannelSMS] = res
			if okStatus(res.Status) {
				ok++
			}
		}
	}
	if has(r.Channels, ChannelInapp) {
		res := s.sendInapp(ctx, r, rec, subject, body)
		results[ChannelInapp] = res
		if okStatus(res.Status) {
			ok++
		}
	}
	raw, err := json.Marshal(results)
	if err != nil {
		return run, err
	}
	p.Result = raw
	if ok > 0 {
		p.Status, p.SentAt = RunSent, ts(now)
		if t := parseTrigger(r.TriggerConfig); r.TriggerKind == TriggerEvent && t.ResendAfterDays > 0 && !strings.HasSuffix(run.RunKey, ":resend") {
			p.ResendCheckAt = ts(now.Add(time.Duration(t.ResendAfterDays) * 24 * time.Hour))
		}
	} else {
		p.Status, p.Note = RunFailed, "Hiçbir kanaldan gönderilemedi."
	}
	out, err := s.q.FinishAutomationRun(ctx, p)
	if err != nil {
		return run, fmt.Errorf("finish automation run: %w", err)
	}
	if out.Status == RunSent {
		if err := s.q.TouchAutomationRuleSent(ctx, db.TouchAutomationRuleSentParams{OrganizationID: r.OrganizationID, ID: r.ID}); err != nil {
			return out, fmt.Errorf("count automation send: %w", err)
		}
	}
	return out, nil
}

func (s *Service) sendInapp(ctx context.Context, r db.AutomationRule, rec db.GetAutomationRecipientRow, subject, body string) channelResult {
	if s.notify == nil {
		return channelResult{Status: communication.StatusFailed, Error: "Bildirim servisi yok."}
	}
	users, err := s.q.ListAutomationTaxpayerMembers(ctx, rec.TaxpayerID)
	if err != nil {
		return channelResult{Status: communication.StatusFailed, Error: "Mükellef kullanıcıları okunamadı."}
	}
	if len(users) == 0 {
		return channelResult{Status: communication.StatusFailed, Error: "Mükellefin uygulama kullanıcısı yok."}
	}
	var link *string
	if s.frontendURL != "" {
		l := s.frontendURL + "/m/" + rec.TaxpayerUuid.String()
		link = &l
	}
	org := r.OrganizationID
	n := 0
	for _, uid := range users {
		user := uid
		if _, err := s.notify.Enqueue(ctx, notifmodel.EnqueueInput{OrganizationID: &org, UserID: &user,
			Channels: []string{notifmodel.ChannelInapp}, Title: subject, Body: body, ActionURL: link,
			SourceEvent: "automation.message",
			Payload:     map[string]any{"kind": "automation", "rule_uuid": r.Uuid.String(), "taxpayer_uuid": rec.TaxpayerUuid.String()},
		}); err != nil {
			s.log.Warn("automation_inapp_failed", "error", err, "rule", r.Uuid)
			continue
		}
		n++
	}
	if n == 0 {
		return channelResult{Status: communication.StatusFailed, Error: "Bildirim kuyruğa alınamadı."}
	}
	return channelResult{Status: communication.StatusSent, Count: n}
}

// processFollowup sends the waiting SMS of "e-posta açılmadıysa SMS" when
// the mail is still unopened, else marks it skipped.
func (s *Service) processFollowup(ctx context.Context, org, id int64) error {
	run, err := s.q.GetAutomationRunForUpdate(ctx, db.GetAutomationRunForUpdateParams{OrganizationID: org, ID: id})
	if err != nil {
		return err
	}
	if run.SmsFollowupStatus != "" || !run.SmsFollowupAt.Valid {
		return nil
	}
	results := map[string]channelResult{}
	_ = json.Unmarshal(run.Result, &results)
	opened := false
	if run.MailMessageID.Valid {
		opened, err = s.q.GetAutomationMailOpened(ctx, db.GetAutomationMailOpenedParams{OrganizationID: org, ID: run.MailMessageID.Int64})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	status := "skipped"
	if opened {
		results[ChannelSMS] = channelResult{Status: "skipped", Error: "E-posta açıldı, SMS gerekmedi."}
	} else {
		r, err := s.q.GetAutomationRuleByID(ctx, db.GetAutomationRuleByIDParams{OrganizationID: org, ID: run.RuleID})
		if err != nil {
			return err
		}
		rec, err := s.q.GetAutomationRecipient(ctx, db.GetAutomationRecipientParams{OrganizationID: org, EngagementID: run.EngagementID.Int64})
		if err != nil {
			return err
		}
		results[ChannelSMS] = s.sendSMS(ctx, r, run, rec.EngagementUuid, run.Body, 0)
		status = "sent"
	}
	raw, err := json.Marshal(results)
	if err != nil {
		return err
	}
	return s.q.FinishAutomationFollowup(ctx, db.FinishAutomationFollowupParams{OrganizationID: org, ID: id,
		SmsFollowupStatus: status, Result: raw})
}

// processResend writes the second run of "olaydan X gün sonra hâlâ
// ödenmediyse tekrar gönder" when the payment is still open.
func (s *Service) processResend(ctx context.Context, org, id int64) error {
	run, err := s.q.GetAutomationRunForUpdate(ctx, db.GetAutomationRunForUpdateParams{OrganizationID: org, ID: id})
	if err != nil {
		return err
	}
	if err := s.q.ClearAutomationResend(ctx, db.ClearAutomationResendParams{OrganizationID: org, ID: id}); err != nil {
		return err
	}
	if !run.EngagementID.Valid {
		return nil
	}
	r, err := s.q.GetAutomationRuleByID(ctx, db.GetAutomationRuleByIDParams{OrganizationID: org, ID: run.RuleID})
	if err != nil {
		return err
	}
	if r.DeletedAt.Valid || !r.Active {
		return nil
	}
	vars := readVars(run.Vars)
	unpaid := true
	if rid, err := uuid.Parse(vars[varReturnUUID]); err == nil {
		paid, err := s.q.GetAutomationReturnPaid(ctx, db.GetAutomationReturnPaidParams{OrganizationID: org, EngagementID: run.EngagementID.Int64, Uuid: rid})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		unpaid = !paid
	} else if vars[varVAT] == "1" && run.Period != "" {
		n, err := s.q.CountAutomationPaidVatReturn(ctx, db.CountAutomationPaidVatReturnParams{OrganizationID: org, EngagementID: run.EngagementID.Int64, Period: run.Period})
		if err != nil {
			return err
		}
		unpaid = n == 0
	}
	if !unpaid {
		return nil
	}
	// The reminder is its own moment: the max delay counts from now.
	vars[varOccurredAt] = s.now().UTC().Format(time.RFC3339)
	_, err = s.insertRun(ctx, r, run.EngagementID.Int64, run.RunKey+":resend", run.Period, s.now(), vars, run.SourceRef)
	return err
}
