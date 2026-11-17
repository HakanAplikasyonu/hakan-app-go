package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskReportDeliveries sends queued taxpayer report emails (issue #386).
const TaskReportDeliveries = "app:reports:deliveries"

// reportDeliveriesCron: every minute; a delivery is sent within a minute of
// being queued and retried later on failure.
const reportDeliveriesCron = "@every 1m"

// NewReportDeliveriesTask builds the periodic sweep.
func NewReportDeliveriesTask() *asynq.Task {
	return asynq.NewTask(TaskReportDeliveries, []byte("{}"))
}

// ReportDeliveriesFunc sends due deliveries and returns how many were sent.
type ReportDeliveriesFunc func(ctx context.Context) (int, error)

// WithReportDeliveries registers the sweep.
func (w *Worker) WithReportDeliveries(fn ReportDeliveriesFunc) *Worker {
	w.mux.HandleFunc(TaskReportDeliveries, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Info("report_deliveries_sent", "sent", n)
		}
		return err
	})
	return w
}

// RegisterReportDeliveriesSchedule adds the sweep to the scheduler.
func RegisterReportDeliveriesSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		reportDeliveriesCron, NewReportDeliveriesTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(0),
		asynq.Timeout(5*time.Minute), asynq.Unique(50*time.Second),
	); err != nil {
		return fmt.Errorf("queue: register report deliveries: %w", err)
	}
	return nil
}
