package entegrator

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testfakes"
)

// schedRecorder is the fake integrator that records every listing's From and can
// fail both boxes or block until released.
type schedRecorder struct {
	*spy
	mu      sync.Mutex
	froms   []time.Time
	err     error
	entered chan struct{}
	release chan struct{}
}

func (r *schedRecorder) list(ctx context.Context, q provider.ListQuery, next func(context.Context, provider.ListQuery) (*provider.Page, error)) (*provider.Page, error) {
	r.mu.Lock()
	r.froms = append(r.froms, q.From)
	err, entered, release := r.err, r.entered, r.release
	r.entered = nil
	r.mu.Unlock()
	if entered != nil {
		close(entered)
		<-release
	}
	if err != nil {
		return nil, err
	}
	return next(ctx, q)
}

func (r *schedRecorder) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	return r.list(ctx, q, r.Fake.ListInbox)
}

func (r *schedRecorder) ListOutbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	return r.list(ctx, q, r.Fake.ListOutbox)
}

func (r *schedRecorder) takeFroms() []time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.froms
	r.froms = nil
	return out
}

// scheduledEnv is a set-up (active) EDM connection behind the recording fake.
func scheduledEnv(t *testing.T) (*env, *schedRecorder) {
	t.Helper()
	e := setupSystem(t, Config{}, "entegrator_edm", "credentials_entered")
	e.setStatus(t, "active")
	r := &schedRecorder{spy: e.spy}
	e.svc.WithOpener(func(context.Context, db.SourceConnection, string) (*Client, error) { return Wrap(r), nil })
	return e, r
}

func (e *env) setStatus(t *testing.T, status string) {
	t.Helper()
	if _, err := e.q.SetSourceConnectionStatus(context.Background(), db.SetSourceConnectionStatusParams{
		TaxpayerID: e.tp, ID: e.conn.ID, Status: status,
	}); err != nil {
		t.Fatal(err)
	}
}

func (e *env) row(t *testing.T) db.SourceConnection {
	t.Helper()
	r, err := e.q.GetSourceConnection(context.Background(), db.GetSourceConnectionParams{TaxpayerID: e.tp, Uuid: e.conn.Uuid})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// ageRuns moves the connection's runs to an hour before the test clock:
// sync_runs.started_at is the database clock, and EDM's MinPollInterval
// (15 minutes) compares it with the test clock.
func (e *env) ageRuns(t *testing.T) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), `UPDATE sync_runs SET started_at = $2, finished_at = $2 WHERE connection_id = $1`,
		e.conn.ID, e.now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
}

func (e *env) addConnection(t *testing.T, tp int64, system, setupState, status string, next *time.Time) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	c, err := e.q.CreateSourceConnection(ctx, db.CreateSourceConnectionParams{TaxpayerID: tp, SourceSystem: system, SetupState: setupState, Meta: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.q.SetSourceConnectionStatus(ctx, db.SetSourceConnectionStatusParams{TaxpayerID: tp, ID: c.ID, Status: status}); err != nil {
		t.Fatal(err)
	}
	if next != nil {
		if err := e.q.SetEntegratorSyncSchedule(ctx, db.SetEntegratorSyncScheduleParams{
			SyncIntervalMinutes: pgtype.Int4{Int32: 60, Valid: true}, NextSyncAt: ts(*next), TaxpayerID: tp, ID: c.ID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return c.Uuid
}

// Zamanlayıcı yalnızca kurulumu bitmiş, etkin İşNet/EDM bağlantılarını
// seçer: artımlı çekim zamanı gelenleri (henüz zamanlanmamış eski
// bağlantılar dahil), gece taraması bütün etkinleri. Kimliği reddedilmiş,
// ilk çekimi bekleyen ya da zamanlanmayan entegratörün bağlantısı seçilmez.
func TestScheduleSelectsConnections(t *testing.T) {
	ctx := context.Background()
	e := setupSystem(t, Config{}, "entegrator_izibiz", "credentials_entered")
	e.setStatus(t, "active") // not a scheduled integrator
	tp2 := e.d.Taxpayer(t, "legal", "4840847211")
	tp3 := e.d.Taxpayer(t, "legal", "9000000017")
	past, future := e.now.Add(-time.Minute), e.now.Add(30*time.Minute)

	legacy := e.addConnection(t, e.tp, "entegrator_edm", "credentials_entered", "active", nil) // set up before the schedule
	later := e.addConnection(t, e.tp, "entegrator_isnet", "active", "active", &future)
	due := e.addConnection(t, tp2, "entegrator_isnet", "active", "rate_limited", &past)
	e.addConnection(t, tp2, "entegrator_edm", "active", "auth_failed", &past)
	e.addConnection(t, tp3, "entegrator_edm", "credentials_entered", "pending", nil) // first load still waiting
	e.addConnection(t, tp3, "entegrator_isnet", "draft", "active", &past)

	type job struct {
		id   uuid.UUID
		kind string
	}
	var got []job
	collect := func(_ context.Context, _ int64, id uuid.UUID, kind string) error {
		got = append(got, job{id, kind})
		return nil
	}
	n, err := e.svc.EnqueueDue(ctx, collect)
	if err != nil {
		t.Fatal(err)
	}
	// Never-scheduled first, then by next_sync_at.
	if n != 2 || !slices.Equal(got, []job{{legacy, RunIncremental}, {due, RunIncremental}}) {
		t.Fatalf("due: %d %v", n, got)
	}
	got = nil
	n, err = e.svc.EnqueueNightly(ctx, collect)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 || len(got) != 3 {
		t.Fatalf("nightly: %d %v", n, got)
	}
	for _, j := range got {
		if j.kind != RunFullScan || (j.id != legacy && j.id != later && j.id != due) {
			t.Fatalf("nightly: %v", got)
		}
	}
}

// Artımlı pencere son başarılı çalışmanın başından 15 dk önce başlar, ama en
// az son 3 günü kapsar; uzun kesintiden sonra kesintinin başına kadar iner.
// Başarılı çalışma sıradaki çekimi bir saat sonraya yazar ve kurulumu bitirir.
func TestIncrementalWindowSinceLastSuccess(t *testing.T) {
	ctx := context.Background()
	e, r := scheduledEnv(t)

	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental); err != nil {
		t.Fatal(err)
	}
	for _, f := range r.takeFroms() {
		if !f.Equal(e.now.Add(-72 * time.Hour)) {
			t.Fatalf("first run from %s, want 72h back", f)
		}
	}
	row := e.row(t)
	if row.SetupState != "active" || row.SyncFailures != 0 || !row.SyncIntervalMinutes.Valid || row.SyncIntervalMinutes.Int32 != 60 ||
		!row.NextSyncAt.Valid || !row.NextSyncAt.Time.Equal(e.now.Add(time.Hour)) {
		t.Fatalf("after success: setup=%s failures=%d interval=%v next=%v", row.SetupState, row.SyncFailures, row.SyncIntervalMinutes, row.NextSyncAt)
	}

	// The last success began five days ago (an outage since): the window
	// reaches back to it, with the overlap.
	lastStart := e.now
	if _, err := e.pool.Exec(ctx, `UPDATE sync_runs SET started_at = $2, finished_at = $2 WHERE connection_id = $1`, e.conn.ID, lastStart); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(5 * 24 * time.Hour)
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental); err != nil {
		t.Fatal(err)
	}
	froms := r.takeFroms()
	if len(froms) == 0 {
		t.Fatal("nothing listed")
	}
	for _, f := range froms {
		if !f.Equal(lastStart.Add(-IncrementalOverlap)) {
			t.Fatalf("after an outage from %s, want %s", f, lastStart.Add(-IncrementalOverlap))
		}
	}

	// A success an hour ago: still the last three days (late documents).
	if _, err := e.pool.Exec(ctx, `UPDATE sync_runs SET started_at = $2, finished_at = $2 WHERE connection_id = $1`, e.conn.ID, e.now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(time.Hour)
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental); err != nil {
		t.Fatal(err)
	}
	for _, f := range r.takeFroms() {
		if !f.Equal(e.now.Add(-72 * time.Hour)) {
			t.Fatalf("recent success: from %s, want 72h back", f)
		}
	}
}

func TestIncrementalStart(t *testing.T) {
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, istanbul)
	if got := incrementalStart(now, time.Time{}, 72*time.Hour); !got.Equal(now.Add(-72 * time.Hour)) {
		t.Fatalf("no success: %s", got)
	}
	if got := incrementalStart(now, now.Add(-100*24*time.Hour), 72*time.Hour); !got.Equal(now.Add(-MaxIncrementalSpan)) {
		t.Fatalf("capped: %s", got)
	}
}

// Gece taraması içinde bulunulan ay ile önceki ayı yeniden listeler; sıradaki
// gece taraması İstanbul saatiyle 03:00.
func TestNightlyScan(t *testing.T) {
	ctx := context.Background()
	e, r := scheduledEnv(t)
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunFullScan); err != nil {
		t.Fatal(err)
	}
	starts := map[string]bool{}
	for _, f := range r.takeFroms() {
		starts[f.In(istanbul).Format(time.DateOnly)] = true
	}
	if len(starts) != 2 || !starts["2026-11-01"] || !starts["2026-10-01"] {
		t.Fatalf("nightly scan windows start %v", starts)
	}
	if got := NextNightly(mustTime(t, "2026-11-15T10:00:00+03:00")); !got.Equal(mustTime(t, "2026-11-16T03:00:00+03:00")) {
		t.Fatalf("next nightly %s", got)
	}
	if got := NextNightly(mustTime(t, "2026-11-15T02:59:00+03:00")); !got.Equal(mustTime(t, "2026-11-15T03:00:00+03:00")) {
		t.Fatalf("next nightly before 03:00 %s", got)
	}
}

// Aynı bağlantıda aynı anda tek çalışma: ikincisi ErrBusy alır. Zamanlanmış
// artımlı çekim bunu sessizce geçer (tarama yeniden seçer); gece taraması
// kuyruğun yeniden denemesi için hatayı döndürür.
func TestScheduledRunLock(t *testing.T) {
	ctx := context.Background()
	e, r := scheduledEnv(t)
	r.entered, r.release = make(chan struct{}), make(chan struct{})
	entered := r.entered
	done := make(chan error, 1)
	go func() {
		_, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental)
		done <- err
	}()
	<-entered
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental); !errors.Is(err, ErrBusy) {
		t.Fatalf("second run: %v", err)
	}
	if err := e.svc.RunScheduledTask(ctx, e.tp, e.conn.Uuid, RunIncremental); err != nil {
		t.Fatalf("busy incremental task: %v", err)
	}
	if err := e.svc.RunScheduledTask(ctx, e.tp, e.conn.Uuid, RunFullScan); !errors.Is(err, ErrBusy) {
		t.Fatalf("busy nightly task: %v", err)
	}
	close(r.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT count(*) FROM sync_runs WHERE taxpayer_id = $1`); n != 1 {
		t.Fatalf("runs = %d", n)
	}
}

// Geçici hata (zaman aşımı, entegratör kapalı) bağlantıyı auth_failed yapmaz:
// durum active kalır, son hata yazılır, sıradaki deneme 5 dk, 10 dk, 20 dk …
// sonra (en çok 4 saat). Başarı sayacı sıfırlar; gerçek kimlik hatası
// auth_failed yapar ve zamanlamayı durdurur.
func TestTransientFailureBacksOff(t *testing.T) {
	ctx := context.Background()
	e, r := scheduledEnv(t)
	r.err = &provider.Error{Kind: provider.KindTimeout, Provider: "fake", Message: "deadline"}
	for i, wait := range []time.Duration{5 * time.Minute, 10 * time.Minute, 20 * time.Minute} {
		rep, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental)
		if err != nil || rep.Status != "failed" {
			t.Fatalf("run %d: %v %+v", i+1, err, rep)
		}
		row := e.row(t)
		if row.Status != "active" || int(row.SyncFailures) != i+1 || !row.NextSyncAt.Time.Equal(e.now.Add(wait)) ||
			!strings.Contains(string(row.LastError), `"timeout"`) {
			t.Fatalf("run %d: status=%s failures=%d next=%s error=%s", i+1, row.Status, row.SyncFailures, row.NextSyncAt.Time, row.LastError)
		}
	}
	if Backoff(30) != MaxBackoff || Backoff(1) != RetryBase {
		t.Fatalf("backoff bounds: %s %s", Backoff(1), Backoff(30))
	}
	// Still due for the sweep (the back-off has passed), not stopped.
	e.now = e.now.Add(time.Hour)
	if n, _ := e.svc.EnqueueDue(ctx, func(context.Context, int64, uuid.UUID, string) error { return nil }); n != 1 {
		t.Fatalf("failing connection not retried: %d", n)
	}

	r.err = nil
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental); err != nil {
		t.Fatal(err)
	}
	row := e.row(t)
	if row.Status != "active" || row.SyncFailures != 0 || len(row.LastError) != 0 || !row.NextSyncAt.Time.Equal(e.now.Add(time.Hour)) {
		t.Fatalf("after recovery: status=%s failures=%d error=%s", row.Status, row.SyncFailures, row.LastError)
	}

	r.err = &provider.Error{Kind: provider.KindAuth, Provider: "fake", Message: "bad password"}
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental); provider.KindOf(err) != provider.KindAuth {
		t.Fatalf("auth: %v", err)
	}
	if row := e.row(t); row.Status != "auth_failed" || row.SyncFailures != 1 {
		t.Fatalf("auth failure: status=%s failures=%d", row.Status, row.SyncFailures)
	}
	e.now = e.now.Add(5 * time.Hour)
	if n, _ := e.svc.EnqueueDue(ctx, func(context.Context, int64, uuid.UUID, string) error { return nil }); n != 0 {
		t.Fatalf("auth_failed connection scheduled: %d", n)
	}
	// The queue handler records the failure and does not ask for a retry.
	if err := e.svc.RunScheduledTask(ctx, e.tp, e.conn.Uuid, RunIncremental); err != nil {
		t.Fatalf("scheduled task: %v", err)
	}
}

// İşNet IP tanımlamayan çağrıyı yanıtsız bırakır; tek seferlik ağ kesintisi
// de aynı görünür (32 §9). Kurulu bağlantıda bu ancak üst üste
// IPNotAllowedAfter kez olursa auth_failed olur; ilk çekimde ise hemen
// (IP tanımı eksik, ofis bir şey yapmalı).
func TestIsnetUnansweredCallsAreTransientOnce(t *testing.T) {
	ctx := context.Background()
	e := setupSystem(t, Config{}, "entegrator_isnet", "credentials_entered")
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	e.svc.WithOpener(func(_ context.Context, _ db.SourceConnection, own string) (*Client, error) {
		return Open("isnet", provider.Config{VKN: own, Endpoint: srv.URL, Options: map[string]string{
			"addressbook_endpoint": srv.URL, "ip_filtered": "true", "timeout": "200ms",
		}})
	})

	// First load (not set up yet): auth_failed at once.
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunBackfill); !ipNotAllowed(err) {
		t.Fatalf("first load: %v", err)
	}
	if row := e.row(t); row.Status != "auth_failed" {
		t.Fatalf("first load status %s", row.Status)
	}

	// Set up connection: transient twice, auth_failed the third time.
	e.setStatus(t, "active")
	if _, err := e.pool.Exec(ctx, `UPDATE source_connections SET sync_failures = 0 WHERE id = $1`, e.conn.ID); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= IPNotAllowedAfter; i++ {
		e.now = e.now.Add(time.Hour)
		if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental); !ipNotAllowed(err) {
			t.Fatalf("run %d: %v", i, err)
		}
		want := "active"
		if i == IPNotAllowedAfter {
			want = "auth_failed"
		}
		if row := e.row(t); row.Status != want || int(row.SyncFailures) != i {
			t.Fatalf("run %d: status=%s failures=%d, want %s", i, row.Status, row.SyncFailures, want)
		}
	}
}

// EDM sahte sunucusuyla zamanlanmış artımlı çekim: yeni faturalar gelir,
// ikinci çalışma çift kayıt yazmaz. Servis kapalıyken (503) bağlantı
// auth_failed olmaz; ekran son hatayı sır içermeyen Türkçe metinle, ardışık
// hata sayısını, son ve sıradaki çekimi gösterir.
func TestEDMScheduledSyncAgainstFake(t *testing.T) {
	ctx := context.Background()
	e := setupSystem(t, Config{}, "entegrator_edm", "credentials_entered")
	e.setStatus(t, "active")
	f := &testfakes.EDM{}
	srv := testfakes.Serve(t, f)
	e.svc.WithOpener(func(_ context.Context, _ db.SourceConnection, own string) (*Client, error) {
		return Open("edm", provider.Config{Username: testfakes.EDMUsername, Password: testfakes.EDMPassword, Endpoint: srv.URL, VKN: own})
	})
	e.now = mustTime(t, "2026-11-15T10:00:00+03:00")
	edmAdd(t, f, 1, "IN", "2026-11-14T09:00:00")
	edmAdd(t, f, 2, "IN", "2026-11-15T08:30:00")

	rep, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental)
	if err != nil || rep.Status != "success" || rep.Counters.New != 2 {
		t.Fatalf("first run: %v %+v", err, rep)
	}
	e.now = e.now.Add(time.Hour)
	e.ageRuns(t)
	if rep, err = e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental); err != nil || rep.Counters.New != 0 || rep.Counters.Listed != 2 {
		t.Fatalf("second run: %v %+v", err, rep)
	}
	if n := e.count(t, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`); n != 2 {
		t.Fatalf("invoices = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM entegrator_documents WHERE taxpayer_id = $1`); n != 2 {
		t.Fatalf("documents = %d", n)
	}

	f.Down = true
	e.now = e.now.Add(time.Hour)
	e.ageRuns(t)
	if rep, err = e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental); err != nil || rep.Status != "failed" {
		t.Fatalf("down: %v %+v", err, rep)
	}
	row := e.row(t)
	if row.Status != "active" || row.SyncFailures != 1 || !row.NextSyncAt.Time.Equal(e.now.Add(RetryBase)) {
		t.Fatalf("down: status=%s failures=%d next=%s", row.Status, row.SyncFailures, row.NextSyncAt.Time)
	}

	conns := NewConnections(e.pool, nil, nil).WithClock(func() time.Time { return e.now })
	v, err := conns.Get(ctx, e.tp, e.conn.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	s := v.Schedule
	if v.State != "active" || s == nil || !s.Active || s.IntervalMinutes != 60 || s.ConsecutiveFailures != 1 ||
		s.NextSyncAt == nil || !s.NextSyncAt.Equal(e.now.Add(RetryBase)) ||
		s.NextNightlyAt == nil || !s.NextNightlyAt.Equal(mustTime(t, "2026-11-16T03:00:00+03:00")) ||
		s.LastSyncAt == nil || s.LastError == nil || s.LastError.Kind == "" || s.LastError.Kind == "auth" {
		t.Fatalf("view: state=%s schedule=%+v", v.State, s)
	}
	if s.LastError.Message != FailureText(s.LastError.Kind, false) || strings.Contains(s.LastError.Message, srv.URL) {
		t.Fatalf("error text %q", s.LastError.Message)
	}

	f.Down = false
	e.now = e.now.Add(RetryBase)
	e.ageRuns(t)
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental); err != nil {
		t.Fatal(err)
	}
	if v, _ = conns.Get(ctx, e.tp, e.conn.Uuid); v.Schedule.ConsecutiveFailures != 0 || v.Schedule.LastError != nil {
		t.Fatalf("after recovery: %+v", v.Schedule)
	}
	// A non-scheduled integrator has no schedule in its view.
	other := e.addConnection(t, e.tp, "entegrator_izibiz", "credentials_entered", "active", nil)
	if v, _ = conns.Get(ctx, e.tp, other); v.Schedule != nil {
		t.Fatalf("izibiz schedule: %+v", v.Schedule)
	}
}
