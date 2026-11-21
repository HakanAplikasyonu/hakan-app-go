package communication

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/providers"
)

// SMSProvider delivers "sms" notification rows through the office's NetGSM
// account (it replaces the old no-op provider). The row's recipient is the
// phone number and its organization the sending office; the body is the
// text. Each delivery is an sms_messages record (source "notification").
type SMSProvider struct{ Svc *Service }

var _ providers.Provider = SMSProvider{}

// Channel implements providers.Provider.
func (SMSProvider) Channel() string { return model.ChannelSMS }

// Deliver implements providers.Provider.
func (p SMSProvider) Deliver(ctx context.Context, n db.Notification, _ *uuid.UUID) (providers.DeliveryResult, error) {
	if p.Svc == nil {
		return providers.DeliveryResult{}, ErrNotConfigured
	}
	if !n.OrganizationID.Valid {
		return providers.DeliveryResult{}, errors.New("communication: sms notification without an office")
	}
	if !n.Recipient.Valid || strings.TrimSpace(n.Recipient.String) == "" {
		return providers.DeliveryResult{}, providers.ErrNoRecipient
	}
	text := strings.TrimSpace(n.Body)
	if text == "" {
		text = strings.TrimSpace(n.Title)
	}
	rule := "notification"
	if n.TemplateCode.Valid && n.TemplateCode.String != "" {
		rule = "notification:" + n.TemplateCode.String
	}
	m, err := p.Svc.SendSMS(ctx, SMSInput{OrganizationID: n.OrganizationID.Int64, Phone: n.Recipient.String, Text: text,
		SourceRule: rule, SourceRef: n.Uuid.String()})
	if err != nil {
		return providers.DeliveryResult{}, err
	}
	if m.Status == StatusFailed {
		return providers.DeliveryResult{}, fmt.Errorf("communication: sms failed: %s %s", m.ErrorCode, m.ErrorMessage)
	}
	return providers.DeliveryResult{Status: model.StatusSent, Provider: "netgsm", ProviderReference: m.Uuid.String()}, nil
}
