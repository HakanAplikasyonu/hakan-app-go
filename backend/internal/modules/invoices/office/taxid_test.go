package office

import "testing"

func TestCounterpartyTaxID(t *testing.T) {
	cases := []struct {
		in        string
		vkn, tckn string
		ok        bool
	}{
		{"9000000017", "9000000017", "", true},
		{" 900 000 0017 ", "9000000017", "", true},
		{"10000000146", "", "10000000146", true},
		{"9000000018", "", "", false},  // VKN check digit
		{"10000000147", "", "", false}, // TCKN check digit
		{"12345", "", "", false},
		{"90000000-17", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		vkn, tckn, ok := CounterpartyTaxID(c.in)
		if vkn != c.vkn || tckn != c.tckn || ok != c.ok {
			t.Errorf("%q: %q %q %v, want %q %q %v", c.in, vkn, tckn, ok, c.vkn, c.tckn, c.ok)
		}
	}
}
