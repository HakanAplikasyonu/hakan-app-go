package docuploads

import (
	"errors"
	"testing"
	"time"
)

func TestSuggestPeriod(t *testing.T) {
	now := time.Date(2026, 11, 30, 22, 0, 0, 0, time.UTC) // İstanbul'da 1 Aralık
	d := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	if p, src := SuggestPeriod(&d, now); p != "2026-10" || src != SourceDocumentDate {
		t.Fatalf("belge tarihi = %s %s", p, src)
	}
	if p, src := SuggestPeriod(nil, now); p != "2026-12" || src != SourceCurrentMonth {
		t.Fatalf("içinde bulunulan ay = %s %s", p, src)
	}
}

func TestDecideLockedMonth(t *testing.T) {
	// Açık ay: seçim gerekmez, gönderilse de yok sayılır.
	if p, err := Decide("2026-11", false, ChoiceReject, "", false); err != nil || p.Period != "2026-11" || p.LockChoice != "" || p.Reject {
		t.Fatalf("open = %+v %v", p, err)
	}
	// Kilitli ay, seçim yok: kullanıcıya sorulur.
	if _, err := Decide("2026-09", true, "", "", false); !errors.Is(err, ErrPeriodLocked) {
		t.Fatalf("locked without choice = %v", err)
	}
	// Reddet: kayıt reddedildi olarak tutulur.
	if p, err := Decide("2026-09", true, ChoiceReject, "", false); err != nil || !p.Reject || p.LockedPeriod != "2026-09" {
		t.Fatalf("reject = %+v %v", p, err)
	}
	// Geçmiş döneme düzeltme: kilitli aya yazılır.
	if p, err := Decide("2026-09", true, ChoiceCorrection, "", false); err != nil || p.Period != "2026-09" || p.LockChoice != ChoiceCorrection {
		t.Fatalf("correction = %+v %v", p, err)
	}
	// Seçilen aya uygula: açık hedef aya yazılır, kilitli ay kayıtta kalır.
	if p, err := Decide("2026-09", true, ChoiceApplySelected, "2026-11", false); err != nil || p.Period != "2026-11" || p.LockedPeriod != "2026-09" {
		t.Fatalf("apply selected = %+v %v", p, err)
	}
	if _, err := Decide("2026-09", true, ChoiceApplySelected, "2026-10", true); !errors.Is(err, ErrTargetLocked) {
		t.Fatalf("locked target = %v", err)
	}
	if _, err := Decide("2026-09", true, ChoiceApplySelected, "2026-09", false); !errors.Is(err, ErrTargetLocked) {
		t.Fatalf("same target = %v", err)
	}
	if _, err := Decide("2026-09", true, ChoiceApplySelected, "eylül", false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad target = %v", err)
	}
	if _, err := Decide("2026-09", true, "sil", "", false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad choice = %v", err)
	}
	if _, err := Decide("2026-13", false, "", "", false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad period = %v", err)
	}
}

func TestFileKind(t *testing.T) {
	cases := map[string]string{
		"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR":           "image",
		"%PDF-1.4\n":                                    "pdf",
		"\xef\xbb\xbf<?xml version=\"1.0\"?><Invoice/>": "xml",
		"PK\x03\x04\x14\x00":                            "zip",
		"merhaba":                                       "",
	}
	for in, want := range cases {
		if got := fileKind([]byte(in)); got != want {
			t.Errorf("fileKind(%q) = %q, want %q", in, got, want)
		}
	}
}
