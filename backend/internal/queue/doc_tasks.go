package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskDocTaskReminders notifies mükellef members of open document tasks
// whose reminder is due (issue #36). It runs on QueueMaintenance.
const TaskDocTaskReminders = "app:doc_tasks:reminders"

// docTaskRemindersCron: hourly. Backoff lives in the tasks themselves
// (next_reminder_at), so a run only touches the tasks that are due.
const docTaskRemindersCron = "@every 1h"

// NewDocTaskRemindersTask builds the periodic reminder sweep.
func NewDocTaskRemindersTask() *asynq.Task {
	return asynq.NewTask(TaskDocTaskReminders, []byte("{}"))
}

// DocTaskRemindersFunc sends due reminders and returns how many members were
// notified.
type DocTaskRemindersFunc func(ctx context.Context) (int, error)

// WithDocTaskReminders registers the reminder sweep.
func (w *Worker) WithDocTaskReminders(fn DocTaskRemindersFunc) *Worker {
	w.mux.HandleFunc(TaskDocTaskReminders, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Info("doc_task_reminders_sent", "notified", n)
		}
		return err
	})
	return w
}

// RegisterDocTaskRemindersSchedule adds the hourly reminder sweep.
func RegisterDocTaskRemindersSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		docTaskRemindersCron, NewDocTaskRemindersTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(10*time.Minute), asynq.Unique(55*time.Minute),
	); err != nil {
		return fmt.Errorf("queue: register doc task reminders: %w", err)
	}
	return nil
}
