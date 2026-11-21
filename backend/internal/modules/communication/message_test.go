package communication

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizePhone(t *testing.T) {
	for in, want := range map[string]string{
		"+90 532 123 45 67": "5321234567",
		"0532-123-4567":     "5321234567",
		"(532) 123 45 67":   "5321234567",
		"905321234567":      "5321234567",
	} {
		if got, err := NormalizePhone(in); err != nil || got != want {
			t.Errorf("%q = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"0212 123 45 67", "12345", "", "+90 532 123"} {
		if _, err := NormalizePhone(bad); !errors.Is(err, ErrInvalidPhone) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestNormalizeText(t *testing.T) {
	got, err := NormalizeText("  KDV tahakkuku 1.250 ₺, 10 € ve 5 £  ")
	if err != nil || got != "KDV tahakkuku 1.250 TL, 10 EUR ve 5 GBP\n" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := NormalizeText("  "); !errors.Is(err, ErrEmptyText) {
		t.Fatal("empty accepted")
	}
}

func TestCampaignWords(t *testing.T) {
	if w := CampaignWords("Bu ay tüm paketlerde %20 İNDİRİM kampanyası"); len(w) < 3 {
		t.Fatalf("campaign words = %v", w)
	}
	if w := CampaignWords("KDV beyannamenizin son günü 28 Kasım. Kazanç tutarınız 12.000 TL."); len(w) != 0 {
		t.Fatalf("informational text flagged: %v", w)
	}
}

func TestSMSLength(t *testing.T) {
	// 150 düz karakter + 5 operatör eki = 155: tek SMS.
	if l := SMSLength(strings.Repeat("a", 150), "TR"); l.Chars != 155 || l.Parts != 1 {
		t.Fatalf("150 = %+v", l)
	}
	// ş iki karakter sayılır: 156 → iki parça.
	if l := SMSLength(strings.Repeat("a", 149)+"ş", "TR"); l.Chars != 156 || l.Parts != 2 {
		t.Fatalf("ş = %+v", l)
	}
	if l := SMSLength(strings.Repeat("a", 300), "TR"); l.Parts != 3 {
		t.Fatalf("300 = %+v", l)
	}
}
