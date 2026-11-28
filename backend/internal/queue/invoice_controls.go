package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskInvoiceControls runs the accountant controls on the periods that
// received new invoices, across offices (fatura risk uyarıları:
// UNUSUAL_AMOUNT, NEW_PARTY_HIGH_AMOUNT, OUT_OF_PERIOD).
const TaskInvoiceControls = "app:warnings:invoice_controls_sweep"

// invoiceControlsCron: invoices arrive in syncs and uploads through the day;
// an hour is soon enough for a review warning and keeps the load low.
const invoiceControlsCron = "@every 1h"

// NewInvoiceControlsTask builds the periodic sweep.
func NewInvoiceControlsTask() *asynq.Task {
	return asynq.NewTask(TaskInvoiceControls, []byte("{}"))
}

// InvoiceControlsFunc runs the sweep and returns how many warnings opened.
type InvoiceControlsFunc func(ctx context.Context) (int, error)

// WithInvoiceControls registers the sweep.
func (w *Worker) WithInvoiceControls(fn InvoiceControlsFunc) *Worker {
	w.mux.HandleFunc(TaskInvoiceControls, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Info("invoice_control_warnings_opened", "count", n)
		}
		return err
	})
	return w
}

// RegisterInvoiceControlsSchedule adds the sweep to the scheduler.
func RegisterInvoiceControlsSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		invoiceControlsCron, NewInvoiceControlsTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(15*time.Minute), asynq.Unique(55*time.Minute),
	); err != nil {
		return fmt.Errorf("queue: register invoice controls: %w", err)
	}
	return nil
}
