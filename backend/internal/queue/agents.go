package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskAgentsOfflineSweep marks desktop agents silent for 30 minutes offline
// and notifies (issue #51, 02 §6).
const TaskAgentsOfflineSweep = "app:agents:offline_sweep"

// The agent reports every 5 minutes; sweeping at the same pace keeps the
// AGENT_OFFLINE notice within 5 minutes of the 30-minute threshold.
const agentsOfflineSweepCron = "@every 5m"

// NewAgentsOfflineSweepTask builds the periodic sweep.
func NewAgentsOfflineSweepTask() *asynq.Task {
	return asynq.NewTask(TaskAgentsOfflineSweep, []byte("{}"))
}

// AgentsOfflineSweepFunc marks silent agents offline and returns how many.
type AgentsOfflineSweepFunc func(ctx context.Context) (int, error)

// WithAgentsOfflineSweep registers the sweep.
func (w *Worker) WithAgentsOfflineSweep(fn AgentsOfflineSweepFunc) *Worker {
	w.mux.HandleFunc(TaskAgentsOfflineSweep, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Warn("desktop_agents_offline", "count", n)
		}
		return err
	})
	return w
}

// RegisterAgentsOfflineSweepSchedule adds the 5-minute sweep.
func RegisterAgentsOfflineSweepSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		agentsOfflineSweepCron, NewAgentsOfflineSweepTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(2*time.Minute), asynq.Unique(4*time.Minute),
	); err != nil {
		return fmt.Errorf("queue: register agents offline sweep: %w", err)
	}
	return nil
}
