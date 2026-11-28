package entegrator

import (
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider/providertest"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// İlk yükleme yeniden eskiye planlanır: önce bu ayın bütün kutuları, sonra
// geçen ay, ... geçen yılın Ocak ayı en sonda (#780).
func TestPlanBackfillNewestFirst(t *testing.T) {
	s := &Service{cfg: Config{}.withDefaults()}
	c := Wrap(providertest.NewFake())
	now := time.Date(2026, 11, 23, 14, 5, 0, 0, istanbul)
	ws := s.plan(c, RunBackfill, now)
	if len(ws) == 0 {
		t.Fatal("no windows")
	}
	boxes := 0
	for _, w := range ws {
		if w.From.Equal(ws[0].From) {
			boxes++
		}
	}
	if months := len(ws) / boxes; months != 23 || len(ws)%boxes != 0 {
		t.Fatalf("months = %d (windows %d, boxes %d), want 23", months, len(ws), boxes)
	}
	if want := time.Date(2026, 11, 1, 0, 0, 0, 0, istanbul); !ws[0].From.Equal(want) || !ws[0].To.Equal(now) {
		t.Fatalf("first window = %v..%v, want the current month", ws[0].From, ws[0].To)
	}
	if want := time.Date(2025, 1, 1, 0, 0, 0, 0, istanbul); !ws[len(ws)-1].From.Equal(want) {
		t.Fatalf("last window starts %v, want %v", ws[len(ws)-1].From, want)
	}
	for i := 1; i < len(ws); i++ {
		if ws[i].From.After(ws[i-1].From) {
			t.Fatalf("window %d (%v) is newer than window %d (%v)", i, ws[i].From, i-1, ws[i-1].From)
		}
	}
	// The fast first step covers exactly the first 2×boxes windows.
	rs := recentStart(now)
	if want := time.Date(2026, 10, 1, 0, 0, 0, 0, istanbul); !rs.Equal(want) {
		t.Fatalf("recentStart = %v, want %v", rs, want)
	}
	fast := 0
	for i, w := range ws {
		if !w.From.Before(rs) {
			if i != fast {
				t.Fatalf("recent window %d after an older one", i)
			}
			fast++
		}
	}
	if fast != RecentMonths*boxes {
		t.Fatalf("fast windows = %d, want %d", fast, RecentMonths*boxes)
	}
}

// Ocak ayında "geçen ay" bir önceki yılın Aralık'ıdır.
func TestRecentStartJanuary(t *testing.T) {
	now := time.Date(2027, 1, 3, 9, 0, 0, 0, istanbul)
	if got, want := recentStart(now), time.Date(2026, 12, 1, 0, 0, 0, 0, istanbul); !got.Equal(want) {
		t.Fatalf("recentStart = %v, want %v", got, want)
	}
	// 2026-01 .. 2027-01 = 13 months, minus the recent two.
	if got := historyMonths(now); got != 11 {
		t.Fatalf("historyMonths = %d, want 11", got)
	}
}

func TestProgressFrom(t *testing.T) {
	now := time.Date(2026, 11, 23, 14, 0, 0, 0, istanbul)
	if p := ProgressFrom(now, nil, 0); p.Phase != "recent" || p.RecentReady || p.HistoryMonthsTotal != 21 {
		t.Fatalf("empty: %+v", p)
	}
	rows := []db.SyncWindowMonthsRow{
		{Month: "2026-11", Total: 3, Settled: 3},
		{Month: "2026-10", Total: 3, Settled: 2},
	}
	if p := ProgressFrom(now, rows, 0); p.RecentReady {
		t.Fatalf("previous month still listing: %+v", p)
	}
	rows[1].Settled = 3
	if p := ProgressFrom(now, rows, 4); p.RecentReady {
		t.Fatalf("UBLs pending: %+v", p)
	}
	rows = append(rows,
		db.SyncWindowMonthsRow{Month: "2026-09", Total: 3, Settled: 3},
		db.SyncWindowMonthsRow{Month: "2026-08", Total: 3, Settled: 3},
		db.SyncWindowMonthsRow{Month: "2026-07", Total: 3, Settled: 1},
	)
	p := ProgressFrom(now, rows, 0)
	if !p.RecentReady || p.Phase != "history" || p.HistoryMonthsDone != 2 || p.HistoryMonthsTotal != 21 {
		t.Fatalf("history: %+v", p)
	}
}

// EDM ve İşNet sınırları saniye hassasiyetinde alır (SecondBounds): bitiş
// tam saniyeye yukarı yuvarlanır, tam saniye olduğu gibi kalır.
func TestSecondBounds(t *testing.T) {
	for _, name := range []string{"edm", "isnet"} {
		if !listProfile(name).SecondBounds {
			t.Errorf("%s: SecondBounds not registered", name)
		}
	}
	last := time.Date(2026, 11, 1, 0, 0, 0, 0, istanbul).Add(-time.Nanosecond)
	if got := ceilSecond(last); !got.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, istanbul)) {
		t.Fatalf("ceilSecond(%s) = %s", last, got)
	}
	whole := time.Date(2026, 11, 15, 19, 26, 0, 0, istanbul)
	if got := ceilSecond(whole); !got.Equal(whole) {
		t.Fatalf("ceilSecond(%s) = %s", whole, got)
	}
}
