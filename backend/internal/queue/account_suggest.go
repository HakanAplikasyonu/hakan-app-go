package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// TaskAccountSuggest proposes account codes for invoices an upload has just
// stored (#319). It runs on QueueImports, after the UBL upload job.
const TaskAccountSuggest = "app:accountrules:suggest"

// AccountSuggestPayload names the engagement and its new invoices.
type AccountSuggestPayload struct {
	OrganizationID int64       `json:"organization_id"`
	EngagementID   int64       `json:"engagement_id"`
	TaxpayerID     int64       `json:"taxpayer_id"`
	Invoices       []uuid.UUID `json:"invoices"`
}

// NewAccountSuggestTask builds the task.
func NewAccountSuggestTask(p AccountSuggestPayload) (*asynq.Task, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("queue: marshal account suggest: %w", err)
	}
	return asynq.NewTask(TaskAccountSuggest, body), nil
}

// WithAccountSuggest registers the account suggestion runner.
func (w *Worker) WithAccountSuggest(fn func(ctx context.Context, p AccountSuggestPayload) error) *Worker {
	w.mux.HandleFunc(TaskAccountSuggest, func(ctx context.Context, task *asynq.Task) error {
		var p AccountSuggestPayload
		if err := json.Unmarshal(task.Payload(), &p); err != nil {
			return fmt.Errorf("queue: unmarshal account suggest: %w", err)
		}
		return fn(ctx, p)
	})
	return w
}
