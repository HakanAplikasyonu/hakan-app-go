package periodq

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestParse(t *testing.T) {
	for raw, want := range map[string]struct {
		period string
		ok     bool
	}{
		"":           {"", true},
		"2026-10":    {"2026-10", true},
		" 2026-01 ":  {"2026-01", true},
		"2026-13":    {"", false},
		"2026-1":     {"", false},
		"26-10":      {"", false},
		"2026-10-01": {"", false},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/x?period="+url.QueryEscape(raw), nil)
		got, ok := Parse(w, r)
		if got != want.period || ok != want.ok {
			t.Errorf("%q: %q %v, want %q %v", raw, got, ok, want.period, want.ok)
		}
		if !ok && w.Code != http.StatusBadRequest {
			t.Errorf("%q: status %d", raw, w.Code)
		}
	}
}
