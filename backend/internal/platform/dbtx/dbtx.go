// Package dbtx stores an ambient pgx.Tx on context so repositories and
// platform helpers (e.g. outbox) share the same transaction.
package dbtx

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
)

type ctxKey struct{}

type hooksKey struct{}

// CommitHooks collects work that must only happen once the ambient
// transaction has committed, such as enqueueing a job that reads rows the
// transaction wrote (#304). The owner of the transaction runs them after a
// successful commit and drops them on rollback.
type CommitHooks struct {
	mu  sync.Mutex
	fns []func(context.Context)
}

// WithCommitHooks returns a child context that carries a fresh hook list.
func WithCommitHooks(ctx context.Context) (context.Context, *CommitHooks) {
	h := &CommitHooks{}
	return context.WithValue(ctx, hooksKey{}, h), h
}

// AfterCommit registers fn to run after the ambient transaction commits. It
// reports false when ctx has no hook list (no transaction owner that runs
// them); the caller then decides what to do instead.
func AfterCommit(ctx context.Context, fn func(context.Context)) bool {
	h, ok := ctx.Value(hooksKey{}).(*CommitHooks)
	if !ok || h == nil {
		return false
	}
	h.mu.Lock()
	h.fns = append(h.fns, fn)
	h.mu.Unlock()
	return true
}

// Run calls the registered hooks in order, once. ctx should outlive the
// request (the hooks run after the answer is ready).
func (h *CommitHooks) Run(ctx context.Context) {
	if h == nil {
		return
	}
	h.mu.Lock()
	fns := h.fns
	h.fns = nil
	h.mu.Unlock()
	for _, fn := range fns {
		fn(ctx)
	}
}

// WithTx returns a child context that carries tx.
func WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, ctxKey{}, tx)
}

// Tx returns the ambient transaction, if any.
func Tx(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(ctxKey{}).(pgx.Tx)
	return tx, ok
}
