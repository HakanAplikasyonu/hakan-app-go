// Package rls runs database work under Postgres row-level security
// (issue #6, G8).
//
// The application connects as the table owner, which RLS does not restrict
// (tables are not FORCE'd), so existing code paths keep working. Work that
// should be isolated by the database as well as by its WHERE clauses runs in
// WithScope: a transaction that switches to a scoped role (SET LOCAL ROLE,
// no BYPASSRLS) and sets the request context with SET LOCAL. The policies of
// migration 000046 then hide every row outside that context:
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
	if err := fn(dbtx.WithTx(ctx, tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
