// Package mailpreview renders every outgoing email with sample data, for
// eyeballing the templates (cmd/mailpreview, `make mail-preview`) and for the
// end-to-end test that every email still composes, parses and fits.
//
// It never talks to a real mail server: Write puts files on disk, and Send
// only accepts a local SMTP catcher (MailHog).
package mailpreview

import (
	"context"
	"fmt"
	"html/template"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/providers"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/templates"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/reconciliation"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/reports"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mail"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mailtpl"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Recipient is where previews are addressed (a reserved test domain).
const Recipient = "onizleme@example.test"

// Template is one stored notification email template.
type Template struct {
	Code, Lang, Subject, Body string
}

// Sample is one rendered email.
type Sample struct {
	// Name is the file stem, e.g. "auth.login_code.tr" or "mutabakat-mektubu".
	Name string
	// Kind: "işlemsel" (transactional) or "bildirim" (notification).
	Kind    string
	Message mail.Message
	// Raw is the composed RFC 5322 message; Tree its MIME structure.
	Raw  []byte
	Tree string
}

// securityTemplates are sent as security (sealed, transactional) emails by
// their callers; the rest are notifications with List-Unsubscribe.
var securityTemplates = map[string]bool{
	"auth.login_code": true, "auth.oauth_link_code": true, "auth.account_deactivation_code": true,
	"auth.email_verification": true, "auth.password_reset": true, "auth.account_deactivated": true,
	"taxpayer.invitation": true, "tenant.invitation": true, "taxpayer.onboarding_consent": true,
}

// Vars fills every variable the stored templates use.
func Vars(frontendURL string) map[string]string {
	return map[string]string{
		"name": "Ayşe Yılmaz", "code": "482913", "expires_minutes": "10", "expires_days": "7",
		"provider": "Google", "org_name": "Örnek Müşavirlik", "taxpayer_name": "Örnek Gıda A.Ş.",
		"inviter_name": "Mehmet Kaya", "role_name": "çalışan", "title": "Ekim 2026 kira faturası",
		"tax_id": "1234567890", "count": "3", "agent_name": "Muhasebe-PC", "minutes": "45",
		"link": strings.TrimRight(frontendURL, "/") + "/invite/ornek-baglanti",
	}
}

// Build renders the stored templates plus the reconciliation letter and the
// taxpayer report (with their PDFs).
func Build(tpls []Template, frontendURL string, now time.Time) ([]Sample, error) {
	p := providers.NewEmailProvider(mail.NoopSender{}, frontendURL, "mail-preview")
	user := uuid.MustParse("00000000-0000-4000-8000-000000000467")
	vars := Vars(frontendURL)
	var out []Sample
	for _, t := range tpls {
		row := db.Notification{
			Title:        templates.Render(t.Subject, vars),
			Body:         templates.Render(t.Body, vars),
			Recipient:    pgtype.Text{String: Recipient, Valid: true},
			TemplateCode: pgtype.Text{String: t.Code, Valid: true},
			Payload:      []byte(`{"` + providers.PayloadLang + `":"` + t.Lang + `"}`),
		}
		kind := "bildirim"
		if securityTemplates[t.Code] {
			row.SealedContent = pgtype.Text{String: "sealed", Valid: true}
			kind = "işlemsel"
		} else {
			row.UserID = pgtype.Int8{Int64: 1, Valid: true}
		}
		out = append(out, Sample{Name: t.Code + "." + t.Lang, Kind: kind, Message: p.Message(row, &user)})
	}

	rnd := mailtpl.New(frontendURL)
	letter := reconciliation.LetterSnapshot{
		Office: "Örnek Müşavirlik", TaxpayerTitle: "Örnek Gıda A.Ş.", TaxpayerTaxID: "1234567890",
		CounterpartyName: "Deniz Ambalaj Ltd. Şti.", CounterpartyTaxID: "9876543210", AccountCode: "120.01.002",
		AsOfDate: "2026-09-30", Balance: "48250.75", Currency: "TRY", Period: "2026-09",
		Note: "Ekstreniz bizdeki kayıtlarla karşılaştırılmak üzere ektedir.",
	}
	link := strings.TrimRight(frontendURL, "/") + "/mutabakat/ornek-anahtar"
	msg, err := reconciliation.LetterEmail(letter, link, rnd)
	if err != nil {
		return nil, fmt.Errorf("letter: %w", err)
	}
	pdf, err := reconciliation.RenderLetterPDF(letter, link)
	if err != nil {
		return nil, fmt.Errorf("letter pdf: %w", err)
	}
	msg.To = []string{Recipient}
	msg.Attachments = []mail.Attachment{{Name: reconciliation.LetterFileName(letter), ContentType: "application/pdf", Data: pdf}}
	out = append(out, Sample{Name: "mutabakat-mektubu", Kind: "işlemsel", Message: msg})

	rep := sampleReport(now)
	msg, err = reports.Email(rep, "Örnek Müşavirlik", rnd)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	pdf, err = reports.RenderPDF(rep, reports.PDFOptions{OfficeName: "Örnek Müşavirlik", Now: now})
	if err != nil {
		return nil, fmt.Errorf("report pdf: %w", err)
	}
	msg.To = []string{Recipient}
	msg.Attachments = []mail.Attachment{{Name: reports.FileName(rep), ContentType: "application/pdf", Data: pdf}}
	out = append(out, Sample{Name: "mukellef-raporu", Kind: "işlemsel", Message: msg})

	opts := mail.ComposeOptions{From: "noreply@app.example.com", FromName: "HAKAN", Now: now}
	for i := range out {
		s := &out[i]
		if s.Message.HTML == "" {
			return nil, fmt.Errorf("%s: no HTML part", s.Name)
		}
		if len(s.Message.HTML) > mailtpl.MaxHTMLBytes {
			return nil, fmt.Errorf("%s: html %d bytes over the clipping limit", s.Name, len(s.Message.HTML))
		}
		if s.Raw, err = mail.Compose(s.Message, opts); err != nil {
			return nil, fmt.Errorf("%s: %w", s.Name, err)
		}
		in, err := mail.Inspect(s.Raw)
		if err != nil {
			return nil, fmt.Errorf("%s: read back: %w", s.Name, err)
		}
		s.Tree = in.Tree
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func sampleReport(now time.Time) reports.Report {
	r := reports.Report{Kind: reports.KindMonth, Period: "2026-10", Label: "Ekim 2026", Currency: "TRY",
		Taxpayer: reports.Taxpayer{Title: "Örnek Gıda A.Ş.", TaxID: "1234567890"},
		Totals: reports.Totals{Revenue: "95000.00", Expenses: "29500.00", Net: "65500.00", VATCalculated: "19000.00",
			VATDeductible: "5800.00", VATPayable: "13200.00"},
		Categories: []reports.Category{
			{Key: "purchase_invoices", Label: "Alış faturaları", Amount: "23500.00", Records: 6, Share: "79.7"},
			{Key: "fuel_receipt", Label: "Akaryakıt", Amount: "6000.00", Records: 4, Share: "20.3"},
		},
		Commentary:  &reports.Commentary{Status: "approved", Body: "Satışlarınız önceki aya göre arttı. Giderler olağan seviyede."},
		Notes:       []string{"Tutarlar KDV hariç ve TL'dir."},
		GeneratedAt: now,
	}
	r.Months = make([]reports.Month, 0, 12)
	for i, p := range reports.Months("2025-11", "2026-10") {
		rev, exp := 60000+i*3000, 25000+i*400
		r.Months = append(r.Months, reports.Month{Period: p, Label: reports.MonthShort(p),
			Revenue: fmt.Sprintf("%d.00", rev), Expenses: fmt.Sprintf("%d.00", exp), Net: fmt.Sprintf("%d.00", rev-exp)})
	}
	return r
}

// Write puts <name>.html, <name>.txt and <name>.eml for every sample and an
// index.html linking them into dir.
func Write(dir string, samples []Sample) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, s := range samples {
		files := map[string][]byte{".html": []byte(s.Message.HTML), ".txt": []byte(s.Message.Body), ".eml": s.Raw}
		for ext, b := range files {
			if err := os.WriteFile(filepath.Join(dir, s.Name+ext), b, 0o644); err != nil {
				return err
			}
		}
	}
	f, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return indexTpl.Execute(f, samples)
}

var indexTpl = template.Must(template.New("index").Funcs(template.FuncMap{
	"kb": func(s string) string { return fmt.Sprintf("%.1f KB", float64(len(s))/1024) },
}).Parse(`<!DOCTYPE html>
<html lang="tr"><head><meta charset="utf-8"><title>E-posta ön izleme</title>
<style>body{font-family:system-ui,sans-serif;margin:24px}td,th{padding:6px 10px;border-bottom:1px solid #ddd;text-align:left}code{font-size:12px}</style>
</head><body>
<h1>E-posta ön izleme</h1>
<p>Örnek verilerle üretildi. HTML sınırı: Gmail kırpması (~102 KB) altında.</p>
<table>
<tr><th>E-posta</th><th>Tür</th><th>Konu</th><th>HTML</th><th>MIME</th><th>Dosyalar</th></tr>
{{range .}}<tr><td>{{.Name}}</td><td>{{.Kind}}</td><td>{{.Message.Subject}}</td><td>{{kb .Message.HTML}}</td><td><code>{{.Tree}}</code></td>
<td><a href="{{.Name}}.html">HTML</a> · <a href="{{.Name}}.txt">metin</a> · <a href="{{.Name}}.eml">.eml</a></td></tr>
{{end}}</table>
</body></html>
`))

// LocalSMTP reports whether addr (host:port) is a local mail catcher. Send
// refuses anything else: previews must never reach a real mailbox.
func LocalSMTP(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1", "mailhog":
		return true
	}
	return false
}

// Sender delivers one message (mail.SMTPSender in the command).
type Sender interface {
	Send(ctx context.Context, msg mail.Message) error
}

// Send delivers every sample through s, which must point at addr, a local
// catcher.
func Send(ctx context.Context, addr string, s Sender, samples []Sample) error {
	if !LocalSMTP(addr) {
		return fmt.Errorf("mailpreview: %s is not a local mail catcher (localhost, 127.0.0.1, mailhog)", addr)
	}
	for _, smp := range samples {
		if err := s.Send(ctx, smp.Message); err != nil {
			return fmt.Errorf("%s: %w", smp.Name, err)
		}
	}
	return nil
}

// Querier runs the template query (a pgx pool or connection).
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// LoadTemplates reads the active email templates.
func LoadTemplates(ctx context.Context, q Querier) ([]Template, error) {
	rows, err := q.Query(ctx, `SELECT code, language, subject, body FROM notification_templates
		WHERE channel = 'email' AND active ORDER BY code, language DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Template
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.Code, &t.Lang, &t.Subject, &t.Body); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
