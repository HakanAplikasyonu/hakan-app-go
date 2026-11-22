package httpserver

import (
	"context"
	"errors"

	"github.com/google/uuid"

	communicationmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/communication"
	doctasksmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/doctasks"
)

// docTaskSMS lets the document task module send the SMS of a missing
// document request (#780, ay başı belge toplama) through the office's
// NetGSM account; each message is an sms_messages record, so it appears in
// the taxpayer's communication history. Test mode is the communication
// module's: the record is written, nothing is sent.
type docTaskSMS struct{ svc *communicationmodule.Service }

var _ doctasksmodule.SMSSender = docTaskSMS{}

// SMSStatus implements doctasksmodule.SMSSender.
func (a docTaskSMS) SMSStatus(ctx context.Context, organizationID int64) (bool, bool, error) {
	st, err := a.svc.Get(ctx, organizationID)
	if err != nil {
		return false, false, err
	}
	return st.Enabled, st.TestMode || !st.LiveAllowed, nil
}

// SendToEngagement implements doctasksmodule.SMSSender.
func (a docTaskSMS) SendToEngagement(ctx context.Context, organizationID int64, engagement uuid.UUID, text, sourceRule, sourceRef string, actorID int64) (doctasksmodule.SMSOutcome, error) {
	m, err := a.svc.SMSToEngagement(ctx, organizationID, engagement, communicationmodule.SMSInput{
		Text: text, SourceRule: sourceRule, SourceRef: sourceRef, SentBy: actorID,
	})
	switch {
	case errors.Is(err, communicationmodule.ErrNotConfigured):
		return doctasksmodule.SMSOutcome{}, doctasksmodule.ErrSMSNotConfigured
	case errors.Is(err, communicationmodule.ErrNoContact):
		return doctasksmodule.SMSOutcome{}, doctasksmodule.ErrSMSNoContact
	case err != nil:
		return doctasksmodule.SMSOutcome{}, err
	}
	return doctasksmodule.SMSOutcome{Status: m.Status, Code: m.ErrorCode, Message: m.ErrorMessage}, nil
}
