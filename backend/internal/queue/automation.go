package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskAutomationTick runs the office automation rules (#780 C, Ayarlar >
// Otomasyonlar): calendar and payment rules due now, database events
// (tahakkuk oluştu, beyanname onaylandı, belge gelmedi), scheduled runs,
// "e-posta açılmadıysa SMS" and "hâlâ ödenmediyse tekrar gönder".
const TaskAutomationTick = "app:automation:tick"

// automationCron: calendar rules fire at a given hour; five minutes keeps
// them on time without loading the database.
const automationCron = "@every 5m"

// NewAutomationTickTask builds the periodic tick.
func NewAutomationTickTask() *asynq.Task {
	return asynq.NewTask(TaskAutomationTick, []byte("{}"))
}

// AutomationTickFunc runs one tick and returns how many runs it processed.
type AutomationTickFunc func(ctx context.Context) (int, error)

// WithAutomationTick registers the tick.
func (w *Worker) WithAutomationTick(fn AutomationTickFunc) *Worker {
	w.mux.HandleFunc(TaskAutomationTick, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Info("automation_runs_processed", "count", n)
		}
		return err
	})
	return w
}

// RegisterAutomationSchedule adds the tick to the scheduler.
func RegisterAutomationSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		automationCron, NewAutomationTickTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(4*time.Minute), asynq.Unique(4*time.Minute),
	); err != nil {
		return fmt.Errorf("queue: register automation tick: %w", err)
	}
	return nil
}
