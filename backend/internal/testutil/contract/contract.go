// Package contract holds helpers for integration contract tests (#196):
// fault injection in front of an httptest fake, and the switch that points
// the same tests at a live sandbox. See docs/entegrator-sozlesme-testleri.md.
package contract

import (
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// Fault wraps a fake: the first Times requests (every request when Times < 0)
// get Status, or hang for Delay; later requests reach Next. With Match set,
// only matching requests count and are faulted (e.g. data calls, not the
// token call).
type Fault struct {
	Next   http.Handler
	Status int
	Times  int64
	Delay  time.Duration
	Match  func(*http.Request) bool
	seen   atomic.Int64
}

// Seen is the number of requests that reached the wrapper.
func (f *Fault) Seen() int64 { return f.seen.Load() }

func (f *Fault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.Match != nil && !f.Match(r) {
		f.Next.ServeHTTP(w, r)
		return
	}
	n := f.seen.Add(1)
	if f.Times < 0 || n <= f.Times {
		if f.Delay > 0 {
			select {
			case <-r.Context().Done():
			case <-time.After(f.Delay):
			}
			return
		}
		if f.Status != 0 {
			http.Error(w, http.StatusText(f.Status), f.Status)
			return
		}
	}
	f.Next.ServeHTTP(w, r)
}

// LiveEnv is the switch for live sandbox runs (off unless "1").
const LiveEnv = "CONTRACT_LIVE"

// Live returns CONTRACT_<NAME>_<KEY> for every key when CONTRACT_LIVE=1 and
// all of them are set; ok is false otherwise (the live run is skipped).
// Credentials are read only from the environment, never from files.
func Live(name string, keys ...string) (map[string]string, bool) {
	if os.Getenv(LiveEnv) != "1" {
		return nil, false
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		v := os.Getenv("CONTRACT_" + name + "_" + k)
		if v == "" {
			return nil, false
		}
		out[k] = v
	}
	return out, true
}

// Optional returns CONTRACT_<NAME>_<KEY>, empty when not set.
func Optional(name, key string) string { return os.Getenv("CONTRACT_" + name + "_" + key) }

// PathPrefix matches requests whose path starts with p.
func PathPrefix(p string) func(*http.Request) bool {
	return func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, p) }
}
