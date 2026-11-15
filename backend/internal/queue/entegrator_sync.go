package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// TaskEntegratorSync runs one integrator sync of a source connection (issue
// #40, 30 §7). It runs on QueueImports; the run itself takes a per-connection
// lock, so duplicate tasks end with entegrator.ErrBusy.
const TaskEntegratorSync = "app:entegrator:sync"

// EntegratorSyncPayload names the connection and the run kind (backfill,
// incremental, full_scan, manual).
type EntegratorSyncPayload struct {
	TaxpayerID     int64     `json:"taxpayer_id"`
	ConnectionUUID uuid.UUID `json:"connection_uuid"`
	Kind           string    `json:"kind"`
}

// NewEntegratorSyncTask builds the task.
func NewEntegratorSyncTask(p EntegratorSyncPayload) (*asynq.Task, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("queue: marshal entegrator sync: %w", err)
	}
	return asynq.NewTask(TaskEntegratorSync, body), nil
}

// WithEntegratorSync registers the integrator sync runner.
func (w *Worker) WithEntegratorSync(fn func(ctx context.Context, p EntegratorSyncPayload) error) *Worker {
	w.mux.HandleFunc(TaskEntegratorSync, func(ctx context.Context, task *asynq.Task) error {
		var p EntegratorSyncPayload
		if err := json.Unmarshal(task.Payload(), &p); err != nil {
			return fmt.Errorf("queue: unmarshal entegrator sync: %w", err)
		}
		return fn(ctx, p)
	})
	return w
}
