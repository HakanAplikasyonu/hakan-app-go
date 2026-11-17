package mailpreview

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mail"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestPreviewEveryEmail renders every email the product sends from a freshly
// migrated database, writes the preview and reads each .eml back with
// net/mail + mime/multipart.
func TestPreviewEveryEmail(t *testing.T) {
	d := testdb.Open(t)
	tpls, err := LoadTemplates(context.Background(), d.Pool)
	if err != nil {
		t.Fatal(err)
	}
	samples, err := Build(tpls, "https://app.example.com", time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != len(tpls)+2 {
		t.Fatalf("%d samples for %d templates", len(samples), len(tpls))
	}
	dir := t.TempDir()
	if err := Write(dir, samples); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range samples {
		raw, err := os.ReadFile(filepath.Join(dir, s.Name+".eml"))
		if err != nil {
			t.Fatal(err)
		}
		in, err := mail.Inspect(raw)
		if err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
		want := "multipart/alternative[text/plain,text/html]"
		if len(s.Message.Attachments) > 0 {
			want = "multipart/mixed[" + want + ",application/pdf]"
		}
		if in.Tree != want {
			t.Errorf("%s: tree %s, want %s", s.Name, in.Tree, want)
		}
		if in.Text == "" || in.HTML == "" || in.Subject == "" {
			t.Errorf("%s: empty part", s.Name)
		}
		for _, h := range []string{"Message-ID", "Date", "MIME-Version"} {
			if in.Header.Get(h) == "" {
				t.Errorf("%s: no %s", s.Name, h)
			}
		}
		if (in.Header.Get("List-Unsubscribe") != "") != (s.Kind == "bildirim") {
			t.Errorf("%s (%s): List-Unsubscribe %q", s.Name, s.Kind, in.Header.Get("List-Unsubscribe"))
		}
		if len(raw) > 2<<20 {
			t.Errorf("%s: %d bytes", s.Name, len(raw))
		}
		if !strings.Contains(string(index), s.Name+".eml") {
			t.Errorf("index lacks %s", s.Name)
		}
	}
}

type recorder struct{ n int }

func (r *recorder) Send(context.Context, mail.Message) error { r.n++; return nil }

func TestSendOnlyToLocalCatcher(t *testing.T) {
	samples := []Sample{{Name: "x", Message: mail.Message{To: []string{Recipient}}}}
	for _, addr := range []string{"smtp.example.com:587", "10.0.0.5:25", "mail:1025", "bad"} {
		r := &recorder{}
		if err := Send(context.Background(), addr, r, samples); err == nil || r.n != 0 {
			t.Errorf("%s: sent to a non-local server", addr)
		}
	}
	for _, addr := range []string{"127.0.0.1:1025", "localhost:1025", "mailhog:1025", "[::1]:1025"} {
		r := &recorder{}
		if err := Send(context.Background(), addr, r, samples); err != nil || r.n != 1 {
			t.Errorf("%s: %v", addr, err)
		}
	}
}
