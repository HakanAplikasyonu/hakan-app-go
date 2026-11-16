package database

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/metrics"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
)

// BestEffort runs a side-effect write — a notification, an activity row, a
// review submission, a data warning — so that its failure never takes the
// caller's main work with it (issue #311).
//
// Inside a transaction (the request's row level security scope, a worker's
// item) fn runs in a savepoint on it. If fn fails, the savepoint is rolled
// back: the main transaction is not aborted ("current transaction is
// aborted", SQLSTATE 25P02, the 500s of #277 and #281) and goes on to
// commit. Without a transaction fn just runs.
//
// The failure is logged (side_effect_failed) and counted
// (side_effect_failures_total{name}); the error is returned too, for callers
// that report it. name is a fixed label ("notification", "activity", ...).
//
// Use it only for writes the request can do without. Money, journal entries,
// approvals and the record the request is about are main work: let their
// errors fail the request.
func BestEffort(ctx context.Context, name string, fn func(ctx context.Context) error) error {
	tx, ok := dbtx.Tx(ctx)
	if !ok {
		if err := fn(ctx); err != nil {
			sideEffectFailed(ctx, name, err)
			return err
		}
		return nil
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		err = fmt.Errorf("%s: savepoint: %w", name, err)
		sideEffectFailed(ctx, name, err)
		return err
	}
	if err := fn(dbtx.WithTx(ctx, sp)); err != nil {
		if rbErr := sp.Rollback(ctx); rbErr != nil {
			err = fmt.Errorf("%w (rollback to savepoint: %v)", err, rbErr)
		}
		sideEffectFailed(ctx, name, err)
		return err
	}
	if err := sp.Commit(ctx); err != nil {
		err = fmt.Errorf("%s: release savepoint: %w", name, err)
		sideEffectFailed(ctx, name, err)
		return err
	}
	return nil
}

func sideEffectFailed(ctx context.Context, name string, err error) {
	metrics.Default.ObserveSideEffectFailure(name)
	slog.Default().WarnContext(ctx, "side_effect_failed", "name", name, "error", err)
}
