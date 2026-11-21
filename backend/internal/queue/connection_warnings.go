package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskConnectionWarnings refreshes the connection health warnings of every
// office (#669 K, INT_CONNECTION_FAILED / INT_SYNC_STALE).
const TaskConnectionWarnings = "app:warnings:connection_sweep"

// connectionWarningsCron: a failure written by any sync (entegratör, banka,
// Paraşüt, KolayBi, Easytrade) reaches the Komuta Merkezi within 15 minutes;
// the stale thresholds are days, so a finer pace buys nothing.
const connectionWarningsCron = "@every 15m"

// NewConnectionWarningsTask builds the periodic sweep.
func NewConnectionWarningsTask() *asynq.Task {
	return asynq.NewTask(TaskConnectionWarnings, []byte("{}"))
}

// ConnectionWarningsFunc runs the sweep and returns how many warnings opened.
type ConnectionWarningsFunc func(ctx context.Context) (int, error)

// WithConnectionWarnings registers the sweep.
func (w *Worker) WithConnectionWarnings(fn ConnectionWarningsFunc) *Worker {
	w.mux.HandleFunc(TaskConnectionWarnings, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Info("connection_warnings_opened", "count", n)
		}
		return err
	})
	return w
}

// RegisterConnectionWarningsSchedule adds the sweep to the scheduler.
func RegisterConnectionWarningsSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		connectionWarningsCron, NewConnectionWarningsTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(5*time.Minute), asynq.Unique(14*time.Minute),
	); err != nil {
		return fmt.Errorf("queue: register connection warnings: %w", err)
	}
	return nil
}
