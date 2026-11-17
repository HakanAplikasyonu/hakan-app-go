package testdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// m15First is the first migration of milestone M15 (issue #453): the
// tables it and later migrations create must index their foreign keys.
const m15First = "000093"

// TestMigrationsUpDownUp (issue #453): every migration applied, every
// down applied in reverse, every up applied again. Each down must run on
// the schema its up left; the second up must give the same schema as the
// first (a down that forgets an object makes the second up fail or differ).
func TestMigrationsUpDownUp(t *testing.T) {
	d := Open(t) // up, once
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, d.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	first := schemaSnapshot(ctx, t, conn)
	if len(first) == 0 {
		t.Fatal("empty schema after up")
	}
	downs := migrationFiles(t, "down")
	for i := len(downs) - 1; i >= 0; i-- {
		applyFile(ctx, t, conn, downs[i])
	}
	if left := schemaSnapshot(ctx, t, conn); len(left) != 0 {
		t.Errorf("after every down %d objects are left, e.g. %s", len(left), left[0])
	}
	for _, f := range migrationFiles(t, "up") {
		applyFile(ctx, t, conn, f)
	}
	second := schemaSnapshot(ctx, t, conn)
	if a, b := strings.Join(first, "\n"), strings.Join(second, "\n"); a != b {
		t.Errorf("schema after up/down/up differs from the first up:\n%s", diffLines(first, second))
	}
}

// TestM15ForeignKeysIndexed (issue #453): a foreign key of an M15 table
// has an index that starts with its columns, so the parent's delete
// (CASCADE / SET NULL) and the lookups by parent do not scan the table.
// Audit columns pointing at users and a bare taxpayer_id (ON DELETE
// RESTRICT: users and taxpayers are not deleted) are exempt.
func TestM15ForeignKeysIndexed(t *testing.T) {
	d := Open(t)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, d.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	tables := m15Tables(t)
	if len(tables) < 20 {
		t.Fatalf("only %d M15 tables found: %v", len(tables), tables)
	}
	rows, err := conn.Query(ctx, `
		SELECT c.conrelid::regclass::text, c.conname, c.confrelid::regclass::text,
		       (SELECT array_agg(a.attname::text ORDER BY k.n) FROM unnest(c.conkey) WITH ORDINALITY k(attnum, n)
		          JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum)
		FROM pg_constraint c
		WHERE c.contype = 'f' AND c.conrelid::regclass::text = ANY($1)
		  AND NOT EXISTS (
		    SELECT 1 FROM pg_index i
		    WHERE i.indrelid = c.conrelid
		      AND (i.indkey::int2[])[0:array_length(c.conkey, 1) - 1] @> c.conkey
		      AND (i.indkey::int2[])[0:array_length(c.conkey, 1) - 1] <@ c.conkey)
		ORDER BY 1, 2`, tables)
	if err != nil {
		t.Fatal(err)
	}
	type fk struct {
		table, name, parent string
		cols                []string
	}
	var fks []fk
	for rows.Next() {
		var k fk
		if err := rows.Scan(&k.table, &k.name, &k.parent, &k.cols); err != nil {
			t.Fatal(err)
		}
		fks = append(fks, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, k := range fks {
		if k.parent == "users" || (k.parent == "taxpayers" && len(k.cols) == 1) {
			continue
		}
		if indexedByLeading(ctx, t, conn, k.table, k.cols) {
			continue
		}
		t.Errorf("%s.%s (%s → %s) has no index starting with its columns", k.table, k.name, strings.Join(k.cols, ", "), k.parent)
	}
}

// indexedByLeading: an index whose first column is the key's last column
// (the record id; (taxpayer_id, id) keys look rows up by the id alone).
func indexedByLeading(ctx context.Context, t *testing.T, conn *pgx.Conn, table string, cols []string) bool {
	t.Helper()
	if len(cols) < 2 {
		return false
	}
	var ok bool
	err := conn.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_index i JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = i.indkey[0]
		WHERE i.indrelid = $1::regclass AND a.attname = $2)`, table, cols[len(cols)-1]).Scan(&ok)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

var createTableRe = regexp.MustCompile(`(?mi)^\s*CREATE TABLE (?:IF NOT EXISTS )?([a-z_][a-z0-9_]*)\s*\(`)

// m15Tables are the tables created by migrations from m15First on.
func m15Tables(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, f := range migrationFiles(t, "up") {
		if filepath.Base(f) < m15First {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range createTableRe.FindAllStringSubmatch(string(raw), -1) {
			out = append(out, m[1])
		}
	}
	return out
}

func migrationFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(MigrationsDir(), "*."+dir+".sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no %s migrations: %v", dir, err)
	}
	sort.Strings(files)
	return files
}

func applyFile(ctx context.Context, t *testing.T, conn *pgx.Conn, f string) {
	t.Helper()
	raw, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.PgConn().Exec(ctx, string(raw)).ReadAll(); err != nil {
		t.Fatalf("%s: %v", filepath.Base(f), err)
	}
}

// schemaSnapshot lists the public schema's objects with their definitions:
// columns, constraints, indexes, policies, triggers, functions, types,
// sequences and views, and the seeded permissions of the system roles.
func schemaSnapshot(ctx context.Context, t *testing.T, conn *pgx.Conn) []string {
	t.Helper()
	queries := []string{
		`SELECT 'column ' || table_name || '.' || column_name || ' ' || data_type || ' null=' || is_nullable || ' default=' || COALESCE(column_default, '')
		   FROM information_schema.columns WHERE table_schema = 'public'`,
		`SELECT 'constraint ' || conrelid::regclass::text || '.' || conname || ' ' || pg_get_constraintdef(oid)
		   FROM pg_constraint WHERE connamespace = 'public'::regnamespace AND conrelid <> 0`,
		`SELECT 'index ' || indexdef FROM pg_indexes WHERE schemaname = 'public'`,
		`SELECT 'policy ' || tablename || '.' || policyname || ' ' || cmd || ' ' || COALESCE(qual, '') || ' ' || COALESCE(with_check, '')
		   FROM pg_policies WHERE schemaname = 'public'`,
		`SELECT 'rls ' || relname || ' ' || relrowsecurity::text || ' ' || relforcerowsecurity::text
		   FROM pg_class WHERE relnamespace = 'public'::regnamespace AND relkind = 'r'`,
		`SELECT 'trigger ' || tgrelid::regclass::text || ' ' || pg_get_triggerdef(oid)
		   FROM pg_trigger WHERE NOT tgisinternal AND tgrelid::regclass::text NOT LIKE 'pg_%'`,
		`SELECT 'function ' || p.oid::regprocedure::text || ' ' || md5(p.prosrc)
		   FROM pg_proc p WHERE p.pronamespace = 'public'::regnamespace
		     AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = p.oid AND d.deptype = 'e')`,
		`SELECT 'type ' || t.typname || ' ' || COALESCE((SELECT string_agg(enumlabel, ',' ORDER BY enumsortorder) FROM pg_enum e WHERE e.enumtypid = t.oid), '')
		   FROM pg_type t WHERE t.typnamespace = 'public'::regnamespace AND t.typtype = 'e'`,
		`SELECT 'sequence ' || sequence_name FROM information_schema.sequences WHERE sequence_schema = 'public'`,
		`SELECT 'view ' || viewname || ' ' || md5(definition) FROM pg_views WHERE schemaname = 'public'`,
	}
	// Seed rows a down removes and the next up must put back.
	var seeded bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass('public.permissions') IS NOT NULL AND to_regclass('public.role_permissions') IS NOT NULL`).Scan(&seeded); err != nil {
		t.Fatal(err)
	}
	if seeded {
		queries = append(queries,
			`SELECT 'permission ' || slug FROM permissions`,
			`SELECT 'role_permission ' || r.slug || ' ' || p.slug FROM role_permissions rp
			   JOIN roles r ON r.id = rp.role_id JOIN permissions p ON p.id = rp.permission_id WHERE r.organization_id IS NULL`)
	}
	var out []string
	for _, q := range queries {
		rows, err := conn.Query(ctx, q)
		if err != nil {
			t.Fatalf("snapshot: %v (%s)", err, q)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(out)
	return out
}

func diffLines(a, b []string) string {
	in := func(xs []string) map[string]bool {
		m := map[string]bool{}
		for _, x := range xs {
			m[x] = true
		}
		return m
	}
	ma, mb := in(a), in(b)
	var sb strings.Builder
	n := 0
	for _, x := range a {
		if !mb[x] && n < 40 {
			fmt.Fprintf(&sb, "- %s\n", x)
			n++
		}
	}
	for _, x := range b {
		if !ma[x] && n < 40 {
			fmt.Fprintf(&sb, "+ %s\n", x)
			n++
		}
	}
	return sb.String()
}
