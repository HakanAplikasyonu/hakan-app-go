package response

import (
	"errors"
	"net/http"
)

// StatusClientClosedRequest is the non-standard 499 used when the client went
// away before the handler finished (nginx convention). Never sent to a live
// client: by definition nobody is listening.
const StatusClientClosedRequest = 499

type serverErrorRecorder interface {
	RecordServerError(err error, publicMessage string)
}

type errorCodeRecorder interface {
	RecordErrorCode(code string)
}

type failureRecorder interface {
	RecordFailure(err error)
}

// Unavailable is implemented by errors that mean a dependency is out of
// service for a while (object store full, issue #247). InternalErr answers
// them with 503 and the error's code instead of a generic 500, so clients
// can say "try again later" and retry safely.
type Unavailable interface {
	error
	UnavailableCode() string
	UnavailableMessage() string
}

// InternalErr writes a generic 500 response and records err for server-side
// logging. When the request context is already cancelled (client disconnected
// or aborted the request) it writes 499 instead: that is not a server fault.
func InternalErr(w http.ResponseWriter, r *http.Request, err error, publicMessage string) {
	if r != nil && r.Context().Err() != nil {
		if rec, ok := recorder[failureRecorder](w); ok {
			if err == nil {
				err = r.Context().Err()
			}
			rec.RecordFailure(err)
		}
		Error(w, r, StatusClientClosedRequest, "CLIENT_CLOSED_REQUEST", "Client closed request")
		return
	}
	if err != nil {
		if rec, ok := recorder[serverErrorRecorder](w); ok {
			rec.RecordServerError(err, publicMessage)
		}
		var u Unavailable
		if errors.As(err, &u) {
			w.Header().Set("Retry-After", "300")
			Error(w, r, http.StatusServiceUnavailable, u.UnavailableCode(), u.UnavailableMessage())
			return
		}
	}
	Internal(w, r, publicMessage)
}

// RecordFailure attaches the cause of a 4xx response to the request so the
// access log can show why (e.g. why a sign-in was rejected). It never changes
// the response. Messages are scrubbed before they are logged.
func RecordFailure(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	if rec, ok := recorder[failureRecorder](w); ok {
		rec.RecordFailure(err)
	}
}

// recorder finds T on w or on a writer it wraps. Middleware that wraps the
// response writer (metrics, tracing, the RLS transaction buffer) exposes the
// writer underneath through Unwrap, the http.ResponseController convention;
// without the walk a 5xx cause or 4xx reason recorded deeper in the chain
// never reaches the access log.
func recorder[T any](w http.ResponseWriter) (T, bool) {
	for i := 0; w != nil && i < 16; i++ {
		if rec, ok := w.(T); ok {
			return rec, true
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			break
		}
		w = u.Unwrap()
	}
	var zero T
	return zero, false
}

// RecordServerErrorOn records a 5xx cause on w (or a writer it wraps)
// without writing a response. For writers that buffer the response and
// forward the recorders (rls.Middleware).
func RecordServerErrorOn(w http.ResponseWriter, err error, publicMessage string) {
	if err == nil {
		return
	}
	if rec, ok := recorder[serverErrorRecorder](w); ok {
		rec.RecordServerError(err, publicMessage)
	}
}

// RecordErrorCodeOn records the API error code on w (or a writer it wraps).
func RecordErrorCodeOn(w http.ResponseWriter, code string) {
	if rec, ok := recorder[errorCodeRecorder](w); ok {
		rec.RecordErrorCode(code)
	}
}
