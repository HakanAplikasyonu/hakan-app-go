package reports

import (
	"errors"
	"testing"
)

func TestParseRange(t *testing.T) {
	m, err := ParseRange("2026-03", "")
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind != KindMonth || m.From != "2026-03" || m.PrevFrom != "2026-02" || m.TrendFrom != "2025-04" {
		t.Fatalf("month range: %+v", m)
	}
	y, err := ParseRange("", "2026")
	if err != nil {
		t.Fatal(err)
	}
	if y.Kind != KindYear || y.From != "2026-01" || y.To != "2026-12" || y.PrevFrom != "2025-01" || y.PrevTo != "2025-12" {
		t.Fatalf("year range: %+v", y)
	}
	for _, c := range [][2]string{{"", ""}, {"2026-13", ""}, {"", "26"}, {"2026-01", "2026"}} {
		if _, err := ParseRange(c[0], c[1]); !errors.Is(err, ErrInvalid) {
			t.Errorf("%v: want ErrInvalid, got %v", c, err)
		}
	}
}

func TestMonthsAndLabels(t *testing.T) {
	got := Months("2025-11", "2026-02")
	if len(got) != 4 || got[0] != "2025-11" || got[3] != "2026-02" {
		t.Fatalf("months: %v", got)
	}
	if MonthName("2026-08") != "Ağustos 2026" || MonthShort("2026-08") != "Ağu 26" {
		t.Fatalf("labels: %s %s", MonthName("2026-08"), MonthShort("2026-08"))
	}
	if CategoryLabel("fuel_receipt") != "Akaryakıt" || CategoryLabel("kira") != "kira" {
		t.Fatal("category labels")
	}
}
