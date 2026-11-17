package reports

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

// Golden files: go test ./internal/modules/reports -run Email -update
var update = flag.Bool("update", false, "rewrite golden files")

// TestReportEmail: the report email is text + HTML in multipart/alternative
// inside multipart/mixed with the PDF, read back with net/mail and
// mime/multipart; HTML and text match the golden files.
func TestReportEmail(t *testing.T) {
	r := sampleReport()
	msg, err := Email(r, "Demo Müşavirlik", mailtpl.New("https://app.example.com"))
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "report-email.html", msg.HTML)
	golden(t, "report-email.txt", msg.Body)
	for _, want := range []string{"₺95.000,00", "Ödenecek KDV (tahmini)", "Müşavir yorumu", "Demo Müşavirlik"} {
		if !strings.Contains(msg.Body, want) || !strings.Contains(msg.HTML, want) {
			t.Errorf("%q missing from a part", want)
		}
	}

	pdf := []byte("%PDF-1.4 rapor")
	msg.To = []string{"muhasebe@ornek.com.tr"}
	msg.Attachments = []mail.Attachment{{Name: FileName(r), ContentType: "application/pdf", Data: pdf}}
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
	if in.Subject != msg.Subject || in.Text != msg.Body || in.HTML != msg.HTML || !bytes.Equal(in.Attachments[FileName(r)], pdf) {
		t.Fatal("parts do not round-trip")
	}
	if in.Header.Get("List-Unsubscribe") != "" {
		t.Fatal("a report is not a notification: no List-Unsubscribe")
	}
}

// A draft comment never reaches the email; a deferred VAT reads as such.
func TestReportEmailDraftAndDeferredVAT(t *testing.T) {
	r := sampleReport()
	r.Commentary.Status = "draft"
	r.Commentary.Body = "Gizli taslak <script>"
	r.Totals.VATPayable = "-1500.00"
	msg, err := Email(r, "", mailtpl.New(""))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(msg.Body+msg.HTML, "Gizli taslak") {
		t.Fatal("draft comment emailed")
	}
	if !strings.Contains(msg.Body, "Devreden KDV (tahmini)") || !strings.Contains(msg.Body, "Mali müşaviriniz") {
		t.Fatalf("body %s", msg.Body)
	}
}

// User-written text (titles, the comment) is escaped in the HTML.
func TestReportEmailEscapes(t *testing.T) {
	r := sampleReport()
	r.Taxpayer.Title = `<img src=x onerror=alert(1)> Ltd.`
	r.Commentary.Body = `<script>alert(2)</script>`
	msg, err := Email(r, `<b>Ofis</b>`, mailtpl.New("https://app.example.com"))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<img src=x", "<script>", "<b>Ofis"} {
		if strings.Contains(msg.HTML, bad) {
			t.Fatalf("unescaped %q", bad)
		}
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
