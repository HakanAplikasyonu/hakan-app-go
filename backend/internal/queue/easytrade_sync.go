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

// Zamanlanmış Easytrade senkronu (issue #385). The sweep finds connections
// whose next_sync_at has come and enqueues one sync per connection; the
// sync itself moves next_sync_at (interval after success, back-off after a
// failure). Easytrade does not allowlist caller IPs, so both run on the
// general pool.
const (
	TaskEasytradeSweep = "app:easytrade:sweep"
	TaskEasytradeSync  = "app:easytrade:sync"
)

// easytradeSweepCron: due connections are picked within five minutes.
const easytradeSweepCron = "@every 5m"

// EasytradeSyncPayload names the connection.
type EasytradeSyncPayload struct {
	TaxpayerID     int64     `json:"taxpayer_id"`
	ConnectionUUID uuid.UUID `json:"connection_uuid"`
}

// EnqueueEasytradeSync enqueues one connection's sync. Unique keeps one
// queued task per connection while it waits or runs; retries come from the
// schedule, not from the queue (MaxRetry 0).
func EnqueueEasytradeSync(c *Client, p EasytradeSyncPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("queue: marshal easytrade sync: %w", err)
	}
	_, err = c.Enqueue(asynq.NewTask(TaskEasytradeSync, body),
		asynq.Queue("default"), asynq.MaxRetry(0), asynq.Timeout(30*time.Minute), asynq.Unique(30*time.Minute))
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

// WithEasytrade registers the sweep (returns how many it enqueued) and the
// per-connection sync.
func (w *Worker) WithEasytrade(sweep func(ctx context.Context) (int, error), sync func(ctx context.Context, p EasytradeSyncPayload) error) *Worker {
	w.mux.HandleFunc(TaskEasytradeSweep, func(ctx context.Context, _ *asynq.Task) error {
		if sweep == nil {
			return nil
		}
		n, err := sweep(ctx)
		if n > 0 {
			w.log.Info("easytrade_syncs_enqueued", "count", n)
		}
		return err
	})
	w.mux.HandleFunc(TaskEasytradeSync, func(ctx context.Context, task *asynq.Task) error {
		var p EasytradeSyncPayload
		if err := json.Unmarshal(task.Payload(), &p); err != nil {
			return fmt.Errorf("queue: unmarshal easytrade sync: %w", err)
		}
		if sync == nil {
			return nil
		}
		return sync(ctx, p)
	})
	return w
}

// RegisterEasytradeSweepSchedule adds the sweep to the scheduler.
func RegisterEasytradeSweepSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		easytradeSweepCron, asynq.NewTask(TaskEasytradeSweep, []byte("{}")),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(2*time.Minute), asynq.Unique(4*time.Minute),
	); err != nil {
		return fmt.Errorf("queue: register easytrade sweep: %w", err)
	}
	return nil
}
