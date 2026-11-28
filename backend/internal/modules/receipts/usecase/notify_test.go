package usecase

import (
	"strings"
	"testing"
)

func TestSubmittedNotice(t *testing.T) {
	title, body := SubmittedNotice("Örnek Gıda Ltd.", "z_report", StatusDraft)
	if title != "Mükelleften Z raporu geldi" || body != "Örnek Gıda Ltd. bir Z raporu gönderdi; onayınızı bekliyor." {
		t.Fatalf("draft = %q %q", title, body)
	}
	_, body = SubmittedNotice("Örnek Gıda Ltd.", "", StatusFailed)
	if !strings.Contains(body, "okunamadı") || !strings.Contains(body, "bir fiş") {
		t.Fatalf("failed = %q", body)
	}
}

func TestDecisionNotice(t *testing.T) {
	title, body := DecisionNotice("Deneme Müşavirlik", "fuel_receipt", true)
	if title != "Fişiniz onaylandı" || body != "Deneme Müşavirlik, gönderdiğiniz akaryakıt fişini onayladı." {
		t.Fatalf("approve = %q %q", title, body)
	}
	title, body = DecisionNotice("", "expense_receipt", false)
	if title != "Fişiniz kabul edilmedi" || !strings.HasPrefix(body, "Müşaviriniz, gönderdiğiniz masraf fişini kabul etmedi.") {
		t.Fatalf("reject = %q %q", title, body)
	}
}
