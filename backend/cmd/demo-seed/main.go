// Command demo-seed builds the reproducible demo environment for the internal
// review (D17, issue #17) on a migrated database.
//
// Usage (repo root):
//
//	make demo-seed                 # create if missing
//	make demo-seed ARGS=-reset     # drop the demo office and build it again
//
// What it creates (all identities invented, see internal/demo):
//   - office "demo-smmm" with an owner, an admin (müşavir) and a member (stajyer)
//   - a taxpayer-side user (not linked yet: taxpayer_members arrives with #4)
//   - two taxpayers with an active engagement each and their assignments
//
// What it does not create yet, and why:
//   - taxpayer identity (VKN, unvan): the taxpayers table gets those columns
//     with #3; the demo values live in internal/demo.
//   - invoices: the UBL set (internal/demo/ubl) is written to the database by
//     the upload flow (#11). Until then upload it from the office panel.
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
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	authmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/model"
	authrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/engagements"
	orgmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/password"
)

// defaultPassword is for local demo accounts only (the command refuses
// production). Override with -password or DEMO_PASSWORD.
const defaultPassword = "Demo1234!"

func main() {
	reset := flag.Bool("reset", false, "drop the demo office (and its taxpayers) before seeding")
	pass := flag.String("password", "", "password for every demo account (default DEMO_PASSWORD or "+defaultPassword+")")
	flag.Parse()

	if *pass == "" {
		*pass = os.Getenv("DEMO_PASSWORD")
	}
	if *pass == "" {
		*pass = defaultPassword
	}

	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}
	if cfg.App.Env == "production" {
		fatal("refusing to seed demo data with APP_ENV=production")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()

	if *reset {
		if err := dropDemo(ctx, pool); err != nil {
			fatal("reset: %v", err)
		}
		fmt.Println("demo office dropped")
	}

	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM organizations WHERE slug = $1)`, demo.OrgSlug).Scan(&exists); err != nil {
		fatal("%v", err)
	}
	if exists {
		fmt.Printf("demo office %q already exists; run with -reset to rebuild it\n", demo.OrgSlug)
		printSummary()
		return
	}
	if err := seed(ctx, pool, *pass); err != nil {
		// The steps run through the services, each in its own transaction;
		// undo a half-built office so the next run starts clean.
		if derr := dropDemo(ctx, pool); derr != nil {
			fatal("%v (cleanup also failed: %v)", err, derr)
		}
		fatal("%v", err)
	}
	printSummary()
}

func seed(ctx context.Context, pool *pgxpool.Pool, pass string) error {
	queries := database.NewQueries(pool)
	users := authrepo.NewPostgres(pool, queries)
	hash, err := password.Hash(pass)
	if err != nil {
		return fmt.Errorf("password: %w", err)
	}

	byEmail := map[string]authmodel.User{}
	for _, m := range append(append([]demo.Member{}, demo.Members...), demo.TaxpayerUser) {
		u, err := ensureUser(ctx, pool, users, m, hash)
		if err != nil {
			return fmt.Errorf("user %s: %w", m.Email, err)
		}
		byEmail[m.Email] = u
	}

	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, queries), nil)
	owner := byEmail[demo.Members[0].Email]
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{
		Name: demo.OrgName, Slug: demo.OrgSlug, OwnerUserUUID: owner.UUID,
	})
	if err != nil {
		return fmt.Errorf("organization: %w", err)
	}
	if org.Slug != demo.OrgSlug {
		return fmt.Errorf("organization slug %q was taken; got %q", demo.OrgSlug, org.Slug)
	}
	for _, m := range demo.Members[1:] {
		if _, err := orgSvc.AddMember(ctx, org.UUID, byEmail[m.Email].UUID, m.RoleKey); err != nil {
			return fmt.Errorf("member %s (%s): %w", m.Email, m.RoleKey, err)
		}
	}

	store := engagements.NewPostgres(queries)
	engSvc := engagements.New(store, nil)
	for _, tp := range demo.Taxpayers {
		var taxpayerID int64
		if err := pool.QueryRow(ctx, `INSERT INTO taxpayers DEFAULT VALUES RETURNING id`).Scan(&taxpayerID); err != nil {
			return fmt.Errorf("taxpayer %s: %w", tp.Key, err)
		}
		eng, err := store.CreateEngagement(ctx, org.ID, taxpayerID)
		if err != nil {
			return fmt.Errorf("engagement %s: %w", tp.Key, err)
		}
		for _, email := range tp.Assignees {
			if err := engSvc.Assign(ctx, org.ID, eng.ID, byEmail[email].UUID); err != nil {
				return fmt.Errorf("assign %s → %s: %w", email, tp.Key, err)
			}
		}
		fmt.Printf("taxpayer %-6s engagement %s\n", tp.Key, eng.Uuid)
	}
	fmt.Printf("demo office %q created\n", demo.OrgSlug)
	return nil
}

// ensureUser creates the account or, when it exists, resets its password so
// the printed credentials always work.
func ensureUser(ctx context.Context, pool *pgxpool.Pool, repo *authrepo.Postgres, m demo.Member, hash string) (authmodel.User, error) {
	u, err := repo.FindUserByEmail(ctx, m.Email)
	if err == nil {
		_, err = pool.Exec(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, hash, u.ID)
		return u, err
	}
	if !errors.Is(err, authrepo.ErrNotFound) {
		return authmodel.User{}, err
	}
	return repo.CreateUser(ctx, authmodel.User{
		Email: m.Email, PasswordHash: hash, Name: m.Name, Surname: m.Surname, Status: "active",
	}, true)
}

// dropDemo deletes the demo office; memberships, engagements and assignments
// go with it (ON DELETE CASCADE). Its taxpayers are removed afterwards when
// nothing else references them. Users are kept and reused.
func dropDemo(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var orgID int64
	err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE slug = $1`, demo.OrgSlug).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT taxpayer_id FROM engagements WHERE organization_id = $1`, orgID)
	if err != nil {
		return err
	}
	taxpayers, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, orgID); err != nil {
		return err
	}
	for _, id := range taxpayers {
		if _, err := tx.Exec(ctx, `DELETE FROM taxpayers t WHERE t.id = $1 AND NOT EXISTS (SELECT 1 FROM engagements e WHERE e.taxpayer_id = t.id)`, id); err != nil {
			return fmt.Errorf("taxpayer %d still referenced: %w", id, err)
		}
	}
	return tx.Commit(ctx)
}

func printSummary() {
	fmt.Println()
	fmt.Printf("Office:     /t/%s  (%s)\n", demo.OrgSlug, demo.OrgName)
	fmt.Println("Accounts (password: -password / DEMO_PASSWORD, default " + defaultPassword + "):")
	for _, m := range demo.Members {
		fmt.Printf("  %-28s %-7s %s %s · %s\n", m.Email, m.RoleKey, m.Name, m.Surname, m.Title)
	}
	fmt.Printf("  %-28s %-7s %s %s · %s (mükellef tarafı, #4 bekleniyor)\n", demo.TaxpayerUser.Email, "-", demo.TaxpayerUser.Name, demo.TaxpayerUser.Surname, demo.TaxpayerUser.Title)
	fmt.Println("Taxpayers:")
	for _, tp := range demo.Taxpayers {
		fmt.Printf("  %-6s %-11s %s (%s VD)\n", tp.Key, tp.TaxID, tp.Title, tp.TaxOffice)
	}
	fmt.Println("UBL set:    backend/internal/demo/ubl (upload from the office panel until #11)")
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "demo-seed: "+format+"\n", args...)
	os.Exit(1)
}
