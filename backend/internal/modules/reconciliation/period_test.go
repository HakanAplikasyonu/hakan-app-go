package reconciliation

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

func TestMonthsBetween(t *testing.T) {
	got := monthsBetween("2025-11", "2026-02")
	want := []string{"2025-11", "2025-12", "2026-01", "2026-02"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if got := monthsBetween("2026-10", "2026-10"); !reflect.DeepEqual(got, []string{"2026-10"}) {
		t.Fatalf("single %v", got)
	}
}

func TestLatestByPeriodValidates(t *testing.T) {
	s := &Service{}
	sc := engagementctx.Scope{}
	for _, c := range [][2]string{{"", "2026-10"}, {"2026-10", "2026-09"}, {"2026-13", "2026-12"}, {"2020-01", "2026-12"}} {
		if _, err := s.LatestByPeriod(context.Background(), sc, c[0], c[1]); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%v: %v", c, err)
		}
	}
}
