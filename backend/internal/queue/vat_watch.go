package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskVATWatch is the daily VAT watch ("KDV nöbetçisi", #780 B).
const TaskVATWatch = "app:vat:watch"

// vatWatchCron: every morning at 07:00 Istanbul. Events are written once
// per engagement and period, so a rerun raises nothing twice.
const vatWatchCron = "CRON_TZ=Europe/Istanbul 0 7 * * *"

// NewVATWatchTask builds the daily sweep.
func NewVATWatchTask() *asynq.Task {
	return asynq.NewTask(TaskVATWatch, []byte("{}"))
}

// VATWatchFunc runs the sweep and returns how many events it raised.
type VATWatchFunc func(ctx context.Context) (int, error)

// WithVATWatch registers the sweep.
func (w *Worker) WithVATWatch(fn VATWatchFunc) *Worker {
	w.mux.HandleFunc(TaskVATWatch, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Info("vat_watch_events_raised", "events", n)
		}
		return err
	})
	return w
}

// RegisterVATWatchSchedule adds the sweep to the scheduler.
func RegisterVATWatchSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		vatWatchCron, NewVATWatchTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(30*time.Minute), asynq.Unique(time.Hour),
	); err != nil {
		return fmt.Errorf("queue: register vat watch: %w", err)
	}
	return nil
}
