package office

import "testing"

func TestMonthName(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"2026-11": "Kasım 2026", "2027-01": "Ocak 2027", "2026-13": "2026-13", "x": "x"} {
		if got := monthName(in); got != want {
			t.Fatalf("monthName(%q) = %q, want %q", in, got, want)
		}
	}
}
