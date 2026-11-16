// Package periodq reads the `period` query parameter that every list
// endpoint shares (issue #333, the panel's global period selector): one
// accounting month, YYYY-MM. What a record's period is depends on the list
// (an invoice's or expense's own period, otherwise the month the record was
// created in, office time Europe/Istanbul); the format and the error are
// the same everywhere.
package periodq

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// CodeInvalid is the error code of a malformed period.
const CodeInvalid = "INVALID_PERIOD"

var re = regexp.MustCompile(`^(19|20)\d{2}-(0[1-9]|1[0-2])$`)

// Valid reports whether p is YYYY-MM.
func Valid(p string) bool { return re.MatchString(p) }

// Parse returns the request's period ("" when absent). A malformed value
// answers 400 INVALID_PERIOD and returns ok=false.
func Parse(w http.ResponseWriter, r *http.Request) (period string, ok bool) {
	p := strings.TrimSpace(r.URL.Query().Get("period"))
	if p == "" {
		return "", true
	}
	if !Valid(p) {
		response.Error(w, r, http.StatusBadRequest, CodeInvalid, "period must be YYYY-MM")
		return "", false
	}
	return p, true
}

// Middleware enforces the contract for every list (issue #333): a GET under
// /v1/tenant/ or /v1/taxpayer/ with a malformed `period` query answers
// 400 INVALID_PERIOD before the handler runs, whatever module serves it.
// Handlers still read the value; this keeps the answer the same everywhere
// (some lists ignored a bad value, others answered VALIDATION_ERROR).
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet &&
			(strings.HasPrefix(r.URL.Path, "/v1/tenant/") || strings.HasPrefix(r.URL.Path, "/v1/taxpayer/")) &&
			r.URL.Query().Has("period") {
			if _, ok := Parse(w, r); !ok {
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
