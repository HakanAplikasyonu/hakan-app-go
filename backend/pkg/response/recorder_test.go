package response

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type recordingWriter struct {
	http.ResponseWriter
	err  error
	code string
	fail error
}

func (w *recordingWriter) RecordServerError(err error, _ string) { w.err = err }
func (w *recordingWriter) RecordErrorCode(code string)           { w.code = code }
func (w *recordingWriter) RecordFailure(err error)               { w.fail = err }

// wrapper is a middleware writer that only exposes Unwrap (metrics, tracing).
type wrapper struct{ http.ResponseWriter }

func (w *wrapper) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func TestRecordersFoundThroughUnwrap(t *testing.T) {
	inner := &recordingWriter{ResponseWriter: httptest.NewRecorder()}
	w := &wrapper{&wrapper{inner}}
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	cause := errors.New("s3 upload failed")
	InternalErr(w, r, cause, "upload failed")
	if !errors.Is(inner.err, cause) {
		t.Fatalf("server error not recorded through wrappers: %v", inner.err)
	}
	if inner.code == "" {
		t.Fatal("error code not recorded through wrappers")
	}

	reason := errors.New("bad password")
	RecordFailure(w, reason)
	if !errors.Is(inner.fail, reason) {
		t.Fatalf("failure not recorded through wrappers: %v", inner.fail)
	}
}

func TestRecorderStopsWithoutUnwrap(t *testing.T) {
	inner := &recordingWriter{ResponseWriter: httptest.NewRecorder()}
	opaque := struct{ http.ResponseWriter }{inner} // no Unwrap
	InternalErr(opaque, httptest.NewRequest(http.MethodGet, "/", nil), errors.New("x"), "x")
	if inner.err != nil {
		t.Fatal("must not reach a writer that is not unwrapped")
	}
}
