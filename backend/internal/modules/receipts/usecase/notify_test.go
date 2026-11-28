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
	title, body := DecisionNotice("Deneme Müşavirlik", "fuel_receipt", true, Rejection{})
	if title != "Fişiniz onaylandı" || body != "Deneme Müşavirlik, gönderdiğiniz akaryakıt fişini onayladı." {
		t.Fatalf("approve = %q %q", title, body)
	}
	title, body = DecisionNotice("", "expense_receipt", false, Rejection{})
	if title != "Fişiniz kabul edilmedi" || !strings.HasPrefix(body, "Müşaviriniz, gönderdiğiniz masraf fişini kabul etmedi.") {
		t.Fatalf("reject = %q %q", title, body)
	}
}

// Ret gerekçesi (#780): uygulama içi metin gerekçeyi söyler; kilit ekranı
// değişkeni yalnız sabit gerekçenin etiketidir, ofisin yazdığı metin değil.
func TestDecisionNoticeWithReason(t *testing.T) {
	_, body := DecisionNotice("Deneme Müşavirlik", "expense_receipt", false, Rejection{Code: RejectUnreadable})
	if !strings.Contains(body, "kabul etmedi. Gerekçe: Fotoğraf okunmuyor.") || !strings.Contains(body, "Gönderdiklerim") {
		t.Fatalf("reason body = %q", body)
	}
	_, body = DecisionNotice("Deneme Müşavirlik", "z_report", false, Rejection{Code: RejectOther, Text: "Ekim ayına ait değil."})
	if !strings.Contains(body, "Gerekçe: Ekim ayına ait değil.") || strings.Contains(body, "Diğer") || strings.Contains(body, "..") {
		t.Fatalf("other body = %q", body)
	}
	_, body = DecisionNotice("", "expense_receipt", false, Rejection{})
	if strings.Contains(body, "Gerekçe") {
		t.Fatalf("no reason body = %q", body)
	}
	if pv := DecisionPushVars(false, Rejection{Code: RejectDuplicate, Text: "Ali Bey 1.250 TL"}); pv["reason"] != "Daha önce gönderilmiş" || len(pv) != 1 {
		t.Fatalf("push vars = %v", pv)
	}
	if DecisionPushVars(false, Rejection{Code: RejectOther, Text: "Ali Bey 1.250 TL"}) != nil || DecisionPushVars(true, Rejection{Code: RejectDuplicate}) != nil ||
		DecisionPushVars(false, Rejection{}) != nil {
		t.Fatal("push vars must be empty for diger, approval and no reason")
	}
}
