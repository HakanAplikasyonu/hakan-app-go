package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
)

// Ambient is a db.DBTX that runs each statement on the transaction carried
// by the context (dbtx.WithTx; rls.WithScope and rls.System put theirs
// there) and on the pool otherwise (issue #166). A repository built on it
// joins the caller's row level security scope without taking a tx argument.
//
// A transaction that has already ended (a goroutine kept the context) falls
// back to the pool: the runtime role, which sees nothing of an enforced
// table, so the fallback fails closed.
type Ambient struct {
	Pool Pool
}

// Pool is what Ambient wraps: a pool (or anything with statements and Begin).
type Pool interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// QueriesFor returns sqlc Queries that join the context's transaction
// (rls.WithScope / rls.System) and use pool otherwise.
func QueriesFor(pool Pool) *db.Queries { return db.New(Ambient{Pool: pool}) }

// Begin starts a transaction that stays in the caller's scope: a savepoint
// on the context's transaction when there is one, else a new transaction on
// pool. Use it instead of pool.Begin.
func Begin(ctx context.Context, pool Pool) (pgx.Tx, error) { return Ambient{Pool: pool}.Begin(ctx) }

// BeginFunc is pgx.BeginFunc with Begin's scope rule.
func BeginFunc(ctx context.Context, pool Pool, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, Ambient{Pool: pool}, fn)
}

var _ db.DBTX = Ambient{}

// NewAmbient wraps a pool.
func NewAmbient(pool *pgxpool.Pool) Ambient { return Ambient{Pool: pool} }

func (a Ambient) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if tx, ok := dbtx.Tx(ctx); ok {
		tag, err := tx.Exec(ctx, sql, args...)
		if !errors.Is(err, pgx.ErrTxClosed) {
			return tag, err
		}
	}
	return a.Pool.Exec(ctx, sql, args...)
}

func (a Ambient) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if tx, ok := dbtx.Tx(ctx); ok {
		rows, err := tx.Query(ctx, sql, args...)
		if !errors.Is(err, pgx.ErrTxClosed) {
			return rows, err
		}
	}
	return a.Pool.Query(ctx, sql, args...)
}

func (a Ambient) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if tx, ok := dbtx.Tx(ctx); ok {
		return ambientRow{ctx: ctx, row: tx.QueryRow(ctx, sql, args...), pool: a.Pool, sql: sql, args: args}
	}
	return a.Pool.QueryRow(ctx, sql, args...)
}

// Begin starts a transaction: a savepoint on the ambient transaction when
// there is one, so the work stays in the caller's scope, else a new one on
// the pool. Use it instead of pool.Begin in code that may run inside
// rls.WithScope.
func (a Ambient) Begin(ctx context.Context) (pgx.Tx, error) {
	if tx, ok := dbtx.Tx(ctx); ok {
		sp, err := tx.Begin(ctx)
		if !errors.Is(err, pgx.ErrTxClosed) {
			return sp, err
		}
	}
	return a.Pool.Begin(ctx)
}

// ambientRow retries on the pool when the ambient tx had already ended.
type ambientRow struct {
	ctx  context.Context
	row  pgx.Row
	pool db.DBTX
	sql  string
	args []any
}

func (r ambientRow) Scan(dest ...any) error {
	err := r.row.Scan(dest...)
	if errors.Is(err, pgx.ErrTxClosed) {
		return r.pool.QueryRow(r.ctx, r.sql, r.args...).Scan(dest...)
	}
	return err
}

// VisibleTaxpayers lists the taxpayers of the office's active engagements the
// user may see (all with read_all, else the assigned ones). It feeds
// rls.Middleware on office-wide routes.
func VisibleTaxpayers(pool *pgxpool.Pool) func(ctx context.Context, orgID, userID int64, readAll bool) ([]int64, error) {
	return func(ctx context.Context, orgID, userID int64, readAll bool) ([]int64, error) {
		rows, err := pool.Query(ctx, `
			SELECT DISTINCT e.taxpayer_id FROM engagements e
			WHERE e.organization_id = $1 AND e.ended_at IS NULL
			  AND ($3 OR EXISTS (SELECT 1 FROM engagement_assignments a
			                     WHERE a.organization_id = e.organization_id
			                       AND a.engagement_id = e.id AND a.user_id = $2))
			ORDER BY 1`, orgID, userID, readAll)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, pgx.RowTo[int64])
	}
}
