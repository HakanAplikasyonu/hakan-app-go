package providers

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/unsubscribe"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mail"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mailtpl"
	"github.com/google/uuid"
)

// PayloadLang is the payload key holding the language an email row was
// rendered in (its template's language), so the layout matches it.
const PayloadLang = "email_lang"

// EmailProvider sends via SMTP: the stored text as the text part and, when
// a renderer is set, the themed HTML alternative built from it.
type EmailProvider struct {
	Mail mail.Sender
	// Render builds the HTML part; nil sends text only.
	Render *mailtpl.Renderer
	// Links signs List-Unsubscribe for notification emails.
	Links unsubscribe.Links
}

// NewEmailProvider wires the themed email layout. frontendURL is the public
// web origin (logo, links); appKey derives the unsubscribe signing key.
func NewEmailProvider(m mail.Sender, frontendURL, appKey string) EmailProvider {
	r := mailtpl.New(frontendURL)
	return EmailProvider{Mail: m, Render: &r, Links: unsubscribe.New(appKey, frontendURL)}
}

func (EmailProvider) Channel() string { return model.ChannelEmail }

func (p EmailProvider) Deliver(ctx context.Context, n db.Notification, userUUID *uuid.UUID) (DeliveryResult, error) {
	to := ""
	if n.Recipient.Valid {
		to = n.Recipient.String
	}
	if to == "" {
		return DeliveryResult{}, ErrNoRecipient
	}
	if p.Mail == nil {
		p.Mail = mail.NoopSender{}
	}
	if err := p.Mail.Send(ctx, p.Message(n, userUUID)); err != nil {
		return DeliveryResult{}, err
	}
	return DeliveryResult{Status: model.StatusSent, Provider: "smtp"}, nil
}

// Message builds the outgoing message of an email row (exported for the
// template preview).
//
// A sealed row is a security email (sign-in code, invitation): transactional,
// sent whatever the preferences say, so it gets no unsubscribe link. Any
// other row addressed to a user is a notification the user can turn off.
func (p EmailProvider) Message(n db.Notification, userUUID *uuid.UUID) mail.Message {
	msg := mail.Message{Subject: n.Title, Body: n.Body}
	if n.Recipient.Valid {
		msg.To = []string{n.Recipient.String}
	}
	notification := !n.SealedContent.Valid && n.UserID.Valid && userUUID != nil
	if notification {
		msg.ListUnsubscribe = p.Links.URL(*userUUID)
	}
	if p.Render == nil {
		return msg
	}
	lang := rowLang(n.Payload)
	action := ""
	if n.ActionUrl.Valid {
		action = n.ActionUrl.String
	}
	e := FromText(n.TemplateCode.String, n.Title, n.Body, action, lang)
	if notification && msg.ListUnsubscribe != "" {
		e.PreferencesURL = p.Links.PreferencesURL()
		e.Reason = reasonText(lang)
	}
	out, err := p.Render.RenderSafe(e)
	if err != nil {
		// An unrenderable row still goes out as plain text rather than not
		// at all.
		return msg
	}
	msg.Body, msg.HTML = out.Text, out.HTML
	return msg
}

func rowLang(payload []byte) string {
	var p map[string]any
	if json.Unmarshal(payload, &p) == nil {
		if l, ok := p[PayloadLang].(string); ok && l != "" {
			return l
		}
	}
	return "tr"
}

func reasonText(lang string) string {
	if lang == "en" {
		return "You receive this email because email notifications are on in your account."
	}
	return "Bu e-postayı hesabınızda e-posta bildirimleri açık olduğu için aldınız."
}

// buttonLabels name the call to action of each template's link.
var buttonLabels = map[string][2]string{ // code -> {tr, en}
	"agents.offline":              {"Ajanı görüntüle", "View agent"},
	"doc_tasks.created":           {"Belgeleri yükle", "Upload documents"},
	"doc_tasks.reminder":          {"Belgeleri yükle", "Upload documents"},
	"doc_tasks.requested":         {"Belgeyi yükle", "Upload document"},
	"doc_tasks.submitted":         {"Belgeyi incele", "Review document"},
	"taxpayer.invitation":         {"Daveti kabul et", "Accept invitation"},
	"tenant.invitation":           {"Daveti kabul et", "Accept invitation"},
	"taxpayer.onboarding_consent": {"Talebi görüntüle", "View request"},
	"warnings.blocker":            {"Uyarıları görüntüle", "View warnings"},
}

func buttonLabel(code, lang string) string {
	l, ok := buttonLabels[code]
	if !ok {
		l = [2]string{"Aç", "Open"}
	}
	if lang == "en" {
		return l[1]
	}
	return l[0]
}

var (
	codeLine = regexp.MustCompile(`^(\S.{0,80}):\s+([0-9]{4,8})$`)
	urlLine  = regexp.MustCompile(`^https?://\S+$`)
)

// FromText reads the structure of a stored email text, which templates
// write as (see migration 000116):
//
//	Merhaba Ayşe,                 greeting: a first one-line paragraph ending in ","
//	                              blank line: paragraph break
//	Giriş kodunuz: 482913         a "label: digits" line: the one-time code
//	Daveti kabul etmek için:      a line ending in ":" right before
//	https://…                     a lone URL line: the button
//
// The stored text itself stays the text part (PlainText): it was written
// to be read, so nothing is lost or reflowed.
func FromText(code, subject, body, actionURL, lang string) mailtpl.Email {
	e := mailtpl.Email{Lang: lang, Subject: subject, PlainText: strings.TrimSpace(body)}
	paras := splitParagraphs(body)
	if len(paras) > 1 && len(paras[0]) == 1 && strings.HasSuffix(paras[0][0], ",") {
		e.Greeting = paras[0][0]
		paras = paras[1:]
	}
	linked := false
	for _, lines := range paras {
		var text []string
		flush := func() {
			if len(text) > 0 {
				e.Blocks = append(e.Blocks, mailtpl.Block{Kind: mailtpl.Paragraph, Text: strings.Join(text, "\n")})
				text = nil
			}
		}
		for i, line := range lines {
			switch {
			case codeLine.MatchString(line):
				m := codeLine.FindStringSubmatch(line)
				flush()
				e.Blocks = append(e.Blocks, mailtpl.Block{Kind: mailtpl.Code, Label: m[1], Text: m[2]})
			case urlLine.MatchString(line) && mailtpl.SafeURL(line):
				// The intro line ("… için:") says what the button says.
				if len(text) > 0 && i > 0 && strings.HasSuffix(text[len(text)-1], ":") {
					text = text[:len(text)-1]
				}
				flush()
				e.Blocks = append(e.Blocks, mailtpl.Block{Kind: mailtpl.Button, Label: buttonLabel(code, lang), URL: line})
				linked = true
			default:
				text = append(text, line)
			}
		}
		flush()
	}
	if !linked && mailtpl.SafeURL(actionURL) {
		e.Blocks = append(e.Blocks, mailtpl.Block{Kind: mailtpl.Button, Label: buttonLabel(code, lang), URL: actionURL})
	}
	e.Preheader = preheader(e)
	return e
}

func splitParagraphs(body string) [][]string {
	var out [][]string
	var cur []string
	for _, line := range strings.Split(strings.ReplaceAll(strings.TrimSpace(body), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(cur) > 0 {
				out = append(out, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// preheader is the first sentence of running text (codes and links stay
// out of inbox previews).
func preheader(e mailtpl.Email) string {
	for _, b := range e.Blocks {
		if b.Kind != mailtpl.Paragraph {
			continue
		}
		s := strings.ReplaceAll(b.Text, "\n", " ")
		if r := []rune(s); len(r) > 110 {
			s = strings.TrimSpace(string(r[:110])) + "…"
		}
		return s
	}
	return e.Subject
}
