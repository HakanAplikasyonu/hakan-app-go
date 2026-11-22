package ledgers

import (
	"reflect"
	"testing"
	"time"
)

func TestBucket(t *testing.T) {
	cases := map[string]string{"2026-01": "2026-03", "2026-03": "2026-03", "2026-04": "2026-06", "2026-11": "2026-12"}
	for in, want := range cases {
		if got := Bucket(PeriodQuarterly, in); got != want {
			t.Errorf("Bucket(%s) = %s, want %s", in, got, want)
		}
		if got := Bucket(PeriodMonthly, in); got != in {
			t.Errorf("monthly Bucket(%s) = %s", in, got)
		}
	}
}

func TestExpectedPeriods(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	started := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	if got := LastCompleted(now); got != "2026-10" {
		t.Fatalf("last completed = %s", got)
	}
	got := ExpectedPeriods(PeriodMonthly, started, now)
	want := []string{"2026-10", "2026-09", "2026-08", "2026-07"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("monthly = %v, want %v", got, want)
	}
	// 3 aylık: Q4 is not completed; Q3 (ending 2026-09) is the last. The
	// engagement started in Q3, so Q3 is expected.
	got = ExpectedPeriods(PeriodQuarterly, started, now)
	if want := []string{"2026-09"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("quarterly = %v, want %v", got, want)
	}
	// Started this month: nothing is due yet.
	if got := ExpectedPeriods(PeriodMonthly, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC), now); len(got) != 0 {
		t.Fatalf("new engagement = %v", got)
	}
	// Old engagement: bounded.
	if got := ExpectedPeriods(PeriodMonthly, time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC), now); len(got) != maxMonthsBack {
		t.Fatalf("old engagement = %d periods", len(got))
	}
	if got := ExpectedPeriods(PeriodQuarterly, time.Time{}, now); len(got) != maxQuartersBack {
		t.Fatalf("no start = %d quarters", len(got))
	}
}

func TestBuildPeriods(t *testing.T) {
	items := []Item{
		{Kind: "isletme_defteri", Period: "2026-10"},
		{Kind: "e_defter_yevmiye", Period: "2026-10"},
		{Kind: "other", Period: "2026-09"},
		{Kind: "isletme_defteri", Period: "2026-05"},
	}
	rows, missing := BuildPeriods(PeriodMonthly, []string{"2026-10", "2026-09", "2026-08"}, items)
	if want := []string{"2026-09", "2026-08"}; !reflect.DeepEqual(missing, want) {
		t.Fatalf("missing = %v, want %v", missing, want)
	}
	if len(rows) != 4 || rows[0].Period != "2026-10" || rows[0].Files != 2 || rows[3].Period != "2026-05" || rows[3].Expected {
		t.Fatalf("rows = %+v", rows)
	}
	if !reflect.DeepEqual(rows[0].Kinds, []string{"e_defter_yevmiye", "isletme_defteri"}) {
		t.Fatalf("kinds = %v", rows[0].Kinds)
	}
	// 3 aylık: a file in any month of the quarter covers it.
	_, missing = BuildPeriods(PeriodQuarterly, []string{"2026-09", "2026-06"}, []Item{{Kind: "isletme_defteri", Period: "2026-08"}})
	if want := []string{"2026-06"}; !reflect.DeepEqual(missing, want) {
		t.Fatalf("quarterly missing = %v, want %v", missing, want)
	}
}

func TestStartOf(t *testing.T) {
	started := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	items := []Item{{Kind: "isletme_defteri", Period: "2026-08"}, {Kind: "other", Period: "2026-01"}}
	got := startOf(started, items)
	if got.Format("2006-01") != "2026-08" {
		t.Fatalf("start = %s", got)
	}
	if got := startOf(started, nil); !got.Equal(started) {
		t.Fatalf("no files: %s", got)
	}
}
