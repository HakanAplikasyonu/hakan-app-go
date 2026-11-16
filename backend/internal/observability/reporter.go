package observability

import (
	"context"
	"sync"
	"time"
)

// Reporter is an error-reporting backend (issue #179). Sentry (or a
// Sentry-protocol tracker such as GlitchTip) is built in; another backend
// can be added with AddReporter without touching call sites, which only use
// CaptureError / CaptureMessage.
//
// Reporters added with AddReporter get scrubbed input: the error text and
// message go through ScrubString and Extra through the key/value scrubber,
// so a backend cannot leak what the built-in one would not send.
type Reporter interface {
	CaptureError(ctx context.Context, err error, opts Options)
	CaptureMessage(ctx context.Context, msg string, opts Options)
	// Flush waits up to timeout for queued events; false = some were lost.
	Flush(timeout time.Duration) bool
}

var (
	reportersMu sync.RWMutex
	reporters   = []Reporter{sentryReporter{}}
)

func reporterList() []Reporter {
	reportersMu.RLock()
	defer reportersMu.RUnlock()
	return reporters
}

// AddReporter registers another backend. It returns a func that removes it
// (tests).
func AddReporter(r Reporter) (remove func()) {
	wrapped := &scrubbingReporter{next: r}
	reportersMu.Lock()
	reporters = append(append([]Reporter(nil), reporters...), wrapped)
	reportersMu.Unlock()
	return func() {
		reportersMu.Lock()
		defer reportersMu.Unlock()
		out := make([]Reporter, 0, len(reporters))
		for _, x := range reporters {
			if x != Reporter(wrapped) {
				out = append(out, x)
			}
		}
		reporters = out
	}
}

// FlushAll flushes every reporter.
func FlushAll(timeout time.Duration) bool {
	ok := true
	for _, r := range reporterList() {
		ok = r.Flush(timeout) && ok
	}
	return ok
}

type scrubbingReporter struct{ next Reporter }

func (s *scrubbingReporter) CaptureError(ctx context.Context, err error, opts Options) {
	s.next.CaptureError(ctx, scrubbedErr{msg: ScrubString(err.Error()), err: err}, scrubOptions(opts))
}

func (s *scrubbingReporter) CaptureMessage(ctx context.Context, msg string, opts Options) {
	s.next.CaptureMessage(ctx, ScrubString(msg), scrubOptions(opts))
}

func (s *scrubbingReporter) Flush(timeout time.Duration) bool { return s.next.Flush(timeout) }

func scrubOptions(o Options) Options {
	if len(o.Extra) > 0 {
		o.Extra = scrubMap(o.Extra)
	}
	if len(o.Tags) > 0 {
		tags := make(map[string]string, len(o.Tags))
		for k, v := range o.Tags {
			if IsSensitiveKey(k) {
				tags[k] = redacted
				continue
			}
			tags[k] = ScrubString(v)
		}
		o.Tags = tags
	}
	return o
}

// scrubbedErr prints the scrubbed text and keeps the chain for errors.Is.
type scrubbedErr struct {
	msg string
	err error
}

func (e scrubbedErr) Error() string { return e.msg }
func (e scrubbedErr) Unwrap() error { return e.err }
