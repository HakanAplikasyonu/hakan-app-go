package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/rules"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

type fixture struct {
	pool                 *pgxpool.Pool
	org, eng, eng2, tp   int64
	tp2, user, otherUser int64
}

func setup(t *testing.T) (context.Context, fixture) {
	t.Helper()
	url := os.Getenv("WARNINGS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("WARNINGS_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := fixture{pool: pool}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	tag := time.Now().Format("150405.000000")
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'W') RETURNING id`, "w-"+tag).Scan(&f.org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', lpad((floor(random() * 9000000000) + 1000000000)::bigint::text, 10, '0'), 'Test') RETURNING id`).Scan(&f.tp))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', lpad((floor(random() * 9000000000) + 1000000000)::bigint::text, 10, '0'), 'Test') RETURNING id`).Scan(&f.tp2))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, f.org, f.tp).Scan(&f.eng))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, f.org, f.tp2).Scan(&f.eng2))
	for _, u := range []*int64{&f.user, &f.otherUser} {
		must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`,
			tag+time.Now().Format(".000000000")+"@example.test").Scan(u))
		must(pool.QueryRow(ctx, `INSERT INTO organization_members (organization_id, user_id, role_id)
			SELECT $1, $2, id FROM roles WHERE slug = 'member' AND scope = 'tenant' AND organization_id IS NULL RETURNING id`, f.org, *u).Scan(new(int64)))
	}
	_, err = pool.Exec(ctx, `INSERT INTO engagement_assignments (organization_id, engagement_id, user_id) VALUES ($1, $2, $3)`, f.org, f.eng, f.user)
	must(err)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, f.org)
		_, _ = pool.Exec(ctx, `DELETE FROM taxpayers WHERE id IN ($1, $2)`, f.tp, f.tp2)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, f.user, f.otherUser)
	})
	return ctx, f
}

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// TestEngineAgainstPostgres covers the #13 acceptance criteria on the real
// data_warnings table: no duplicates on re-sync, auto_resolved when the
// condition goes away, severity from configuration, accountant decisions.
func TestEngineAgainstPostgres(t *testing.T) {
	ctx, f := setup(t)
	q := db.New(f.pool)
	store := NewWarnings(q)
	cfg := NewConfig(q)
	e := engine.New(store, cfg)
	subj := engine.Subject{OrganizationID: f.org, EngagementID: f.eng, TaxpayerID: f.tp}

	bad := rules.InvoiceTotalsInput{
		InvoiceID: 900001, Period: "2026-10",
		Lines:        []rules.InvoiceLine{{ID: 900101, LineExtension: d("100"), Rate: "20", Tax: d("20")}},
		TaxExclusive: d("100"), HeaderTax: d("20.50"), Payable: d("120.50"),
	}
	rep, err := engine.Run(ctx, e, subj, rules.InvoiceTotals{}, bad)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Created != 1 {
		t.Fatalf("first run = %+v", rep)
	}
	for i := 0; i < 3; i++ {
		if rep, err = engine.Run(ctx, e, subj, rules.InvoiceTotals{}, bad); err != nil || rep.Created != 0 || rep.Refreshed != 1 {
			t.Fatalf("re-run %d = %+v %v", i, rep, err)
		}
	}
	count := func(status string) int {
		var n int
		if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM data_warnings WHERE engagement_id = $1 AND status = $2`, f.eng, status).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count("open") != 1 {
		t.Fatalf("open = %d after re-sync, want 1", count("open"))
	}

	// Severity from configuration, not code.
	blocker := catalog.SeverityBlocker
	if err := cfg.Upsert(ctx, f.org, 0, rules.CodeTaxTotal, engine.Layer{Severity: &blocker}, 0); err != nil {
		t.Fatal(err)
	}
	_, _ = engine.Run(ctx, e, subj, rules.InvoiceTotals{}, bad)
	var sev string
	_ = f.pool.QueryRow(ctx, `SELECT severity FROM data_warnings WHERE engagement_id = $1 AND status = 'open'`, f.eng).Scan(&sev)
	if sev != "blocker" || count("open") != 1 {
		t.Fatalf("severity = %s open = %d", sev, count("open"))
	}

	// Listing: assigned user sees it, unassigned does not, read_all does.
	list := func(userID int64, all bool) int64 {
		_, total, err := store.List(ctx, Filter{OrganizationID: f.org, UserID: userID, AllEngagements: all, Severity: "blocker", Limit: 20})
		if err != nil {
			t.Fatal(err)
		}
		return total
	}
	if list(f.user, false) != 1 || list(f.otherUser, false) != 0 || list(f.otherUser, true) != 1 {
		t.Fatalf("visibility: assigned=%d unassigned=%d read_all=%d", list(f.user, false), list(f.otherUser, false), list(f.otherUser, true))
	}
	rows, _, _ := store.List(ctx, Filter{OrganizationID: f.org, EngagementID: f.eng, AllEngagements: true, Status: "open", Code: rules.CodeTaxTotal, Period: "2026-10", EntityType: "invoice", Limit: 20})
	if len(rows) != 1 || rows[0].Producer != "control:invoice_totals" || rows[0].Period == nil {
		t.Fatalf("filtered rows = %+v", rows)
	}

	// Condition goes away -> auto_resolved.
	fixed := bad
	fixed.HeaderTax, fixed.Payable = d("20"), d("120")
	if rep, err = engine.Run(ctx, e, subj, rules.InvoiceTotals{}, fixed); err != nil || rep.AutoResolved != 1 {
		t.Fatalf("fixed run = %+v %v", rep, err)
	}
	if count("open") != 0 || count("auto_resolved") != 1 {
		t.Fatalf("open=%d auto_resolved=%d", count("open"), count("auto_resolved"))
	}

	// Comes back, the accountant acknowledges; the same condition stays quiet.
	_, _ = engine.Run(ctx, e, subj, rules.InvoiceTotals{}, bad)
	rows, _, _ = store.List(ctx, Filter{OrganizationID: f.org, EngagementID: f.eng, AllEngagements: true, Status: "open", Limit: 20})
	if len(rows) != 1 {
		t.Fatalf("reopened rows = %d", len(rows))
	}
	ok, err := store.Decide(ctx, f.org, f.eng, rows[0].UUID, catalog.StatusAcknowledged, f.user)
	if err != nil || !ok {
		t.Fatalf("decide = %v %v", ok, err)
	}
	if ok, _ := store.Decide(ctx, f.org, f.eng2, rows[0].UUID, catalog.StatusResolved, f.user); ok {
		t.Fatal("decision through another engagement must not apply")
	}
	if rep, _ = engine.Run(ctx, e, subj, rules.InvoiceTotals{}, bad); rep.Suppressed != 1 || count("open") != 0 {
		t.Fatalf("after ack = %+v open = %d", rep, count("open"))
	}
	// Different amounts: a new open warning.
	worse := bad
	worse.HeaderTax = d("25")
	if rep, _ = engine.Run(ctx, e, subj, rules.InvoiceTotals{}, worse); rep.Created < 1 || count("open") < 1 {
		t.Fatalf("changed condition = %+v", rep)
	}
}

// TestListLinksEveryRecordType: a warning on a bank transaction carries the
// transaction's public uuid so the office can open the record (#528).
func TestListLinksEveryRecordType(t *testing.T) {
	ctx, f := setup(t)
	store := NewWarnings(db.New(f.pool))
	var box, txn int64
	var txnUUID string
	if err := f.pool.QueryRow(ctx, `INSERT INTO cash_boxes (taxpayer_id, name, currency) VALUES ($1, 'Banka', 'TRY') RETURNING id`, f.tp).Scan(&box); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO bank_transactions (taxpayer_id, cash_box_id, source_system, source_id, capture_method, booked_at,
			transaction_date, debit_credit, amount, currency, description, period)
		VALUES ($1, $2, 'bank_test', 'w1', 'api', '2026-10-20', '2026-10-20', 'A', 100, 'TRY', 'x', '2026-10') RETURNING id, uuid::text`, f.tp, box).Scan(&txn, &txnUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, entity_type, entity_id,
			code, severity, params, producer, status)
		VALUES ($1, $2, $3, 'bank_transaction', $4, 'TEST_BANK', 'warning', '{}', 'test', 'open')`, f.org, f.eng, f.tp, txn); err != nil {
		t.Fatal(err)
	}
	rows, _, err := store.List(ctx, Filter{OrganizationID: f.org, EngagementID: f.eng, AllEngagements: true, Status: "open", EntityType: "bank_transaction", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].EntityUUID == nil || rows[0].EntityUUID.String() != txnUUID {
		t.Fatalf("rows = %+v, want entity uuid %s", rows, txnUUID)
	}
}
