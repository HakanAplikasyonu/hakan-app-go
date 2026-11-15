package doctasks

import (
	"context"
	"fmt"
	"strconv"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
)

// reminderBatch caps the engagements handled per run; the rest wait for the
// next run (their next_reminder_at is still due).
const reminderBatch = 500

// ReminderReport is what a reminder run did.
type ReminderReport struct {
	Engagements int `json:"engagements"`
	Tasks       int `json:"tasks"`
	Notified    int `json:"notified"`
}

// SendReminders notifies the members of every mükellef with due open tasks
// (one notification per member and engagement, not per task) and moves each
// task's next reminder out with backoff: ReminderBaseDays after the task is
// opened, then ×2 after every reminder, at most MaxReminders. Tasks of ended
// engagements are skipped. It is the scheduled job's entry point.
func (s *Service) SendReminders(ctx context.Context) (ReminderReport, error) {
	var rep ReminderReport
	now := s.now()
	rows, err := s.q.ListDueDocumentTaskReminders(ctx, db.ListDueDocumentTaskRemindersParams{Now: ts(now), Lim: reminderBatch})
	if err != nil {
		return rep, fmt.Errorf("doctasks: due reminders: %w", err)
	}
	for _, r := range rows {
		rep.Engagements++
		rep.Tasks += int(r.TaskCount)
		rep.Notified += s.notifyMembers(ctx, TemplateReminder, r.OrganizationID, r.TaxpayerID, int(r.TaskCount))
		// Marked even when nobody could be told (no member yet): backoff
		// keeps the job from retrying the same group every run.
		if _, err := s.q.MarkDocumentTasksReminded(ctx, db.MarkDocumentTasksRemindedParams{
			Now: ts(now), MaxReminders: int32(s.cfg.MaxReminders), BaseDays: int32(s.cfg.ReminderBaseDays), //nolint:gosec // small config values
			OrganizationID: r.OrganizationID, EngagementID: r.EngagementID, Ids: r.TaskIds,
		}); err != nil {
			return rep, fmt.Errorf("doctasks: mark reminded: %w", err)
		}
	}
	return rep, nil
}

// SendRemindersCount adapts SendReminders to the queue handler signature.
func (s *Service) SendRemindersCount(ctx context.Context) (int, error) {
	rep, err := s.SendReminders(ctx)
	return rep.Notified, err
}

// notifyMembers sends template to every member of the taxpayer (in-app, which
// mirrors to mobile push, and email). Failures are logged: a notification
// never blocks a task. Returns how many members were notified.
func (s *Service) notifyMembers(ctx context.Context, template string, organizationID, taxpayerID int64, count int) int {
	if s.notifier == nil {
		return 0
	}
	users, err := s.q.ListDocumentTaskRecipients(ctx, taxpayerID)
	if err != nil {
		s.log.Warn("doctasks_recipients_failed", "error", err, "taxpayer_id", taxpayerID)
		return 0
	}
	if len(users) == 0 {
		return 0
	}
	names, err := s.q.GetDocumentTaskNotifyNames(ctx, db.GetDocumentTaskNotifyNamesParams{OrganizationID: organizationID, TaxpayerID: taxpayerID})
	if err != nil {
		s.log.Warn("doctasks_notify_names_failed", "error", err, "taxpayer_id", taxpayerID)
		return 0
	}
	link := s.link(names.TaxpayerUuid.String())
	sent := 0
	for _, uid := range users {
		org, user := organizationID, uid
		_, err := s.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
			OrganizationID: &org,
			UserID:         &user,
			Channels:       []string{notifmodel.ChannelInapp, notifmodel.ChannelEmail},
			TemplateCode:   template,
			SourceEvent:    template,
			ActionURL:      &link,
			TemplateVars: map[string]string{
				"org_name":      names.OrganizationName,
				"taxpayer_name": names.TaxpayerTitle,
				"count":         strconv.Itoa(count),
				"link":          link,
			},
			Payload: map[string]any{
				"kind":          "doc_tasks",
				"taxpayer_uuid": names.TaxpayerUuid.String(),
				"count":         count,
			},
		})
		if err != nil {
			s.log.Warn("doctasks_notify_failed", "error", err, "template", template, "taxpayer_id", taxpayerID)
			continue
		}
		sent++
	}
	return sent
}
