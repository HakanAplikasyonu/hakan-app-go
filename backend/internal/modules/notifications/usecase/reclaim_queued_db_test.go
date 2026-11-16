package usecase

import (
	"context"
	"slices"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// The sweep query picks only queued rows that never started, are due and
// are older than the threshold: what a #304 commit race left behind.
func TestListOrphanedQueuedNotificationIDs(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()

	insert := func(status string, attempts int, ageMin int, scheduledInMin *int) int64 {
		t.Helper()
		var id int64
		var sched any
		if scheduledInMin != nil {
			sched = *scheduledInMin
		}
		err := d.Pool.QueryRow(ctx, `
			INSERT INTO notifications (channel, status, attempt_count, created_at, scheduled_at)
			VALUES ('email', $1, $2, NOW() - make_interval(mins => $3::int),
			        CASE WHEN $4::int IS NULL THEN NULL ELSE NOW() + make_interval(mins => $4::int) END)
			RETURNING id`, status, attempts, ageMin, sched).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	past, future := -5, 30

	lost := insert("queued", 0, 60, nil)
	lostDue := insert("queued", 0, 60, &past)
	_ = insert("queued", 0, 2, nil)      // fresh: its task may still be pending
	_ = insert("queued", 0, 60, &future) // scheduled for later
	_ = insert("queued", 1, 60, nil)     // already started once
	_ = insert("processing", 1, 60, nil) // ReclaimStuck's job
	_ = insert("sent", 1, 60, nil)
	_ = insert("failed", 0, 60, nil)

	ids, err := db.New(d.Pool).ListOrphanedQueuedNotificationIDs(ctx, DefaultOrphanedQueuedMinutes)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{lost, lostDue}; !slices.Equal(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
}
