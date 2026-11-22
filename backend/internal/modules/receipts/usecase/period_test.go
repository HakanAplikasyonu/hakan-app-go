package usecase

import (
	"testing"
	"time"
)

// Mükellefin gönderdiği Z raporu / masraf fişi belgenin kendi ayına düşer;
// tarihi okunamayan fiş boş dönemde kalmaz, Türkiye saatiyle yükleme ayına
// düşer (Oğuz'un demo geri bildirimi).
func TestDraftPeriod(t *testing.T) {
	now := time.Date(2026, 10, 31, 22, 30, 0, 0, time.UTC) // İstanbul'da 1 Kasım 01:30
	cases := []struct {
		name    string
		date    time.Time
		hasDate bool
		want    string
	}{
		{"belge tarihi önceki ay", time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), true, "2026-09"},
		{"belge tarihi aynı ay", time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), true, "2026-10"},
		{"tarih yok: Türkiye saatiyle yükleme ayı", time.Time{}, false, "2026-11"},
	}
	for _, c := range cases {
		if got := DraftPeriod(c.date, c.hasDate, now); got != c.want {
			t.Errorf("%s: DraftPeriod = %s, want %s", c.name, got, c.want)
		}
	}
}
