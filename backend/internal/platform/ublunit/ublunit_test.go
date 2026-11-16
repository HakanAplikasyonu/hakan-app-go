package ublunit

import "testing"

func TestName(t *testing.T) {
	for code, want := range map[string]string{"C62": "adet", "kgm": "kg", "LTR": "lt", "MTR": "m", "HUR": "saat", "DAY": "gün", "XYZ": "XYZ", "": ""} {
		if got := Name(code); got != want {
			t.Errorf("Name(%q) = %q, want %q", code, got, want)
		}
	}
}
