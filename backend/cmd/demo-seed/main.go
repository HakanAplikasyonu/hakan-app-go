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
//   - two taxpayers (VKN/TCKN, unvan, vergi dairesi) with an active
//     engagement each and their assignments
//   - a taxpayer-side user, owner of Atlas Gıda (taxpayer_members), who signs
//     in to the mükellef panel (/m)
//
// Invoices are not seeded: upload the UBL set (internal/demo/ubl) from the
// office panel (UBL yükle) so the demo shows the real upload flow. -reset
// removes that data too (only for demo-only taxpayers, see dropDemo).
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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	authmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/model"
	authrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/repository"
	bankposting "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/posting"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/engagements"
	orgmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/support"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/taxpayers"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/password"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// defaultPassword is for local demo accounts only (the command refuses
// production). Override with -password or DEMO_PASSWORD.
const defaultPassword = "Demo1234!"

func main() {
	reset := flag.Bool("reset", false, "drop the demo office (and its taxpayers) before seeding")
	pass := flag.String("password", "", "password for every demo account (default DEMO_PASSWORD or "+defaultPassword+")")
	// Ofisin ClickFix widget kimliği (#387): verilirse demo ofisin destek
	// ayarına yazılır ve mükellef portalında Destek düğmesi görünür. Boş
	// varsayılan: e2e / görsel testler dış betik yüklemez.
	widget := flag.String("clickfix-widget-id", "", "ClickFix widget id of the demo office (support channel of its taxpayers; default DEMO_CLICKFIX_WIDGET_ID)")
	flag.Parse()
	if *widget == "" {
		*widget = os.Getenv("DEMO_CLICKFIX_WIDGET_ID")
	}

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
	payrollSecret = cfg.Encryption.Key

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()

	if st, err := storage.NewFromConfig(ctx, cfg.Storage); err == nil {
		seedStore = fallbackStore{primary: st, memory: storage.NewMemory()}
	} else {
		fmt.Printf("object store unavailable (%v); demo export files are not stored\n", err)
	}

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
	if err := seed(ctx, pool, *pass, *widget); err != nil {
		// The steps run through the services, each in its own transaction;
		// undo a half-built office so the next run starts clean.
		if derr := dropDemo(ctx, pool); derr != nil {
			fatal("%v (cleanup also failed: %v)", err, derr)
		}
		fatal("%v", err)
	}
	printSummary()
}

func seed(ctx context.Context, pool *pgxpool.Pool, pass, clickfixWidgetID string) error {
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
	if id, err := support.NormalizeWidgetID(clickfixWidgetID); err != nil {
		return fmt.Errorf("clickfix widget id: %w", err)
	} else if id != "" {
		if _, err := queries.UpsertOfficeSupportSettings(ctx, db.UpsertOfficeSupportSettingsParams{
			OrganizationID: org.ID, ClickfixWidgetID: id,
		}); err != nil {
			return fmt.Errorf("support settings: %w", err)
		}
		fmt.Printf("demo office ClickFix widget %s\n", id)
	}
	for _, m := range demo.Members[1:] {
		if _, err := orgSvc.AddMember(ctx, org.UUID, byEmail[m.Email].UUID, m.RoleKey); err != nil {
			return fmt.Errorf("member %s (%s): %w", m.Email, m.RoleKey, err)
		}
	}

	store := engagements.NewPostgres(queries)
	engSvc := engagements.New(store, nil)
	tpSvc := taxpayers.New(taxpayers.NewPostgres(queries))
	for _, tp := range demo.Taxpayers {
		identity, err := ensureTaxpayer(ctx, tpSvc, tp)
		if err != nil {
			return fmt.Errorf("taxpayer %s: %w", tp.Key, err)
		}
		created, err := engSvc.Create(ctx, org.ID, identity.UUID)
		if err != nil {
			return fmt.Errorf("engagement %s: %w", tp.Key, err)
		}
		eng, err := store.GetEngagement(ctx, org.ID, created.UUID)
		if err != nil {
			return fmt.Errorf("engagement %s: %w", tp.Key, err)
		}
		if err := seedReportContact(ctx, pool, eng.TaxpayerID, tp.Key); err != nil {
			return fmt.Errorf("report contact → %s: %w", tp.Key, err)
		}
		for _, email := range tp.Assignees {
			if err := engSvc.Assign(ctx, org.ID, eng.ID, byEmail[email].UUID); err != nil {
				return fmt.Errorf("assign %s → %s: %w", email, tp.Key, err)
			}
		}
		if tp.Key == demo.TaxpayerUserOf {
			if err := linkTaxpayerUser(ctx, pool, identity.UUID.String(), byEmail[demo.TaxpayerUser.Email].ID); err != nil {
				return fmt.Errorf("taxpayer user → %s: %w", tp.Key, err)
			}
			// Pending low-confidence receipt fields in the review queue (#318).
			if err := seedReview(ctx, pool, org.ID, eng.ID, identity.UUID.String()); err != nil {
				return fmt.Errorf("review queue → %s: %w", tp.Key, err)
			}
			// Vouchers, Luca / Zirve exports and their transfers (#391).
			scope := engagementctx.Scope{ID: eng.ID, OrganizationID: org.ID, TaxpayerID: eng.TaxpayerID}
			if err := seedJournals(ctx, pool, scope, byEmail[demo.Members[1].Email].ID); err != nil {
				return fmt.Errorf("journals → %s: %w", tp.Key, err)
			}
			// Bank month to match to the trial balance (#383).
			if err := seedBank(ctx, pool, bankposting.Subject{OrganizationID: org.ID, EngagementID: eng.ID,
				TaxpayerID: eng.TaxpayerID}, byEmail[demo.Members[1].Email].ID); err != nil {
				return fmt.Errorf("bank → %s: %w", tp.Key, err)
			}
			// WhatsApp Web uzantısı: hatırlanan sohbet ve iki kayıt (#388).
			if err := seedWhatsApp(ctx, pool, org.ID, eng.ID); err != nil {
				return fmt.Errorf("whatsapp → %s: %w", tp.Key, err)
			}
			// Easytrade connection, sync history, financial transactions and Z (#385).
			if err := seedEasytrade(ctx, pool, eng.TaxpayerID); err != nil {
				return fmt.Errorf("easytrade → %s: %w", tp.Key, err)
			}
			// Taxpayer report: charts, comment, schedule, history (#386).
			if err := seedReport(ctx, pool, org.ID, eng.ID, eng.TaxpayerID, tp.Key, demo.OrgName, time.Now()); err != nil {
				return fmt.Errorf("report → %s: %w", tp.Key, err)
			}
			// Personel, Eylül ve Ekim bordroları, SGK ve bordro PDF'leri (#382).
			if err := seedPayroll(ctx, pool, scope, byEmail[demo.Members[1].Email].ID); err != nil {
				return fmt.Errorf("payroll → %s: %w", tp.Key, err)
			}
			// Eylül ve Ekim 2026 mizanları, ofis mobilde mizan raporu (#393).
			if err := seedMizan(ctx, pool, scope, byEmail[demo.Members[1].Email].ID); err != nil {
				return fmt.Errorf("mizan → %s: %w", tp.Key, err)
			}
			// Expenses with categories and Z reports with continuity warnings (#384).
			if err := seedReceipts(ctx, pool, scope, byEmail[demo.Members[1].Email].ID); err != nil {
				return fmt.Errorf("receipts → %s: %w", tp.Key, err)
			}
			// Başka programın masraf raporu ortak Excel içe aktarmadan, ofis
			// şablonuyla (#395).
			if err := seedExcelImport(ctx, pool, scope, byEmail[demo.Members[1].Email].ID); err != nil {
				return fmt.Errorf("excel import → %s: %w", tp.Key, err)
			}
			// Belge arşivi ve İSG evrakları (yaklaşan bitiş uyarısıyla) (#393).
			if err := seedArchive(ctx, pool, eng.TaxpayerID, byEmail[demo.Members[1].Email].ID, time.Now()); err != nil {
				return fmt.Errorf("archive → %s: %w", tp.Key, err)
			}
			// Ekim mutabakatı: BA/BS beyanı, karşılaştırma, mektuplar (#389).
			// Mizan, masraf ve bordrodan sonra: kaynaklar yerinde olsun.
			if err := seedReconciliation(ctx, pool, scope, byEmail[demo.Members[1].Email].ID); err != nil {
				return fmt.Errorf("reconciliation → %s: %w", tp.Key, err)
			}
			// Co-pilot: the owner's command history (#392).
			if err := seedCopilot(ctx, pool, org.ID, eng.ID); err != nil {
				return fmt.Errorf("copilot → %s: %w", tp.Key, err)
			}
		}
		fmt.Printf("taxpayer %-6s %s engagement %s\n", tp.Key, identity.TaxID, created.UUID)
	}
	fmt.Printf("demo office %q created\n", demo.OrgSlug)
	return nil
}

// ensureTaxpayer registers the demo identity or reuses it when a previous run
// (or -reset keeping a still-referenced row) left it in the catalog.
func ensureTaxpayer(ctx context.Context, svc *taxpayers.Service, tp demo.Taxpayer) (taxpayers.Taxpayer, error) {
	t, err := svc.Create(ctx, taxpayers.CreateInput{Kind: tp.Kind, TaxID: tp.TaxID, Title: tp.Title, TaxOffice: tp.TaxOffice})
	if errors.Is(err, taxpayers.ErrAlreadyExists) {
		found, ok, ferr := svc.FindByTaxID(ctx, tp.TaxID)
		if ferr != nil {
			return taxpayers.Taxpayer{}, ferr
		}
		if !ok {
			return taxpayers.Taxpayer{}, err
		}
		return found, nil
	}
	return t, err
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

// linkTaxpayerUser makes the demo taxpayer-side account a member of its
// company (issue #4 taxpayer_members), so it lands in the mükellef panel.
func linkTaxpayerUser(ctx context.Context, pool *pgxpool.Pool, taxpayerUUID string, userID int64) error {
	tag, err := pool.Exec(ctx, `
		INSERT INTO taxpayer_members (taxpayer_id, user_id, role)
		SELECT id, $2, $3 FROM taxpayers WHERE uuid = $1
		ON CONFLICT (taxpayer_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		taxpayerUUID, userID, demo.TaxpayerUserRole)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("taxpayer %s not found", taxpayerUUID)
	}
	return nil
}

func printSummary() {
	fmt.Println()
	fmt.Printf("Office:     /t/%s  (%s)\n", demo.OrgSlug, demo.OrgName)
	fmt.Println("Accounts (password: -password / DEMO_PASSWORD, default " + defaultPassword + "):")
	for _, m := range demo.Members {
		fmt.Printf("  %-28s %-7s %s %s · %s\n", m.Email, m.RoleKey, m.Name, m.Surname, m.Title)
	}
	fmt.Printf("  %-28s %-7s %s %s · %s (mükellef paneli: /m)\n", demo.TaxpayerUser.Email, demo.TaxpayerUserRole, demo.TaxpayerUser.Name, demo.TaxpayerUser.Surname, demo.TaxpayerUser.Title)
	fmt.Println("Taxpayers:")
	for _, tp := range demo.Taxpayers {
		fmt.Printf("  %-6s %-11s %s (%s VD)\n", tp.Key, tp.TaxID, tp.Title, tp.TaxOffice)
	}
	fmt.Println("UBL set:    backend/internal/demo/ubl (ofis panelinden \"UBL yükle\" ile yükleyin)")
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "demo-seed: "+format+"\n", args...)
	os.Exit(1)
}
