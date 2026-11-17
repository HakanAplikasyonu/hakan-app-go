package easytrade

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testfakes"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testutil/contract"
)

// TestScheduledSync is #385's schedule: a new connection is due at once,
// a scheduled run reads the open periods window by window and moves
// next_sync_at by the interval; a failing window makes the run partial and
// the next run comes after the back-off.
func TestScheduledSync(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	tp := d.Taxpayer(t, "legal", "9000000017")
	f := testfakes.EasytradeSample()
	fault := &contract.Fault{Next: f, Status: http.StatusInternalServerError, Times: 0, Match: contract.PathPrefix("/company/expenses")}
	srv := httptest.NewServer(fault)
	t.Cleanup(srv.Close)
	kr, _ := secrets.ParseKeyring("", "", "test-legacy-key-32-bytes-long!!!")
	svc := NewService(d.Pool, secrets.New(kr, secrets.NewPGStore(db.New(d.Pool))), srv.URL, srv.Client())
	svc.interval = 0
	now := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	c, err := svc.Connect(ctx, tp, "a@b.c", testfakes.EasytradePassword, testfakes.EasytradeCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	if c.SyncIntervalMinutes == nil || *c.SyncIntervalMinutes != 240 || c.NextSyncAt == nil || !c.NextSyncAt.Equal(now) {
		t.Fatalf("default schedule: %+v", c)
	}
	var due []uuid.UUID
	collect := func(_ context.Context, _ int64, id uuid.UUID) error { due = append(due, id); return nil }
	if n, err := svc.EnqueueDue(ctx, collect); err != nil || n != 1 || due[0] != c.UUID {
		t.Fatalf("due: %d %v %v", n, due, err)
	}

	res, err := svc.RunScheduled(ctx, tp, c.UUID)
	if err != nil {
		t.Fatal(err)
	}
	// Oct + Nov × invoices + expenses.
	if res.Status != "success" || res.Kind != RunScheduled || res.Counters.WindowsDone != 4 || res.Counters.InvoicesCreated != 4 || res.Counters.ExpensesCreated != 1 {
		t.Fatalf("run: %+v", res)
	}
	list, _ := svc.List(ctx, tp)
	got := list[0]
	if got.NextSyncAt == nil || !got.NextSyncAt.Equal(now.Add(4*time.Hour)) || got.SyncFailures != 0 || got.LastRun == nil ||
		got.LastRun.Status != "success" || got.LastSyncAt == nil || got.LastSuccessAt == nil {
		t.Fatalf("after success: %+v", got)
	}
	due = nil
	if n, _ := svc.EnqueueDue(ctx, collect); n != 0 {
		t.Fatalf("not due yet, enqueued %d", n)
	}
	var windows int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM sync_windows w JOIN source_connections c ON c.id = w.connection_id WHERE c.uuid = $1 AND w.status = 'done'`, c.UUID).Scan(&windows)
	if windows != 4 {
		t.Fatalf("%d done windows", windows)
	}

	// Expenses answer 500: invoices still land, the run is partial, retry in 5 minutes.
	fault.Times = -1
	now = now.Add(4 * time.Hour)
	res, err = svc.RunScheduled(ctx, tp, c.UUID)
	if err != nil || res.Status != "partial" || res.Counters.WindowsFailed != 2 || res.Counters.InvoicesExisting != 4 {
		t.Fatalf("partial run: %+v %v", res, err)
	}
	list, _ = svc.List(ctx, tp)
	if list[0].SyncFailures != 1 || !list[0].NextSyncAt.Equal(now.Add(RetryBase)) || list[0].Status != "active" {
		t.Fatalf("after partial: %+v", list[0])
	}
	now = now.Add(RetryBase)
	res, _ = svc.RunScheduled(ctx, tp, c.UUID)
	list, _ = svc.List(ctx, tp)
	if res.Status != "partial" || list[0].SyncFailures != 2 || !list[0].NextSyncAt.Equal(now.Add(2*RetryBase)) {
		t.Fatalf("second failure: %+v %+v", res, list[0])
	}
	if err := svc.RunTask(ctx, tp, c.UUID); err != nil {
		t.Fatalf("source failures do not fail the task: %v", err)
	}

	// Recovered: back to the interval.
	fault.Times = 0
	now = now.Add(time.Hour)
	if res, err := svc.RunScheduled(ctx, tp, c.UUID); err != nil || res.Status != "success" {
		t.Fatalf("recovered: %+v %v", res, err)
	}
	list, _ = svc.List(ctx, tp)
	if list[0].SyncFailures != 0 || !list[0].NextSyncAt.Equal(now.Add(4*time.Hour)) {
		t.Fatalf("after recovery: %+v", list[0])
	}

	// One run at a time per connection.
	unlock, err := svc.lock(ctx, mustID(t, d, c.UUID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunScheduled(ctx, tp, c.UUID); !errors.Is(err, ErrBusy) {
		t.Fatalf("while locked: %v", err)
	}
	unlock()

	runs, err := svc.Runs(ctx, tp, c.UUID, 20)
	if err != nil || len(runs) != 5 || runs[0].Status != "success" || runs[1].Status != "partial" || runs[1].Error != "" {
		t.Fatalf("runs: %+v %v", runs, err)
	}

	// Schedule off, invalid interval, back on.
	if _, err := svc.SetSchedule(ctx, tp, c.UUID, 10*time.Minute); !errors.Is(err, ErrBadInterval) {
		t.Fatalf("10m: %v", err)
	}
	off, err := svc.SetSchedule(ctx, tp, c.UUID, 0)
	if err != nil || off.SyncIntervalMinutes != nil || off.NextSyncAt != nil {
		t.Fatalf("off: %+v %v", off, err)
	}
	now = now.Add(48 * time.Hour)
	if n, _ := svc.EnqueueDue(ctx, collect); n != 0 {
		t.Fatalf("schedule off, enqueued %d", n)
	}
	on, err := svc.SetSchedule(ctx, tp, c.UUID, time.Hour)
	if err != nil || *on.SyncIntervalMinutes != 60 || !on.NextSyncAt.Equal(now) {
		t.Fatalf("on: %+v %v", on, err)
	}

	// License expired: the connection is disabled and drops off the schedule.
	f.License = false
	if _, err := svc.RunScheduled(ctx, tp, c.UUID); !errors.Is(err, ErrLicense) {
		t.Fatalf("license: %v", err)
	}
	now = now.Add(48 * time.Hour)
	if n, _ := svc.EnqueueDue(ctx, collect); n != 0 {
		t.Fatalf("disabled connection enqueued")
	}
	if err := svc.RunTask(ctx, tp, c.UUID); err != nil {
		t.Fatalf("disabled: %v", err)
	}
}

func mustID(t *testing.T, d *testdb.DB, id uuid.UUID) int64 {
	t.Helper()
	var n int64
	if err := d.Pool.QueryRow(context.Background(), `SELECT id FROM source_connections WHERE uuid = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
