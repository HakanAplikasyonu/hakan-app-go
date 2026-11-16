// Package rls runs database work under Postgres row-level security
// (issue #6, G8).
//
// Application connections run as hakan_rls_runtime (migration 000074,
// issue #166; database.NewPostgresPool). Row level security applies to every
// query. Tables still in transition have an rls_legacy policy that lets the
// runtime role see everything, as before; enforced tables
// (internal/database/rls_enforced.txt) answer only inside one of:
//
//   - WithScope: a transaction that switches to a scoped role (SET LOCAL
//     ROLE, no BYPASSRLS) and sets the request context with SET LOCAL. The
//     policies of migration 000046 then hide every row outside that context;
//   - System: a deliberate, named bypass for work that is not done on behalf
//     of one tenant (workers spanning tenants, the demo seed, maintenance).
//
// WithScope policies:
//
//	org tables         organization_id = app.organization_id
//	engagement tables  + assigned to app.user_id, or app.engagements_read_all
//	taxpayer tables    taxpayer_id = ANY(app.taxpayer_ids)
//	platform tables    nothing
//
// An empty scope sees no scoped rows at all.
package rls

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Role is a scoped database role created by migration 000046.
type Role string

const (
	// RoleApp is used by API requests.
	RoleApp Role = "hakan_rls_app"
	// RoleWorker is used by background jobs.
	RoleWorker Role = "hakan_rls_worker"
)

// Scope is the context of one scoped transaction.
type Scope struct {
	Role               Role
	OrganizationID     int64
	UserID             int64
	TaxpayerIDs        []int64
	EngagementsReadAll bool
}

// ErrUnknownRole guards SET ROLE, which cannot take a bind parameter.
var ErrUnknownRole = errors.New("rls: unknown role")

// Beginner starts transactions (*pgxpool.Pool, pgx.Conn).
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

var _ Beginner = (*pgxpool.Pool)(nil)

// WithScope runs fn in a transaction under s. The transaction is also put on
// the context (dbtx.WithTx) for helpers that pick up an ambient tx. fn's
// error rolls back; otherwise the transaction commits.
func WithScope(ctx context.Context, db Beginner, s Scope, fn func(ctx context.Context, tx pgx.Tx) error) error {
	role := s.Role
	if role == "" {
		role = RoleApp
	}
	if role != RoleApp && role != RoleWorker {
		return fmt.Errorf("%w: %q", ErrUnknownRole, role)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("rls: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	s.Role = role
	if err := Apply(ctx, tx, s); err != nil {
		return err
	}
	return runCommitting(ctx, tx, fn)
}

// runCommitting runs fn with the transaction (and a commit hook list) on the
// context, commits, then runs the hooks (#304: jobs enqueued for rows written
// here must not run before those rows are visible). A rollback drops them.
func runCommitting(ctx context.Context, tx pgx.Tx, fn func(ctx context.Context, tx pgx.Tx) error) error {
	inner, hooks := dbtx.WithCommitHooks(dbtx.WithTx(ctx, tx))
	if err := fn(inner, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	hooks.Run(context.WithoutCancel(ctx))
	return nil
}

// Apply puts an open transaction under s (SET LOCAL ROLE and the context
// settings) until it ends. For code that already owns its transaction, such
// as a worker's per-item transaction (issue #166). Do not use it on a
// savepoint you will release inside another scope: SET LOCAL outlives it.
func Apply(ctx context.Context, tx pgx.Tx, s Scope) error {
	role := s.Role
	if role == "" {
		role = RoleApp
	}
	if role != RoleApp && role != RoleWorker {
		return fmt.Errorf("%w: %q", ErrUnknownRole, role)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+string(role)); err != nil {
		return fmt.Errorf("rls: set role: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.organization_id', $1, true),
		        set_config('app.user_id', $2, true),
		        set_config('app.taxpayer_ids', $3, true),
		        set_config('app.engagements_read_all', $4, true)`,
		idOrEmpty(s.OrganizationID), idOrEmpty(s.UserID), idArray(s.TaxpayerIDs), onOff(s.EngagementsReadAll),
	); err != nil {
		return fmt.Errorf("rls: set context: %w", err)
	}
	return nil
}

func idOrEmpty(id int64) string {
	if id <= 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}

func idArray(ids []int64) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			parts = append(parts, strconv.FormatInt(id, 10))
		}
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return ""
}

// ErrNoReason guards System: every bypass says why.
var ErrNoReason = errors.New("rls: System needs a reason")

// System runs fn in a transaction that deliberately bypasses row level
// security: the role falls back to the login role (SET LOCAL ROLE NONE) and
// app.system is on, which the rls_system policy of enforced tables accepts
// for a table owner that is not a superuser. Use it only for work that is
// not done on behalf of one tenant; reason names that work (greppable,
// reviewable). The transaction is put on the context like WithScope's.
func System(ctx context.Context, db Beginner, reason string, fn func(ctx context.Context, tx pgx.Tx) error) error {
	if strings.TrimSpace(reason) == "" {
		return ErrNoReason
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("rls: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE NONE"); err != nil {
		return fmt.Errorf("rls: reset role: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.system', 'on', true)`); err != nil {
		return fmt.Errorf("rls: set system: %w", err)
	}
	return runCommitting(ctx, tx, fn)
}

// InScope reports whether ctx carries a scoped (or System) transaction: work
// started there is already under row level security.
func InScope(ctx context.Context) bool {
	_, ok := dbtx.Tx(ctx)
	return ok
}

// Job is the scope of a background job working for one office's engagement
// of a taxpayer (worker role, all of the office's engagements visible).
func Job(orgID int64, taxpayerIDs ...int64) Scope {
	return Scope{Role: RoleWorker, OrganizationID: orgID, EngagementsReadAll: true, TaxpayerIDs: taxpayerIDs}
}
