package repository_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	invoicerepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/ublmap"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warningsrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

const demoPeriod = "2026-10"

type demoTaxpayer struct {
	key     string
	sub     usecase.Subject
	invoice int // demo files loaded
}

// loadDemo builds the demo office's taxpayers and loads the demo UBL set
// (internal/demo/ubl) through the same mapper and store as the upload flow.
func loadDemo(t *testing.T, d *testdb.DB) (orgID int64, tps []demoTaxpayer) {
	t.Helper()
	ctx := context.Background()
	orgID = d.Organization(t)
	for _, tp := range demo.Taxpayers {
		taxpayerID := d.Taxpayer(t, tp.Kind, tp.TaxID)
		engID := d.Engagement(t, orgID, taxpayerID)
		var engUUID, tpUUID uuid.UUID
		if err := d.Pool.QueryRow(ctx, `SELECT e.uuid, t.uuid FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id WHERE e.id = $1`, engID).Scan(&engUUID, &tpUUID); err != nil {
			t.Fatal(err)
		}
		files, err := demo.UBL(tp.UBLPrefix)
		if err != nil || len(files) == 0 {
			t.Fatalf("demo UBL %s: %v (%d files)", tp.Key, err, len(files))
		}
		for _, f := range files {
			r, err := ublmap.MapXML(f.Data, ublmap.Options{OwnTaxID: tp.TaxID})
			if err != nil {
				t.Fatalf("%s: %v", f.Name, err)
			}
			tx, err := d.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := invoicerepo.SaveInvoice(ctx, tx, taxpayerID, r.Invoice, 0); err != nil {
				_ = tx.Rollback(ctx)
				t.Fatalf("%s: save: %v", f.Name, err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
		tps = append(tps, demoTaxpayer{key: tp.Key, invoice: len(files), sub: usecase.Subject{
			OrganizationID: orgID, EngagementID: engID, EngagementUUID: engUUID, TaxpayerID: taxpayerID, TaxpayerUUID: tpUUID,
		}})
	}
	return orgID, tps
}

// sqlTotals is the acceptance reference: the same rule written directly as one
// SQL sum over invoice_tax_summaries, independent of the module's queries.
type sqlTotals struct{ output, input, outputW, inputW decimal.Decimal }

func referenceTotals(t *testing.T, pool *pgxpool.Pool, taxpayerID int64, period string) sqlTotals {
	t.Helper()
	var o, i, ow, iw string
	err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(SUM(ROUND(s.vat * x.fx, 2) - ROUND(s.withholding * x.fx, 2)) FILTER (WHERE i.direction = 'outgoing'), 0)::text,
		       COALESCE(SUM(ROUND(s.vat * x.fx, 2)) FILTER (WHERE i.direction = 'incoming'), 0)::text,
		       COALESCE(SUM(ROUND(s.withholding * x.fx, 2)) FILTER (WHERE i.direction = 'outgoing'), 0)::text,
		       COALESCE(SUM(ROUND(s.withholding * x.fx, 2)) FILTER (WHERE i.direction = 'incoming'), 0)::text
		FROM invoice_tax_summaries s
		JOIN invoices i ON i.id = s.invoice_id,
		LATERAL (SELECT CASE WHEN i.currency = 'TRY' THEN 1 ELSE i.exchange_rate END AS fx) x
		WHERE s.taxpayer_id = $1 AND i.period = $2
		  AND i.status_normalized = 'active' AND i.deleted_at_source IS NULL AND x.fx IS NOT NULL`,
		taxpayerID, period).Scan(&o, &i, &ow, &iw)
	if err != nil {
		t.Fatal(err)
	}
	return sqlTotals{decimal.RequireFromString(o), decimal.RequireFromString(i), decimal.RequireFromString(ow), decimal.RequireFromString(iw)}
}

func assertMatches(t *testing.T, name string, f usecase.Forecast, ref sqlTotals) {
	t.Helper()
	pairs := []struct {
		field     string
		got, want decimal.Decimal
	}{
		{"output_vat", f.Totals.OutputVAT.Dec(), ref.output},
		{"input_vat", f.Totals.InputVAT.Dec(), ref.input},
		{"output_withholding", f.Totals.OutputWithholding.Dec(), ref.outputW},
		{"input_withholding", f.Totals.InputWithholding.Dec(), ref.inputW},
	}
	for _, p := range pairs {
		if !p.got.Equal(p.want) {
			t.Errorf("%s %s: forecast %s, invoice_tax_summaries %s", name, p.field, p.got, p.want)
		}
	}
	net := ref.output.Sub(ref.input).Sub(f.Totals.CarriedIn.Dec())
	if !f.Totals.Net.Dec().Equal(net) {
		t.Errorf("%s net %s, want %s", name, f.Totals.Net.Dec(), net)
	}
}

func newService(pool *pgxpool.Pool, now *time.Time) *usecase.Service {
	q := db.New(pool)
	cfg := warningsrepo.NewConfig(q)
	return usecase.New(repository.New(q), cfg, engine.New(warningsrepo.NewWarnings(q), cfg)).
		WithClock(func() time.Time { return *now })
}

// Kabul ölçütü (#33): demo verisinde tahmin, invoice_tax_summary toplamıyla aynı.
func TestDemoForecastMatchesInvoiceTaxSummaries(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	orgID, tps := loadDemo(t, d)
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, usecase.Istanbul)
	svc := newService(d.Pool, &now)

	byKey := map[string]usecase.Forecast{}
	for _, tp := range tps {
		f, err := svc.Forecast(ctx, tp.sub, demoPeriod)
		if err != nil {
			t.Fatal(err)
		}
		assertMatches(t, tp.key, f, referenceTotals(t, d.Pool, tp.sub.TaxpayerID, demoPeriod))
		if f.Totals.InvoiceCount != int64(tp.invoice) {
			t.Errorf("%s: %d invoices counted, %d demo files loaded", tp.key, f.Totals.InvoiceCount, tp.invoice)
		}
		if f.CarryForward.Source != "none" || !f.Totals.CarriedIn.Dec().IsZero() {
			t.Errorf("%s: no earlier data, carry %+v", tp.key, f.CarryForward)
		}
		if f.Calendar.DueDate != "2026-11-30" || f.Calendar.DaysLeft != 15 || !f.Calendar.RolledForward {
			t.Errorf("%s: calendar %+v", tp.key, f.Calendar)
		}
		t.Logf("%s %s: hesaplanan %s (tevkifat %s), indirilecek %s, net %s, faturalar %d",
			tp.key, demoPeriod, f.Totals.OutputVAT.Dec().StringFixed(2), f.Totals.OutputWithholding.Dec().StringFixed(2),
			f.Totals.InputVAT.Dec().StringFixed(2), f.Totals.Net.Dec().StringFixed(2), f.Totals.InvoiceCount)
		byKey[tp.key] = f
	}
	atlas, selin := byKey["atlas"], byKey["selin"]
	// The demo set covers tevkifat (atlas-03) and an incoming return
	// (atlas-05); make sure they are really in the numbers.
	if !atlas.Totals.OutputWithholding.Dec().IsPositive() || !atlas.Totals.InputVAT.Dec().IsPositive() || !atlas.Totals.OutputVAT.Dec().IsPositive() {
		t.Errorf("atlas totals look empty: %+v", atlas.Totals)
	}
	if !selin.Totals.OutputVAT.Dec().IsZero() || !selin.Totals.InputVAT.Dec().IsPositive() || !selin.Totals.CarryOut.Dec().IsPositive() {
		t.Errorf("selin only buys: %+v", selin.Totals)
	}
	hasIade := false
	for _, l := range atlas.Lines {
		if l.Type == "IADE" && l.Direction == usecase.DirectionIncoming {
			hasIade = true
		}
	}
	if !hasIade {
		t.Error("atlas IADE line missing")
	}

	// Cancelled and deleted-at-source invoices drop out on both sides.
	a := tps[0]
	if _, err := d.Pool.Exec(ctx, `UPDATE invoices SET status_normalized = 'cancelled'
		WHERE id = (SELECT id FROM invoices WHERE taxpayer_id = $1 AND direction = 'outgoing' ORDER BY id LIMIT 1)`, a.sub.TaxpayerID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `UPDATE invoices SET deleted_at_source = NOW()
		WHERE id = (SELECT id FROM invoices WHERE taxpayer_id = $1 AND direction = 'incoming' ORDER BY id LIMIT 1)`, a.sub.TaxpayerID); err != nil {
		t.Fatal(err)
	}
	f, err := svc.Forecast(ctx, a.sub, demoPeriod)
	if err != nil {
		t.Fatal(err)
	}
	assertMatches(t, "atlas after cancel", f, referenceTotals(t, d.Pool, a.sub.TaxpayerID, demoPeriod))
	if f.Excluded.Cancelled != 1 || f.Excluded.DeletedAtSource != 1 || f.Totals.OutputVAT.Dec().Equal(atlas.Totals.OutputVAT.Dec()) {
		t.Errorf("exclusions not applied: %+v %+v", f.Excluded, f.Totals)
	}

	// Devreden KDV: November has no data; selin's October credit carries in.
	s := tps[1]
	nov, err := svc.Forecast(ctx, s.sub, "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if nov.CarryForward.Source != "computed" || !nov.Totals.CarriedIn.Dec().Equal(selin.Totals.CarryOut.Dec()) {
		t.Errorf("carry into 2026-11: %s, want %s (%+v)", nov.Totals.CarriedIn.Dec(), selin.Totals.CarryOut.Dec(), nov.CarryForward)
	}
	// A manual opening into October restarts the chain.
	if _, err := svc.SetOpening(ctx, s.sub, usecase.OpeningInput{Period: demoPeriod, Amount: "1000.00"}, 0); err != nil {
		t.Fatal(err)
	}
	oct, _ := svc.Forecast(ctx, s.sub, demoPeriod)
	nov, _ = svc.Forecast(ctx, s.sub, "2026-11")
	if !oct.Totals.CarriedIn.Dec().Equal(decimal.NewFromInt(1000)) || oct.CarryForward.Source != "opening" ||
		!nov.Totals.CarriedIn.Dec().Equal(oct.Totals.CarryOut.Dec()) {
		t.Errorf("opening: oct %+v nov %+v", oct.Totals, nov.Totals)
	}
	ops, err := svc.ListOpenings(ctx, s.sub)
	if err != nil || len(ops) != 1 || ops[0].Period != demoPeriod {
		t.Fatalf("openings %v %+v", err, ops)
	}
	// Openings are office work product: another office's engagement of the
	// same taxpayer does not see them.
	if err := svc.DeleteOpening(ctx, usecase.Subject{OrganizationID: d.Organization(t), EngagementID: s.sub.EngagementID}, demoPeriod); err != usecase.ErrOpeningNotFound {
		t.Fatalf("cross-office delete: %v", err)
	}
	if err := svc.DeleteOpening(ctx, s.sub, demoPeriod); err != nil {
		t.Fatal(err)
	}

	items, total, err := svc.Office(ctx, usecase.OfficeFilter{OrganizationID: orgID, AllEngagements: true, Limit: 20}, demoPeriod)
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("office %v %d %d", err, total, len(items))
	}
	items, total, _ = svc.Office(ctx, usecase.OfficeFilter{OrganizationID: orgID, UserID: d.User(t), Limit: 20}, demoPeriod)
	if total != 0 || len(items) != 0 {
		t.Fatalf("unassigned member sees %d", total)
	}
}

func TestEvaluateWritesDataWarnings(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	orgID, tps := loadDemo(t, d)
	a := tps[0]
	now := time.Date(2026, 11, 26, 9, 0, 0, 0, usecase.Istanbul) // son gün 30 Kasım
	svc := newService(d.Pool, &now)
	cfg := warningsrepo.NewConfig(db.New(d.Pool))

	count := func(code, status string) int {
		var n int
		if err := d.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM data_warnings
			WHERE organization_id = $1 AND engagement_id = $2 AND code = $3 AND status = $4
			  AND producer = 'control:vat_forecast' AND entity_type = 'period' AND period = $5`,
			orgID, a.sub.EngagementID, code, status, demoPeriod).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Default limit is null: only the deadline warning.
	ev, err := svc.Evaluate(ctx, a.sub, []string{demoPeriod})
	if err != nil {
		t.Fatal(err)
	}
	if ev[0].Created != 1 || count(usecase.CodeDeadlineNear, "open") != 1 || count(usecase.CodePayableThreshold, "open") != 0 {
		t.Fatalf("default config: %+v", ev[0])
	}

	// Office sets a limit; the engagement moves the filing day.
	if err := cfg.Upsert(ctx, orgID, 0, usecase.CodePayableThreshold, engine.Layer{Params: []byte(`{"limit": "1"}`)}, 0); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Upsert(ctx, orgID, a.sub.EngagementID, usecase.CodeDeadlineNear, engine.Layer{Params: []byte(`{"due_day": 20}`)}, 0); err != nil {
		t.Fatal(err)
	}
	f, err := svc.Forecast(ctx, a.sub, demoPeriod)
	if err != nil {
		t.Fatal(err)
	}
	if f.Calendar.DueDate != "2026-11-20" || f.Checks.PayableLimit == nil || !f.Checks.PayableLimitReached {
		t.Fatalf("config not applied: %+v %+v", f.Calendar, f.Checks)
	}
	ev, err = svc.Evaluate(ctx, a.sub, []string{demoPeriod})
	if err != nil {
		t.Fatal(err)
	}
	// Deadline passed (20 Kasım) → auto-resolved; payable ≥ 1 → raised.
	if ev[0].Created != 1 || ev[0].AutoResolved != 1 || count(usecase.CodePayableThreshold, "open") != 1 ||
		count(usecase.CodeDeadlineNear, "auto_resolved") != 1 {
		t.Fatalf("after config: %+v", ev[0])
	}
	var params string
	if err := d.Pool.QueryRow(ctx, `SELECT params::text FROM data_warnings WHERE organization_id = $1 AND engagement_id = $2 AND code = $3 AND status = 'open'`,
		orgID, a.sub.EngagementID, usecase.CodePayableThreshold).Scan(&params); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(params, `"limit": "1.00"`) || !strings.Contains(params, f.Totals.Payable.Dec().StringFixed(2)) {
		t.Errorf("params %s", params)
	}
}
