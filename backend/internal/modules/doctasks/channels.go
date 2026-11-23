package doctasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Belge talebi kanalları (Oğuz'un geri bildirimi): the office picks how the
// taxpayer hears about a document request (push = in-app notification with
// mobile push, SMS, e-mail), the due date and an optional reminder N days
// before it. Every send is a record in the taxpayer's communication history
// (push_messages / sms_messages / mail_messages, communication module), with
// the task uuid as source_ref; the task's per-channel status is read back
// from there.

// Channels of a document request.
const (
	ChannelPush  = "push"
	ChannelSMS   = "sms"
	ChannelEmail = "email"
)

// AllChannels are the accepted channel values, in display order.
var AllChannels = []string{ChannelPush, ChannelSMS, ChannelEmail}

// Source rules of the communication records.
const (
	SourceRequest  = "doc_request"
	SourceReminder = "doc_request_reminder"
)

// SettingChannels is the office setting (organizations.settings) that turns
// the channel choice, the "send now" button and the scheduled channel
// reminder on. Missing or true: on. False: a request notifies the app users
// as before (in-app + e-mail notification) and /send answers 409.
const SettingChannels = "doc_request_channels"

// Delivery statuses the service sets itself (the others come from the
// records: sent, delivered, dry_run, failed, uncertain, pending).
const (
	DeliverySkipped = "skipped"
)

// remindHour is the hour (Türkiye time) the scheduled reminder goes out.
const remindHour = 10

// sendCooldown keeps the "send now" button from flooding the taxpayer.
const sendCooldown = 10 * time.Minute

var (
	// ErrChannelsDisabled: the office turned document request channels off.
	ErrChannelsDisabled = errors.New("doctasks: document request channels are off")
	// ErrRecentlySent: the task was sent a reminder moments ago.
	ErrRecentlySent = errors.New("doctasks: reminded moments ago")
)

// ChannelDelivery is the latest send of one channel for a task.
type ChannelDelivery struct {
	Channel string `json:"channel"`
	// Kind: request (the first message) or reminder.
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	Code        string     `json:"code,omitempty"`
	Message     string     `json:"message,omitempty"`
	At          *time.Time `json:"at"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
}

// SourceRecord is a communication record found for a task.
type SourceRecord struct {
	SourceRef, SourceRule, Channel, Status, Code, Message string
	At                                                    time.Time
	DeliveredAt                                           *time.Time
}

// ChannelSender sends through the office's channels and records each send in
// the communication history (communication module). Nil: channels are off.
type ChannelSender interface {
	SMS(ctx context.Context, organizationID int64, engagement uuid.UUID, text, sourceRule, sourceRef string, actorID int64) (SMSOutcome, error)
	Mail(ctx context.Context, organizationID int64, engagement uuid.UUID, subject, body, sourceRule, sourceRef string, actorID int64) (SMSOutcome, error)
	RecordPush(ctx context.Context, organizationID, engagementID int64, title, body string, recipients int, sourceRule, sourceRef string, actorID int64) error
	Latest(ctx context.Context, organizationID, engagementID int64, rules, refs []string) ([]SourceRecord, error)
}

// WithChannels enables document request channels.
func (s *Service) WithChannels(c ChannelSender) *Service {
	s.channels = c
	return s
}

// SendInput is the body of POST .../document-tasks/{taskUuid}/send.
type SendInput struct {
	// Channels: push, sms, email; empty = the task's own channels (push when
	// it has none).
	Channels []string `json:"channels"`
	// Reminder: the message is a reminder ("hâlâ bekleniyor"); false sends
	// the request again. Default true.
	Reminder *bool `json:"reminder"`
}

// ChannelsEnabled reads SettingChannels from an organization's settings.
func ChannelsEnabled(settings []byte) bool {
	var m map[string]any
	if len(settings) == 0 || json.Unmarshal(settings, &m) != nil {
		return true
	}
	on, ok := m[SettingChannels].(bool)
	return !ok || on
}

func (s *Service) channelsOn(ctx context.Context, organizationID int64) bool {
	if s.channels == nil {
		return false
	}
	org, err := s.q.GetOrganizationByID(ctx, organizationID)
	if err != nil {
		s.log.Warn("doctasks_org_settings_failed", "error", err, "organization_id", organizationID)
		return false
	}
	return ChannelsEnabled(org.Settings)
}

// cleanChannels validates and de-duplicates channels (display order).
func cleanChannels(in []string) ([]string, error) {
	seen := map[string]bool{}
	for _, c := range in {
		c = strings.ToLower(strings.TrimSpace(c))
		if !slices.Contains(AllChannels, c) {
			return nil, fmt.Errorf("%w: channel must be push, sms or email", ErrBadInput)
		}
		seen[c] = true
	}
	out := []string{}
	for _, c := range AllChannels {
		if seen[c] {
			out = append(out, c)
		}
	}
	return out, nil
}

// remindAt is 10:00 (Türkiye time) of the day N days before due. A time
// already gone today becomes now (the next hourly run sends it); an earlier
// day is refused.
func (s *Service) remindAt(due time.Time, days int) (time.Time, error) {
	if days < 0 || days > 30 {
		return time.Time{}, fmt.Errorf("%w: remind_days_before must be between 0 and 30", ErrBadInput)
	}
	d := due.In(istanbul).AddDate(0, 0, -days)
	at := time.Date(d.Year(), d.Month(), d.Day(), remindHour, 0, 0, 0, istanbul)
	now := s.now()
	if at.Before(now) {
		n := now.In(istanbul)
		if n.Year() == d.Year() && n.YearDay() == d.YearDay() {
			return now, nil
		}
		return time.Time{}, fmt.Errorf("%w: the reminder day is already past; pick fewer days or a later due date", ErrBadInput)
	}
	return at, nil
}

// sendChannels sends the request (or a reminder) of t on every channel and
// returns one outcome per channel. A channel never fails the call.
func (s *Service) sendChannels(ctx context.Context, scope engagementctx.Scope, actorID int64, t Task, channels []string, reminder bool) []ChannelDelivery {
	out := make([]ChannelDelivery, 0, len(channels))
	if s.channels == nil {
		return out
	}
	names, err := s.q.GetDocumentTaskNotifyNames(ctx, db.GetDocumentTaskNotifyNamesParams{OrganizationID: scope.OrganizationID, TaxpayerID: scope.TaxpayerID})
	if err != nil {
		s.log.Warn("doctasks_notify_names_failed", "error", err)
		for _, c := range channels {
			out = append(out, ChannelDelivery{Channel: c, Kind: kindOf(reminder), Status: SMSFailed, Code: "error"})
		}
		return out
	}
	rule := SourceRequest
	if reminder {
		rule = SourceReminder
	}
	ref := t.UUID.String()
	link := s.link(names.TaxpayerUuid.String()) + "/" + ref
	text := RequestText(names.OrganizationName, taskTitle(t), t.DueAt, reminder)
	for _, c := range channels {
		d := ChannelDelivery{Channel: c, Kind: kindOf(reminder)}
		var o SMSOutcome
		err = nil
		switch c {
		case ChannelPush:
			o = s.sendPush(ctx, scope, actorID, t, names, link, text, rule, reminder)
		case ChannelSMS:
			o, err = s.channels.SMS(ctx, scope.OrganizationID, scope.UUID, text, rule, ref, actorID)
			o = outcomeOf(o, err)
		case ChannelEmail:
			subject := fmt.Sprintf("%s belge istiyor: %s", strings.TrimSpace(names.OrganizationName), taskTitle(t))
			if reminder {
				subject = "Hatırlatma: " + taskTitle(t)
			}
			body := text
			if t.Note != "" {
				body += "\n\nNot: " + t.Note
			}
			body += "\n\nYüklemek için: " + link
			o, err = s.channels.Mail(ctx, scope.OrganizationID, scope.UUID, subject, body, rule, ref, actorID)
			o = outcomeOf(o, err)
		}
		if err != nil && !errors.Is(err, ErrSMSNotConfigured) && !errors.Is(err, ErrSMSNoContact) {
			s.log.Warn("doctasks_channel_send_failed", "error", err, "channel", c, "engagement_id", scope.ID)
		}
		d.Status, d.Code, d.Message = o.Status, o.Code, o.Message
		now := s.now()
		d.At = &now
		out = append(out, d)
	}
	return out
}

func kindOf(reminder bool) string {
	if reminder {
		return "reminder"
	}
	return "request"
}

func outcomeOf(o SMSOutcome, err error) SMSOutcome {
	switch {
	case errors.Is(err, ErrSMSNotConfigured):
		return SMSOutcome{Status: DeliverySkipped, Code: SMSCodeNotConfigured}
	case errors.Is(err, ErrSMSNoContact):
		return SMSOutcome{Status: DeliverySkipped, Code: SMSCodeNoContact}
	case err != nil:
		return SMSOutcome{Status: SMSFailed, Code: "error"}
	}
	return o
}

// sendPush notifies the taxpayer's app users (in-app, mirrored to mobile
// push) and records the send.
func (s *Service) sendPush(ctx context.Context, scope engagementctx.Scope, actorID int64, t Task, names db.GetDocumentTaskNotifyNamesRow, link, text, rule string, reminder bool) SMSOutcome {
	users, err := s.q.ListDocumentTaskRecipients(ctx, scope.TaxpayerID)
	if err != nil {
		s.log.Warn("doctasks_recipients_failed", "error", err, "taxpayer_id", scope.TaxpayerID)
		return SMSOutcome{Status: SMSFailed, Code: "error"}
	}
	title := taskTitle(t)
	if reminder {
		title = "Hatırlatma: " + title
	}
	sent := 0
	if s.notifier != nil {
		for _, uid := range users {
			org, user := scope.OrganizationID, uid
			if _, err := s.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
				OrganizationID: &org, UserID: &user,
				Channels:     []string{notifmodel.ChannelInapp},
				TemplateCode: TemplateRequested, SourceEvent: TemplateRequested, ActionURL: &link,
				TemplateVars: map[string]string{"org_name": names.OrganizationName, "taxpayer_name": names.TaxpayerTitle, "title": title, "link": link},
				Payload: map[string]any{"kind": "doc_task", "task_uuid": t.UUID.String(), "taxpayer_uuid": names.TaxpayerUuid.String(),
					"reminder": reminder},
			}); err != nil {
				s.log.Warn("doctasks_notify_failed", "error", err, "template", TemplateRequested)
				continue
			}
			sent++
		}
	}
	pushTitle := "Yeni belge talebi"
	if reminder {
		pushTitle = "Belge hatırlatması"
	}
	if err := s.channels.RecordPush(ctx, scope.OrganizationID, scope.ID, pushTitle, text, sent, rule, t.UUID.String(), actorID); err != nil {
		s.log.Warn("doctasks_push_record_failed", "error", err, "engagement_id", scope.ID)
	}
	if sent == 0 {
		return SMSOutcome{Status: SMSFailed, Code: "no_user"}
	}
	return SMSOutcome{Status: "sent"}
}

// RequestText is the SMS / push text of a document request or reminder.
func RequestText(office, title string, due time.Time, reminder bool) string {
	if r := []rune(title); len(r) > 120 {
		title = string(r[:117]) + "..."
	}
	date := due.In(istanbul).Format("02.01.2006")
	if reminder {
		return fmt.Sprintf("%s hatırlatma: %s henüz gelmedi. Son tarih %s. Mükellef uygulamasından fotoğraf ya da PDF olarak yükleyebilirsiniz.",
			strings.TrimSpace(office), title, date)
	}
	return fmt.Sprintf("%s sizden belge istiyor: %s. Son tarih %s. Mükellef uygulamasından fotoğraf ya da PDF olarak yükleyebilirsiniz.",
		strings.TrimSpace(office), title, date)
}

// Send sends the task now on the chosen channels ("Hatırlat" button, office
// mobile app). Only an open task can be sent; a reminder within the last
// minutes is refused.
func (s *Service) Send(ctx context.Context, scope engagementctx.Scope, actorID int64, id uuid.UUID, in SendInput) (Task, error) {
	if !s.channelsOn(ctx, scope.OrganizationID) {
		return Task{}, ErrChannelsDisabled
	}
	channels, err := cleanChannels(in.Channels)
	if err != nil {
		return Task{}, err
	}
	t, err := s.Get(ctx, scope, id)
	if err != nil {
		return Task{}, err
	}
	if t.Status != StatusOpen {
		return t, ErrClosed
	}
	if len(channels) == 0 {
		channels = t.Channels
	}
	if len(channels) == 0 {
		channels = []string{ChannelPush}
	}
	reminder := in.Reminder == nil || *in.Reminder
	if reminder && t.ChannelRemindedAt != nil && s.now().Sub(*t.ChannelRemindedAt) < sendCooldown {
		return t, ErrRecentlySent
	}
	sent := s.sendChannels(ctx, scope, actorID, t, channels, reminder)
	if reminder {
		if _, err := s.q.MarkDocumentTaskChannelReminded(ctx, db.MarkDocumentTaskChannelRemindedParams{
			Now: ts(s.now()), OrganizationID: scope.OrganizationID, EngagementID: scope.ID, ID: t.id,
		}); err != nil {
			return Task{}, fmt.Errorf("doctasks: mark reminded: %w", err)
		}
	}
	out, err := s.Get(ctx, scope, id)
	if err != nil {
		return Task{}, err
	}
	out.Deliveries = mergeDeliveries(out.Deliveries, sent)
	return out, nil
}

// mergeDeliveries puts the outcomes just seen (with their skip reasons) over
// the ones read from the records.
func mergeDeliveries(read, now []ChannelDelivery) []ChannelDelivery {
	by := map[string]ChannelDelivery{}
	for _, d := range read {
		by[d.Channel] = d
	}
	for _, d := range now {
		if d.Status == DeliverySkipped || by[d.Channel].Status == "" {
			by[d.Channel] = d
		}
	}
	out := []ChannelDelivery{}
	for _, c := range AllChannels {
		if d, ok := by[c]; ok {
			out = append(out, d)
		}
	}
	return out
}

// withDeliveries fills the per-channel status of tasks from the
// communication history. A chosen channel without a record is "skipped"
// (no contact on record, or SMS not set up).
func (s *Service) withDeliveries(ctx context.Context, scope engagementctx.Scope, tasks []Task) {
	if s.channels == nil || len(tasks) == 0 {
		return
	}
	refs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		refs = append(refs, t.UUID.String())
	}
	recs, err := s.channels.Latest(ctx, scope.OrganizationID, scope.ID, []string{SourceRequest, SourceReminder}, refs)
	if err != nil {
		s.log.Warn("doctasks_deliveries_failed", "error", err, "engagement_id", scope.ID)
		return
	}
	by := map[string][]ChannelDelivery{}
	for _, r := range recs {
		at := r.At
		kind := "request"
		if r.SourceRule == SourceReminder {
			kind = "reminder"
		}
		status := r.Status
		if r.Channel == ChannelSMS && r.DeliveredAt != nil && status == "sent" {
			status = "delivered"
		}
		by[r.SourceRef] = append(by[r.SourceRef], ChannelDelivery{Channel: r.Channel, Kind: kind, Status: status,
			Code: r.Code, Message: r.Message, At: &at, DeliveredAt: r.DeliveredAt})
	}
	for i := range tasks {
		got := by[tasks[i].UUID.String()]
		have := map[string]bool{}
		for _, d := range got {
			have[d.Channel] = true
		}
		for _, c := range tasks[i].Channels {
			if !have[c] {
				got = append(got, ChannelDelivery{Channel: c, Kind: "request", Status: DeliverySkipped})
			}
		}
		if len(got) > 0 {
			slices.SortStableFunc(got, func(a, b ChannelDelivery) int {
				return slices.Index(AllChannels, a.Channel) - slices.Index(AllChannels, b.Channel)
			})
			tasks[i].Deliveries = got
		}
	}
}

// sendChannelReminders sends the scheduled reminders (N days before due) of
// requests with channels. With the office setting off the reminder is
// dropped. Runs inside the reminder job (every office).
func (s *Service) sendChannelReminders(ctx context.Context, rep *ReminderReport) error {
	if s.channels == nil {
		return nil
	}
	now := s.now()
	rows, err := s.q.ListDueChannelReminders(ctx, db.ListDueChannelRemindersParams{Now: ts(now), Lim: reminderBatch})
	if err != nil {
		return fmt.Errorf("doctasks: due channel reminders: %w", err)
	}
	enabled := map[int64]bool{}
	for _, r := range rows {
		on, ok := enabled[r.OrganizationID]
		if !ok {
			on = s.channelsOn(ctx, r.OrganizationID)
			enabled[r.OrganizationID] = on
		}
		scope := engagementctx.Scope{ID: r.EngagementID, UUID: r.EngagementUuid, OrganizationID: r.OrganizationID, TaxpayerID: r.TaxpayerID}
		sent := false
		if on {
			t, err := s.Get(ctx, scope, r.Uuid)
			if err != nil {
				return err
			}
			channels := t.Channels
			if len(channels) == 0 {
				channels = []string{ChannelPush}
			}
			rep.Tasks++
			for _, d := range s.sendChannels(ctx, scope, 0, t, channels, true) {
				if d.Status != SMSFailed && d.Status != DeliverySkipped {
					rep.Notified++
					sent = true
				}
			}
		}
		if _, err := s.q.FinishScheduledChannelReminder(ctx, db.FinishScheduledChannelReminderParams{
			Sent: sent, Now: ts(now), OrganizationID: r.OrganizationID, EngagementID: r.EngagementID, ID: r.ID,
		}); err != nil {
			return fmt.Errorf("doctasks: finish channel reminder: %w", err)
		}
	}
	return nil
}

func int2(v *int) pgtype.Int2 {
	if v == nil {
		return pgtype.Int2{}
	}
	return pgtype.Int2{Int16: int16(*v), Valid: true} //nolint:gosec // 0..30, validated
}
