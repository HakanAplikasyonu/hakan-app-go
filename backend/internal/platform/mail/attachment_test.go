package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

// TestSendWithAttachment parses what the server received as a MIME message
// and checks the text part and the attachment round-trip (issue #386).
func TestSendWithAttachment(t *testing.T) {
	f := startFakeSMTP(t, false, nil)
	s := senderTo(t, f, "none", false)
	pdf := bytes.Repeat([]byte("%PDF-1.4 rapor "), 40)
	err := s.Send(context.Background(), Message{
		To:      []string{"mukellef@example.com"},
		Subject: "Ekim 2026 raporu",
		Body:    "Sayın mükellef,\nraporunuz ektedir.",
		Attachments: []Attachment{{
			Name: "rapor-ekim-2026-şirket.pdf", ContentType: "application/pdf", Data: pdf,
		}},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	msg, err := mail.ReadMessage(strings.NewReader(f.body()))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mt, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/mixed" {
		t.Fatalf("content type %q: %v", mt, err)
	}
	r := multipart.NewReader(msg.Body, params["boundary"])
	text, err := r.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	tb, _ := io.ReadAll(text)
	if !strings.Contains(string(tb), "raporunuz ektedir") {
		t.Fatalf("text part: %q", tb)
	}
	att, err := r.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if att.FileName() != "rapor-ekim-2026-şirket.pdf" {
		t.Fatalf("file name: %q", att.FileName())
	}
	raw, _ := io.ReadAll(att)
	got, err := base64.StdEncoding.DecodeString(strings.NewReplacer("\r", "", "\n", "").Replace(string(raw)))
	if err != nil || !bytes.Equal(got, pdf) {
		t.Fatalf("attachment round trip failed: %v", err)
	}
	for _, line := range strings.Split(string(raw), "\r\n") {
		if len(line) > 76 {
			t.Fatalf("base64 line longer than 76: %d", len(line))
		}
	}
}
