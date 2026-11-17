package reconciliation

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mail"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mailtpl"
)

// Golden files: go test ./internal/modules/reconciliation -run LetterEmail -update
var update = flag.Bool("update", false, "rewrite golden files")

func sampleLetter() LetterSnapshot {
	return LetterSnapshot{TaxpayerTitle: "Atlas Gıda A.Ş.", TaxpayerTaxID: "4730074556", CounterpartyName: "Öz Şeker Ltd. Şti.",
		CounterpartyTaxID: "1111111111", AsOfDate: "2026-10-31", Balance: "-12345.60", Currency: "TRY",
		Office: "Demo Müşavirlik", Note: "Ekstre ektedir."}
}

// TestLetterEmail: golden HTML and text; with the PDF the message is
// multipart/mixed[multipart/alternative[text/plain,text/html],application/pdf]
// read back with net/mail and mime/multipart.
func TestLetterEmail(t *testing.T) {
	link := "https://app.example.com/mutabakat/tok3n"
	msg, err := LetterEmail(sampleLetter(), link, mailtpl.New("https://app.example.com"))
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "letter-email.html", msg.HTML)
	golden(t, "letter-email.txt", msg.Body)
	flat := strings.ReplaceAll(msg.Body, "\n", " ")
	if !strings.Contains(flat, "12.345,60 TL alacak") || !strings.Contains(msg.Body, link) || !strings.Contains(msg.Body, "Not:\nEkstre ektedir.") {
		t.Fatalf("text %s", msg.Body)
	}

	pdf := []byte("%PDF-1.4 mektup")
	msg.To = []string{"muhasebe@ozseker.com.tr"}
	msg.Attachments = []mail.Attachment{{Name: LetterFileName(sampleLetter()), ContentType: "application/pdf", Data: pdf}}
	raw, err := mail.Compose(msg, mail.ComposeOptions{From: "noreply@app.example.com", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	in, err := mail.Inspect(raw)
	if err != nil {
		t.Fatal(err)
	}
	if in.Tree != "multipart/mixed[multipart/alternative[text/plain,text/html],application/pdf]" {
		t.Fatalf("tree %s", in.Tree)
	}
	if in.Subject != msg.Subject || in.Text != msg.Body || in.HTML != msg.HTML || !bytes.Equal(in.Attachments[LetterFileName(sampleLetter())], pdf) {
		t.Fatal("parts do not round-trip")
	}
}

// A plain-http origin (staging) still sends the letter: the answer address
// is in the text, the HTML does not link it.
func TestLetterEmailInsecureLinkStillSends(t *testing.T) {
	msg, err := LetterEmail(sampleLetter(), "http://app.example.com/mutabakat/x", mailtpl.New(""))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(msg.HTML, `href="http://app.example.com`) || !strings.Contains(msg.Body, "http://app.example.com/mutabakat/x") {
		t.Fatal("insecure link handling")
	}
}

func TestLetterEmailEscapes(t *testing.T) {
	s := sampleLetter()
	s.CounterpartyName = `<img src=x onerror=alert(1)>`
	s.Note = `<script>alert(2)</script>`
	msg, err := LetterEmail(s, "https://app.example.com/m/x", mailtpl.New(""))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(msg.HTML, "<img src=x") || strings.Contains(msg.HTML, "<script>") {
		t.Fatal("unescaped input in HTML")
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
