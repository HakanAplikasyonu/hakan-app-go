package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestRuntimeRoleIsSubjectToRLS: the role application connections run as
// cannot skip row level security (issue #166).
func TestRuntimeRoleIsSubjectToRLS(t *testing.T) {
	d := testdb.Open(t)
	var user string
	var super, bypass bool
	err := d.Pool.QueryRow(context.Background(), `
		SELECT current_user, r.rolsuper, r.rolbypassrls FROM pg_roles r WHERE r.rolname = current_user`).
		Scan(&user, &super, &bypass)
	if err != nil {
		t.Fatal(err)
	}
	if user != RuntimeRole || super || bypass {
		t.Fatalf("connection runs as %s (superuser=%v, bypassrls=%v), want %s without either", user, super, bypass, RuntimeRole)
	}
}

// TestRLSEnforcement: every table with row level security is either in
// transition (rls_legacy policy for the runtime role; FORCE only together
// with rls_system) or enforced (no rls_legacy, FORCE, rls_system policy) and
// then listed in rls_enforced.txt.
func TestRLSEnforcement(t *testing.T) {
	d := testdb.Open(t)
	rows, err := d.Pool.Query(context.Background(), `
		SELECT c.relname, c.relforcerowsecurity,
		       EXISTS (SELECT 1 FROM pg_policies p WHERE p.schemaname = 'public' AND p.tablename = c.relname AND p.policyname = 'rls_legacy'),
		       EXISTS (SELECT 1 FROM pg_policies p WHERE p.schemaname = 'public' AND p.tablename = c.relname AND p.policyname = 'rls_system')
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relrowsecurity
		ORDER BY c.relname`)
	if err != nil {
		t.Fatal(err)
	}
	type state struct{ force, legacy, system bool }
	got, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (struct {
		name string
		state
	}, error) {
		var v struct {
			name string
			state
		}
		err := r.Scan(&v.name, &v.force, &v.legacy, &v.system)
		return v, err
	})
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, name := range readEnforcedTables(t) {
		listed[name] = true
	}
	var enforced []string
	for _, tb := range got {
		switch {
		// In transition: rls_legacy for the runtime role. Since migration
		// 000086 (#302) the owner is also forced, with its rls_system
		// policy (rls_owner_force); tables without an office or taxpayer
		// column may still be unforced.
		case tb.legacy && tb.force == tb.system:
			if listed[tb.name] {
				t.Errorf("%s is in rls_enforced.txt but still has rls_legacy (its migration must call rls_enforce)", tb.name)
			}
		case !tb.legacy && tb.force && tb.system:
			enforced = append(enforced, tb.name)
			if !listed[tb.name] {
				t.Errorf("%s is enforced but missing from rls_enforced.txt", tb.name)
			}
		default:
			t.Errorf("%s is half-way: legacy=%v force=%v system=%v (use rls_legacy or rls_enforce, not by hand)", tb.name, tb.legacy, tb.force, tb.system)
		}
		delete(listed, tb.name)
	}
	for name := range listed {
		t.Errorf("%s is in rls_enforced.txt but has no row level security", name)
	}
	t.Logf("enforced: %v; in transition: %d", enforced, len(got)-len(enforced))
}

// leakFixture inserts one row of its table for the given office and
// engagement through q (an rls.System transaction in the test).
type leakFixture func(ctx context.Context, q pgx.Tx, org, eng, user int64) error

// leakFixtures has one entry per enforced table (TestEnforcedTablesDoNotLeak
// fails for an enforced table without one).
var leakFixtures = map[string]leakFixture{
	"vat_carry_forwards": func(ctx context.Context, q pgx.Tx, org, eng, user int64) error {
		_, err := q.Exec(ctx, `
			INSERT INTO vat_carry_forwards (organization_id, engagement_id, period, amount, updated_by)
			VALUES ($1, $2, '2026-01', 100, $3)`, org, eng, user)
		return err
	},
	"export_settings": func(ctx context.Context, q pgx.Tx, org, eng, user int64) error {
		_, err := q.Exec(ctx, `
			INSERT INTO export_settings (organization_id, engagement_id, updated_by)
			VALUES ($1, $2, $3)`, org, eng, user)
		return err
	},
	"petitions": func(ctx context.Context, q pgx.Tx, org, eng, user int64) error {
		_, err := q.Exec(ctx, `
			INSERT INTO petitions (organization_id, engagement_id, template_key, template_version, rendered, draft, created_by)
			VALUES ($1, $2, 'genel_dilekce', 1, '{}'::jsonb, true, $3)`, org, eng, user)
		return err
	},
	"whatsapp_chat_links": func(ctx context.Context, q pgx.Tx, org, eng, user int64) error {
		_, err := q.Exec(ctx, `
			INSERT INTO whatsapp_chat_links (organization_id, engagement_id, chat_key, created_by)
			VALUES ($1, $2, '9055' || lpad(($2::bigint)::text, 8, '0'), $3)`, org, eng, user)
		return err
	},
	"whatsapp_captures": func(ctx context.Context, q pgx.Tx, org, eng, user int64) error {
		_, err := q.Exec(ctx, `
			INSERT INTO whatsapp_captures (organization_id, engagement_id, taxpayer_id, target, content_type, size_bytes, sha256, created_by)
			VALUES ($1, $2, COALESCE((SELECT e.taxpayer_id FROM engagements e WHERE e.id = $2), 0),
			        'document', 'application/pdf', 10, repeat('a', 64), $3)`, org, eng, user)
		return err
	},
}

// cascadesWithOffice: enforced tables whose rows go with their engagement
// (ON DELETE CASCADE); the leak test also checks that cascade.
var cascadesWithOffice = map[string]bool{
	"petitions": true, "vat_carry_forwards": true, "export_settings": true,
	"whatsapp_chat_links": true, "whatsapp_captures": true,
}

// leakTaxIDs are distinct VKNs with valid check digits, two per enforced table.
var leakTaxIDs = validVKNs(64)

func validVKNs(n int) []string {
	var out []string
	for base := 141000000; len(out) < n; base++ {
		for c := 0; c < 10; c++ {
			v := fmt.Sprintf("%d%d", base, c)
			if vknValid(v) {
				out = append(out, v)
				break
			}
		}
	}
	return out
}

// vknValid is the VKN check digit rule (internal/platform/taxid), repeated so
// this package does not import it for a fixture.
func vknValid(v string) bool {
	sum := 0
	for i := 0; i < 9; i++ {
		t := (int(v[i]-'0') + 10 - (i + 1)) % 10
		if t == 9 {
			sum += 9
			continue
		}
		p := 1
		for k := 0; k < 10-(i+1); k++ {
			p *= 2
		}
		sum += (t * p) % 9
	}
	return (10-sum%10)%10 == int(v[9]-'0')
}

// TestEnforcedTablesDoNotLeak is the cross-tenant test for every enforced
// table: with one row for office A and one for office B,
//   - an unscoped runtime connection sees and writes nothing;
//   - office A's scope sees only A's row and cannot write a row for B;
//   - rls.System sees both.
func TestEnforcedTablesDoNotLeak(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	tables := readEnforcedTables(t)
	sort.Strings(tables)
	for i, table := range tables {
		t.Run(table, func(t *testing.T) {
			// Fresh offices per table: the cascade step deletes B's engagement.
			orgA, orgB := d.Organization(t), d.Organization(t)
			userA, userB := d.User(t), d.User(t)
			engA := d.Engagement(t, orgA, d.Taxpayer(t, "legal", leakTaxIDs[2*i]))
			engB := d.Engagement(t, orgB, d.Taxpayer(t, "legal", leakTaxIDs[2*i+1]))
			fx, ok := leakFixtures[table]
			if !ok {
				t.Fatalf("no leak fixture for enforced table %s (add one to leakFixtures)", table)
			}
			err := rls.System(ctx, d.Pool, "test fixture", func(ctx context.Context, tx pgx.Tx) error {
				if err := fx(ctx, tx, orgA, engA, userA); err != nil {
					return err
				}
				return fx(ctx, tx, orgB, engB, userB)
			})
			if err != nil {
				t.Fatalf("fixture: %v", err)
			}
			countSQL := "SELECT count(*) FROM " + pgx.Identifier{table}.Sanitize()

			// Unscoped runtime connection.
			var n int
			if err := d.Pool.QueryRow(ctx, countSQL).Scan(&n); err != nil || n != 0 {
				t.Errorf("unscoped runtime sees %d rows (%v), want 0", n, err)
			}
			err = rls.System(ctx, d.Pool, "test: copy as runtime", func(ctx context.Context, tx pgx.Tx) error {
				// Leave System's bypass inside a savepoint: back to the runtime role.
				if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+RuntimeRole); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, "SELECT set_config('app.system', '', true)")
				if err != nil {
					return err
				}
				return fx(ctx, tx, orgA, engA, userA)
			})
			if !isRLSViolation(err) {
				t.Errorf("unscoped runtime insert: %v, want a row level security violation", err)
			}

			// Office A's scope.
			scopeA := rls.Scope{OrganizationID: orgA, UserID: userA, EngagementsReadAll: true}
			err = rls.WithScope(ctx, d.Pool, scopeA, func(ctx context.Context, tx pgx.Tx) error {
				if err := tx.QueryRow(ctx, countSQL).Scan(&n); err != nil {
					return err
				}
				if n != 1 {
					t.Errorf("office A sees %d rows, want only its own 1", n)
				}
				var other int
				if err := tx.QueryRow(ctx, countSQL+" WHERE organization_id = $1", orgB).Scan(&other); err != nil {
					return err
				}
				if other != 0 {
					t.Errorf("office A sees %d of office B's rows", other)
				}
				tag, err := tx.Exec(ctx, "UPDATE "+pgx.Identifier{table}.Sanitize()+" SET organization_id = organization_id WHERE organization_id = $1", orgB)
				if err != nil {
					return err
				}
				if tag.RowsAffected() != 0 {
					t.Errorf("office A updated %d of office B's rows", tag.RowsAffected())
				}
				return nil
			})
			if err != nil {
				t.Fatalf("scope A: %v", err)
			}
			err = rls.WithScope(ctx, d.Pool, scopeA, func(ctx context.Context, tx pgx.Tx) error {
				return fx(ctx, tx, orgB, engB, userA)
			})
			if !isRLSViolation(err) {
				t.Errorf("office A writing a row for office B: %v, want a row level security violation", err)
			}

			// Deliberate bypass sees both.
			err = rls.System(ctx, d.Pool, "test: count", func(ctx context.Context, tx pgx.Tx) error {
				return tx.QueryRow(ctx, countSQL).Scan(&n)
			})
			if err != nil || n != 2 {
				t.Errorf("rls.System sees %d rows (%v), want 2", n, err)
			}

			// Deleting an office still removes its rows: foreign key cascades
			// are not filtered by row level security.
			if cascadesWithOffice[table] {
				if _, err := d.Pool.Exec(ctx, `DELETE FROM engagements WHERE id = $1`, engB); err != nil {
					t.Fatalf("delete office B's engagement: %v", err)
				}
				err = rls.System(ctx, d.Pool, "test: count", func(ctx context.Context, tx pgx.Tx) error {
					return tx.QueryRow(ctx, countSQL+" WHERE organization_id = $1", orgB).Scan(&n)
				})
				if err != nil || n != 0 {
					t.Errorf("office B's rows after its engagement was deleted: %d (%v), want 0", n, err)
				}
			}
		})
	}
}

func TestSystemNeedsReason(t *testing.T) {
	d := testdb.Open(t)
	err := rls.System(context.Background(), d.Pool, " ", func(context.Context, pgx.Tx) error { return nil })
	if !errors.Is(err, rls.ErrNoReason) {
		t.Fatalf("err = %v, want ErrNoReason", err)
	}
}

func isRLSViolation(err error) bool {
	var pg *pgconn.PgError
	// 42501 insufficient_privilege: "new row violates row-level security policy".
	return errors.As(err, &pg) && pg.Code == "42501"
}

func readEnforcedTables(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("rls_enforced.txt")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}
