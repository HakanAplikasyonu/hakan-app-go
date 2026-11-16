package rls

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

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
			s := FromRequest(ctx)
			if s.OrganizationID > 0 && len(s.TaxpayerIDs) == 0 && taxpayers != nil {
				ids, err := taxpayers(ctx, s.OrganizationID, s.UserID, s.EngagementsReadAll)
				if err != nil {
					response.InternalErr(w, r, err, "Could not resolve the data scope")
					return
				}
				s.TaxpayerIDs = ids
			}
			buf := &bufferedWriter{header: http.Header{}, status: http.StatusOK}
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
type bufferedWriter struct {
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
