package doctasks

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/communication"
)

// CommunicationChannels sends document request channels through the
// communication module: the office's NetGSM account (SMS), its mailer
// (e-mail) and the push record. Test mode (dry_run) and number checks are
// the communication module's; every send is a record of the taxpayer's
// communication history.
type CommunicationChannels struct{ Svc *communication.Service }

var _ ChannelSender = CommunicationChannels{}

func commErr(err error) error {
	switch {
	case errors.Is(err, communication.ErrNotConfigured):
		return ErrSMSNotConfigured
	case errors.Is(err, communication.ErrNoContact):
		return ErrSMSNoContact
	}
	return err
}

// SMS implements ChannelSender.
func (c CommunicationChannels) SMS(ctx context.Context, organizationID int64, engagement uuid.UUID, text, sourceRule, sourceRef string, actorID int64) (SMSOutcome, error) {
	m, err := c.Svc.SMSToEngagement(ctx, organizationID, engagement, communication.SMSInput{
		Text: text, SourceRule: sourceRule, SourceRef: sourceRef, SentBy: actorID,
	})
	if err != nil {
		return SMSOutcome{}, commErr(err)
	}
	return SMSOutcome{Status: m.Status, Code: m.ErrorCode, Message: m.ErrorMessage}, nil
}

// Mail implements ChannelSender.
func (c CommunicationChannels) Mail(ctx context.Context, organizationID int64, engagement uuid.UUID, subject, body, sourceRule, sourceRef string, actorID int64) (SMSOutcome, error) {
	m, err := c.Svc.MailToEngagement(ctx, organizationID, engagement, communication.MailInput{
		Subject: subject, Body: body, SourceRule: sourceRule, SourceRef: sourceRef, SentBy: actorID,
	})
	if err != nil {
		return SMSOutcome{}, commErr(err)
	}
	return SMSOutcome{Status: m.Status, Message: m.ErrorMessage}, nil
}

// RecordPush implements ChannelSender.
func (c CommunicationChannels) RecordPush(ctx context.Context, organizationID, engagementID int64, title, body string, recipients int, sourceRule, sourceRef string, actorID int64) error {
	eng := engagementID
	_, err := c.Svc.RecordPush(ctx, communication.PushInput{OrganizationID: organizationID, EngagementID: &eng,
		Title: title, Body: body, Recipients: recipients, SourceRule: sourceRule, SourceRef: sourceRef, SentBy: actorID})
	return err
}

// Latest implements ChannelSender.
func (c CommunicationChannels) Latest(ctx context.Context, organizationID, engagementID int64, rules, refs []string) ([]SourceRecord, error) {
	rows, err := c.Svc.LatestBySource(ctx, organizationID, engagementID, rules, refs)
	if err != nil {
		return nil, err
	}
	out := make([]SourceRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, SourceRecord{SourceRef: r.SourceRef, SourceRule: r.SourceRule, Channel: r.Channel, Status: r.Status,
			Code: r.ErrorCode, Message: r.ErrorMessage, At: r.At, DeliveredAt: r.DeliveredAt})
	}
	return out, nil
}
