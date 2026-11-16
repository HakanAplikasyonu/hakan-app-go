// Package testfakes holds the httptest stand-ins of the external services
// the backend calls (#318): KolayBi, Paraşüt, Easytrade, Vakıfbank and an
// OpenAI-compatible AI provider. Each fake follows the service's documented
// contract (docs/kaynak, the adapters' contract tests) closely enough for
// the adapter to run its real path: login / token exchange, paging, detail
// calls, SOAP envelopes, chat completions.
//
// The adapters' own tests and the write sweep
// (internal/httpserver.TestWriteEndpointsNo5xx) share them, so a contract
// change is made in one place. Credentials the fakes accept are the
// exported constants; anything else answers 401 like the real service.
//
// Test-only: nothing in the production build imports this package.
package testfakes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Serve starts h on an httptest server closed with the test.
func Serve(t testing.TB, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
