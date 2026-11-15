package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// TaskUBLUploadProcess processes one UBL / ZIP upload job (issue #11). It runs
// on QueueImports.
const TaskUBLUploadProcess = "app:invoices:ubl_upload"

// UBLUploadPayload identifies a ubl_upload_jobs row.
type UBLUploadPayload struct {
	JobID int64 `json:"job_id"`
}

// NewUBLUploadProcessTask builds the task.
func NewUBLUploadProcessTask(jobID int64) (*asynq.Task, error) {
	body, err := json.Marshal(UBLUploadPayload{JobID: jobID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal ubl upload: %w", err)
	}
	return asynq.NewTask(TaskUBLUploadProcess, body), nil
}

// WithUBLUpload registers the UBL upload processor.
func (w *Worker) WithUBLUpload(fn func(ctx context.Context, jobID int64) error) *Worker {
	w.mux.HandleFunc(TaskUBLUploadProcess, func(ctx context.Context, task *asynq.Task) error {
		var p UBLUploadPayload
		if err := json.Unmarshal(task.Payload(), &p); err != nil {
			return fmt.Errorf("queue: unmarshal ubl upload: %w", err)
		}
		return fn(ctx, p.JobID)
	})
	return w
}
