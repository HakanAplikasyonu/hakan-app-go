package communication

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

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
)

// Elle toplu gönderim (#780 B): hedef (seçili mükellefler, grup, dinamik
// süzgeç ya da son gönderimlerden biri) çözülür, BuildPlan ile kime ne
// gideceği çıkarılır, onaydan sonra gönderim arka planda tek tek yapılır.
// Her mesaj SendSMS / SendMail ile bulk_id taşıyarak kaydedilir; deneme
// modu aynen geçerlidir. İlerleme bulk_sends satırındadır, ekran onu okur.

// Target kinds.
const (
	TargetSelection = "selection"
	TargetGroup     = "group"
	TargetFilter    = "filter"
	TargetRecent    = "recent"
)

// Bulk statuses. "interrupted" is only a view: a running send whose
// progress has not moved for StallAfter (the server restarted mid-send).
const (
	BulkRunning     = "running"
	BulkDone        = "done"
	BulkFailed      = "failed"
	BulkInterrupted = "interrupted"
)

// StallAfter: a running send that has not moved this long is interrupted.
const StallAfter = 10 * time.Minute

// RecentBulkSends is the size of the "Son gönderilenler" set (per user).
const RecentBulkSends = 5

// Target says whom a bulk send goes to.
type Target struct {
	Kind            string      `json:"kind"`
	EngagementUUIDs []uuid.UUID `json:"engagement_uuids,omitempty"`
	GroupUUID       *uuid.UUID  `json:"group_uuid,omitempty"`
	TaxKind         string      `json:"tax_kind,omitempty"`
	AssigneeUUID    *uuid.UUID  `json:"assignee_uuid,omitempty"`
	BulkUUID        *uuid.UUID  `json:"bulk_uuid,omitempty"`
	// Label names a filter target on the screen ("KDV · Ayşe Yılmaz").
	Label string `json:"label,omitempty"`
}

// BulkInput is the body of preview and send.
type BulkInput struct {
	Channel string `json:"channel"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Target  Target `json:"target"`
}

// Sample is the first personalized message.
type Sample struct {
	Title   string `json:"title"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	Chars   int    `json:"chars"`
	Parts   int    `json:"parts"`
}

// BulkPreview is what the confirmation dialog shows.
type BulkPreview struct {
	Channel       string      `json:"channel"`
	TargetLabel   string      `json:"target_label"`
	Targets       int         `json:"targets"`
	Recipients    int         `json:"recipients"`
	Duplicates    []Duplicate `json:"duplicates"`
	Skipped       []Skip      `json:"skipped"`
	TooLong       []TooLong   `json:"too_long"`
	MaxParts      int         `json:"max_parts"`
	PartLimit     int         `json:"part_limit"`
	MaxRecipients int         `json:"max_recipients"`
	TestMode      bool        `json:"test_mode"`
	Blocked       string      `json:"blocked"`
	Sample        *Sample     `json:"sample"`
}

// BulkFailure is one message that did not go.
type BulkFailure struct {
	Title        string `json:"title"`
	Recipient    string `json:"recipient"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message"`
}

// BulkView is a bulk send with its progress.
type BulkView struct {
	UUID         uuid.UUID     `json:"uuid"`
	Channel      string        `json:"channel"`
	Subject      string        `json:"subject"`
	Body         string        `json:"body"`
	TargetKind   string        `json:"target_kind"`
	TargetLabel  string        `json:"target_label"`
	Status       string        `json:"status"`
	Total        int           `json:"total"`
	Processed    int           `json:"processed"`
	Sent         int           `json:"sent"`
	Failed       int           `json:"failed"`
	Duplicates   int           `json:"duplicates"`
	Skipped      []Skip        `json:"skipped"`
	Failures     []BulkFailure `json:"failures"`
	ErrorMessage string        `json:"error_message"`
	CreatedAt    time.Time     `json:"created_at"`
	FinishedAt   *time.Time    `json:"finished_at"`
}

func (s *Service) bulkView(b db.BulkSend) BulkView {
	v := BulkView{UUID: b.Uuid, Channel: b.Channel, Subject: b.Subject, Body: b.Body, TargetKind: b.TargetKind,
		TargetLabel: b.TargetLabel, Status: b.Status, Total: int(b.Total), Processed: int(b.Processed), Sent: int(b.Sent),
		Failed: int(b.Failed), Duplicates: int(b.Duplicates), Skipped: []Skip{}, Failures: []BulkFailure{},
		ErrorMessage: b.ErrorMessage, CreatedAt: b.CreatedAt.Time, FinishedAt: tsPtr(b.FinishedAt)}
	_ = json.Unmarshal(b.Skipped, &v.Skipped)
	if v.Skipped == nil {
		v.Skipped = []Skip{}
	}
	if b.Status == BulkRunning && b.UpdatedAt.Valid && s.now().Sub(b.UpdatedAt.Time) > StallAfter {
		v.Status = BulkInterrupted
		v.ErrorMessage = "Gönderim yarıda kaldı (sunucu yeniden başladı). Kalanlar için yeni bir toplu gönderim başlatın."
	}
	return v
}

// WithLinkBase sets the web address :link points to (the taxpayer portal).
func (s *Service) WithLinkBase(base string) *Service {
	s.linkBase = strings.TrimRight(strings.TrimSpace(base), "/")
	return s
}

// WithLogger sets the logger of background sends.
func (s *Service) WithLogger(l *slog.Logger) *Service { s.log = l; return s }

func (s *Service) vars(ctx context.Context, org int64) (Vars, error) {
	name, err := s.q.GetOrganizationNameForMessage(ctx, org)
	if err != nil {
		return Vars{}, fmt.Errorf("office name: %w", err)
	}
	v := Vars{Office: name, Period: DeclarationPeriod(s.now())}
	if s.linkBase != "" {
		v.Link = s.linkBase + "/m"
	}
	return v, nil
}

// resolve turns a target into engagement ids and a screen label.
func (s *Service) resolve(ctx context.Context, org, actor int64, t Target) ([]int64, string, error) {
	switch t.Kind {
	case TargetSelection:
		if len(t.EngagementUUIDs) == 0 {
			return nil, "", fmt.Errorf("%w: en az bir mükellef seçin", ErrInvalidInput)
		}
		if len(t.EngagementUUIDs) > MaxBulkRecipients {
			return nil, "", ErrTooManyRecipients
		}
		ids, err := s.q.EngagementIDsByUUIDs(ctx, db.EngagementIDsByUUIDsParams{OrganizationID: org, EngagementUuids: t.EngagementUUIDs})
		return ids, fmt.Sprintf("Seçili %d mükellef", len(t.EngagementUUIDs)), err
	case TargetGroup:
		if t.GroupUUID == nil {
			return nil, "", fmt.Errorf("%w: grup seçin", ErrInvalidInput)
		}
		g, err := s.q.GetTaxpayerGroup(ctx, db.GetTaxpayerGroupParams{OrganizationID: org, Uuid: *t.GroupUUID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrNotFound
		} else if err != nil {
			return nil, "", err
		}
		ids, err := s.q.GroupEngagementIDs(ctx, db.GroupEngagementIDsParams{OrganizationID: org, GroupID: g.ID})
		return ids, "Grup: " + g.Name, err
	case TargetFilter:
		if t.TaxKind != "" && !validTaxKind(t.TaxKind) {
			return nil, "", fmt.Errorf("%w: bilinmeyen beyan türü", ErrInvalidInput)
		}
		p := db.EngagementIDsByFilterParams{OrganizationID: org, TaxKind: t.TaxKind, SincePeriod: s.sincePeriod()}
		if t.AssigneeUUID != nil {
			p.Assignee = pgtype.UUID{Bytes: *t.AssigneeUUID, Valid: true}
		}
		ids, err := s.q.EngagementIDsByFilter(ctx, p)
		label := strings.TrimSpace(t.Label)
		if len([]rune(label)) > 120 {
			label = string([]rune(label)[:120])
		}
		if label == "" {
			label = "Süzgeç"
		}
		return ids, label, err
	case TargetRecent:
		if t.BulkUUID == nil {
			return nil, "", fmt.Errorf("%w: önceki gönderimi seçin", ErrInvalidInput)
		}
		b, err := s.q.GetBulkSend(ctx, db.GetBulkSendParams{OrganizationID: org, Uuid: *t.BulkUUID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!b.CreatedBy.Valid || b.CreatedBy.Int64 != actor)) {
			return nil, "", ErrNotFound
		} else if err != nil {
			return nil, "", err
		}
		return b.EngagementIds, "Tekrar: " + b.TargetLabel, nil
	}
	return nil, "", fmt.Errorf("%w: hedef türü geçersiz", ErrInvalidInput)
}

type prepared struct {
	plan     Plan
	ids      []int64
	label    string
	testMode bool
}

func (s *Service) prepare(ctx context.Context, org, actor int64, in BulkInput) (prepared, error) {
	in.Channel = strings.TrimSpace(in.Channel)
	if in.Channel != ChannelSMS && in.Channel != ChannelMail {
		return prepared{}, fmt.Errorf("%w: kanal SMS ya da e-posta olmalı", ErrInvalidInput)
	}
	row, err := s.load(ctx, org)
	if err != nil {
		return prepared{}, err
	}
	encoding := "TR"
	testMode := row == nil || row.TestMode
	if in.Channel == ChannelSMS {
		if row == nil || !row.Enabled {
			return prepared{}, ErrNotConfigured
		}
		encoding = row.Encoding
		testMode = testMode || !s.live
	} else {
		testMode = testMode || s.mailer == nil
	}
	ids, label, err := s.resolve(ctx, org, actor, in.Target)
	if err != nil {
		return prepared{}, err
	}
	if len(ids) > MaxBulkRecipients {
		return prepared{}, ErrTooManyRecipients
	}
	contacts := []Contact{}
	if len(ids) > 0 {
		rows, err := s.q.ListMessageContacts(ctx, db.ListMessageContactsParams{OrganizationID: org, EngagementIds: ids})
		if err != nil {
			return prepared{}, fmt.Errorf("bulk contacts: %w", err)
		}
		for _, r := range rows {
			contacts = append(contacts, Contact{EngagementID: r.EngagementID, EngagementUUID: r.EngagementUuid, Title: r.Title,
				Phone: r.ContactPhone, Email: r.ContactEmail})
		}
	}
	v, err := s.vars(ctx, org)
	if err != nil {
		return prepared{}, err
	}
	plan, err := BuildPlan(in.Channel, encoding, in.Subject, in.Body, contacts, v)
	if err != nil {
		return prepared{}, err
	}
	activeIDs := make([]int64, 0, len(contacts))
	for _, c := range contacts {
		activeIDs = append(activeIDs, c.EngagementID)
	}
	return prepared{plan: plan, ids: activeIDs, label: label, testMode: testMode}, nil
}

// Preview shows what a bulk send would do; nothing is sent or recorded.
func (s *Service) Preview(ctx context.Context, org, actor int64, in BulkInput) (BulkPreview, error) {
	p, err := s.prepare(ctx, org, actor, in)
	if err != nil {
		return BulkPreview{}, err
	}
	out := BulkPreview{Channel: p.plan.Channel, TargetLabel: p.label, Targets: p.plan.Targets, Recipients: len(p.plan.Items),
		Duplicates: p.plan.Duplicates, Skipped: p.plan.Skipped, TooLong: p.plan.TooLong, MaxParts: p.plan.MaxParts,
		PartLimit: MaxBulkSMSParts, MaxRecipients: MaxBulkRecipients, TestMode: p.testMode, Blocked: p.plan.Blocked()}
	if len(p.plan.Items) > 0 {
		it := p.plan.Items[0]
		out.Sample = &Sample{Title: it.Title, To: it.To, Subject: it.Subject, Text: it.Text, Chars: it.Length.Chars, Parts: it.Length.Parts}
	}
	return out, nil
}

// StartBulk records a bulk send and sends it in the background. A plan
// with a blocker is refused (ErrBulkBlocked) and nothing is recorded.
func (s *Service) StartBulk(ctx context.Context, org, actor int64, in BulkInput) (BulkView, error) {
	p, err := s.prepare(ctx, org, actor, in)
	if err != nil {
		return BulkView{}, err
	}
	if msg := p.plan.Blocked(); msg != "" {
		if len(p.plan.Items) == 0 && len(p.plan.Campaign) == 0 {
			return BulkView{}, ErrNoRecipients
		}
		return BulkView{}, fmt.Errorf("%w: %s", ErrBulkBlocked, msg)
	}
	skipped, err := json.Marshal(p.plan.Skipped)
	if err != nil {
		return BulkView{}, err
	}
	subject := ""
	if in.Channel == ChannelMail {
		subject = strings.TrimSpace(in.Subject)
	}
	b, err := s.q.InsertBulkSend(ctx, db.InsertBulkSendParams{OrganizationID: org, Channel: in.Channel, Subject: subject,
		Body: strings.TrimSpace(in.Body), TargetKind: in.Target.Kind, TargetLabel: p.label, EngagementIds: p.ids,
		Total: int32(len(p.plan.Items)), Duplicates: int32(len(p.plan.Duplicates)), Skipped: skipped, CreatedBy: actorID(actor)}) //nolint:gosec // at most 500
	if err != nil {
		return BulkView{}, fmt.Errorf("record bulk send: %w", err)
	}
	items := p.plan.Items
	start := func(context.Context) {
		s.bulkWG.Add(1)
		go func() { //nolint:gosec // G118: the send outlives the request on purpose; the request ctx carries a finished tx
			defer s.bulkWG.Done()
			// The request's transaction (if any) has ended; the send runs on
			// the pool, outside the request.
			s.runBulk(context.Background(), b, items, actor)
		}()
	}
	if !dbtx.AfterCommit(ctx, start) {
		start(ctx)
	}
	return s.bulkView(b), nil
}

// WaitBulk waits for the background sends started so far (tests).
func (s *Service) WaitBulk() { s.bulkWG.Wait() }

func (s *Service) logger() *slog.Logger {
	if s.log != nil {
		return s.log
	}
	return slog.Default()
}

// runBulk sends the items one by one and records the progress.
func (s *Service) runBulk(ctx context.Context, b db.BulkSend, items []PlanItem, actor int64) {
	org := b.OrganizationID
	finish := func(status, msg string) {
		if err := s.q.FinishBulkSend(ctx, db.FinishBulkSendParams{Status: status, ErrorMessage: msg, OrganizationID: org, ID: b.ID}); err != nil {
			s.logger().Error("bulk_send_finish_failed", "bulk", b.Uuid, "error", err)
		}
	}
	defer func() {
		if r := recover(); r != nil {
			s.logger().Error("bulk_send_panic", "bulk", b.Uuid, "panic", r)
			finish(BulkFailed, "Gönderim beklenmeyen bir hatayla durdu.")
		}
	}()
	bulkID := b.Uuid
	for _, it := range items {
		engagement := it.EngagementID
		var status string
		var err error
		if b.Channel == ChannelSMS {
			var m db.SmsMessage
			m, err = s.SendSMS(ctx, SMSInput{OrganizationID: org, EngagementID: &engagement, Phone: it.To, Text: it.Text,
				SourceRule: "bulk", SourceRef: bulkID.String(), SentBy: actor, BulkID: &bulkID})
			status = m.Status
		} else {
			var m db.MailMessage
			m, err = s.SendMail(ctx, MailInput{OrganizationID: org, EngagementID: &engagement, To: it.To, Subject: it.Subject,
				Body: it.Text, SourceRule: "bulk", SourceRef: bulkID.String(), SentBy: actor, BulkID: &bulkID})
			status = m.Status
		}
		if errors.Is(err, ErrNotConfigured) {
			finish(BulkFailed, UserMessage(err))
			return
		}
		step := db.StepBulkSendParams{OrganizationID: org, ID: b.ID}
		switch {
		case err != nil:
			msg := UserMessage(err)
			if msg == "" {
				msg = "Gönderim sırasında hata oluştu."
				s.logger().Error("bulk_send_item_failed", "bulk", b.Uuid, "error", err)
			}
			step.FailedDelta = 1
			step.Failure, _ = json.Marshal(BulkFailure{Title: it.Title, Recipient: it.To, Status: StatusFailed, ErrorMessage: msg})
		case status == StatusSent || status == StatusDryRun:
			step.SentDelta = 1
		default:
			// failed / uncertain: the record carries the reason.
			step.FailedDelta = 1
		}
		if err := s.q.StepBulkSend(ctx, step); err != nil {
			s.logger().Error("bulk_send_step_failed", "bulk", b.Uuid, "error", err)
		}
	}
	finish(BulkDone, "")
}

// GetBulk returns one bulk send with its failures.
func (s *Service) GetBulk(ctx context.Context, org int64, id uuid.UUID) (BulkView, error) {
	b, err := s.q.GetBulkSend(ctx, db.GetBulkSendParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return BulkView{}, ErrNotFound
	} else if err != nil {
		return BulkView{}, fmt.Errorf("get bulk send: %w", err)
	}
	v := s.bulkView(b)
	rows, err := s.q.ListBulkSendFailures(ctx, db.ListBulkSendFailuresParams{OrganizationID: org, BulkID: pgtype.UUID{Bytes: b.Uuid, Valid: true}})
	if err != nil {
		return BulkView{}, fmt.Errorf("bulk failures: %w", err)
	}
	for _, r := range rows {
		v.Failures = append(v.Failures, BulkFailure{Title: r.Title, Recipient: r.Recipient, Status: r.Status, ErrorMessage: r.ErrorMessage})
	}
	var extra []BulkFailure
	if json.Unmarshal(b.Failures, &extra) == nil {
		v.Failures = append(v.Failures, extra...)
	}
	return v, nil
}

// RecentBulk lists the user's last bulk sends ("Son gönderilenler").
func (s *Service) RecentBulk(ctx context.Context, org, actor int64) ([]BulkView, error) {
	rows, err := s.q.ListRecentBulkSends(ctx, db.ListRecentBulkSendsParams{OrganizationID: org, CreatedBy: actorID(actor), Lim: RecentBulkSends})
	if err != nil {
		return nil, fmt.Errorf("recent bulk sends: %w", err)
	}
	out := make([]BulkView, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.bulkView(r))
	}
	return out, nil
}
