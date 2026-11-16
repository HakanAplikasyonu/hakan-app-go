package database

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/metrics"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestBestEffortKeepsTransaction (issue #311): a failing side effect inside
// a transaction is rolled back to its savepoint — its own writes vanish, the
// transaction stays usable and commits the main work; the failure is
// counted. A successful side effect commits with the transaction.
func TestBestEffortKeepsTransaction(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	before := testutil.ToFloat64(metrics.Default.SideEffectFailures("test_side_effect"))
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE main_work (v text NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	ctx = dbtx.WithTx(ctx, tx)
	exec := func(ctx context.Context, sql string) error {
		q, _ := dbtx.Tx(ctx)
		_, err := q.Exec(ctx, sql)
		return err
	}
	if err := exec(ctx, `INSERT INTO main_work VALUES ('main')`); err != nil {
		t.Fatal(err)
	}
	err = BestEffort(ctx, "test_side_effect", func(ctx context.Context) error {
		if err := exec(ctx, `INSERT INTO main_work VALUES ('half')`); err != nil {
			return err
		}
		return exec(ctx, `INSERT INTO main_work VALUES (NULL)`) // NOT NULL: fails
	})
	if err == nil {
		t.Fatal("failing side effect returned nil")
	}
	if err := BestEffort(ctx, "test_side_effect", func(ctx context.Context) error {
		return exec(ctx, `INSERT INTO main_work VALUES ('ok')`)
	}); err != nil {
		t.Fatal(err)
	}
	// The transaction is not aborted and keeps the main row and the good side effect.
	rows, err := tx.Query(ctx, `SELECT v FROM main_work ORDER BY v`)
	if err != nil {
		t.Fatalf("transaction aborted after the failed side effect: %v", err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(got) != 2 || got[0] != "main" || got[1] != "ok" {
		t.Fatalf("rows = %v %v, want [main ok] (the failed side effect's rows rolled back)", got, err)
	}
	if after := testutil.ToFloat64(metrics.Default.SideEffectFailures("test_side_effect")); after != before+1 {
		t.Fatalf("failures counted = %v, want %v", after, before+1)
	}
	// Without a transaction the function just runs and its error comes back.
	boom := errors.New("boom")
	if err := BestEffort(context.Background(), "test_side_effect", func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("no-transaction error = %v", err)
	}
}
