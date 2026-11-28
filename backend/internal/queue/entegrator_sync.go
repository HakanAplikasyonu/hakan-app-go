package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// TaskEntegratorSync runs one integrator sync of a source connection (issue
// #40, 30 §7). It runs on QueueFixedEgress (EntegratorSyncOptions, issue
// #45): integrators allowlist the caller IP. The run itself takes a per-connection
// lock, so duplicate tasks end with entegrator.ErrBusy.
const TaskEntegratorSync = "app:entegrator:sync"

// EntegratorSyncPayload names the connection and the run kind (backfill,
// incremental, full_scan, manual).
type EntegratorSyncPayload struct {
	TaxpayerID     int64     `json:"taxpayer_id"`
	ConnectionUUID uuid.UUID `json:"connection_uuid"`
	Kind           string    `json:"kind"`
	// Scheduled: queued by the sweep or the nightly scan; integrator
	// failures are retried by the schedule, not by the queue.
	Scheduled bool `json:"scheduled,omitempty"`
}

// NewEntegratorSyncTask builds the task.
func NewEntegratorSyncTask(p EntegratorSyncPayload) (*asynq.Task, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("queue: marshal entegrator sync: %w", err)
	}
	return asynq.NewTask(TaskEntegratorSync, body), nil
}

// WithEntegratorSync registers the integrator sync runner.
func (w *Worker) WithEntegratorSync(fn func(ctx context.Context, p EntegratorSyncPayload) error) *Worker {
	w.mux.HandleFunc(TaskEntegratorSync, func(ctx context.Context, task *asynq.Task) error {
		var p EntegratorSyncPayload
		if err := json.Unmarshal(task.Payload(), &p); err != nil {
			return fmt.Errorf("queue: unmarshal entegrator sync: %w", err)
		}
		return fn(ctx, p)
	})
	return w
}

// Düzenli entegratör senkronu (çapraz denetim A1 "Kalan", 30 §7): the sweep
// enqueues an incremental run for every İşNet/EDM connection whose
// next_sync_at has come (hourly, back-off after failures); the nightly task
// enqueues a full scan of the open months for every such connection. Both
// are scheduled by the general pool on the maintenance queue and the runs
// go to the fixed-egress pool.
const (
	TaskEntegratorSweep   = "app:entegrator:sweep"
	TaskEntegratorNightly = "app:entegrator:nightly"
)

const (
	// entegratorSweepCron: due connections are picked within five minutes.
	entegratorSweepCron = "@every 5m"
	// entegratorNightlyCron: 03:00 Istanbul (entegrator.NightlyHour).
	entegratorNightlyCron = "CRON_TZ=Europe/Istanbul 0 3 * * *"
)

// ScheduledEntegratorSyncOptions are the enqueue options of a scheduled
// run. Unique keeps one queued task per connection and kind while it waits
// or runs. An incremental run is not retried by the queue (the schedule
// does it); a nightly scan that found the connection busy is retried.
func ScheduledEntegratorSyncOptions(kind string) []asynq.Option {
	if kind == "full_scan" {
		return []asynq.Option{asynq.Queue(QueueFixedEgress), asynq.MaxRetry(5),
			asynq.Timeout(2 * time.Hour), asynq.Unique(2 * time.Hour)}
	}
	return []asynq.Option{asynq.Queue(QueueFixedEgress), asynq.MaxRetry(0),
		asynq.Timeout(30 * time.Minute), asynq.Unique(30 * time.Minute)}
}

// EnqueueScheduledEntegratorSync enqueues one scheduled run; a duplicate of
// a task still queued or running is not an error.
func EnqueueScheduledEntegratorSync(c *Client, p EntegratorSyncPayload) error {
	p.Scheduled = true
	task, err := NewEntegratorSyncTask(p)
	if err != nil {
		return err
	}
	_, err = c.Enqueue(task, ScheduledEntegratorSyncOptions(p.Kind)...)
	if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

// WithEntegratorSchedule registers the sweep and the nightly scan; each
// returns how many runs it enqueued.
func (w *Worker) WithEntegratorSchedule(sweep, nightly func(ctx context.Context) (int, error)) *Worker {
	w.mux.HandleFunc(TaskEntegratorSweep, func(ctx context.Context, _ *asynq.Task) error {
		if sweep == nil {
			return nil
		}
		n, err := sweep(ctx)
		if n > 0 {
			w.log.Info("entegrator_syncs_enqueued", "count", n, "kind", "incremental")
		}
		return err
	})
	w.mux.HandleFunc(TaskEntegratorNightly, func(ctx context.Context, _ *asynq.Task) error {
		if nightly == nil {
			return nil
		}
		n, err := nightly(ctx)
		if n > 0 {
			w.log.Info("entegrator_syncs_enqueued", "count", n, "kind", "full_scan")
		}
		return err
	})
	return w
}

// RegisterEntegratorSchedule adds the sweep and the nightly scan to the
// scheduler.
func RegisterEntegratorSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		entegratorSweepCron, asynq.NewTask(TaskEntegratorSweep, []byte("{}")),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(2*time.Minute), asynq.Unique(4*time.Minute),
	); err != nil {
		return fmt.Errorf("queue: register entegrator sweep: %w", err)
	}
	if _, err := scheduler.Register(
		entegratorNightlyCron, asynq.NewTask(TaskEntegratorNightly, []byte("{}")),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(2),
		asynq.Timeout(10*time.Minute), asynq.Unique(time.Hour),
	); err != nil {
		return fmt.Errorf("queue: register entegrator nightly scan: %w", err)
	}
	return nil
}
