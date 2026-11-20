package workday

import (
	"testing"
	"time"
)

func TestNext(t *testing.T) {
	trt := time.FixedZone("TRT", 3*60*60)
	d := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02", s, trt)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, c := range []struct {
		in, want string
		moved    bool
	}{
		{"2026-11-26", "2026-11-26", false}, // Perşembe
		{"2026-11-28", "2026-11-30", true},  // Cumartesi → Pazartesi
		{"2026-05-26", "2026-05-26", false}, // Kurban arifesi yarım gün, süre kaymaz
		{"2026-05-28", "2026-06-01", true},  // Kurban Bayramı + hafta sonu
		{"2027-03-10", "2027-03-12", true},  // Ramazan Bayramı
		{"2026-10-29", "2026-10-30", true},  // Cumhuriyet Bayramı
		{"2026-12-31", "2026-12-31", false},
		{"2027-01-01", "2027-01-04", true}, // Yılbaşı Cuma + hafta sonu
	} {
		got, moved := Next(d(c.in))
		if got.Format("2006-01-02") != c.want || moved != c.moved {
			t.Errorf("Next(%s) = %s %v, want %s %v", c.in, got.Format("2006-01-02"), moved, c.want, c.moved)
		}
	}
	if !Covered(2026) || Covered(2031) {
		t.Fatal("covered years")
	}
}
