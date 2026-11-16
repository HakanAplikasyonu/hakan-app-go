package rls_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// TestUserRowPolicies (issue #307, migration 000088): notifications and
// activity_events. An office scope reads and writes its own office's rows
// (any user's) and reads only its own user's office-less rows (for
// notifications also user-less ones); it cannot write an office-less row or
// another office's row. A taxpayer scope reads its user's office-less rows
// and writes nothing. rls.System writes office-less rows.
func TestUserRowPolicies(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	const notif = `INSERT INTO notifications (organization_id, user_id, channel, status, priority, title, body, payload)
		VALUES ($1, $2, 'inapp', 'queued', 'normal', $3, '', '{}') RETURNING id`
	const event = `INSERT INTO activity_events (organization_id, actor_user_id, action, resource, payload)
		VALUES ($1, $2, $3, 'test', '{}') RETURNING id`
	err := rls.System(ctx, f.d.Pool, "test fixture", func(ctx context.Context, tx pgx.Tx) error {
		for _, r := range []struct {
			org, user any
			name      string
		}{
			{f.orgA, f.staffA, "a-staff"}, {f.orgB, f.ownerB, "b-owner"},
			{nil, f.ownerA, "null-ownerA"}, {nil, f.staffA, "null-staffA"}, {nil, nil, "null-nobody"},
		} {
			if _, err := tx.Exec(ctx, notif, r.org, r.user, r.name); err != nil {
				return err
			}
			if r.user != nil {
				if _, err := tx.Exec(ctx, event, r.org, r.user, r.name); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	names := func(ctx context.Context, tx pgx.Tx, sql string) map[string]bool {
		rows, err := tx.Query(ctx, sql)
		if err != nil {
			t.Fatal(err)
		}
		got, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, g := range got {
			out[g] = true
		}
		return out
	}
	same := func(what string, got map[string]bool, want ...string) {
		t.Helper()
		ok := len(got) == len(want)
		for _, w := range want {
			ok = ok && got[w]
		}
		if !ok {
			t.Errorf("%s = %v, want %v", what, got, want)
		}
	}
	refused := func(ctx context.Context, tx pgx.Tx, what, sql string, args ...any) {
		t.Helper()
		sp, err := tx.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tag, err := sp.Exec(ctx, sql, args...)
		if !isRLSError(err) && (err != nil || tag.RowsAffected() != 0) {
			t.Errorf("%s: %v (rows %d), want it refused", what, err, tag.RowsAffected())
		}
		_ = sp.Rollback(ctx)
	}

	// Office A, signed in as its owner.
	err = rls.WithScope(ctx, f.d.Pool, rls.Scope{OrganizationID: f.orgA, UserID: f.ownerA}, func(ctx context.Context, tx pgx.Tx) error {
		same("office A notifications", names(ctx, tx, `SELECT title FROM notifications`), "a-staff", "null-ownerA", "null-nobody")
		same("office A activity", names(ctx, tx, `SELECT action FROM activity_events`), "a-staff", "null-ownerA")
		// Notifying another user of the office (RETURNING needs the read policy too).
		if _, err := tx.Exec(ctx, notif, f.orgA, f.staffA, "a-new"); err != nil {
			t.Errorf("office A notifying its member: %v", err)
		}
		if _, err := tx.Exec(ctx, event, f.orgA, f.ownerA, "a-new"); err != nil {
			t.Errorf("office A recording activity: %v", err)
		}
		refused(ctx, tx, "office A writing an office-less notification", notif, nil, f.ownerA, "x")
		refused(ctx, tx, "office A writing office B's notification", notif, f.orgB, f.ownerB, "x")
		refused(ctx, tx, "office A writing office-less activity", event, nil, f.ownerA, "x")
		refused(ctx, tx, "office A marking its user's office-less notification", `UPDATE notifications SET status = 'read' WHERE organization_id IS NULL`)
		refused(ctx, tx, "office A deleting office-less activity", `DELETE FROM activity_events WHERE organization_id IS NULL`)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Taxpayer scope as office A's staff member: own office-less rows only.
	err = rls.WithScope(ctx, f.d.Pool, rls.Scope{UserID: f.staffA, TaxpayerIDs: []int64{f.tpA}}, func(ctx context.Context, tx pgx.Tx) error {
		same("taxpayer scope notifications", names(ctx, tx, `SELECT title FROM notifications`), "null-staffA", "null-nobody")
		same("taxpayer scope activity", names(ctx, tx, `SELECT action FROM activity_events`), "null-staffA")
		refused(ctx, tx, "taxpayer scope writing an office notification", notif, f.orgA, f.ownerA, "x")
		refused(ctx, tx, "taxpayer scope writing its own office-less notification", notif, nil, f.staffA, "x")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// rls.System writes office-less rows (user-level notifications).
	if err := rls.System(ctx, f.d.Pool, "test: user notification", func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, notif, nil, f.ownerB, "system")
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
