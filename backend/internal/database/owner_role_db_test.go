package database

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestNonSuperuserOwnerIsForced (issue #312) runs in CI's
// "backend (non-superuser owner)" job, where the tables belong to
// hakan_owner from scripts/db-roles.sql. It is skipped when the owner is a
// superuser (FORCE does not apply to one).
//
//   - The owner does not inherit the runtime role: otherwise rls_legacy
//     (TO hakan_rls_runtime) would open every table in transition to it and
//     FORCE would mean nothing.
//   - Without app.system the owner sees no row of a forced table; inside
//     app.system (rls.System) it does.
//   - The SECURITY DEFINER functions the policies call still read their
//     tables (migration 000089).
func TestNonSuperuserOwnerIsForced(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, d.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close(ctx) }()
	var super bool
	var name string
	if err := owner.QueryRow(ctx, `SELECT rolsuper, rolname FROM pg_roles WHERE rolname = current_user`).Scan(&super, &name); err != nil {
		t.Fatal(err)
	}
	if super {
		t.Skipf("owner %s is a superuser: FORCE does not apply (the non-superuser CI job runs this)", name)
	}
	var inherits bool
	if err := owner.QueryRow(ctx, `SELECT pg_has_role(current_user, 'hakan_rls_runtime', 'USAGE')`).Scan(&inherits); err != nil {
		t.Fatal(err)
	}
	if inherits {
		t.Fatalf("owner %s inherits hakan_rls_runtime: its rls_legacy policies would apply to the owner (grant it WITH INHERIT FALSE, scripts/db-roles.sql)", name)
	}

	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", "1234567890")
	eng := d.Engagement(t, org, tp)
	user := d.User(t)
	d.Member(t, org, user, "member")
	d.Assign(t, org, eng, user)

	var plain int
	if err := owner.QueryRow(ctx, `SELECT count(*) FROM engagements`).Scan(&plain); err != nil {
		t.Fatal(err)
	}
	if plain != 0 {
		t.Fatalf("owner without app.system sees %d engagements, want 0 (FORCE)", plain)
	}
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.system', 'on', true)`); err != nil {
		t.Fatal(err)
	}
	var sys int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM engagements`).Scan(&sys); err != nil || sys != 1 {
		t.Fatalf("owner with app.system sees %d engagements (%v), want 1", sys, err)
	}
	_ = tx.Rollback(ctx)

	// The definer function sees the assignment under FORCE (000089).
	tx, err = owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var visible bool
	if err := tx.QueryRow(ctx, `
		SELECT set_config('app.organization_id', $1::bigint::text, true), set_config('app.user_id', $2::bigint::text, true)`,
		org, user).Scan(new(string), new(string)); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT app_engagement_visible($1, $2)`, org, eng).Scan(&visible); err != nil || !visible {
		t.Fatalf("app_engagement_visible for the assigned user = %v (%v), want true", visible, err)
	}
	var leaked string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(current_setting('app.system', true), '')`).Scan(&leaked); err != nil || leaked == "on" {
		t.Fatalf("app.system after the function = %q (%v): it must not outlive the call", leaked, err)
	}
}
