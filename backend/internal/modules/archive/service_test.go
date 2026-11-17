package archive

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestExpiry(t *testing.T) {
	t.Parallel()
	today := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	d := func(y int, m time.Month, day int) pgtype.Date {
		return date(time.Date(y, m, day, 0, 0, 0, 0, time.UTC))
	}
	for _, c := range []struct {
		in   pgtype.Date
		want string
		days *int
	}{
		{pgtype.Date{}, "none", nil},
		{d(2026, 11, 16), "expired", ptr(-1)},
		{d(2026, 11, 17), "expiring", ptr(0)},
		{d(2026, 12, 17), "expiring", ptr(30)},
		{d(2026, 12, 18), "valid", ptr(31)},
	} {
		st, days := expiry(c.in, today)
		if st != c.want || (days == nil) != (c.days == nil) || (days != nil && *days != *c.days) {
			t.Errorf("expiry(%v) = %s %v, want %s %v", c.in.Time, st, days, c.want, c.days)
		}
	}
}

func ptr(n int) *int { return &n }

func TestTodayIsIstanbul(t *testing.T) {
	t.Parallel()
	// 16 Kasım 22:30 UTC is already 17 Kasım in Türkiye.
	s := &Service{now: func() time.Time { return time.Date(2026, 11, 16, 22, 30, 0, 0, time.UTC) }}
	if got := s.today().Format(time.DateOnly); got != "2026-11-17" {
		t.Fatalf("today = %s", got)
	}
}

func TestSniffAndFileName(t *testing.T) {
	t.Parallel()
	if ct, ext, ok := sniff([]byte("%PDF-1.7\n...")); !ok || ct != "application/pdf" || ext != ".pdf" {
		t.Fatalf("pdf = %s %s %v", ct, ext, ok)
	}
	if ct, _, ok := sniff([]byte("\x89PNG\r\n\x1a\n0000")); !ok || ct != "image/png" {
		t.Fatalf("png = %s %v", ct, ok)
	}
	if _, _, ok := sniff([]byte("MZ\x90\x00 not a document")); ok {
		t.Fatal("executable accepted")
	}
	if got := cleanFileName(`C:\evrak\risk değerlendirmesi.pdf`, ".pdf"); got != "risk değerlendirmesi.pdf" {
		t.Fatalf("name = %q", got)
	}
	if got := cleanFileName("  ", ".png"); got != "belge.png" {
		t.Fatalf("empty name = %q", got)
	}
}
