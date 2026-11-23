package usecase

import (
	"testing"
	"time"
)

// #780: belge kendi tarihinin ayına düşer; tarih okunamadıysa yükleme ayına
// (ofis saati) düşer, tarih boş kalır.
func TestDocPeriod(t *testing.T) {
	now := time.Date(2026, 11, 30, 22, 30, 0, 0, time.UTC) // 01:30 on 1 Dec in Türkiye
	if got := DocPeriod(time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), true, now); got != "2026-10" {
		t.Fatalf("dated = %s", got)
	}
	if got := DocPeriod(time.Time{}, false, now); got != "2026-12" {
		t.Fatalf("undated = %s (office time)", got)
	}
}
