package rls_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// #304: work registered with dbtx.AfterCommit runs after the transaction
// committed (and sees its rows from another connection), never on rollback.
func TestCommitHooksRunAfterCommitOnly(t *testing.T) {
	d := testdb.Open(t)
	org := d.Organization(t)
	ctx := context.Background()
	scope := rls.Scope{OrganizationID: org}

	var seen int
	err := rls.WithScope(ctx, d.Pool, scope, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO notifications (organization_id, channel, status, priority, title, body, payload, max_attempts)
			VALUES ($1, 'inapp', 'queued', 'normal', 'commit-hook', '', '{}', 5)`, org); err != nil {
			return err
		}
		if !dbtx.AfterCommit(ctx, func(ctx context.Context) {
			// A separate connection, as the worker would use.
			_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE title = 'commit-hook'`).Scan(&seen)
		}) {
			t.Error("WithScope context carries no commit hook list")
		}
		if seen != 0 {
			t.Error("hook ran before commit")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithScope: %v", err)
	}
	if seen != 1 {
		t.Fatalf("hook saw %d committed rows, want 1", seen)
	}

	ran := false
	boom := errors.New("rollback")
	err = rls.System(ctx, d.Pool, "test: commit hooks on rollback", func(ctx context.Context, _ pgx.Tx) error {
		dbtx.AfterCommit(ctx, func(context.Context) { ran = true })
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("System = %v, want the fn error", err)
	}
	if ran {
		t.Fatal("hook ran although the transaction rolled back")
	}
}
