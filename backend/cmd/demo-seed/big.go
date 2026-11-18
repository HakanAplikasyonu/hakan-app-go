package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	authmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/model"
	authrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/repository"
	orgmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/password"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// bigScale sizes the big demo (sunum): offices (the regular demo office
// first), taxpayers per office and months of history before the current
// month. Tests use a small one.
type bigScale struct {
	Offices, Taxpayers, Months int
}

// scaleBig is `-scale=big` (make demo-big).
var scaleBig = bigScale{Offices: 3, Taxpayers: 30, Months: 24}

func parseScale(s string) (bigScale, bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "small":
		return bigScale{}, false, nil
	case "big":
		return scaleBig, true, nil
	}
	return bigScale{}, false, fmt.Errorf("unknown -scale %q (small | big)", s)
}

// bigOffice is a built office of the big demo.
type bigOffice struct {
	def   demo.BigOffice
	idx   int
	orgID int64
	// staff: office members by email (owner first).
	staff     []int64
	taxpayers []bigClient
}

// bigClient is one big taxpayer with its engagement.
type bigClient struct {
	demo.BigTaxpayer
	taxpayerID, engagementID int64
	// assignees: office member user ids assigned to the engagement.
	assignees []int64
}

// bigWorld is everything the data steps share.
type bigWorld struct {
	scale   bigScale
	anchor  time.Time // first day of the current month (Europe/Istanbul)
	now     time.Time
	periods []string // oldest first; last = current month
	offices []*bigOffice
	cps     []demo.Counterparty
}

// istanbul is the calendar the tax deadlines follow.
var istanbul = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return l
	}
	return time.FixedZone("TRT", 3*3600)
}()

func newBigWorld(sc bigScale, now time.Time) *bigWorld {
	local := now.In(istanbul)
	anchor := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.UTC)
	w := &bigWorld{scale: sc, anchor: anchor, now: now}
	for m := sc.Months; m >= 0; m-- {
		w.periods = append(w.periods, anchor.AddDate(0, -m, 0).Format("2006-01"))
	}
	w.cps = demo.GenerateCounterparties(240)
	return w
}

// seedBig adds the big demo on top of the regular one (seed already built
// demo.OrgSlug): extra offices, dozens of taxpayers per office and months of
// history. Bulk rows go in with COPY inside system transactions (rls.System);
// tax returns and reconciliations go through the services so the screens
// read what the application itself would have written.
func seedBig(ctx context.Context, pool *pgxpool.Pool, pass string, sc bigScale, now time.Time) error {
	start := time.Now()
	w := newBigWorld(sc, now)
	if err := bigOffices(ctx, pool, w, pass); err != nil {
		return err
	}
	if err := bigTaxpayers(ctx, pool, w); err != nil {
		return err
	}
	cnt := &bigCounts{}
	for _, o := range w.offices {
		t0 := time.Now()
		ms := buildModels(w, o)
		ids, err := writeDocuments(ctx, pool, w, o, ms, cnt)
		if err != nil {
			return fmt.Errorf("%s documents: %w", o.def.Slug, err)
		}
		for _, m := range ms {
			tax := m.defaultTaxDue(w)
			m.buildBank(w, w.cps, tax)
			m.buildVouchers(w, tax)
		}
		if err := writeLedger(ctx, pool, w, o, ms, ids, cnt); err != nil {
			return fmt.Errorf("%s ledger: %w", o.def.Slug, err)
		}
		fmt.Printf("big demo %-14s %d taxpayers (%s)\n", o.def.Slug, len(ms), time.Since(t0).Round(time.Millisecond))
	}
	if _, err := pool.Exec(ctx, `ANALYZE`); err != nil {
		return fmt.Errorf("analyze: %w", err)
	}
	cnt.print()
	fmt.Printf("big demo: %d offices, %d taxpayers, %s … %s (%s)\n", len(w.offices), w.clientCount(),
		w.periods[0], w.periods[len(w.periods)-1], time.Since(start).Round(time.Millisecond))
	return nil
}

func (w *bigWorld) clientCount() int {
	n := 0
	for _, o := range w.offices {
		n += len(o.taxpayers)
	}
	return n
}

// bigOffices creates the offices after the first (the regular demo office
// exists) and their members; every account gets the demo password.
func bigOffices(ctx context.Context, pool *pgxpool.Pool, w *bigWorld, pass string) error {
	queries := database.NewQueries(pool)
	users := authrepo.NewPostgres(pool, queries)
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, queries), nil)
	hash, err := password.Hash(pass)
	if err != nil {
		return fmt.Errorf("password: %w", err)
	}
	n := w.scale.Offices
	if n > len(demo.BigOffices) {
		n = len(demo.BigOffices)
	}
	for i := 0; i < n; i++ {
		def := demo.BigOffices[i]
		byEmail := map[string]authmodel.User{}
		for _, m := range append(append([]demo.Member{}, def.Members...), def.TaxpayerUser) {
			u, err := ensureUser(ctx, pool, users, m, hash)
			if err != nil {
				return fmt.Errorf("user %s: %w", m.Email, err)
			}
			byEmail[m.Email] = u
		}
		o := &bigOffice{def: def, idx: i}
		if i == 0 {
			if err := pool.QueryRow(ctx, `SELECT id FROM organizations WHERE slug = $1`, def.Slug).Scan(&o.orgID); err != nil {
				return fmt.Errorf("office %s: %w", def.Slug, err)
			}
		} else {
			org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{
				Name: def.Name, Slug: def.Slug, OwnerUserUUID: byEmail[def.Members[0].Email].UUID,
			})
			if err != nil {
				return fmt.Errorf("office %s: %w", def.Slug, err)
			}
			if org.Slug != def.Slug {
				return fmt.Errorf("office slug %q was taken; got %q", def.Slug, org.Slug)
			}
			for _, m := range def.Members[1:] {
				if _, err := orgSvc.AddMember(ctx, org.UUID, byEmail[m.Email].UUID, m.RoleKey); err != nil {
					return fmt.Errorf("member %s: %w", m.Email, err)
				}
			}
			o.orgID = org.ID
		}
		for _, m := range def.Members {
			o.staff = append(o.staff, byEmail[m.Email].ID)
		}
		o.staff = append(o.staff, byEmail[def.TaxpayerUser.Email].ID)
		w.offices = append(w.offices, o)
	}
	return nil
}

// owner, müşavir and portal user ids of an office (staff: owner first,
// portal user last).
func (o *bigOffice) owner() int64      { return o.staff[0] }
func (o *bigOffice) accountant() int64 { return o.staff[1] }
func (o *bigOffice) portalUser() int64 { return o.staff[len(o.staff)-1] }

// bigTaxpayers registers the generated taxpayers, their engagements (most
// older than the window, some started inside it), the assignments and the
// portal user of each office (owner of the office's first big taxpayer).
func bigTaxpayers(ctx context.Context, pool *pgxpool.Pool, w *bigWorld) error {
	return rls.System(ctx, pool, "demo-seed: big taxpayers", func(ctx context.Context, tx pgx.Tx) error {
		for _, o := range w.offices {
			gen := demo.GenerateTaxpayers(o.idx, w.scale.Taxpayers)
			for _, tp := range gen {
				c := bigClient{BigTaxpayer: tp}
				if err := tx.QueryRow(ctx, `
					INSERT INTO taxpayers (kind, tax_id, title, tax_office, contact_phone, contact_email)
					VALUES ($1, $2, $3, $4, $5, $6)
					ON CONFLICT (tax_id) DO UPDATE SET title = EXCLUDED.title
					RETURNING id`, tp.Kind, tp.TaxID, tp.Title, tp.TaxOffice, tp.Phone, tp.Email).Scan(&c.taxpayerID); err != nil {
					return fmt.Errorf("taxpayer %s: %w", tp.TaxID, err)
				}
				started := w.anchor.AddDate(0, -(w.scale.Months + 12 + tp.Index%30), 0)
				if tp.StartMonth < w.scale.Months {
					started = w.anchor.AddDate(0, -tp.StartMonth, 0)
				}
				if err := tx.QueryRow(ctx, `
					INSERT INTO engagements (organization_id, taxpayer_id, status, started_at, created_at)
					VALUES ($1, $2, 'active', $3, $3) RETURNING id`, o.orgID, c.taxpayerID, started).Scan(&c.engagementID); err != nil {
					return fmt.Errorf("engagement %s: %w", tp.TaxID, err)
				}
				// Müşavir (admin) every second client, personel the rest; a few
				// both. The owner sees all (read_all).
				staff := o.staff[1 : len(o.staff)-1]
				c.assignees = []int64{staff[tp.Index%len(staff)]}
				if tp.Index%4 == 0 && len(staff) > 1 {
					c.assignees = append(c.assignees, staff[(tp.Index+1)%len(staff)])
				}
				for _, u := range c.assignees {
					if _, err := tx.Exec(ctx, `INSERT INTO engagement_assignments (organization_id, engagement_id, user_id, created_at)
						VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, o.orgID, c.engagementID, u, started); err != nil {
						return fmt.Errorf("assignment %s: %w", tp.TaxID, err)
					}
				}
				o.taxpayers = append(o.taxpayers, c)
			}
			if len(o.taxpayers) > 0 {
				if _, err := tx.Exec(ctx, `
					INSERT INTO taxpayer_members (taxpayer_id, user_id, role) VALUES ($1, $2, 'owner')
					ON CONFLICT (taxpayer_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
					o.taxpayers[0].taxpayerID, o.portalUser()); err != nil {
					return fmt.Errorf("portal user: %w", err)
				}
			}
		}
		return nil
	})
}

// printBigSummary lists the offices and accounts of the big demo.
func printBigSummary(sc bigScale) {
	fmt.Println()
	fmt.Println("Büyük demo (sunum) ofisleri — parola: -password / DEMO_PASSWORD, varsayılan " + defaultPassword)
	for i, o := range demo.BigOffices {
		if i >= sc.Offices {
			break
		}
		fmt.Printf("  /t/%-14s %s\n", o.Slug, o.Name)
		for _, m := range o.Members {
			fmt.Printf("      %-30s %-7s %s %s · %s\n", m.Email, m.RoleKey, m.Name, m.Surname, m.Title)
		}
		first := demo.GenerateTaxpayers(i, 1)
		if len(first) > 0 {
			fmt.Printf("      %-30s %-7s %s %s · %s (mükellef paneli /m)\n", o.TaxpayerUser.Email, "owner",
				o.TaxpayerUser.Name, o.TaxpayerUser.Surname, first[0].Title)
		}
	}
}
