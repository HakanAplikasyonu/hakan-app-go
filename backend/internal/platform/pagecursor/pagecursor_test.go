package pagecursor

import (
	"errors"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	d := time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC)
	c := Encode(d, 3252)
	gd, id, err := Decode(c)
	if err != nil || !gd.Equal(d) || id != 3252 {
		t.Fatalf("decode(%q) = %v %d %v", c, gd, id, err)
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	for _, s := range []string{"", "x", "ZDF8MjAyNS0wMS0xM3w", Encode(time.Now(), 1) + "!", "ZDJ8MjAyNS0wMS0xM3wx"} {
		if _, _, err := Decode(s); !errors.Is(err, ErrInvalid) {
			t.Errorf("Decode(%q) err = %v", s, err)
		}
	}
}
