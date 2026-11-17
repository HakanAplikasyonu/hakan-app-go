package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"mime"
	"mime/multipart"
	netmail "net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/templates"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mail"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mailtpl"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Golden files: go test ./internal/modules/notifications/providers -update
// (needs TEST_DATABASE_URL: the texts come from the migrated templates).
var update = flag.Bool("update", false, "rewrite golden files")

const front = "https://app.example.com"

var user = uuid.MustParse("0b5e1d2c-3f4a-4b5c-8d6e-7f8091a2b3c4")

func TestFromTextStructure(t *testing.T) {
	body := "Merhaba Ayşe,\n\nŞifrenizi sıfırlamak için aşağıdaki kodu girin.\n\nŞifre sıfırlama kodunuz: 482913\n\nKod 10 dakika geçerlidir."
	e := FromText("auth.password_reset", "Şifre sıfırlama kodu", body, "", "tr")
	if e.Greeting != "Merhaba Ayşe," {
		t.Fatalf("greeting %q", e.Greeting)
	}
	if len(e.Blocks) != 3 || e.Blocks[1].Kind != mailtpl.Code || e.Blocks[1].Label != "Şifre sıfırlama kodunuz" || e.Blocks[1].Text != "482913" {
		t.Fatalf("blocks %+v", e.Blocks)
	}
	if e.PlainText != body {
		t.Fatal("text part must be the stored text")
	}
	if strings.Contains(e.Preheader, "482913") {
		t.Fatal("code leaked into the preheader")
	}

	body = "Merhaba,\n\nAli sizi davet etti.\n\nDaveti kabul etmek için:\nhttps://app.example.com/invite/abc\n\nBağlantı 7 gün geçerlidir."
	e = FromText("tenant.invitation", "Davet", body, "", "tr")
	var kinds []mailtpl.Kind
	for _, b := range e.Blocks {
		kinds = append(kinds, b.Kind)
	}
	if len(kinds) != 3 || kinds[1] != mailtpl.Button || e.Blocks[1].Label != "Daveti kabul et" || e.Blocks[1].URL != "https://app.example.com/invite/abc" {
		t.Fatalf("blocks %+v", e.Blocks)
	}
	for _, b := range e.Blocks {
		if strings.HasSuffix(b.Text, ":") {
			t.Fatalf("intro line kept next to the button: %q", b.Text)
		}
	}

	// No link in the text: the row's action URL becomes the button.
	e = FromText("warnings.blocker", "Uyarı", "Merhaba,\n\n2 uyarı açıldı.", front+"/t/x/warnings", "en")
	if last := e.Blocks[len(e.Blocks)-1]; last.Kind != mailtpl.Button || last.Label != "View warnings" {
		t.Fatalf("action button %+v", last)
	}
	// An unsafe URL never becomes a button.
	e = FromText("x", "x", "Merhaba,\n\nAç:\njavascript:alert(1)", "http://evil.example/x", "tr")
	for _, b := range e.Blocks {
		if b.Kind == mailtpl.Button {
			t.Fatalf("unsafe button %+v", b)
		}
	}
}

func TestMessageKinds(t *testing.T) {
	p := NewEmailProvider(&mail.Recorder{}, front, "test-app-key")
	row := db.Notification{
		Title: "Belge bekleniyor", Body: "Merhaba,\n\nBelge bekleniyor.\n\nBelgeleri yüklemek için:\n" + front + "/m/x",
		Recipient:    pgtype.Text{String: "ayse@example.com", Valid: true},
		UserID:       pgtype.Int8{Int64: 7, Valid: true},
		TemplateCode: pgtype.Text{String: "doc_tasks.created", Valid: true},
		Payload:      []byte(`{"email_lang":"tr"}`),
	}
	msg := p.Message(row, &user)
	if msg.ListUnsubscribe == "" || !strings.HasPrefix(msg.ListUnsubscribe, front+"/api/v1/notifications/unsubscribe?") {
		t.Fatalf("notification without List-Unsubscribe: %q", msg.ListUnsubscribe)
	}
	if !strings.Contains(msg.HTML, front+"/profile") || !strings.Contains(msg.Body, front+"/profile") {
		t.Fatal("notification footer lacks the preferences link")
	}

	sealed := row
	sealed.SealedContent = pgtype.Text{String: "x", Valid: true}
	if msg := p.Message(sealed, &user); msg.ListUnsubscribe != "" || strings.Contains(msg.HTML, "/profile") {
		t.Fatal("security email must not carry unsubscribe")
	}
	noUser := row
	noUser.UserID = pgtype.Int8{}
	if msg := p.Message(noUser, nil); msg.ListUnsubscribe != "" {
		t.Fatal("email without an account must not carry unsubscribe")
	}
	if msg := (EmailProvider{}).Message(row, &user); msg.HTML != "" || msg.Body != row.Body {
		t.Fatal("without a renderer the stored text goes out alone")
	}
}

func TestDeliverRecords(t *testing.T) {
	rec := &mail.Recorder{}
	p := NewEmailProvider(rec, front, "k")
	_, err := p.Deliver(context.Background(), db.Notification{
		Title: "x", Body: "Merhaba,\n\ny", Recipient: pgtype.Text{String: "a@example.com", Valid: true},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.Sent(); len(got) != 1 || got[0].HTML == "" {
		t.Fatalf("recorded %+v", got)
	}
}

// securityTemplates are sealed (transactional) rows; the rest notify a user.
var securityTemplates = map[string]bool{
	"auth.login_code": true, "auth.oauth_link_code": true, "auth.account_deactivation_code": true,
	"auth.email_verification": true, "auth.password_reset": true, "auth.account_deactivated": true,
	"taxpayer.invitation": true, "tenant.invitation": true, "taxpayer.onboarding_consent": true,
}

var sampleVars = map[string]string{
	"name": "Ayşe Yılmaz", "code": "482913", "expires_minutes": "10", "expires_days": "7",
	"provider": "Google", "org_name": "Örnek Müşavirlik", "taxpayer_name": "Örnek Gıda A.Ş.",
	"inviter_name": "Mehmet Kaya", "role_name": "çalışan", "title": "Ekim 2026 kira faturası",
	"tax_id": "1234567890", "count": "3", "agent_name": "Muhasebe-PC", "minutes": "45",
	"link": front + "/invite/0f3c2a1b",
}

// TestStoredEmailTemplates renders every email template the migrations
// store, exactly as delivery does, and checks it end to end: golden HTML and
// text (Turkish), the MIME tree read back with net/mail + mime/multipart, no
// unfilled variable, a code box for code templates and a button for link
// templates.
func TestStoredEmailTemplates(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	rows, err := d.Pool.Query(ctx, `SELECT code, language, subject, body FROM notification_templates WHERE channel = 'email' AND active ORDER BY code, language`)
	if err != nil {
		t.Fatal(err)
	}
	type tpl struct{ code, lang, subject, body string }
	var all []tpl
	for rows.Next() {
		var r tpl
		if err := rows.Scan(&r.code, &r.lang, &r.subject, &r.body); err != nil {
			t.Fatal(err)
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(all) < 30 {
		t.Fatalf("only %d email templates", len(all))
	}
	p := NewEmailProvider(&mail.Recorder{}, front, "test-app-key")
	for _, r := range all {
		t.Run(r.code+"."+r.lang, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]string{PayloadLang: r.lang})
			row := db.Notification{
				Title:        templates.Render(r.subject, sampleVars),
				Body:         templates.Render(r.body, sampleVars),
				Recipient:    pgtype.Text{String: "ayse@example.com", Valid: true},
				TemplateCode: pgtype.Text{String: r.code, Valid: true},
				Payload:      payload,
			}
			if securityTemplates[r.code] {
				row.SealedContent = pgtype.Text{String: "sealed", Valid: true}
			} else {
				row.UserID = pgtype.Int8{Int64: 7, Valid: true}
			}
			msg := p.Message(row, &user)
			if msg.HTML == "" {
				t.Fatal("no HTML part")
			}
			if strings.Contains(msg.HTML+msg.Body+msg.Subject, "{{") {
				t.Fatal("unfilled template variable")
			}
			if strings.Contains(r.body, "{{code}}") && !strings.Contains(msg.HTML, ">482913</td>") {
				t.Fatal("code template without a code box")
			}
			if strings.Contains(r.body, "{{link}}") && !strings.Contains(msg.HTML, `href="`+sampleVars["link"]+`" target="_blank"`) {
				t.Fatal("link template without a button")
			}
			if (msg.ListUnsubscribe != "") == securityTemplates[r.code] {
				t.Fatalf("List-Unsubscribe = %q for security=%v", msg.ListUnsubscribe, securityTemplates[r.code])
			}
			if len(msg.HTML) > mailtpl.MaxHTMLBytes {
				t.Fatalf("html %d bytes", len(msg.HTML))
			}
			for _, line := range strings.Split(msg.Body, "\n") {
				if n := len([]rune(line)); n > 76 && !strings.Contains(line, "://") {
					t.Fatalf("text line of %d runes: %q", n, line)
				}
			}
			raw, err := mail.Compose(msg, mail.ComposeOptions{From: "noreply@app.example.com", FromName: "HAKAN",
				Now: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC), MessageID: "golden@app.example.com"})
			if err != nil {
				t.Fatal(err)
			}
			checkMIME(t, raw, msg)
			if r.lang == "tr" {
				golden(t, r.code+".html", msg.HTML)
				golden(t, r.code+".txt", msg.Body)
			}
		})
	}
}

func checkMIME(t *testing.T, raw []byte, msg mail.Message) {
	t.Helper()
	m, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	if err != nil || subject != msg.Subject {
		t.Fatalf("subject %q (%v)", subject, err)
	}
	mt, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/alternative" {
		t.Fatalf("content type %q (%v)", mt, err)
	}
	r := multipart.NewReader(m.Body, params["boundary"])
	var types []string
	var text, html string
	for {
		part, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(part)
		ct, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		types = append(types, ct)
		switch ct {
		case "text/plain":
			text = strings.ReplaceAll(string(b), "\r\n", "\n")
		case "text/html":
			html = strings.ReplaceAll(string(b), "\r\n", "\n")
		}
	}
	if strings.Join(types, ",") != "text/plain,text/html" {
		t.Fatalf("parts %v", types)
	}
	if text != msg.Body || html != msg.HTML {
		t.Fatal("parts do not round-trip")
	}
	if got := m.Header.Get("List-Unsubscribe"); (got != "") != (msg.ListUnsubscribe != "") {
		t.Fatalf("List-Unsubscribe header %q", got)
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update): %v", path, err)
	}
	if got != string(want) {
		t.Fatalf("%s differs from golden (run with -update to accept):\n%s", name, got)
	}
}
