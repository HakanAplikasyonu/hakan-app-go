// Package connections raises the connection health warnings (#669 K,
// 05 §3.5): a taxpayer's integration connection (source_connections: e-fatura
// entegratörü, Paraşüt, KolayBi, Easytrade, banka web servisi) that fails or
// has not fetched data for too long shows up in the taxpayer's warnings and
// in the Komuta Merkezi, and auto-resolves once the connection recovers.
//
// The warning points at the connection (entity source_connection) and is
// written to the current month (Europe/Istanbul), so the Komuta Merkezi
// matrix counts it in the current period's cell.
package connections

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
)

// Producer of the connection warnings (05 §1: control:<rule>).
const Producer = "control:source_connection"

// Codes (05 §3.5, seed 000129_connection_warnings).
const (
	CodeFailed = "INT_CONNECTION_FAILED"
	CodeStale  = "INT_SYNC_STALE"
)

// Default thresholds, used only when the configuration has no such param
// (the seed sets the same values; an office or engagement overrides them in
// warning_rule_configs). An explicit null turns that check off.
const (
	// DefaultStaleDays: a set-up connection whose last successful fetch is
	// older than this many days is stale. Syncs run at least daily (the
	// integrators every few hours), so three days means several missed runs
	// and covers a long weekend of a bank that publishes nothing.
	DefaultStaleDays = 3.0
	// DefaultNeverSyncedDays: a set-up connection that never fetched
	// successfully is stale this many days after it was created: the first
	// backfill runs right after setup, so a day is ample.
	DefaultNeverSyncedDays = 1.0
)

// FailingGrace: a set-up (active) connection whose last attempt failed for
// a transient reason (rate_limited, or a last_error such as a bank timeout)
// warns only when it has not fetched successfully for this long (or, never
// having succeeded, was created this long ago). The sweep runs every 15
// minutes and the next successful fetch clears last_error: without the
// grace an occasional timeout would open and close a warning many times a
// day. auth_failed, and a failure during setup, warn at once: they do not
// heal by themselves.
const FailingGrace = 6 * time.Hour

// RecoverHold: an open INT_CONNECTION_FAILED warning does not close on the
// first successful fetch but once the connection has stayed healthy this
// long (four sweeps). A connection that fails again in the meantime keeps
// its warning open and the hold starts over. Together with FailingGrace a
// flaky bank does not open and close the warning many times a day (#688).
const RecoverHold = time.Hour

// recoveredAt is the param holding when the connection was first seen
// healthy again while its warning was open (RFC 3339, UTC). It is set once,
// so the params stay stable during the hold.
const recoveredAt = "recovered_at"

// waitingSetup are setup states that wait on a person (the taxpayer's
// authorisation, the bank instruction), not on a sync: such a connection is
// never stale. A failure there is still INT_CONNECTION_FAILED.
var waitingSetup = map[string]bool{
	"draft": true, "authorization_requested": true, "form_ready": true, "submitted_to_bank": true,
}

// Connection is the part of a source_connections row the rule reads.
type Connection struct {
	ID            int64
	SourceSystem  string
	SetupState    string
	Status        string
	LastError     json.RawMessage
	LastSuccessAt *time.Time
	CreatedAt     time.Time
	// OpenFailed: the params of the connection's open INT_CONNECTION_FAILED
	// warning in this period, nil when there is none (RecoverHold).
	OpenFailed map[string]any
}

// Input is one taxpayer's connections at one moment.
type Input struct {
	// Period the warnings are written to: the current month (YYYY-MM).
	Period      string
	Now         time.Time
	Connections []Connection
}

// Rule checks connection health (engine.Rule[Input]). It owns both codes on
// every connection and raises at most one: a failure hides staleness.
//
// Params are stable on purpose (an acknowledged warning stays acknowledged
// until they change): dates, never "days since".
type Rule struct{}

var _ engine.Rule[Input] = Rule{}

// Producer implements engine.Rule.
func (Rule) Producer() string { return Producer }

// Codes implements engine.Rule.
func (Rule) Codes() []string { return []string{CodeFailed, CodeStale} }

// Check implements engine.Rule.
func (Rule) Check(in Input, cfg map[string]engine.Effective) ([]engine.Outcome, error) {
	stale, staleOn, err := paramDays(cfg, CodeStale, "stale_days", DefaultStaleDays)
	if err != nil {
		return nil, err
	}
	never, neverOn, err := paramDays(cfg, CodeStale, "never_synced_days", DefaultNeverSyncedDays)
	if err != nil {
		return nil, err
	}
	out := make([]engine.Outcome, 0, len(in.Connections))
	for _, c := range in.Connections {
		id := c.ID
		o := engine.Outcome{
			Entity: engine.Entity{Type: catalog.EntitySourceConnection, ID: &id, Period: in.Period},
			Owned:  []string{CodeFailed, CodeStale},
		}
		if f, ok := check(c, in.Now, stale, staleOn, never, neverOn); ok {
			o.Findings = []engine.Finding{f}
		}
		out = append(out, o)
	}
	return out, nil
}

// check is the verdict on one connection. A disabled connection is the
// office's decision: no warning.
func check(c Connection, now time.Time, stale float64, staleOn bool, never float64, neverOn bool) (engine.Finding, bool) {
	if c.Status == "disabled" {
		return engine.Finding{}, false
	}
	params := map[string]any{"source": SourceName(c.SourceSystem), "source_system": c.SourceSystem}
	failing := Failing(c)
	if failing && !transientWithinGrace(c, now) {
		params["status"] = c.Status
		if r := errorReason(c.LastError); r != "" {
			params["reason"] = r
		}
		if c.LastSuccessAt != nil {
			params["last_success_at"] = day(*c.LastSuccessAt)
		}
		return engine.Finding{Code: CodeFailed, Params: params}, true
	}
	if c.OpenFailed != nil {
		if f, ok := hold(c.OpenFailed, failing, now); ok {
			return f, true
		}
	}
	if failing {
		return engine.Finding{}, false // transient, within the grace period
	}
	if waitingSetup[c.SetupState] {
		return engine.Finding{}, false
	}
	if c.LastSuccessAt != nil {
		if staleOn && now.Sub(*c.LastSuccessAt) > days(stale) {
			params["since"] = day(*c.LastSuccessAt)
			return engine.Finding{Code: CodeStale, Params: params}, true
		}
		return engine.Finding{}, false
	}
	if neverOn && now.Sub(c.CreatedAt) > days(never) {
		params["since"] = day(c.CreatedAt)
		params["never_synced"] = true
		return engine.Finding{Code: CodeStale, Params: params}, true
	}
	return engine.Finding{}, false
}

// hold keeps an open failure warning during RecoverHold: the stored params
// are re-raised unchanged (an acknowledged warning stays acknowledged), with
// recovered_at added on the first healthy sweep. A transient failure within
// FailingGrace keeps it open and drops recovered_at, so the hold restarts.
func hold(open map[string]any, failing bool, now time.Time) (engine.Finding, bool) {
	p := make(map[string]any, len(open)+1)
	for k, v := range open {
		p[k] = v
	}
	if failing {
		delete(p, recoveredAt)
		return engine.Finding{Code: CodeFailed, Params: p}, true
	}
	since := now
	if s, ok := p[recoveredAt].(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		}
	} else {
		p[recoveredAt] = now.UTC().Format(time.RFC3339)
	}
	if now.Sub(since) >= RecoverHold {
		return engine.Finding{}, false
	}
	return engine.Finding{Code: CodeFailed, Params: p}, true
}

// transientWithinGrace: a set-up connection's transient failure (anything
// but auth_failed) that is still within FailingGrace of its last success.
func transientWithinGrace(c Connection, now time.Time) bool {
	if c.Status == "auth_failed" || c.SetupState != "active" {
		return false
	}
	since := c.CreatedAt
	if c.LastSuccessAt != nil {
		since = *c.LastSuccessAt
	}
	return now.Sub(since) <= FailingGrace
}

// Failing: the connection's last attempt failed. auth_failed and
// rate_limited say so outright; any other (non-disabled) status with a
// last_error means the last run failed, since every successful run clears
// last_error.
func Failing(c Connection) bool {
	switch c.Status {
	case "auth_failed", "rate_limited":
		return true
	case "disabled":
		return false
	}
	return hasError(c.LastError)
}

func hasError(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return true // unreadable, but something was written
	}
	switch t := v.(type) {
	case nil:
		return false
	case map[string]any:
		return len(t) > 0
	case string:
		return t != ""
	}
	return true
}

// errorReason is the stable part of last_error: the bank's reason, the
// provider error kind or code. The message is left out (it may carry ids or
// times and would reopen an acknowledged warning on every run).
func errorReason(raw json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	for _, k := range []string{"reason", "kind", "code"} {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// paramDays reads a day threshold. Absent: def. Null: the check is off.
func paramDays(cfg map[string]engine.Effective, code, key string, def float64) (float64, bool, error) {
	eff, ok := cfg[code]
	if !ok {
		return def, true, nil
	}
	v, ok := eff.Params[key]
	if !ok {
		return def, true, nil
	}
	if v == nil {
		return 0, false, nil
	}
	f, ok := v.(float64)
	if !ok || f < 0 {
		return 0, false, fmt.Errorf("%s.%s: want a non-negative number of days", code, key)
	}
	return f, true, nil
}

func days(n float64) time.Duration { return time.Duration(n * float64(24*time.Hour)) }

func day(t time.Time) string { return t.In(istanbul).Format("2006-01-02") }
