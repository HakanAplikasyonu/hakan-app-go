package connections

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
)

var now = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

func ago(d time.Duration) *time.Time { t := now.Add(-d); return &t }

// defaults is the seeded configuration (000129_connection_warnings).
func defaults() map[string]engine.Effective {
	return map[string]engine.Effective{
		CodeFailed: {Code: CodeFailed, Enabled: true, Severity: catalog.SeverityBlocker, Params: map[string]any{}},
		CodeStale: {Code: CodeStale, Enabled: true, Severity: catalog.SeverityWarning,
			Params: map[string]any{"stale_days": 3.0, "never_synced_days": 1.0}},
	}
}

func verdict(t *testing.T, c Connection, cfg map[string]engine.Effective) (string, map[string]any) {
	t.Helper()
	if c.ID == 0 {
		c.ID = 7
	}
	outs, err := Rule{}.Check(Input{Period: "2026-11", Now: now, Connections: []Connection{c}}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 {
		t.Fatalf("want one outcome, got %d", len(outs))
	}
	o := outs[0]
	if o.Entity.Type != catalog.EntitySourceConnection || o.Entity.ID == nil || *o.Entity.ID != c.ID || o.Entity.Period != "2026-11" {
		t.Fatalf("entity = %+v", o.Entity)
	}
	if len(o.Owned) != 2 {
		t.Fatalf("owned = %v", o.Owned)
	}
	switch len(o.Findings) {
	case 0:
		return "", nil
	case 1:
		return o.Findings[0].Code, o.Findings[0].Params
	}
	t.Fatalf("more than one finding: %v", o.Findings)
	return "", nil
}

func TestRuleVerdicts(t *testing.T) {
	cases := []struct {
		name string
		c    Connection
		want string
	}{
		{"healthy", Connection{SourceSystem: "parasut", SetupState: "active", Status: "active", LastSuccessAt: ago(2 * time.Hour), CreatedAt: *ago(90 * 24 * time.Hour)}, ""},
		{"auth failed", Connection{SourceSystem: "entegrator_izibiz", SetupState: "credentials_entered", Status: "auth_failed",
			LastError: json.RawMessage(`{"kind":"auth","message":"401 at 10:01"}`), LastSuccessAt: ago(time.Hour)}, CodeFailed},
		{"rate limited within the grace period", Connection{SourceSystem: "easytrade", SetupState: "active", Status: "rate_limited", LastSuccessAt: ago(time.Hour)}, ""},
		{"rate limited past the grace period", Connection{SourceSystem: "easytrade", SetupState: "active", Status: "rate_limited", LastSuccessAt: ago(7 * time.Hour)}, CodeFailed},
		{"one timeout on an active connection is no warning", Connection{SourceSystem: "bank_vakifbank", SetupState: "active", Status: "pending",
			LastError: json.RawMessage(`{"reason":"bank_error"}`), LastSuccessAt: ago(20 * time.Minute)}, ""},
		{"errors for hours on an active connection warn", Connection{SourceSystem: "bank_vakifbank", SetupState: "active", Status: "pending",
			LastError: json.RawMessage(`{"reason":"bank_error"}`), LastSuccessAt: ago(6*time.Hour + time.Minute)}, CodeFailed},
		{"auth failed on an active connection warns at once", Connection{SourceSystem: "parasut", SetupState: "active", Status: "auth_failed",
			LastError: json.RawMessage(`{"kind":"auth"}`), LastSuccessAt: ago(time.Minute)}, CodeFailed},
		{"pending with error (bank refused at setup)", Connection{SourceSystem: "bank_vakifbank", SetupState: "failed", Status: "pending",
			LastError: json.RawMessage(`{"reason":"bank_error","message":"x"}`), CreatedAt: now}, CodeFailed},
		{"failure while waiting on the bank still shows", Connection{SourceSystem: "bank_vakifbank", SetupState: "submitted_to_bank", Status: "auth_failed", CreatedAt: now}, CodeFailed},
		{"json null error is no error", Connection{SourceSystem: "kolaybi", SetupState: "active", Status: "active", LastError: json.RawMessage(`null`), LastSuccessAt: ago(time.Hour)}, ""},
		{"disabled never warns", Connection{SourceSystem: "parasut", SetupState: "active", Status: "disabled",
			LastError: json.RawMessage(`{"kind":"auth"}`), LastSuccessAt: ago(400 * time.Hour)}, ""},
		{"stale", Connection{SourceSystem: "parasut", SetupState: "active", Status: "active", LastSuccessAt: ago(73 * time.Hour)}, CodeStale},
		{"not yet stale", Connection{SourceSystem: "parasut", SetupState: "active", Status: "active", LastSuccessAt: ago(71 * time.Hour)}, ""},
		{"never synced, a day old", Connection{SourceSystem: "easytrade", SetupState: "active", Status: "active", CreatedAt: *ago(25 * time.Hour)}, CodeStale},
		{"never synced, just created", Connection{SourceSystem: "easytrade", SetupState: "active", Status: "active", CreatedAt: *ago(2 * time.Hour)}, ""},
		{"waiting on the bank is not stale", Connection{SourceSystem: "bank_vakifbank", SetupState: "submitted_to_bank", Status: "pending", CreatedAt: *ago(72 * time.Hour)}, ""},
		{"draft is not stale", Connection{SourceSystem: "parasut", SetupState: "authorization_requested", Status: "pending", CreatedAt: *ago(72 * time.Hour)}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := verdict(t, tc.c, defaults())
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRuleParams(t *testing.T) {
	_, p := verdict(t, Connection{SourceSystem: "entegrator_izibiz", SetupState: "credentials_entered", Status: "auth_failed",
		LastError: json.RawMessage(`{"kind":"auth","message":"401 at 10:01"}`), LastSuccessAt: ago(30 * time.Hour)}, defaults())
	if p["source"] != "İzibiz" || p["source_system"] != "entegrator_izibiz" || p["status"] != "auth_failed" ||
		p["reason"] != "auth" || p["last_success_at"] != "2026-11-19" {
		t.Fatalf("failed params = %v", p)
	}
	if _, ok := p["message"]; ok {
		t.Fatal("the raw message must not be a param (it reopens acknowledged warnings)")
	}
	_, p = verdict(t, Connection{SourceSystem: "parasut", SetupState: "active", Status: "active", LastSuccessAt: ago(5 * 24 * time.Hour)}, defaults())
	if p["source"] != "Paraşüt" || p["since"] != "2026-11-15" || p["never_synced"] != nil {
		t.Fatalf("stale params = %v", p)
	}
	_, p = verdict(t, Connection{SourceSystem: "easytrade", SetupState: "active", Status: "active", CreatedAt: *ago(3 * 24 * time.Hour)}, defaults())
	if p["since"] != "2026-11-17" || p["never_synced"] != true {
		t.Fatalf("never synced params = %v", p)
	}
}

func TestRuleThresholdsFromConfig(t *testing.T) {
	c := Connection{SourceSystem: "parasut", SetupState: "active", Status: "active", LastSuccessAt: ago(30 * time.Hour)}
	cfg := defaults()
	cfg[CodeStale].Params["stale_days"] = 1.0
	if got, _ := verdict(t, c, cfg); got != CodeStale {
		t.Fatalf("office threshold 1 day: got %q", got)
	}
	// An explicit null turns the check off.
	cfg[CodeStale].Params["stale_days"] = nil
	c.LastSuccessAt = ago(400 * time.Hour)
	if got, _ := verdict(t, c, cfg); got != "" {
		t.Fatalf("stale check off: got %q", got)
	}
	// Missing params fall back to the documented defaults.
	cfg = defaults()
	cfg[CodeStale] = engine.Effective{Code: CodeStale, Enabled: true, Severity: catalog.SeverityWarning, Params: map[string]any{}}
	if got, _ := verdict(t, c, cfg); got != CodeStale {
		t.Fatalf("default threshold: got %q", got)
	}
	cfg[CodeStale].Params["stale_days"] = "three"
	if _, err := (Rule{}).Check(Input{Now: now, Connections: []Connection{{ID: 1}}}, cfg); err == nil {
		t.Fatal("a bad threshold must fail")
	}
}

func TestSourceName(t *testing.T) {
	for sys, want := range map[string]string{
		"entegrator_izibiz": "İzibiz", "bank_vakifbank": "Vakıfbank", "kolaybi": "KolayBi",
		"entegrator_bilinmeyen": "entegrator_bilinmeyen", "bank_ziraat": "bank_ziraat",
	} {
		if got := SourceName(sys); got != want {
			t.Errorf("SourceName(%q) = %q, want %q", sys, got, want)
		}
	}
}

func TestPeriodIsIstanbulMonth(t *testing.T) {
	// 31 Ekim 22:30 UTC is already 1 Kasım in Istanbul.
	if got := Period(time.Date(2026, 10, 31, 22, 30, 0, 0, time.UTC)); got != "2026-11" {
		t.Fatalf("Period = %s", got)
	}
}

// fakeQueries serves connections; the auto-resolve query is recorded.
type fakeQueries struct {
	conns    []db.SourceConnection
	resolved []db.AutoResolveOtherConnectionWarningsParams
	subjects []db.ListConnectionWarningSubjectsRow
}

func (f *fakeQueries) ListSourceConnections(_ context.Context, taxpayerID int64) ([]db.SourceConnection, error) {
	var out []db.SourceConnection
	for _, c := range f.conns {
		if c.TaxpayerID == taxpayerID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeQueries) AutoResolveOtherConnectionWarnings(_ context.Context, arg db.AutoResolveOtherConnectionWarningsParams) (int64, error) {
	f.resolved = append(f.resolved, arg)
	return 0, nil
}

func (f *fakeQueries) ListConnectionWarningSubjects(context.Context) ([]db.ListConnectionWarningSubjectsRow, error) {
	return f.subjects, nil
}

type staticConfig struct{}

func (staticConfig) Resolve(_ context.Context, _ engine.Subject, codes []string) (map[string]engine.Effective, error) {
	all := defaults()
	out := map[string]engine.Effective{}
	for _, c := range codes {
		if e, ok := all[c]; ok {
			out[c] = e
		}
	}
	return out, nil
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func openCodes(store *engine.MemoryStore) map[string]int {
	out := map[string]int{}
	for _, w := range store.All() {
		if w.Status == catalog.StatusOpen {
			out[w.Code]++
		}
	}
	return out
}

// The warning opens on failure, survives a re-run as one warning and
// auto-resolves once the connection recovers.
func TestCheckRaisesAndAutoResolves(t *testing.T) {
	store := engine.NewMemoryStore()
	eng := engine.New(store, staticConfig{}).WithClock(func() time.Time { return now })
	q := &fakeQueries{
		conns: []db.SourceConnection{{
			ID: 7, TaxpayerID: 100, SourceSystem: "bank_vakifbank", SetupState: "failed", Status: "auth_failed",
			LastError: []byte(`{"reason":"bad_credentials"}`), CreatedAt: ts(now.Add(-time.Hour)),
		}},
		subjects: []db.ListConnectionWarningSubjectsRow{{OrganizationID: 1, EngagementID: 10, TaxpayerID: 100}},
	}
	c := New(q, eng).WithClock(func() time.Time { return now })
	created, err := c.Sweep(context.Background())
	if err != nil || created != 1 {
		t.Fatalf("sweep: created=%d err=%v", created, err)
	}
	if got := openCodes(store); got[CodeFailed] != 1 || len(got) != 1 {
		t.Fatalf("open = %v", got)
	}
	s := engine.Subject{OrganizationID: 1, EngagementID: 10, TaxpayerID: 100}
	if _, err := c.Check(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if got := openCodes(store); got[CodeFailed] != 1 {
		t.Fatalf("re-run duplicated: %v", got)
	}
	last := q.resolved[len(q.resolved)-1]
	if last.OrganizationID != 1 || last.EngagementID != 10 || last.Period != "2026-11" || len(last.ConnectionIds) != 1 || last.ConnectionIds[0] != 7 {
		t.Fatalf("resolve others = %+v", last)
	}

	// The bank accepts the new credentials: active, last_error cleared.
	q.conns[0].Status, q.conns[0].SetupState, q.conns[0].LastError = "active", "active", nil
	q.conns[0].LastSuccessAt = ts(now)
	rep, err := c.Check(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if rep.AutoResolved != 1 || len(openCodes(store)) != 0 {
		t.Fatalf("recovery: report=%+v open=%v", rep, openCodes(store))
	}
}
