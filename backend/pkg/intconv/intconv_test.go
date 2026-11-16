package intconv

import (
	"math"
	"testing"
)

func TestParseInt32(t *testing.T) {
	cases := []struct {
		in   string
		want int32
	}{
		{"", 50}, {"abc", 50}, {"0", 50}, {"1", 1}, {"100", 100}, {"101", 50},
		{"-5", 50}, {"4294967297", 50}, {"99999999999999999999", 50},
	}
	for _, c := range cases {
		if got := ParseInt32(c.in, 50, 1, 100); got != c.want {
			t.Errorf("ParseInt32(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestClamp(t *testing.T) {
	if Int32(math.MaxInt32+1) != math.MaxInt32 || Int32(math.MinInt32-1) != math.MinInt32 || Int32(7) != 7 {
		t.Fatal("Int32 clamp")
	}
	if Int16(1<<20) != math.MaxInt16 || Int16(-1<<20) != math.MinInt16 || Int16(-3) != -3 {
		t.Fatal("Int16 clamp")
	}
	if Uint32(-1) != 0 || Uint32(math.MaxUint32+1) != math.MaxUint32 || Uint32(9) != 9 {
		t.Fatal("Uint32 clamp")
	}
	if Uint64(-1) != 0 || Uint64(5) != 5 {
		t.Fatal("Uint64 clamp")
	}
}
