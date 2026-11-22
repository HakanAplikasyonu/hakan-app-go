package foreigninvoices

import "testing"

// Mükellef dosyası (#780 A2): PDF ve fotoğraf kabul, diğerleri ret.
func TestSniff(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	cases := []struct {
		data []byte
		ct   string
		ok   bool
	}{
		{[]byte("%PDF-1.4\n%x\n"), "application/pdf", true},
		{png, "image/png", true},
		{[]byte("\xff\xd8\xff\xe0\x00\x10JFIF"), "image/jpeg", true},
		{[]byte("PK\x03\x04 docx"), "", false},
		{[]byte("merhaba"), "", false},
	}
	for _, c := range cases {
		ct, _, _, ok := Sniff(c.data)
		if ct != c.ct || ok != c.ok {
			t.Errorf("Sniff(%q) = %q %v", c.data[:4], ct, ok)
		}
	}
}

func TestCleanFileNameAndStatus(t *testing.T) {
	if got := cleanFileName(`C:\fatura\google.pdf`, ".pdf"); got != "google.pdf" {
		t.Fatalf("name = %q", got)
	}
	if got := cleanFileName("", ".png"); got != "yabanci-fatura.png" {
		t.Fatalf("empty name = %q", got)
	}
	if TaxpayerStatus(StatusConfirmed) != "processed" || TaxpayerStatus(StatusDraft) != "in_review" {
		t.Fatal("taxpayer status words")
	}
}
