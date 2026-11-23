package taxpayers

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
)

// failingNotifier is a mailer that cannot queue anything.
type failingNotifier struct{}

func (failingNotifier) Enqueue(context.Context, notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	return nil, errors.New("smtp: connection refused")
}

// syncBuffer is a log sink safe for the handler's writes.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestInviteReturnsInviteURL: the response carries the same link the email
// does, and email_sent is true when the email was queued.
func TestInviteReturnsInviteURL(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	org := e.d.Organization(t)
	tp := e.d.Taxpayer(t, KindLegal, "1234567890")
	eng := e.d.Engagement(t, org, tp)

	out, err := e.members.Invite(ctx, InviteInput{
		OrgID: org, OrgName: "Ofis", EngagementID: eng, TaxpayerID: tp, Email: "sahip@example.test",
	})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	e.mail.mu.Lock()
	link := e.mail.sent[len(e.mail.sent)-1].TemplateVars["link"]
	e.mail.mu.Unlock()
	if out.InviteURL == "" || out.InviteURL != link {
		t.Fatalf("invite_url = %q, email link %q", out.InviteURL, link)
	}
	if !strings.HasPrefix(out.InviteURL, "https://app.test/invite/m/") {
		t.Fatalf("invite_url = %q", out.InviteURL)
	}
	if !out.EmailSent || out.EmailError != nil {
		t.Fatalf("email_sent = %v, email_error = %v", out.EmailSent, out.EmailError)
	}
	token := out.InviteURL[strings.LastIndex(out.InviteURL, "/")+1:]
	if _, err := e.members.Preview(ctx, token); err != nil {
		t.Fatalf("preview of invite_url: %v", err)
	}
}

// TestInviteMailFailureKeepsInvitation: a mailer error does not fail the
// invitation; the response says email_sent=false with a reason, the link
// works and the error is logged without the token.
func TestInviteMailFailureKeepsInvitation(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	var logs syncBuffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	m := NewMembers(NewMemberPostgres(e.d.Pool), failingNotifier{}, InvitationConfig{FrontendURL: "https://app.test"}, log)
	org := e.d.Organization(t)
	tp := e.d.Taxpayer(t, KindLegal, "1234567890")
	eng := e.d.Engagement(t, org, tp)

	out, err := m.Invite(ctx, InviteInput{
		OrgID: org, OrgName: "Ofis", EngagementID: eng, TaxpayerID: tp, Email: "sahip@example.test",
	})
	if err != nil {
		t.Fatalf("invite must not fail on mail error: %v", err)
	}
	if out.EmailSent || out.EmailError == nil || *out.EmailError != EmailErrorEnqueueFailed {
		t.Fatalf("email_sent = %v, email_error = %v", out.EmailSent, out.EmailError)
	}
	if out.InviteURL == "" {
		t.Fatal("invite_url missing on mail failure")
	}
	list, err := m.ListInvitations(ctx, org, eng)
	if err != nil || len(list) != 1 || list[0].Status != InvitationPending {
		t.Fatalf("invitation not kept: %+v, %v", list, err)
	}
	token := out.InviteURL[strings.LastIndex(out.InviteURL, "/")+1:]
	if _, err := m.Preview(ctx, token); err != nil {
		t.Fatalf("link must work after mail failure: %v", err)
	}
	got := logs.String()
	if !strings.Contains(got, "taxpayer_invitation_email_enqueue_failed") || !strings.Contains(got, "connection refused") {
		t.Fatalf("mail error not logged: %s", got)
	}
	if !strings.Contains(got, "example.test") || strings.Contains(got, "sahip@") {
		t.Fatalf("log must name the domain, not the address: %s", got)
	}
	if strings.Contains(got, token) {
		t.Fatal("log must not contain the invitation token")
	}
}

// TestInviteMailNotConfigured: production without SMTP queues the email but
// tells the office it will not arrive.
func TestInviteMailNotConfigured(t *testing.T) {
	e := newMemberEnv(t)
	m := NewMembers(NewMemberPostgres(e.d.Pool), e.mail,
		InvitationConfig{FrontendURL: "https://app.test", MailUnavailable: EmailErrorNotConfigured}, nil)
	org := e.d.Organization(t)
	tp := e.d.Taxpayer(t, KindLegal, "1234567890")
	eng := e.d.Engagement(t, org, tp)
	out, err := m.Invite(context.Background(), InviteInput{
		OrgID: org, OrgName: "Ofis", EngagementID: eng, TaxpayerID: tp, Email: "sahip@example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.EmailSent || out.EmailError == nil || *out.EmailError != EmailErrorNotConfigured || out.InviteURL == "" {
		t.Fatalf("result = %+v", out)
	}
}

// TestResendInvitationRotatesLink: resend returns a new invite_url that
// works; the old link no longer does.
func TestResendInvitationRotatesLink(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	org := e.d.Organization(t)
	tp := e.d.Taxpayer(t, KindLegal, "1234567890")
	eng := e.d.Engagement(t, org, tp)
	first, err := e.members.Invite(ctx, InviteInput{
		OrgID: org, OrgName: "Ofis", EngagementID: eng, TaxpayerID: tp, Email: "sahip@example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	again, err := e.members.Resend(ctx, ResendInput{OrgID: org, OrgName: "Ofis", EngagementID: eng, UUID: first.UUID})
	if err != nil {
		t.Fatalf("resend: %v", err)
	}
	if again.InviteURL == "" || again.InviteURL == first.InviteURL || !again.EmailSent {
		t.Fatalf("resend result = %+v", again)
	}
	if got := e.mail.lastToken(t); !strings.HasSuffix(again.InviteURL, "/"+got) {
		t.Fatalf("resent email link differs from invite_url")
	}
	oldToken := first.InviteURL[strings.LastIndex(first.InviteURL, "/")+1:]
	if _, err := e.members.Preview(ctx, oldToken); !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("old link after resend: %v", err)
	}
	if err := e.members.RevokeInvitation(ctx, org, eng, first.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.members.Resend(ctx, ResendInput{OrgID: org, EngagementID: eng, UUID: first.UUID}); !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("resend of revoked invitation: %v", err)
	}
}
