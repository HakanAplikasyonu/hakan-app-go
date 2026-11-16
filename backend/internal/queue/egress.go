package queue

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/hibiken/asynq"
)

// QueueFixedEgress carries calls that must leave from the fixed public IP
// (issue #45): integrator syncs and the egress IP check. Only workers with
// WORKER_POOL=fixed-egress (or all) consume it.
const QueueFixedEgress = "fixed-egress"

// Worker pools (WORKER_POOL).
const (
	PoolAll         = "all"          // every queue (single-host and development)
	PoolGeneral     = "general"      // every queue except fixed-egress
	PoolFixedEgress = "fixed-egress" // only fixed-egress
)

// TaskEgressCheck checks the fixed-egress public IP.
const TaskEgressCheck = "app:egress:check"

// EntegratorSyncOptions are the enqueue options of an integrator sync: it
// runs on the fixed-egress pool, because integrators allowlist the caller IP.
func EntegratorSyncOptions() []asynq.Option {
	return []asynq.Option{asynq.Queue(QueueFixedEgress), asynq.MaxRetry(3)}
}

// EnqueueEntegratorSync enqueues an integrator sync on the fixed-egress queue.
func EnqueueEntegratorSync(c *Client, p EntegratorSyncPayload) error {
	task, err := NewEntegratorSyncTask(p)
	if err != nil {
		return err
	}
	_, err = c.Enqueue(task, EntegratorSyncOptions()...)
	return err
}

// RegisterEgressCheckSchedule schedules the IP check on the fixed-egress
// queue every interval.
func RegisterEgressCheckSchedule(scheduler *asynq.Scheduler, interval time.Duration) error {
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	spec := fmt.Sprintf("@every %s", interval)
	if _, err := scheduler.Register(spec, asynq.NewTask(TaskEgressCheck, nil),
		asynq.Queue(QueueFixedEgress), asynq.MaxRetry(0), asynq.Unique(interval/2)); err != nil {
		return fmt.Errorf("queue: register egress check: %w", err)
	}
	return nil
}

// WithEgressCheck registers the IP check runner. Its error fails the task:
// the error handler logs and reports it (the IP-change alarm).
func (w *Worker) WithEgressCheck(fn func(ctx context.Context) error) *Worker {
	w.mux.HandleFunc(TaskEgressCheck, func(ctx context.Context, _ *asynq.Task) error {
		return fn(ctx)
	})
	return w
}

// PoolQueues returns the queues (with priorities) a worker pool consumes.
func PoolQueues(pool string) map[string]int {
	general := map[string]int{
		"default":          1,
		QueueNotifications: 2,
		QueueExports:       2,
		QueueImports:       2,
		QueueBulk:          2,
		QueueSearch:        2,
		QueueMaintenance:   1,
	}
	switch pool {
	case PoolFixedEgress:
		return map[string]int{QueueFixedEgress: 1}
	case PoolGeneral:
		return general
	default:
		general[QueueFixedEgress] = 2
		return general
	}
}

// AllQueues lists every queue name, sorted (metrics, issue #179).
func AllQueues() []string {
	q := PoolQueues(PoolAll)
	out := make([]string, 0, len(q))
	for name := range q {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
