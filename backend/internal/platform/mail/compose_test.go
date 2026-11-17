package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	netmail "net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Golden files: go test ./internal/platform/mail -run Golden -update
var update = flag.Bool("update", false, "rewrite golden files")

func fixedOptions() ComposeOptions {
	return ComposeOptions{
		From:      "noreply@posta.example.com",
		FromName:  "Bildirim Merkezi",
		ReplyTo:   "destek@example.com",
		Now:       time.Date(2026, 11, 17, 9, 30, 0, 0, time.FixedZone("TRT", 3*3600)),
		MessageID: "0123456789abcdef@posta.example.com",
		Boundary:  func(n int) string { return fmt.Sprintf("sinir-%d-0123456789", n) },
	}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update): %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s differs from golden (run with -update to accept):\n--- got ---\n%s", name, got)
	}
}

var sampleText = "Merhaba Ayşe,\n\nGiriş kodunuz: 482913\nKod 10 dakika geçerlidir.\n"
var sampleHTML = `<!DOCTYPE html><html lang="tr"><body><p>Merhaba Ayşe,</p><p>Giriş kodunuz: <strong>482913</strong></p></body></html>`

func TestComposeGolden(t *testing.T) {
	pdf := bytes.Repeat([]byte("%PDF-1.4 rapor "), 12)
	cases := []struct {
		name string
		msg  Message
	}{
		{"text.eml", Message{To: []string{"ayse@example.com"}, Subject: "Giriş kodunuz: 482913", Body: sampleText}},
		{"alternative.eml", Message{
			To: []string{"ayse@example.com"}, Subject: "Belge bekleniyor", Body: sampleText, HTML: sampleHTML,
			ListUnsubscribe: "https://app.example.com/api/v1/notifications/unsubscribe?u=1&sig=ab",
		}},
		{"mixed.eml", Message{
			To: []string{"muhasebe@ornek.com.tr"}, Subject: "Ekim 2026 raporu", Body: sampleText, HTML: sampleHTML,
			ReplyTo:     "Örnek Müşavirlik <ofis@ornek.com.tr>",
			Attachments: []Attachment{{Name: "rapor-ekim-2026-şirket.pdf", ContentType: "application/pdf", Data: pdf}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := Compose(tc.msg, fixedOptions())
			if err != nil {
				t.Fatal(err)
			}
			golden(t, tc.name, raw)
		})
	}
}

// parsed is a message read back with net/mail and mime/multipart only.
type parsed struct {
	header netmail.Header
	// tree is the MIME structure, e.g. "multipart/mixed[multipart/alternative[text/plain,text/html],application/pdf]".
	tree  string
	text  string
	html  string
	files map[string][]byte
}

func parse(t *testing.T, raw []byte) parsed {
	t.Helper()
	m, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("net/mail: %v", err)
	}
	p := parsed{header: m.Header, files: map[string][]byte{}}
	p.tree = walk(t, &p, m.Header.Get("Content-Type"), m.Header.Get("Content-Transfer-Encoding"), "", m.Body)
	return p
}

func walk(t *testing.T, p *parsed, ctype, cte, filename string, body io.Reader) string {
	t.Helper()
	mt, params, err := mime.ParseMediaType(ctype)
	if err != nil {
		t.Fatalf("content type %q: %v", ctype, err)
	}
	if strings.HasPrefix(mt, "multipart/") {
		r := multipart.NewReader(body, params["boundary"])
		var kids []string
		for {
			part, err := r.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("next part of %s: %v", mt, err)
			}
			// NextPart already decoded quoted-printable and dropped the
			// header; base64 is left to the reader.
			kids = append(kids, walk(t, p, part.Header.Get("Content-Type"), part.Header.Get("Content-Transfer-Encoding"), part.FileName(), part))
		}
		return mt + "[" + strings.Join(kids, ",") + "]"
	}
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(cte, "quoted-printable") {
		if data, err = io.ReadAll(quotedprintable.NewReader(bytes.NewReader(data))); err != nil {
			t.Fatalf("quoted-printable: %v", err)
		}
	}
	if strings.EqualFold(cte, "base64") {
		dec, err := base64.StdEncoding.DecodeString(strings.NewReplacer("\r", "", "\n", "").Replace(string(data)))
		if err != nil {
			t.Fatalf("base64: %v", err)
		}
		data = dec
	}
	if params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8") {
		t.Fatalf("charset %q, want UTF-8", params["charset"])
	}
	switch {
	case filename != "":
		p.files[filename] = data
	case mt == "text/plain":
		p.text = string(data)
	case mt == "text/html":
		p.html = string(data)
	}
	return mt
}

func TestComposeRoundTrip(t *testing.T) {
	pdf := bytes.Repeat([]byte{0, 1, 2, 250, 251, '\r', '\n', '.'}, 300)
	longLine := strings.Repeat("çok uzun satır ", 200) // > 998 octets unencoded
	cases := []struct {
		name string
		msg  Message
		tree string
	}{
		{"text", Message{To: []string{"a@example.com"}, Subject: "Şifre sıfırlama", Body: sampleText}, "text/plain"},
		{"alternative", Message{To: []string{"a@example.com"}, Subject: "x", Body: sampleText, HTML: sampleHTML + longLine},
			"multipart/alternative[text/plain,text/html]"},
		{"mixed text", Message{To: []string{"a@example.com"}, Subject: "x", Body: sampleText,
			Attachments: []Attachment{{Name: "a.pdf", ContentType: "application/pdf", Data: pdf}}},
			"multipart/mixed[text/plain,application/pdf]"},
		{"mixed alternative", Message{To: []string{"a@example.com", "b@example.com"}, Subject: "Mutabakat — 30.09.2026", Body: sampleText, HTML: sampleHTML,
			Attachments: []Attachment{
				{Name: "mutabakat-şirket.pdf", ContentType: "application/pdf", Data: pdf},
				{Name: "ek.bin", Data: []byte("x")},
			}},
			"multipart/mixed[multipart/alternative[text/plain,text/html],application/pdf,application/octet-stream]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := Compose(tc.msg, ComposeOptions{From: "noreply@posta.example.com", FromName: "Bildirim Merkezi"})
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(raw), "\r\n") {
				if len(line) > 998 {
					t.Fatalf("line %d is %d octets (RFC 5322 limit 998)", i, len(line))
				}
				if strings.ContainsRune(line, '\n') {
					t.Fatalf("bare LF in line %d", i)
				}
			}
			p := parse(t, raw)
			if p.tree != tc.tree {
				t.Fatalf("MIME tree = %s, want %s", p.tree, tc.tree)
			}
			if got := strings.ReplaceAll(p.text, "\r\n", "\n"); got != tc.msg.Body {
				t.Fatalf("text part = %q, want %q", got, tc.msg.Body)
			}
			if tc.msg.HTML != "" && strings.ReplaceAll(p.html, "\r\n", "\n") != tc.msg.HTML {
				t.Fatalf("html part does not round-trip")
			}
			for _, a := range tc.msg.Attachments {
				if !bytes.Equal(p.files[a.Name], a.Data) {
					t.Fatalf("attachment %q does not round-trip", a.Name)
				}
			}
			dec := new(mime.WordDecoder)
			subj, err := dec.DecodeHeader(p.header.Get("Subject"))
			if err != nil || subj != tc.msg.Subject {
				t.Fatalf("subject = %q (%v), want %q", subj, err, tc.msg.Subject)
			}
			if _, err := p.header.Date(); err != nil {
				t.Fatalf("Date: %v", err)
			}
			id := p.header.Get("Message-ID")
			if !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, "@posta.example.com>") {
				t.Fatalf("Message-ID %q not on the sender domain", id)
			}
			if p.header.Get("MIME-Version") != "1.0" {
				t.Fatal("MIME-Version missing")
			}
			from, err := p.header.AddressList("From")
			if err != nil || from[0].Name != "Bildirim Merkezi" || from[0].Address != "noreply@posta.example.com" {
				t.Fatalf("From = %v (%v)", from, err)
			}
			to, err := p.header.AddressList("To")
			if err != nil || len(to) != len(tc.msg.To) {
				t.Fatalf("To = %v (%v)", to, err)
			}
		})
	}
}

func TestComposeMessageIDUnique(t *testing.T) {
	msg := Message{To: []string{"a@example.com"}, Subject: "x", Body: "y"}
	a, _ := Compose(msg, ComposeOptions{From: "n@example.com"})
	b, _ := Compose(msg, ComposeOptions{From: "n@example.com"})
	ida := parse(t, a).header.Get("Message-ID")
	idb := parse(t, b).header.Get("Message-ID")
	if ida == idb {
		t.Fatalf("Message-ID repeated: %s", ida)
	}
}

func TestComposeListUnsubscribe(t *testing.T) {
	base := Message{To: []string{"a@example.com"}, Subject: "x", Body: "y"}

	raw, _ := Compose(base, ComposeOptions{})
	if h := parse(t, raw).header; h.Get("List-Unsubscribe") != "" || h.Get("List-Unsubscribe-Post") != "" {
		t.Fatal("transactional mail must not carry List-Unsubscribe")
	}

	base.ListUnsubscribe = "https://app.example.com/u?t=1"
	raw, _ = Compose(base, ComposeOptions{})
	h := parse(t, raw).header
	if h.Get("List-Unsubscribe") != "<https://app.example.com/u?t=1>" || h.Get("List-Unsubscribe-Post") != "List-Unsubscribe=One-Click" {
		t.Fatalf("headers: %q / %q", h.Get("List-Unsubscribe"), h.Get("List-Unsubscribe-Post"))
	}

	base.ListUnsubscribe = "http://localhost:3000/u?t=1"
	raw, _ = Compose(base, ComposeOptions{})
	if h := parse(t, raw).header; h.Get("List-Unsubscribe") == "" || h.Get("List-Unsubscribe-Post") != "" {
		t.Fatal("one-click POST is only advertised over HTTPS")
	}

	base.ListUnsubscribe = "/relative"
	if _, err := Compose(base, ComposeOptions{}); err == nil {
		t.Fatal("relative unsubscribe URL accepted")
	}
}

func TestComposeReplyTo(t *testing.T) {
	msg := Message{To: []string{"a@example.com"}, Subject: "x", Body: "y"}
	raw, _ := Compose(msg, ComposeOptions{ReplyTo: "destek@example.com"})
	if got := parse(t, raw).header.Get("Reply-To"); got != "<destek@example.com>" {
		t.Fatalf("default Reply-To = %q", got)
	}
	msg.ReplyTo = "Ofis Çalışanı <ofis@example.com>"
	raw, _ = Compose(msg, ComposeOptions{ReplyTo: "destek@example.com"})
	rt, err := parse(t, raw).header.AddressList("Reply-To")
	if err != nil || rt[0].Address != "ofis@example.com" || rt[0].Name != "Ofis Çalışanı" {
		t.Fatalf("Reply-To = %v (%v)", rt, err)
	}
	msg.ReplyTo = "not an address"
	if _, err := Compose(msg, ComposeOptions{}); err == nil {
		t.Fatal("invalid Reply-To accepted")
	}
	raw, _ = Compose(Message{To: []string{"a@example.com"}, Subject: "x", Body: "y"}, ComposeOptions{})
	if parse(t, raw).header.Get("Reply-To") != "" {
		t.Fatal("Reply-To without configuration")
	}
}

func TestComposeRejectsHeaderInjection(t *testing.T) {
	for name, msg := range map[string]Message{
		"to":          {To: []string{"a@example.com\r\nBcc: x@example.com"}, Body: "y"},
		"reply-to":    {To: []string{"a@example.com"}, ReplyTo: "a@example.com\nBcc: x@example.com", Body: "y"},
		"unsubscribe": {To: []string{"a@example.com"}, ListUnsubscribe: "https://a.example\r\nBcc: x", Body: "y"},
	} {
		if _, err := Compose(msg, ComposeOptions{}); err == nil {
			t.Errorf("%s: header injection accepted", name)
		}
	}
	raw, err := Compose(Message{To: []string{"a@example.com"}, Subject: "Ali\r\nBcc: x@example.com", Body: "y"}, ComposeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if parse(t, raw).header.Get("Bcc") != "" {
		t.Fatal("subject injected a header")
	}
}

func TestRecorderValidates(t *testing.T) {
	var r Recorder
	if err := r.Send(context.Background(), Message{To: []string{"a@example.com"}, Subject: "x", Body: "y"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Send(context.Background(), Message{To: []string{"a@example.com\nBcc: b@example.com"}, Body: "y"}); err == nil {
		t.Fatal("recorder accepted a message SMTP would refuse")
	}
	if len(r.Sent()) != 1 {
		t.Fatalf("recorded %d", len(r.Sent()))
	}
}
