package officedash

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type recNotifier struct {
	mu  sync.Mutex
	got []notifmodel.EnqueueInput
}

func (r *recNotifier) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, in)
	return nil, nil
}

func (r *recNotifier) users() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []int64{}
	for _, in := range r.got {
		out = append(out, *in.UserID)
	}
	return out
}

// TestBlockerAlerts (issue #241): a new blocking warning is announced once to
// the engagement's assignees (the owners when nobody is assigned); old,
// non-blocking and ended-engagement warnings are not.
func TestBlockerAlerts(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org := d.Organization(t)
	owner, assignee, other := d.User(t), d.User(t), d.User(t)
	d.Member(t, org, owner, "owner")
	d.Member(t, org, assignee, "member")
	d.Member(t, org, other, "member")
	tpA, tpB, tpC := d.Taxpayer(t, "legal", "9100000011"), d.Taxpayer(t, "legal", "9100000012"), d.Taxpayer(t, "legal", "9100000013")
	engA, engB, engC := d.Engagement(t, org, tpA), d.Engagement(t, org, tpB), d.Engagement(t, org, tpC)
	d.Assign(t, org, engA, assignee)

	warn := func(eng, tp int64, severity string, age time.Duration) {
		t.Helper()
		if _, err := d.Pool.Exec(ctx, `
			INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, entity_type, entity_id, code, severity, producer, created_at)
			VALUES ($1, $2, $3, 'period', $4, 'TEST_CODE', $5, 'test', NOW() - $6::interval)`,
			org, eng, tp, testdb.Next(), severity, age.String()); err != nil {
			t.Fatal(err)
		}
	}
	warn(engA, tpA, "blocker", time.Minute)
	warn(engA, tpA, "blocker", 2*time.Minute)
	warn(engA, tpA, "warning", time.Minute) // not a blocker
	warn(engB, tpB, "blocker", time.Minute) // nobody assigned → owners
	warn(engB, tpB, "blocker", 48*time.Hour) // too old
	warn(engC, tpC, "blocker", time.Minute)
	d.EndEngagement(t, engC) // ended

	n := &recNotifier{}
	a := &Alerts{Q: database.QueriesFor(d.Pool), Notifier: n, FrontendURL: "https://app.test"}
	run := func() int {
		t.Helper()
		var sent int
		// As in the worker: one deliberate row level security bypass.
		if err := rls.System(ctx, d.Pool, "test: blocker alerts", func(ctx context.Context, _ pgx.Tx) error {
			var err error
			sent, err = a.SendBlockerAlerts(ctx)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return sent
	}
	if sent := run(); sent != 2 {
		t.Fatalf("sent = %d, want 2 (assignee of A, owner of B); got users %v", sent, n.users())
	}
	got := map[int64]notifmodel.EnqueueInput{}
	for _, in := range n.got {
		got[*in.UserID] = in
	}
	if in, ok := got[assignee]; !ok || in.TemplateVars["count"] != "2" || in.Payload["kind"] != "warnings_blocker" ||
		!strings.Contains(*in.ActionURL, "/warnings?engagement=") {
		t.Fatalf("assignee alert = %+v", got[assignee])
	}
	if in, ok := got[owner]; !ok || in.TemplateVars["count"] != "1" {
		t.Fatalf("owner alert = %+v", got[owner])
	}
	if _, ok := got[other]; ok {
		t.Fatal("an unassigned member was alerted")
	}
	if sent := run(); sent != 0 {
		t.Fatalf("second run sent %d", sent)
	}
	// A new blocker later is announced on its own.
	warn(engA, tpA, "blocker", 0)
	if sent := run(); sent != 1 {
		t.Fatalf("new blocker sent %d", sent)
	}
}
