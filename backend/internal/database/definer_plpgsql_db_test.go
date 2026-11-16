package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestDefinerFunctionsPlpgsql (issue #317): the managed Postgres alternative
// to migration 000089 (scripts/managed-postgres/definer-functions-plpgsql.sql)
// replaces the two SECURITY DEFINER functions with plpgsql that turns
// app.system on inside the body and restores it. They give the same
// answers, carry no app.system setting (which would need GRANT SET ON
// PARAMETER), and do not leak the setting to the caller. With a
// non-superuser owner (CI backend-owner job) the function must see the
// assignment under FORCE.
func TestDefinerFunctionsPlpgsql(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	raw, err := os.ReadFile(filepath.Join(testdb.MigrationsDir(), "..", "..", "scripts", "managed-postgres", "definer-functions-plpgsql.sql"))
	if err != nil {
		t.Fatal(err)
	}
	owner, err := pgx.Connect(ctx, d.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close(ctx) }()
	for i := 0; i < 2; i++ { // idempotent
		if _, err := owner.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("apply (%d): %v", i+1, err)
		}
	}
	rows, err := owner.Query(ctx, `SELECT proname, (SELECT lanname FROM pg_language l WHERE l.oid = prolang)::text, COALESCE(array_to_string(proconfig, ','), '')
		FROM pg_proc WHERE proname IN ('app_engagement_visible', 'engagement_document_visible')`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name, lang, cfg string
		if err := rows.Scan(&name, &lang, &cfg); err != nil {
			t.Fatal(err)
		}
		if lang != "plpgsql" || strings.Contains(cfg, "app.system") || !strings.Contains(cfg, "search_path=public") {
			t.Errorf("%s: language %s, config %q (want plpgsql, search_path only)", name, lang, cfg)
		}
	}
	rows.Close()

	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", "1234567890")
	eng := d.Engagement(t, org, tp)
	user := d.User(t)
	d.Member(t, org, user, "member")
	d.Assign(t, org, eng, user)

	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1::bigint::text, true), set_config('app.user_id', $2::bigint::text, true)`, org, user); err != nil {
		t.Fatal(err)
	}
	var assigned, other, document bool
	var leaked string
	if err := tx.QueryRow(ctx, `SELECT app_engagement_visible($1, $2)`, org, eng).Scan(&assigned); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT engagement_document_visible($1, CURRENT_DATE)`, eng).Scan(&document); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT COALESCE(current_setting('app.system', true), '')`).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.user_id', '999999999', true)`); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT app_engagement_visible($1, $2)`, org, eng).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if !assigned || other || !document || leaked == "on" {
		t.Fatalf("assigned=%v (want true) other=%v (want false) document=%v (want true) app.system after=%q (want not on)", assigned, other, document, leaked)
	}
}
