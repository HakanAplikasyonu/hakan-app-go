package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/providers"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/queue"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
)

type deliverTestQuerier struct {
	pushTestQuerier
	row      db.Notification
	claimErr error
	getErr   error
	sent     int
	stuck    []int64
	orphaned []int64
}

func (d *deliverTestQuerier) MarkNotificationProcessing(context.Context, int64) (db.Notification, error) {
	if d.claimErr != nil {
		return db.Notification{}, d.claimErr
	}
	return d.row, nil
}

func (d *deliverTestQuerier) GetNotificationByID(context.Context, int64) (db.Notification, error) {
	if d.getErr != nil {
		return db.Notification{}, d.getErr
	}
	return d.row, nil
}

func (d *deliverTestQuerier) MarkNotificationSent(context.Context, db.MarkNotificationSentParams) (db.Notification, error) {
	d.sent++
	d.row.Status = model.StatusDelivered
	return d.row, nil
}

func (d *deliverTestQuerier) ListStuckProcessingNotificationIDs(context.Context, int32) ([]int64, error) {
	return d.stuck, nil
}

func (d *deliverTestQuerier) ListOrphanedQueuedNotificationIDs(context.Context, int32) ([]int64, error) {
	return d.orphaned, nil
}

// recordingEnqueuer records tasks; ids in conflict answer ErrTaskIDConflict.
type recordingEnqueuer struct {
	tasks    []*asynq.Task
	opts     [][]asynq.Option
	conflict map[string]bool
}

func (r *recordingEnqueuer) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	for _, o := range opts {
		if o.Type() == asynq.TaskIDOpt && r.conflict[o.Value().(string)] {
			return nil, fmt.Errorf("queue: enqueue: %w", asynq.ErrTaskIDConflict)
		}
	}
	r.tasks = append(r.tasks, task)
	r.opts = append(r.opts, opts)
	return &asynq.TaskInfo{}, nil
}

func TestDeliverResumesProcessingAfterClaimMiss(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row: db.Notification{
			ID:      7,
			Channel: model.ChannelInapp,
			Status:  model.StatusProcessing,
			Title:   "Hello",
		},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	if err := svc.Deliver(context.Background(), 7); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if q.sent != 1 {
		t.Fatalf("MarkNotificationSent calls = %d, want 1", q.sent)
	}
}

// The task ran before the writing transaction committed (#273): the row is
// not visible yet, so the task fails and is retried instead of being
// dropped with the notification left queued forever.
func TestDeliverRetriesInvisibleRow(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{claimErr: pgx.ErrNoRows, getErr: pgx.ErrNoRows}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	if err := svc.Deliver(context.Background(), 7); !errors.Is(err, ErrNotificationNotVisible) {
		t.Fatalf("Deliver = %v, want ErrNotificationNotVisible", err)
	}
	if q.sent != 0 {
		t.Fatalf("MarkNotificationSent calls = %d, want 0", q.sent)
	}
}

func TestDeliverSkipsTerminalStatus(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row: db.Notification{
			ID:      7,
			Channel: model.ChannelInapp,
			Status:  model.StatusDelivered,
		},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	if err := svc.Deliver(context.Background(), 7); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if q.sent != 0 {
		t.Fatalf("MarkNotificationSent calls = %d, want 0", q.sent)
	}
}

func TestReclaimStuckDeliversProcessingRows(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row: db.Notification{
			ID:      3,
			Channel: model.ChannelInapp,
			Status:  model.StatusProcessing,
		},
		stuck: []int64{3},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	n, err := svc.ReclaimStuck(context.Background(), 0)
	if err != nil {
		t.Fatalf("ReclaimStuck: %v", err)
	}
	if n != 1 {
		t.Fatalf("reclaimed = %d, want 1", n)
	}
	if q.sent != 1 {
		t.Fatalf("MarkNotificationSent calls = %d, want 1", q.sent)
	}
}

// Rows lost before #309 sit queued with no task: the sweep puts them back
// on the normal deliver task, one fixed task id per row.
func TestReclaimQueuedEnqueuesDeliverTasks(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{orphaned: []int64{5, 9}}
	enq := &recordingEnqueuer{conflict: map[string]bool{"notification-requeue:9": true}}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	n, err := svc.ReclaimQueued(context.Background(), enq, DefaultOrphanedQueuedMinutes)
	if err != nil {
		t.Fatalf("ReclaimQueued: %v", err)
	}
	if n != 1 || len(enq.tasks) != 1 {
		t.Fatalf("requeued = %d, tasks = %d, want 1, 1 (9 already enqueued)", n, len(enq.tasks))
	}
	if got := enq.tasks[0].Type(); got != queue.TaskNotificationDeliver {
		t.Fatalf("task type = %q, want %q", got, queue.TaskNotificationDeliver)
	}
	p, err := queue.ParseNotificationDeliverPayload(enq.tasks[0].Payload())
	if err != nil || p.NotificationID != 5 {
		t.Fatalf("payload = %+v, %v; want id 5", p, err)
	}
	var gotQueue, gotID string
	for _, o := range enq.opts[0] {
		switch o.Type() {
		case asynq.QueueOpt:
			gotQueue = o.Value().(string)
		case asynq.TaskIDOpt:
			gotID = o.Value().(string)
		}
	}
	if gotQueue != queue.QueueNotifications || gotID != "notification-requeue:5" {
		t.Fatalf("queue = %q, task id = %q", gotQueue, gotID)
	}
	if q.sent != 0 {
		t.Fatalf("MarkNotificationSent calls = %d, want 0 (delivery is the task's job)", q.sent)
	}
}

// Without a queue the rows are delivered inline through Deliver's claim.
func TestReclaimQueuedDeliversInlineWithoutQueue(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		row:      db.Notification{ID: 5, Channel: model.ChannelInapp, Status: model.StatusProcessing},
		orphaned: []int64{5},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	n, err := svc.ReclaimQueued(context.Background(), nil, 0)
	if err != nil {
		t.Fatalf("ReclaimQueued: %v", err)
	}
	if n != 1 || q.sent != 1 {
		t.Fatalf("requeued = %d, sent = %d, want 1, 1", n, q.sent)
	}
}

// A duplicate task that runs after the row was delivered is a no-op: the
// claim misses and the terminal status ends it.
func TestReclaimQueuedDuplicateAfterDeliveryIsNoop(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row:      db.Notification{ID: 5, Channel: model.ChannelInapp, Status: model.StatusSent},
		orphaned: []int64{5},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	if _, err := svc.ReclaimQueued(context.Background(), nil, 0); err != nil {
		t.Fatalf("ReclaimQueued: %v", err)
	}
	if q.sent != 0 {
		t.Fatalf("MarkNotificationSent calls = %d, want 0", q.sent)
	}
}
