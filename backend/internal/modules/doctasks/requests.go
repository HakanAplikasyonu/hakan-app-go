package doctasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Document request (issue #240): a task the office opens by hand, not tied
// to a bank transaction.
const (
	CodeDocumentRequest = "DOC_REQUEST"
	KindDocumentRequest = "document_request"
)

// RequestInput opens a document request.
type RequestInput struct {
	Title string `json:"title"`
	Note  string `json:"note"`
	// DueDate is the last day (YYYY-MM-DD, Türkiye time); DueDays is used
	// when it is empty.
	DueDate string `json:"due_date"`
	DueDays int    `json:"due_days"`
	// Period defaults to the month of the due date (Türkiye time).
	Period string `json:"period"`
}

// ErrRequestExists: an open request with the same title already exists;
// the existing task is returned with it.
var ErrRequestExists = errors.New("doctasks: request already open")

// RerequestInput asks again after an upload that was not enough.
type RerequestInput struct {
	Note    string `json:"note"`
	DueDate string `json:"due_date"`
	DueDays int    `json:"due_days"`
}

var istanbul = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("TRT", 3*60*60)
}()

// dueAt resolves the due moment: end of the given day (Türkiye time), or
// now + days (default policy when both are empty). Past dates are refused.
func (s *Service) dueAt(date string, days int) (time.Time, error) {
	now := s.now()
	if date = strings.TrimSpace(date); date != "" {
		d, err := time.ParseInLocation(time.DateOnly, date, istanbul)
		if err != nil {
			return time.Time{}, fmt.Errorf("%w: due_date must be YYYY-MM-DD", ErrBadInput)
		}
		end := time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, 0, istanbul)
		if end.Before(now) {
			return time.Time{}, fmt.Errorf("%w: due_at is in the past", ErrBadInput)
		}
		if end.After(now.AddDate(0, 0, 180)) {
			return time.Time{}, fmt.Errorf("%w: due_at must be within 180 days", ErrBadInput)
		}
		return end, nil
	}
	if days <= 0 {
		days = s.cfg.DueDays
	}
	if days > 90 {
		return time.Time{}, fmt.Errorf("%w: due_days must be at most 90", ErrBadInput)
	}
	return now.AddDate(0, 0, days), nil
}

func cleanText(v string, max int, field string) (string, error) {
	v = strings.TrimSpace(v)
	if len([]rune(v)) > max {
		return "", fmt.Errorf("%w: %s is longer than %d characters", ErrBadInput, field, max)
	}
	return v, nil
}

// CreateRequest opens a document request for the engagement's taxpayer and
// tells the mükellef's members.
func (s *Service) CreateRequest(ctx context.Context, scope engagementctx.Scope, actorID int64, in RequestInput) (Task, error) {
	title, err := cleanText(in.Title, 200, "title")
	if err != nil {
		return Task{}, err
	}
	if title == "" {
		return Task{}, fmt.Errorf("%w: title is required", ErrBadInput)
	}
	note, err := cleanText(in.Note, 1000, "note")
	if err != nil {
		return Task{}, err
	}
	due, err := s.dueAt(in.DueDate, in.DueDays)
	if err != nil {
		return Task{}, err
	}
	period := in.Period
	if period == "" {
		period = due.In(istanbul).Format("2006-01")
	}
	if !ValidPeriod(period) {
		return Task{}, ErrBadPeriod
	}
	// Aynı talep açıkken yenisi açılmaz; mükellefe ikinci bildirim gitmez.
	if open, err := s.q.OpenDocumentRequestByTitle(ctx, db.OpenDocumentRequestByTitleParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Title: title,
	}); err == nil {
		t, err := s.Get(ctx, scope, open)
		if err != nil {
			return Task{}, err
		}
		return t, ErrRequestExists
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Task{}, fmt.Errorf("doctasks: open request: %w", err)
	}
	row, err := s.q.CreateDocumentRequest(ctx, db.CreateDocumentRequestParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID,
		Period: period, Title: title, Note: note, DueAt: ts(due),
		NextReminderAt: ts(s.now().AddDate(0, 0, s.cfg.ReminderBaseDays)),
		CreatedBy:      pgtype.Int8{Int64: actorID, Valid: actorID > 0},
	})
	if err != nil {
		return Task{}, fmt.Errorf("doctasks: create request: %w", err)
	}
	t, err := s.Get(ctx, scope, row.Uuid)
	if err != nil {
		return Task{}, err
	}
	s.notifyRequested(ctx, scope, t)
	return t, nil
}

// Complete accepts the uploaded document: the task is done (matched).
func (s *Service) Complete(ctx context.Context, scope engagementctx.Scope, id uuid.UUID) (Task, error) {
	n, err := s.q.CompleteDocumentTask(ctx, db.CompleteDocumentTaskParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id})
	if err != nil {
		return Task{}, err
	}
	t, err := s.Get(ctx, scope, id)
	if err != nil {
		return Task{}, err
	}
	if n == 0 {
		return t, ErrClosed
	}
	return t, nil
}

// ErrNotSubmitted: only a submitted task can be asked again.
var ErrNotSubmitted = errors.New("doctasks: task is not submitted")

// Rerequest reopens a submitted task with a new note and due date and tells
// the mükellef again.
func (s *Service) Rerequest(ctx context.Context, scope engagementctx.Scope, id uuid.UUID, in RerequestInput) (Task, error) {
	note, err := cleanText(in.Note, 1000, "note")
	if err != nil {
		return Task{}, err
	}
	if note == "" {
		return Task{}, fmt.Errorf("%w: note is required (what is missing)", ErrBadInput)
	}
	due, err := s.dueAt(in.DueDate, in.DueDays)
	if err != nil {
		return Task{}, err
	}
	n, err := s.q.RerequestDocumentTask(ctx, db.RerequestDocumentTaskParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id,
		Note: note, DueAt: ts(due), NextReminderAt: ts(s.now().AddDate(0, 0, s.cfg.ReminderBaseDays)),
	})
	if err != nil {
		return Task{}, err
	}
	t, err := s.Get(ctx, scope, id)
	if err != nil {
		return Task{}, err
	}
	if n == 0 {
		return t, ErrNotSubmitted
	}
	s.notifyRequested(ctx, scope, t)
	return t, nil
}

// taskTitle is the line shown in notifications.
func taskTitle(t Task) string {
	if t.Title != "" {
		return t.Title
	}
	switch t.Kind {
	case "undocumented_payment":
		return "Ödeme için belge"
	case "uninvoiced_collection":
		return "Tahsilat için fatura"
	}
	return "Eksik belge"
}

// notifyRequested tells the mükellef's members about a new / repeated request.
func (s *Service) notifyRequested(ctx context.Context, scope engagementctx.Scope, t Task) {
	if s.notifier == nil {
		return
	}
	users, err := s.q.ListDocumentTaskRecipients(ctx, scope.TaxpayerID)
	if err != nil || len(users) == 0 {
		if err != nil {
			s.log.Warn("doctasks_recipients_failed", "error", err, "taxpayer_id", scope.TaxpayerID)
		}
		return
	}
	names, err := s.q.GetDocumentTaskNotifyNames(ctx, db.GetDocumentTaskNotifyNamesParams{OrganizationID: scope.OrganizationID, TaxpayerID: scope.TaxpayerID})
	if err != nil {
		s.log.Warn("doctasks_notify_names_failed", "error", err)
		return
	}
	link := s.link(names.TaxpayerUuid.String()) + "/" + t.UUID.String()
	for _, uid := range users {
		s.enqueue(ctx, TemplateRequested, scope.OrganizationID, uid, link, map[string]string{
			"org_name": names.OrganizationName, "taxpayer_name": names.TaxpayerTitle, "title": taskTitle(t), "link": link,
		}, map[string]any{"kind": "doc_task", "task_uuid": t.UUID.String(), "taxpayer_uuid": names.TaxpayerUuid.String()})
	}
}

// notifyOffice tells the office that the mükellef uploaded for a task: the
// member who opened it and the engagement's assignees. The payload carries
// what the müşavir mobil app needs to open the task (issue #241).
func (s *Service) notifyOffice(ctx context.Context, organizationID, engagementID, taxpayerID int64, createdBy pgtype.Int8, t Task) {
	if s.notifier == nil {
		return
	}
	// The upload runs in the mükellef's row level security scope (issue
	// #166), where office membership is not visible. Who to tell is an
	// office fact read on purpose outside that scope: a separate connection
	// (never a nested System on the request's transaction).
	var (
		users          []int64
		names          db.GetDocumentTaskNotifyNamesRow
		engagementUUID string
	)
	err := rls.System(detached(ctx), s.db, "doctasks: tell the office about a mükellef upload", func(sctx context.Context, _ pgx.Tx) error {
		var err error
		if users, err = s.q.ListDocumentTaskOfficeRecipients(sctx, db.ListDocumentTaskOfficeRecipientsParams{
			OrganizationID: organizationID, EngagementID: engagementID, CreatedBy: createdBy,
		}); err != nil || len(users) == 0 {
			return err
		}
		if names, err = s.q.GetDocumentTaskNotifyNames(sctx, db.GetDocumentTaskNotifyNamesParams{OrganizationID: organizationID, TaxpayerID: taxpayerID}); err != nil {
			return err
		}
		e, err := s.q.GetEngagementUUIDByID(sctx, db.GetEngagementUUIDByIDParams{OrganizationID: organizationID, ID: engagementID})
		if err != nil {
			return err
		}
		engagementUUID = e.String()
		return nil
	})
	if err != nil {
		s.log.Warn("doctasks_office_recipients_failed", "error", err, "engagement_id", engagementID)
		return
	}
	if len(users) == 0 {
		return
	}
	// The office panel's engagement page; `task` lets the müşavir mobil app
	// open the task itself (issue #241, hakanofis links).
	link := strings.TrimRight(s.cfg.FrontendURL, "/") + "/t/" + names.OrganizationSlug + "/engagements/" + engagementUUID + "?task=" + t.UUID.String()
	// The notifications are the office's rows (organization_id = the office):
	// in the mükellef's scope (no office) the notifications policy refuses
	// them (migration 000088, #307) and the failed insert aborted the
	// upload's transaction. Write them as the office, like #281.
	err = rls.ForOffice(ctx, organizationID, taxpayerID, func(ctx context.Context) error {
		for _, uid := range users {
			s.enqueue(ctx, TemplateSubmitted, organizationID, uid, link, map[string]string{
				"org_name": names.OrganizationName, "taxpayer_name": names.TaxpayerTitle, "title": taskTitle(t), "link": link,
			}, map[string]any{
				"kind": "doc_task_submitted", "task_uuid": t.UUID.String(), "engagement_uuid": engagementUUID,
				"organization_slug": names.OrganizationSlug, "taxpayer_uuid": names.TaxpayerUuid.String(), "status": t.Status,
			})
		}
		return nil
	})
	if err != nil {
		s.log.Warn("doctasks_office_notify_failed", "error", err, "engagement_id", engagementID)
	}
}

// detached keeps the request's values (logging, tracing) but drops its
// database transaction, so rls.System opens its own connection.
func detached(ctx context.Context) context.Context { return noTx{ctx} }

type noTx struct{ context.Context }

func (c noTx) Value(key any) any {
	if v := c.Context.Value(key); v != nil {
		if _, isTx := v.(pgx.Tx); isTx {
			return nil
		}
		return v
	}
	return nil
}

func (s *Service) enqueue(ctx context.Context, template string, organizationID, userID int64, link string, vars map[string]string, payload map[string]any) {
	org, user := organizationID, userID
	if _, err := s.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
		OrganizationID: &org, UserID: &user,
		Channels:     []string{notifmodel.ChannelInapp, notifmodel.ChannelEmail},
		TemplateCode: template, SourceEvent: template, ActionURL: &link,
		TemplateVars: vars, Payload: payload,
	}); err != nil {
		s.log.Warn("doctasks_notify_failed", "error", err, "template", template)
	}
}
