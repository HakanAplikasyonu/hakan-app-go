package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskStaleReceiptReads fails the receipt captures whose background AI read
// did not finish (#780): a server restart or crash during the read leaves
// them "processing"; the office's "Okunamayan fiş" flow then shows them.
const TaskStaleReceiptReads = "app:receipts:stale_reads"

// A capture is stale after 10 minutes; sweeping every 3 minutes fails it
// within 13 minutes of the upload.
const staleReceiptReadsCron = "@every 3m"

// NewStaleReceiptReadsTask builds the periodic sweep.
func NewStaleReceiptReadsTask() *asynq.Task {
	return asynq.NewTask(TaskStaleReceiptReads, []byte("{}"))
}

// StaleReceiptReadsFunc fails stale captures and returns how many.
type StaleReceiptReadsFunc func(ctx context.Context) (int, error)

// WithStaleReceiptReads registers the sweep.
func (w *Worker) WithStaleReceiptReads(fn StaleReceiptReadsFunc) *Worker {
	w.mux.HandleFunc(TaskStaleReceiptReads, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Warn("receipt_reads_failed_stale", "count", n)
		}
		return err
	})
	return w
}

// RegisterStaleReceiptReadsSchedule adds the 3-minute sweep.
func RegisterStaleReceiptReadsSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		staleReceiptReadsCron, NewStaleReceiptReadsTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(2*time.Minute), asynq.Unique(2*time.Minute),
	); err != nil {
		return fmt.Errorf("queue: register stale receipt reads: %w", err)
	}
	return nil
}
