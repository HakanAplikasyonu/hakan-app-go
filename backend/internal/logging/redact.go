package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/observability"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
)

// Redact wraps next so no personal data reaches the log sinks (stdout, the
// app_logs table, the error tracker). The message and every string, error
// and Stringer attribute go through observability.ScrubString (e-mail,
// phone, tokens, IBAN, VKN, TCKN); attributes whose key looks sensitive
// (password, token, vkn, iban …) become "[redacted]". Numbers, booleans,
// times and durations pass unchanged (issue #179).
//
// Records logged with a request context also get request_id and the office's
// organization_id (internal ids, not personal data).
func Redact(next slog.Handler) slog.Handler {
	if _, ok := next.(*redactHandler); ok {
		return next
	}
	return &redactHandler{next: next}
}

type redactHandler struct{ next slog.Handler }

func (h *redactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h *redactHandler) Handle(ctx context.Context, rec slog.Record) error {
	out := slog.NewRecord(rec.Time, rec.Level, observability.ScrubString(rec.Message), rec.PC)
	has := map[string]bool{}
	rec.Attrs(func(a slog.Attr) bool {
		has[a.Key] = true
		out.AddAttrs(redactAttr(a))
		return true
	})
	// Correlation ids of the request, unless the call site set them.
	if !has["request_id"] {
		if id := RequestIDFromContext(ctx); id != "" {
			out.AddAttrs(slog.String("request_id", id))
		}
	}
	if !has["organization_id"] {
		if sc, ok := orgctx.From(ctx); ok && sc.ID > 0 {
			out.AddAttrs(slog.Int64("organization_id", sc.ID))
		}
	}
	return h.next.Handle(ctx, out)
}

func (h *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		clean[i] = redactAttr(a)
	}
	return &redactHandler{next: h.next.WithAttrs(clean)}
}

func (h *redactHandler) WithGroup(name string) slog.Handler {
	return &redactHandler{next: h.next.WithGroup(name)}
}

const redactedValue = "[redacted]"

func redactAttr(a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		group := a.Value.Group()
		clean := make([]slog.Attr, len(group))
		for i, g := range group {
			clean[i] = redactAttr(g)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(clean...)}
	}
	if observability.IsSensitiveKey(a.Key) {
		return slog.String(a.Key, redactedValue)
	}
	switch a.Value.Kind() {
	case slog.KindString:
		return slog.String(a.Key, observability.ScrubString(a.Value.String()))
	case slog.KindAny:
		switch v := a.Value.Any().(type) {
		case error:
			return slog.Any(a.Key, scrubbedError{msg: observability.ScrubString(v.Error()), err: v})
		case []byte:
			return slog.String(a.Key, observability.ScrubString(string(v)))
		case nil, bool, int, int64, uint64, float64:
			return a
		default:
			// Keep structured values structured: scrub their JSON text.
			// The scrubber only rewrites inside strings and drops secret
			// pairs, so the result is still JSON unless a bare number was
			// replaced; then the text is logged as a string.
			b, err := json.Marshal(v)
			if err != nil {
				return slog.String(a.Key, observability.ScrubString(fmt.Sprint(v)))
			}
			clean := observability.ScrubString(string(b))
			if json.Valid([]byte(clean)) {
				return slog.Any(a.Key, json.RawMessage(clean))
			}
			return slog.String(a.Key, clean)
		}
	default:
		return a
	}
}

// scrubbedError keeps the error chain (errors.Is / As still work for the
// error tracker) while printing the scrubbed text.
type scrubbedError struct {
	msg string
	err error
}

func (e scrubbedError) Error() string { return e.msg }
func (e scrubbedError) Unwrap() error { return e.err }
