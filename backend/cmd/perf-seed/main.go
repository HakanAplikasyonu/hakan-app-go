// Command perf-seed builds the load-test data set (issue #182) on a migrated
// database: one office ("perf-smmm") with an owner, N taxpayers with an
// active engagement each, M invoices and open warnings on some of them.
//
// Usage (repo root, local or staging only):
//
//	make perf-seed                                  # 500 taxpayers, 10k invoices
//	make perf-seed ARGS="-taxpayers 500 -invoices 10000 -reset"
//
// The distribution is skewed on purpose: 30% of the invoices belong to the
// first taxpayer, so deep pages of one list are measured too. Identities are
// invented (VKN 98xxxxxxxx, *.perf.test addresses).
//
// It refuses to run with APP_ENV=production.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	authmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/model"
	authrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/repository"
	orgmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/password"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

const (
	orgSlug         = "perf-smmm"
	ownerEmail      = "perf-owner@perf.test"
	defaultPassword = "Perf1234!"
	taxIDPrefix     = "98"
	sourceSystem    = "perf_seed"
)

func main() {
	nTaxpayers := flag.Int("taxpayers", 500, "taxpayers (each with an active engagement)")
	nInvoices := flag.Int("invoices", 10000, "invoices in total")
	reset := flag.Bool("reset", false, "drop the perf office and its data first")
	pass := flag.String("password", "", "owner password (default PERF_PASSWORD or "+defaultPassword+")")
	flag.Parse()
	if *pass == "" {
		*pass = os.Getenv("PERF_PASSWORD")
	}
	if *pass == "" {
		*pass = defaultPassword
	}
	if *nTaxpayers < 1 || *nTaxpayers > 99_999_999 || *nInvoices < 0 {
		fatal("invalid -taxpayers / -invoices")
	}

	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}
	if cfg.App.Env == "production" {
		fatal("refusing to seed load-test data with APP_ENV=production")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()

	if *reset {
		if err := drop(ctx, pool); err != nil {
			fatal("reset: %v", err)
		}
		fmt.Println("perf office dropped")
	}
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM organizations WHERE slug = $1)`, orgSlug).Scan(&exists); err != nil {
		fatal("%v", err)
	}
	if exists {
		fmt.Printf("perf office %q already exists; run with -reset to rebuild it\n", orgSlug)
		return
	}
	start := time.Now()
	if err := seed(ctx, pool, *pass, *nTaxpayers, *nInvoices); err != nil {
		if derr := drop(ctx, pool); derr != nil {
			fatal("%v (cleanup also failed: %v)", err, derr)
		}
		fatal("%v", err)
	}
	fmt.Printf("perf office %q: %d taxpayers, %d invoices in %s\n", orgSlug, *nTaxpayers, *nInvoices, time.Since(start).Round(time.Millisecond))
	fmt.Printf("login: %s / (password)\n", ownerEmail)
}

func seed(ctx context.Context, pool *pgxpool.Pool, pass string, nTaxpayers, nInvoices int) error {
	queries := database.NewQueries(pool)
	users := authrepo.NewPostgres(pool, queries)
	hash, err := password.Hash(pass)
	if err != nil {
		return err
	}
	owner, err := users.FindUserByEmail(ctx, ownerEmail)
	switch {
	case err == nil:
		if _, err := pool.Exec(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, hash, owner.ID); err != nil {
			return err
		}
	case errors.Is(err, authrepo.ErrNotFound):
		owner, err = users.CreateUser(ctx, authmodel.User{
			Email: ownerEmail, PasswordHash: hash, Name: "Perf", Surname: "Sahip", Status: "active",
		}, true)
		if err != nil {
			return fmt.Errorf("owner: %w", err)
		}
	default:
		return err
	}
	org, err := orgusecase.New(orgrepository.NewPostgres(pool, queries), nil).CreateOrganization(ctx, orgmodel.CreateOrganizationInput{
		Name: "Perf SMMM", Slug: orgSlug, OwnerUserUUID: owner.UUID,
	})
	if err != nil {
		return fmt.Errorf("organization: %w", err)
	}

	steps := []struct {
		name string
		sql  string
		args []any
	}{
		{"taxpayers", `
			INSERT INTO taxpayers (kind, tax_id, title, tax_office, contact_phone, contact_email)
			SELECT 'legal', $1 || lpad(g::text, 8, '0'), 'Perf Mükellef ' || g || ' Ltd. Şti.', 'Perf VD',
			       '+90 555 ' || lpad(g::text, 7, '0'), 'mukellef' || g || '@perf.test'
			FROM generate_series(1, $2) g`, []any{taxIDPrefix, nTaxpayers}},
		{"engagements", `
			INSERT INTO engagements (organization_id, taxpayer_id, status, started_at)
			SELECT $1, t.id, 'active', TIMESTAMPTZ '2024-01-01'
			FROM taxpayers t WHERE t.tax_id LIKE $2 || '%'`, []any{org.ID, taxIDPrefix}},
		// 30% of the invoices on the first taxpayer, the rest round-robin.
		{"invoices", `
			WITH tp AS (
				SELECT id, row_number() OVER (ORDER BY tax_id) AS n FROM taxpayers WHERE tax_id LIKE $1 || '%'
			), g AS (
				SELECT i, CASE WHEN i % 10 < 3 THEN 1 ELSE 1 + (i % $2) END AS n,
				       DATE '2025-01-01' + (i % 540) AS d
				FROM generate_series(1, $3) i
			)
			INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number,
				direction, type, issue_date, period, currency, tax_exclusive_amount, tax_amount,
				tax_inclusive_amount, payable_amount, tax_summary_source, customer_name, customer_tax_number)
			SELECT tp.id, $4, 'perf-' || g.i, 'api', 'PRF' || to_char(g.d, 'YYYY') || lpad(g.i::text, 9, '0'),
				CASE WHEN g.i % 2 = 0 THEN 'outgoing' ELSE 'incoming' END, 'SATIS', g.d, to_char(g.d, 'YYYY-MM'), 'TRY',
				(g.i % 1000) * 10, (g.i % 1000) * 2, (g.i % 1000) * 12, (g.i % 1000) * 12, 'header_estimate',
				'Karşı Taraf ' || (g.i % 300), '97' || lpad((g.i % 300)::text, 8, '0')
			FROM g JOIN tp ON tp.n = g.n`, []any{taxIDPrefix, nTaxpayers, nInvoices, sourceSystem}},
		// One open warning on every 20th invoice.
		{"warnings", `
			INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, entity_type, entity_id,
				code, severity, producer, period)
			SELECT $1, e.id, i.taxpayer_id, 'invoice', i.id, 'PERF_CHECK', 'warning', 'perf_seed', i.period
			FROM invoices i JOIN engagements e ON e.taxpayer_id = i.taxpayer_id AND e.organization_id = $1
			WHERE i.source_system = $2 AND i.id % 20 = 0`, []any{org.ID, sourceSystem}},
	}
	// Bulk rows span every taxpayer of the office: one system transaction.
	return rls.System(ctx, pool, "perf-seed bulk data", func(ctx context.Context, tx pgx.Tx) error {
		for _, st := range steps {
			tag, err := tx.Exec(ctx, st.sql, st.args...)
			if err != nil {
				return fmt.Errorf("%s: %w", st.name, err)
			}
			fmt.Printf("%-12s %d\n", st.name, tag.RowsAffected())
		}
		_, err := tx.Exec(ctx, `ANALYZE taxpayers, engagements, invoices, data_warnings`)
		return err
	})
}

// drop removes the perf office, its taxpayers and their data.
func drop(ctx context.Context, pool *pgxpool.Pool) error {
	return rls.System(ctx, pool, "perf-seed reset", func(ctx context.Context, tx pgx.Tx) error {
		return dropTx(ctx, tx)
	})
}

func dropTx(ctx context.Context, tx pgx.Tx) error {
	for _, q := range []string{
		`DELETE FROM data_warnings WHERE producer = 'perf_seed'`,
		`DELETE FROM invoices WHERE source_system = 'perf_seed'`,
		`DELETE FROM engagements WHERE organization_id IN (SELECT id FROM organizations WHERE slug = '` + orgSlug + `')`,
		`DELETE FROM taxpayers WHERE tax_id LIKE '` + taxIDPrefix + `%' AND title LIKE 'Perf Mükellef %'`,
		`DELETE FROM organizations WHERE slug = '` + orgSlug + `'`,
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			return fmt.Errorf("%s: %w", q, err)
		}
	}
	return nil
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "perf-seed: "+format+"\n", args...)
	os.Exit(1)
}
