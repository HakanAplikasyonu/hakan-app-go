package easytrade

import (
	"testing"
	"time"
)

func TestWindowsCutAtMonths(t *testing.T) {
	d := func(s string) time.Time { v, _ := time.Parse("2006-01-02", s); return v }
	got := windows(d("2026-09-20"), d("2026-11-03"))
	want := [][2]string{{"2026-09-20", "2026-10-01"}, {"2026-10-01", "2026-11-01"}, {"2026-11-01", "2026-11-04"}}
	if len(got) != len(want) {
		t.Fatalf("%d windows: %+v", len(got), got)
	}
	for i, w := range want {
		if got[i].From.Format("2006-01-02") != w[0] || got[i].To.Format("2006-01-02") != w[1] {
			t.Fatalf("window %d = %s…%s, want %v", i, got[i].From, got[i].To, w)
		}
	}
	if n := len(windows(d("2026-10-05"), d("2026-10-05"))); n != 1 {
		t.Fatalf("one day: %d windows", n)
	}
}

func TestOpenPeriods(t *testing.T) {
	// 31 Oct 22:30 UTC is already 1 Nov in Istanbul: open periods Oct–Nov.
	from, to := openPeriods(time.Date(2026, 10, 31, 22, 30, 0, 0, time.UTC))
	if from.Format("2006-01-02") != "2026-10-01" || to.Format("2006-01-02") != "2026-11-01" {
		t.Fatalf("open periods %s…%s", from, to)
	}
	from, _ = openPeriods(time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	if from.Format("2006-01-02") != "2025-12-01" {
		t.Fatalf("January: from %s", from)
	}
}

func TestNextAfterBacksOff(t *testing.T) {
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	every := 4 * time.Hour
	for failures, want := range []time.Duration{every, 5 * time.Minute, 10 * time.Minute, 20 * time.Minute, 40 * time.Minute, 80 * time.Minute, 160 * time.Minute, every, every} {
		if got := nextAfter(now, every, failures).Sub(now); got != want {
			t.Fatalf("failures %d: next in %s, want %s", failures, got, want)
		}
	}
	if got := nextAfter(now, every, 500).Sub(now); got != every {
		t.Fatalf("many failures: %s", got)
	}
}
