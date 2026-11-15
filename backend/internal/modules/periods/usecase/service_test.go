package usecase

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestPeriods(t *testing.T) {
	got, err := Periods("2026-02", 4)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"2025-11", "2025-12", "2026-01", "2026-02"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("periods = %v", got)
	}
	for _, bad := range []struct {
		until string
		n     int
	}{{"2026-13", 3}, {"2026-01", 0}, {"2026-01", MaxMatrixPeriods + 1}, {"", 1}} {
		if _, err := Periods(bad.until, bad.n); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Periods(%q, %d) err = %v", bad.until, bad.n, err)
		}
	}
}

func TestReadinessReasons(t *testing.T) {
	r := readiness("2026-03", Counts{BlockersByCode: map[string]int64{}})
	if !r.Ready || r.Locked || len(r.Reasons) != 0 {
		t.Fatalf("empty: %+v", r)
	}
	r = readiness("2026-03", Counts{
		BlockersByCode:     map[string]int64{"PARTY_MISMATCH": 2, "LOCKED_PERIOD_CHANGE": 1},
		OpenChangeRequests: 1,
		Lock:               &Lock{Period: "2026-03"},
	})
	if r.Ready || !r.Locked || r.OpenBlockers != 3 || len(r.Reasons) != 3 {
		t.Fatalf("blocked: %+v", r)
	}
	if r.Reasons[0].WarningCode != "LOCKED_PERIOD_CHANGE" || r.Reasons[2].Code != ReasonOpenChangeRequests {
		t.Fatalf("reason order: %+v", r.Reasons)
	}
}

func TestCurrentPeriodIsTurkeyTime(t *testing.T) {
	// 31 Mart 22:30 UTC = 1 Nisan 01:30 İstanbul.
	s := New(nil).WithClock(func() time.Time { return time.Date(2026, 3, 31, 22, 30, 0, 0, time.UTC) })
	if p := s.CurrentPeriod(); p != "2026-04" {
		t.Fatalf("current period = %s", p)
	}
}

func TestValidation(t *testing.T) {
	s := New(nil)
	if _, err := s.Readiness(t.Context(), Subject{}, "2026-01"); !errors.Is(err, ErrEngagementScoped) {
		t.Fatalf("no subject: %v", err)
	}
	sub := Subject{OrganizationID: 1, EngagementID: 2, TaxpayerID: 3}
	if _, err := s.Unlock(t.Context(), sub, "2026-01", " ", 0); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("unlock without reason: %v", err)
	}
	if _, _, err := s.ListChangeRequests(t.Context(), sub, ChangeRequestFilter{Status: "pending"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad status: %v", err)
	}
}
