package rls

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// TaxpayerLister returns the taxpayers an office user may see across the
// office (office-wide routes have no engagement or taxpayer in the context).
type TaxpayerLister func(ctx context.Context, orgID, userID int64, readAll bool) ([]int64, error)

// Middleware runs the request in one rls.WithScope transaction (issue #166).
// Put it after the organization (and engagement) resolvers. Repositories on
// database.NewQueries / database.Ambient join that transaction through the
// context, so every statement of the request is filtered by row level
// security for the caller's scope.
//
// The response is buffered and written only after the transaction is done:
// a 2xx is never sent for work that rolled back. A 4xx/5xx answer rolls the
// transaction back. Not for streaming responses.
//
// On office-wide routes (no engagement or taxpayer in the context) taxpayers
// lists the taxpayers of the engagements the user may see, so taxpayer data
// of the office stays readable; nil leaves the scope without taxpayers.
func Middleware(db Beginner, taxpayers TaxpayerLister, log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if _, scoped := dbtx.Tx(ctx); scoped {
				// Already in a request scope (the tenant scope opened by
				// RequireOrganization): nothing to add.
				next.ServeHTTP(w, r)
				return
			}
			s := FromRequest(ctx)
			if s.OrganizationID > 0 && len(s.TaxpayerIDs) == 0 && taxpayers != nil {
				ids, err := taxpayers(ctx, s.OrganizationID, s.UserID, s.EngagementsReadAll)
				if err != nil {
					response.InternalErr(w, r, err, "Could not resolve the data scope")
					return
				}
				s.TaxpayerIDs = ids
			}
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				// Reads stream (file downloads, PDFs): nothing to commit
				// before the answer.
				sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
				err := WithScope(ctx, db, s, func(ctx context.Context, _ pgx.Tx) error {
					next.ServeHTTP(sw, r.WithContext(ctx))
					if sw.status >= http.StatusBadRequest {
						return errRollback
					}
					return nil
				})
				if err != nil && !errors.Is(err, errRollback) {
					log.ErrorContext(ctx, "rls_scope_failed", "error", err, "path", r.URL.Path)
					if !sw.wrote {
						response.InternalErr(w, r, err, "Could not complete the request")
					}
				}
				return
			}
			buf := &bufferedWriter{out: w, header: http.Header{}, status: http.StatusOK}
			err := WithScope(ctx, db, s, func(ctx context.Context, _ pgx.Tx) error {
				next.ServeHTTP(buf, r.WithContext(ctx))
				if buf.status >= http.StatusBadRequest {
					return errRollback
				}
				return nil
			})
			if err != nil && !errors.Is(err, errRollback) {
				log.ErrorContext(ctx, "rls_scope_commit_failed", "error", err, "path", r.URL.Path)
				response.InternalErr(w, r, err, "Could not complete the request")
				return
			}
			buf.flush(w)
		})
	}
}

var errRollback = errors.New("rls: handler answered with an error")

// bufferedWriter holds the response until the transaction has committed.
// It forwards the error recorders of the writer it wraps (the access log's
// statusWriter), so a handler's 5xx cause and 4xx reason still reach the
// log and error reporting.
type bufferedWriter struct {
	out         http.ResponseWriter
	header      http.Header
	status      int
	wroteHeader bool
	body        bytes.Buffer
}

func (b *bufferedWriter) Header() http.Header { return b.header }

func (b *bufferedWriter) WriteHeader(code int) {
	if b.wroteHeader {
		return
	}
	b.wroteHeader = true
	b.status = code
}

func (b *bufferedWriter) Write(p []byte) (int, error) {
	if !b.wroteHeader {
		b.WriteHeader(http.StatusOK)
	}
	return b.body.Write(p)
}

func (b *bufferedWriter) flush(w http.ResponseWriter) {
	for k, v := range b.header {
		w.Header()[k] = v
	}
	w.WriteHeader(b.status)
	_, _ = w.Write(b.body.Bytes())
}

// statusWriter records the status of a streamed answer.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(p []byte) (int, error) {
	if !s.wrote {
		s.wrote = true
	}
	return s.ResponseWriter.Write(p)
}

// Flush keeps streaming handlers working.
func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the streamed writer so response recorders (5xx cause,
// error code) reach the access log's recorder.
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Narrow sets the taxpayers of the request's scope to one taxpayer (the
// engagement's), on the transaction the context carries. Without one it
// does nothing. RequireEngagement calls it: the scope opened for the office
// is narrowed to the engagement.
func Narrow(ctx context.Context, taxpayerID int64) error {
	tx, ok := dbtx.Tx(ctx)
	if !ok || taxpayerID <= 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `SELECT set_config('app.taxpayer_ids', $1, true)`, idArray([]int64{taxpayerID}))
	return err
}

// Recorders unwraps to the writer the response is flushed to, so
// response.InternalErr / RecordFailure reach the access log's recorder. The
// buffer has no Unwrap: http.ResponseController must not flush around it.
func (b *bufferedWriter) RecordServerError(err error, publicMessage string) {
	response.RecordServerErrorOn(b.out, err, publicMessage)
}

// RecordErrorCode forwards the API error code to the wrapped writer.
func (b *bufferedWriter) RecordErrorCode(code string) {
	response.RecordErrorCodeOn(b.out, code)
}

// RecordFailure forwards the 4xx cause to the wrapped writer.
func (b *bufferedWriter) RecordFailure(err error) {
	response.RecordFailure(b.out, err)
}
