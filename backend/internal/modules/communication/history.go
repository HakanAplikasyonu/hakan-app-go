package communication

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// Channels of the communication history.
const (
	ChannelSMS   = "sms"
	ChannelEmail = "email"
)

// HistoryItem is one message to a taxpayer, as proof: who sent what, on
// which channel, to which number or address, and when it was sent,
// delivered (SMS, NetGSM report) or opened / downloaded (e-mail).
type HistoryItem struct {
	UUID    uuid.UUID `json:"uuid"`
	Channel string    `json:"channel"`
	// To is the phone number (SMS) or the e-mail address.
	To            string     `json:"to"`
	Subject       string     `json:"subject"`
	Body          string     `json:"body"`
	Status        string     `json:"status"`
	MsgHeader     string     `json:"msgheader"`
	ProviderJobID string     `json:"provider_job_id"`
	ErrorCode     string     `json:"error_code"`
	ErrorMessage  string     `json:"error_message"`
	SourceRule    string     `json:"source_rule"`
	SourceRef     string     `json:"source_ref"`
	SentByName    string     `json:"sent_by_name"`
	CreatedAt     time.Time  `json:"created_at"`
	SentAt        *time.Time `json:"sent_at"`
	DeliveredAt   *time.Time `json:"delivered_at"`
	OpenedAt      *time.Time `json:"opened_at"`
	DownloadedAt  *time.Time `json:"downloaded_at"`
}

// History lists the engagement's messages newest first; channel "" means
// all, "sms" or "email" one channel.
func (s *Service) History(ctx context.Context, org, engagementID int64, channel string, limit int32) ([]HistoryItem, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	eng := pgtype.Int8{Int64: engagementID, Valid: true}
	out := []HistoryItem{}
	if channel == "" || channel == ChannelSMS {
		rows, err := s.q.ListEngagementSmsMessages(ctx, db.ListEngagementSmsMessagesParams{OrganizationID: org, EngagementID: eng, Lim: limit})
		if err != nil {
			return nil, fmt.Errorf("list engagement sms: %w", err)
		}
		for _, r := range rows {
			out = append(out, HistoryItem{UUID: r.Uuid, Channel: ChannelSMS, To: r.Phone, Body: r.Body, Status: r.Status,
				MsgHeader: r.Msgheader, ProviderJobID: r.ProviderJobID, ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage,
				SourceRule: r.SourceRule, SourceRef: r.SourceRef, SentByName: r.SentByName, CreatedAt: r.CreatedAt.Time,
				SentAt: tsPtr(r.SentAt), DeliveredAt: tsPtr(r.DeliveredAt)})
		}
	}
	if channel == "" || channel == ChannelEmail {
		rows, err := s.q.ListEngagementMailMessages(ctx, db.ListEngagementMailMessagesParams{OrganizationID: org, EngagementID: eng, Lim: limit})
		if err != nil {
			return nil, fmt.Errorf("list engagement mail: %w", err)
		}
		for _, r := range rows {
			out = append(out, HistoryItem{UUID: r.Uuid, Channel: ChannelEmail, To: r.ToEmail, Subject: r.Subject, Body: r.Body,
				Status: r.Status, ErrorMessage: r.ErrorMessage, SourceRule: r.SourceRule, SourceRef: r.SourceRef,
				SentByName: r.SentByName, CreatedAt: r.CreatedAt.Time, SentAt: tsPtr(r.SentAt),
				OpenedAt: tsPtr(r.OpenedAt), DownloadedAt: tsPtr(r.DownloadedAt)})
		}
	}
	SortHistory(out)
	if len(out) > int(limit) {
		out = out[:limit]
	}
	return out, nil
}

// SortHistory orders newest first (stable for equal times).
func SortHistory(items []HistoryItem) {
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
}
