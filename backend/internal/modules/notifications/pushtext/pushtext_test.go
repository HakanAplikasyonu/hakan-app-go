package pushtext

import (
	"strings"
	"testing"
)

func TestForRendersSafeVarsOnly(t *testing.T) {
	got := For("exports.ready", "tr", map[string]string{"resource": "Kullanıcılar", "email": "a@b.c"})
	if got.Title != "Dışa aktarım hazır" || !strings.Contains(got.Body, "Kullanıcılar") || strings.Contains(got.Body, "a@b.c") {
		t.Fatalf("got %+v", got)
	}
	en := For("bulk.completed", "en", map[string]string{"succeeded": "12"})
	if en.Title != "Bulk action finished" || en.Body != "The bulk action on 12 records has finished." {
		t.Fatalf("en %+v", en)
	}
}

func TestForFallsBackWhenVarMissing(t *testing.T) {
	got := For("exports.ready", "tr", nil)
	if strings.Contains(got.Title+got.Body, "{{") || got.Body != "Yeni bir bildiriminiz var." {
		t.Fatalf("missing var leaked: %+v", got)
	}
	unknown := For("auth.welcome", "en", nil)
	if unknown.Title != "Notification" || unknown.Body != "You have a new notification." {
		t.Fatalf("unknown kind %+v", unknown)
	}
	if !Known("receipt_submitted") || !Known("receipt_approved") || !Known("receipt_rejected") {
		t.Fatal("receipt push texts missing (#780 Senaryo B)")
	}
	if !Known("bulk.failed") || Known("nope") {
		t.Fatal("Known mismatch")
	}
}

// Ret gerekçesi (#780): gerekçe etiketi kilit ekranında görünür; yoksa eski
// genel metin kalır.
func TestReceiptRejectedReason(t *testing.T) {
	got := For("receipt_rejected", "tr", map[string]string{"reason": "Fotoğraf okunmuyor"})
	if got.Title != "Fişiniz kabul edilmedi" || got.Body != "Müşaviriniz gönderdiğiniz fişi kabul etmedi: Fotoğraf okunmuyor." {
		t.Fatalf("with reason = %+v", got)
	}
	plain := For("receipt_rejected", "tr", nil)
	if plain.Body != "Müşaviriniz gönderdiğiniz fişi kabul etmedi; ayrıntı uygulamada." {
		t.Fatalf("without reason = %+v", plain)
	}
	if en := For("receipt_rejected", "en", map[string]string{"reason": "x"}); !strings.HasSuffix(en.Body, ": x.") {
		t.Fatalf("en = %+v", en)
	}
	// Other kinds ignore the reason.
	if got := For("receipt_approved", "tr", map[string]string{"reason": "x"}); strings.Contains(got.Body, "x") {
		t.Fatalf("approved = %+v", got)
	}
}
