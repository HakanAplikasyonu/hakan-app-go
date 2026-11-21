package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskSmsDeliveryReports polls NetGSM's delivery report for recently sent
// SMS (issue #780 D, İletişim merkezi).
const TaskSmsDeliveryReports = "app:communication:sms-delivery-reports"

// smsDeliveryReportsCron: every 10 minutes. Operators usually answer within
// minutes; the sweep stops polling a message after three days.
const smsDeliveryReportsCron = "@every 10m"

// NewSmsDeliveryReportsTask builds the periodic sweep.
func NewSmsDeliveryReportsTask() *asynq.Task {
	return asynq.NewTask(TaskSmsDeliveryReports, []byte("{}"))
}

// SmsDeliveryReportsFunc polls the reports and returns how many records
// changed (delivered or failed).
type SmsDeliveryReportsFunc func(ctx context.Context) (int, error)

// WithSmsDeliveryReports registers the sweep.
func (w *Worker) WithSmsDeliveryReports(fn SmsDeliveryReportsFunc) *Worker {
	w.mux.HandleFunc(TaskSmsDeliveryReports, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Info("sms_delivery_reports_updated", "updated", n)
		}
		return err
	})
	return w
}

// RegisterSmsDeliveryReportsSchedule adds the sweep to the scheduler.
func RegisterSmsDeliveryReportsSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		smsDeliveryReportsCron, NewSmsDeliveryReportsTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(0),
		asynq.Timeout(8*time.Minute), asynq.Unique(9*time.Minute),
	); err != nil {
		return fmt.Errorf("queue: register sms delivery reports: %w", err)
	}
	return nil
}
