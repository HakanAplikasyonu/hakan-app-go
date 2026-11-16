package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskBlockerAlerts tells offices about new blocking warnings (issue #241).
const TaskBlockerAlerts = "app:office:blocker_alerts"

// blockerAlertsCron: every 10 minutes; each warning is announced once.
const blockerAlertsCron = "@every 10m"

// NewBlockerAlertsTask builds the periodic sweep.
func NewBlockerAlertsTask() *asynq.Task {
	return asynq.NewTask(TaskBlockerAlerts, []byte("{}"))
}

// BlockerAlertsFunc sends the alerts and returns how many were enqueued.
type BlockerAlertsFunc func(ctx context.Context) (int, error)

// WithBlockerAlerts registers the sweep.
func (w *Worker) WithBlockerAlerts(fn BlockerAlertsFunc) *Worker {
	w.mux.HandleFunc(TaskBlockerAlerts, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Info("blocker_alerts_sent", "notified", n)
		}
		return err
	})
	return w
}

// RegisterBlockerAlertsSchedule adds the sweep to the scheduler.
func RegisterBlockerAlertsSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		blockerAlertsCron, NewBlockerAlertsTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(5*time.Minute), asynq.Unique(9*time.Minute),
	); err != nil {
		return fmt.Errorf("queue: register blocker alerts: %w", err)
	}
	return nil
}
