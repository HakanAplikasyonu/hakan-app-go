package journals

import (
	"testing"
	"time"
)

// #574: içe aktarılan dönemin mizan durumu.
func TestTransferCheck(t *testing.T) {
	imported := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	before, after := imported.Add(-time.Hour), imported.Add(time.Hour)
	cases := []struct {
		name    string
		checked bool
		runAt   *time.Time
		issues  int64
		want    string
	}{
		{"never compared", false, nil, 0, CheckNone},
		{"compared before import", true, &before, 0, CheckStale},
		{"stale even with differences", true, &before, 4, CheckStale},
		{"matches", true, &after, 0, CheckOK},
		{"differences", true, &after, 3, CheckDifferences},
	}
	for _, c := range cases {
		got := transferCheck(c.checked, c.runAt, c.issues, &imported)
		if got.Status != c.want {
			t.Errorf("%s: %s, want %s", c.name, got.Status, c.want)
		}
		if c.want == CheckDifferences && got.Issues != c.issues {
			t.Errorf("%s: issues %d", c.name, got.Issues)
		}
	}
	if got := transferCheck(true, &before, 0, nil); got.Status != CheckOK {
		t.Errorf("no import time: %s", got.Status)
	}
}
