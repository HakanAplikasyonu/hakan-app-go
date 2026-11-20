package connections

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// The sweep opens INT_CONNECTION_FAILED for failing connections across
// offices, closes last month's warning and that of a recovered connection
// (seed 000129 supplies the codes' configuration). A taxpayer has one active
// engagement, so the two offices follow two taxpayers.
func TestSweepDB(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	orgA, orgB := d.Organization(t), d.Organization(t)
	tp, tpB := d.Taxpayer(t, "legal", "9000006691"), d.Taxpayer(t, "legal", "9000006692")
	engA, engB := d.Engagement(t, orgA, tp), d.Engagement(t, orgB, tpB)

	// Fixtures and reads go through the row level security bypass, as in
	// the worker.
	sys := func(fn func(ctx context.Context, tx pgx.Tx) error) {
		t.Helper()
		if err := rls.System(ctx, d.Pool, "test: connection warnings", fn); err != nil {
			t.Fatal(err)
		}
	}
	failing := func(taxpayer int64) int64 {
		t.Helper()
		var id int64
		sys(func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `
			INSERT INTO source_connections (taxpayer_id, source_system, setup_state, status, last_error, last_success_at)
			VALUES ($1, 'bank_vakifbank', 'active', 'auth_failed', '{"reason":"bad_credentials"}', NOW() - interval '2 days')
			RETURNING id`, taxpayer).Scan(&id)
		})
		return id
	}
	conn, connB := failing(tp), failing(tpB)
	// An open warning of an earlier month on the same connection.
	sys(func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
		INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, period, entity_type, entity_id,
			code, severity, params, producer)
		VALUES ($1, $2, $3, '2000-01', 'source_connection', $4, 'INT_CONNECTION_FAILED', 'warning', '{}', $5)`,
			orgA, engA, tp, conn, Producer)
		return err
	})

	sweep := func() int {
		t.Helper()
		var n int
		sys(func(ctx context.Context, _ pgx.Tx) error {
			q := database.NewQueries(d.Pool)
			var err error
			n, err = New(q, engine.New(repository.NewWarnings(q), repository.NewConfig(q))).Sweep(ctx)
			return err
		})
		return n
	}
	openByEngagement := func() map[int64][]string {
		t.Helper()
		out := map[int64][]string{}
		sys(func(ctx context.Context, tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `
				SELECT engagement_id, code || '@' || COALESCE(period, '') FROM data_warnings
				WHERE taxpayer_id = ANY($1::bigint[]) AND producer = $2 AND status = 'open' ORDER BY id`, []int64{tp, tpB}, Producer)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var eng int64
				var s string
				if err := rows.Scan(&eng, &s); err != nil {
					return err
				}
				out[eng] = append(out[eng], s)
			}
			return rows.Err()
		})
		return out
	}

	// Other tests' connections may share the database: at least these two.
	if n := sweep(); n < 2 {
		t.Fatalf("created = %d, want one per office", n)
	}
	want := "INT_CONNECTION_FAILED@" + Period(time.Now())
	got := openByEngagement()
	if len(got[engA]) != 1 || got[engA][0] != want || len(got[engB]) != 1 || got[engB][0] != want {
		t.Fatalf("open = %v, want %s on both engagements (last month's closed)", got, want)
	}
	if n := sweep(); n != 0 {
		t.Fatalf("re-run created %d", n)
	}

	sys(func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE source_connections SET status = 'active', last_error = NULL, last_success_at = NOW()
			WHERE (taxpayer_id, id) IN (($1, $2), ($3, $4))`, tp, conn, tpB, connB)
		return err
	})
	sweep()
	if got := openByEngagement(); len(got) != 0 {
		t.Fatalf("recovered connection still warns: %v", got)
	}
}
