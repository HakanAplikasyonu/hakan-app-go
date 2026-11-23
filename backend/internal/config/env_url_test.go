package config

import "testing"

// TestGetEnvURLFallsBack: a local demo passing EASYTRADE_API_BASE_URL as an
// empty (or quoted-empty) value still reaches Easytrade's default address.
func TestGetEnvURLFallsBack(t *testing.T) {
	const def = "https://pro.easytradetr.com/api"
	for in, want := range map[string]string{
		"": def, "  ": def, `""`: def, `''`: def,
		"http://127.0.0.1:9000/api/": "http://127.0.0.1:9000/api",
		`"https://x.test/api"`:       "https://x.test/api",
	} {
		t.Setenv("EASYTRADE_API_BASE_URL", in)
		if got := getEnvURL("EASYTRADE_API_BASE_URL", def); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}
