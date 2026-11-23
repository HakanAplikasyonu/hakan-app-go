package communication

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// PushInput records one in-app notification (mobile push included) sent to
// the taxpayer's app users, so it appears in the communication history next
// to SMS and e-mail. The notification itself goes through the notifications
// module; this is the proof of the send.
type PushInput struct {
	OrganizationID int64
	EngagementID   *int64
	Title          string
	Body           string
	// Recipients is how many app users were notified; 0 records a failed
	// send ("no app user").
	Recipients int
	SourceRule string
	SourceRef  string
	SentBy     int64
}

// App message statuses.
const (
	PushNoUserCode    = "no_user"
	PushNoUserMessage = "Mükellefin uygulamayı kullanan kimsesi yok; bildirim gönderilemedi."
)

// RecordPush writes the in-app notification record.
func (s *Service) RecordPush(ctx context.Context, in PushInput) (db.PushMessage, error) {
	in.Title, in.Body = strings.TrimSpace(in.Title), strings.TrimSpace(in.Body)
	if in.Title == "" || in.Body == "" {
		return db.PushMessage{}, ErrEmptyText
	}
	p := db.InsertPushMessageParams{OrganizationID: in.OrganizationID, Title: in.Title, Body: in.Body,
		Recipients: int32(max(in.Recipients, 0)), Status: StatusSent, //nolint:gosec // small count
		SourceRule: in.SourceRule, SourceRef: in.SourceRef, SentBy: actorID(in.SentBy)}
	if in.EngagementID != nil {
		p.EngagementID = pgtype.Int8{Int64: *in.EngagementID, Valid: true}
	}
	if in.Recipients > 0 {
		p.SentAt = pgtype.Timestamptz{Time: s.now(), Valid: true}
	} else {
		p.Status, p.ErrorCode, p.ErrorMessage = StatusFailed, PushNoUserCode, PushNoUserMessage
	}
	m, err := s.q.InsertPushMessage(ctx, p)
	if err != nil {
		return db.PushMessage{}, fmt.Errorf("record app message: %w", err)
	}
	return m, nil
}

// pushRecipients is the "to" column of an in-app record in the history.
func pushRecipients(n int32) string {
	return strconv.Itoa(int(n)) + " uygulama kullanıcısı"
}

// SourceDelivery is the latest message of one channel for a source record
// (e.g. the channel status of a document request).
type SourceDelivery struct {
	SourceRef    string
	SourceRule   string
	Channel      string
	Status       string
	ErrorCode    string
	ErrorMessage string
	At           time.Time
	DeliveredAt  *time.Time
}

// LatestBySource returns, for every source ref, the latest message of each
// channel written by one of the source rules.
func (s *Service) LatestBySource(ctx context.Context, org, engagementID int64, rules, refs []string) ([]SourceDelivery, error) {
	if len(refs) == 0 || len(rules) == 0 {
		return nil, nil
	}
	rows, err := s.q.LatestMessagesBySource(ctx, db.LatestMessagesBySourceParams{OrganizationID: org,
		EngagementID: pgtype.Int8{Int64: engagementID, Valid: true}, SourceRules: rules, SourceRefs: refs})
	if err != nil {
		return nil, fmt.Errorf("latest messages by source: %w", err)
	}
	out := make([]SourceDelivery, 0, len(rows))
	for _, r := range rows {
		out = append(out, SourceDelivery{SourceRef: r.SourceRef, SourceRule: r.SourceRule, Channel: r.Channel, Status: r.Status,
			ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage, At: r.CreatedAt.Time, DeliveredAt: tsPtr(r.DeliveredAt)})
	}
	return out, nil
}
