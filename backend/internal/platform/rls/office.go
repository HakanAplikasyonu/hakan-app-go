package rls

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
)

// ErrNotOfficeOfTaxpayer guards ForOffice: the office does not serve one of
// the scope's own taxpayers.
var ErrNotOfficeOfTaxpayer = errors.New("rls: office does not serve the scope's taxpayer")

// ForOffice runs fn, inside the context's scoped transaction, as work for the
// office orgID on its engagement of taxpayerID (issue #281).
//
// A mükellef panel request runs in a taxpayer scope: no office. Its receipt
// photo, correction, invoice upload or document upload is handed to the
// office, and the code that handles it (receipts, review queue, data
// warnings, notifications, audit) writes the office's own records:
// suggestions, data_warnings and so on. Those tables answer to the office
// policies, which refused every such row (SQLSTATE 42501) and aborted the
// request's transaction.
//
// ForOffice checks, under the taxpayer scope, that taxpayerID is one of the
// scope's own taxpayers and that orgID has an active engagement of it. It
// then switches the transaction to the office scope narrowed to that
// taxpayer (same role and user, every engagement of the office visible as in
// Job) in a savepoint, runs fn, and puts the taxpayer scope back. If fn
// fails, the savepoint is rolled back with its writes and settings.
//
// Inside an office scope of orgID it just runs fn; inside another office's
// scope it refuses. Without a scoped transaction (request scope off) there
// is nothing to switch and it just runs fn.
func ForOffice(ctx context.Context, orgID, taxpayerID int64, fn func(ctx context.Context) error) error {
	tx, ok := dbtx.Tx(ctx)
	if !ok {
		return fn(ctx)
	}
	var role, org, user, taxpayers, readAll string
	var system bool
	if err := tx.QueryRow(ctx, `
		SELECT current_user::text,
		       COALESCE(current_setting('app.organization_id', true), ''),
		       COALESCE(current_setting('app.user_id', true), ''),
		       COALESCE(current_setting('app.taxpayer_ids', true), ''),
		       COALESCE(current_setting('app.engagements_read_all', true), ''),
		       COALESCE(current_setting('app.system', true), '') = 'on'`).
		Scan(&role, &org, &user, &taxpayers, &readAll, &system); err != nil {
		return fmt.Errorf("rls: read scope: %w", err)
	}
	if system {
		return fn(ctx)
	}
	switch org {
	case "":
	case strconv.FormatInt(orgID, 10):
		return fn(ctx)
	default:
		return fmt.Errorf("%w: scope is office %s, not %d", ErrNotOfficeOfTaxpayer, org, orgID)
	}
	scopeRole := Role(role)
	if scopeRole != RoleApp && scopeRole != RoleWorker {
		// Not a scoped role: nothing is under row level security here.
		return fn(ctx)
	}
	var serves bool
	if err := tx.QueryRow(ctx, `
		SELECT $2::bigint = ANY (app_current_taxpayer_ids()) AND EXISTS (
		    SELECT 1 FROM engagements
		    WHERE organization_id = $1 AND taxpayer_id = $2 AND status = 'active')`,
		orgID, taxpayerID).Scan(&serves); err != nil {
		return fmt.Errorf("rls: check office: %w", err)
	}
	if !serves {
		return fmt.Errorf("%w: office %d, taxpayer %d", ErrNotOfficeOfTaxpayer, orgID, taxpayerID)
	}
	userID, _ := strconv.ParseInt(user, 10, 64)

	sp, err := tx.Begin(ctx)
	if err != nil {
		return fmt.Errorf("rls: savepoint: %w", err)
	}
	defer func() { _ = sp.Rollback(ctx) }()
	if err := Apply(ctx, sp, Scope{Role: scopeRole, OrganizationID: orgID, UserID: userID,
		TaxpayerIDs: []int64{taxpayerID}, EngagementsReadAll: true}); err != nil {
		return err
	}
	if err := fn(dbtx.WithTx(ctx, sp)); err != nil {
		return err // the deferred rollback also restores the taxpayer scope
	}
	if err := sp.Commit(ctx); err != nil {
		return fmt.Errorf("rls: release savepoint: %w", err)
	}
	// SET LOCAL outlives RELEASE SAVEPOINT: put the taxpayer scope back.
	return restore(ctx, tx, scopeRole, org, user, taxpayers, readAll)
}

func restore(ctx context.Context, tx pgx.Tx, role Role, org, user, taxpayers, readAll string) error {
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+string(role)); err != nil {
		return fmt.Errorf("rls: restore role: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.organization_id', $1, true),
		        set_config('app.user_id', $2, true),
		        set_config('app.taxpayer_ids', $3, true),
		        set_config('app.engagements_read_all', $4, true)`,
		org, user, taxpayers, readAll); err != nil {
		return fmt.Errorf("rls: restore scope: %w", err)
	}
	return nil
}
