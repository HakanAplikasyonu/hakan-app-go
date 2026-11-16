package usecase

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/providers"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/queue"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
)

// raceQuerier: the first claim misses (its snapshot predates the commit),
// the follow-up read already sees the committed, queued row (#304).
type raceQuerier struct {
	deliverTestQuerier
	claims    int
	claimMiss int // how many claims miss before one succeeds
}

func (r *raceQuerier) MarkNotificationProcessing(context.Context, int64) (db.Notification, error) {
	r.claims++
	if r.claims <= r.claimMiss {
		return db.Notification{}, pgx.ErrNoRows
	}
	r.row.Status = model.StatusProcessing
	return r.row, nil
}

func newRaceQuerier(miss int) *raceQuerier {
	return &raceQuerier{
		claimMiss: miss,
		deliverTestQuerier: deliverTestQuerier{row: db.Notification{
			ID: 14, Channel: model.ChannelInapp, Status: model.StatusQueued, Title: "Onay",
		}},
	}
}

// #304: a queued row seen after a missed claim used to return nil, which
// dropped the e-mail for good (status stayed "queued", no retry).
func TestDeliverClaimsQueuedRowAfterCommitRace(t *testing.T) {
	t.Parallel()
	q := newRaceQuerier(1)
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	if err := svc.Deliver(context.Background(), 14); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if q.claims != 2 || q.sent != 1 {
		t.Fatalf("claims=%d sent=%d, want 2 claims and 1 send", q.claims, q.sent)
	}
}

func TestDeliverRetriesWhenQueuedRowCannotBeClaimed(t *testing.T) {
	t.Parallel()
	q := newRaceQuerier(2)
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	err := svc.Deliver(context.Background(), 14)
	if !errors.Is(err, ErrNotificationNotVisible) || !errors.Is(err, queue.ErrRetrySoon) {
		t.Fatalf("Deliver = %v, want a retry-soon not-visible error", err)
	}
	if q.sent != 0 {
		t.Fatalf("sent %d, want 0", q.sent)
	}
}

type recordingQueue struct{ tasks []*asynq.Task }

func (r *recordingQueue) Enqueue(task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	r.tasks = append(r.tasks, task)
	return &asynq.TaskInfo{}, nil
}

// Inside a request transaction the deliver task is enqueued only after the
// commit hooks run (#304), never while the row is still invisible.
func TestScheduleDeliverWaitsForCommit(t *testing.T) {
	t.Parallel()
	enq := &recordingQueue{}
	svc := New(newRaceQuerier(0), enq, nil, slog.Default())

	ctx, hooks := dbtx.WithCommitHooks(dbtx.WithTx(context.Background(), fakeTx{}))
	if err := svc.scheduleDeliver(ctx, 14); err != nil {
		t.Fatalf("scheduleDeliver: %v", err)
	}
	if len(enq.tasks) != 0 {
		t.Fatalf("enqueued %d tasks before commit", len(enq.tasks))
	}
	hooks.Run(context.Background())
	if len(enq.tasks) != 1 || enq.tasks[0].Type() != queue.TaskNotificationDeliver {
		t.Fatalf("after commit: %d tasks", len(enq.tasks))
	}
}

// Without a transaction there is nothing to wait for.
func TestScheduleDeliverOutsideTransactionEnqueuesNow(t *testing.T) {
	t.Parallel()
	enq := &recordingQueue{}
	svc := New(newRaceQuerier(0), enq, nil, slog.Default())
	if err := svc.scheduleDeliver(context.Background(), 14); err != nil {
		t.Fatalf("scheduleDeliver: %v", err)
	}
	if len(enq.tasks) != 1 {
		t.Fatalf("enqueued %d, want 1", len(enq.tasks))
	}
}

// fakeTx only marks the context as transactional.
type fakeTx struct{ pgx.Tx }
