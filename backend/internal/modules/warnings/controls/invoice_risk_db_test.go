package controls

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// riskBook is one taxpayer seen through one office engagement, with helpers
// to write invoices and read the invoice risk warnings (fatura risk uyarısı:
// UNUSUAL_AMOUNT, NEW_PARTY_HIGH_AMOUNT, OUT_OF_PERIOD). Thresholds come from
// the warning_codes seed (000255): 10× the median and 50.000 TL, at least 3
// invoices of history, 100.000 TL for a new counterparty.
type riskBook struct {
	t    *testing.T
	d    *testdb.DB
	subj engine.Subject
	r    *Runner
	q    *db.Queries
}

func newRiskBook(t *testing.T, d *testdb.DB) *riskBook {
	t.Helper()
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", fmt.Sprintf("%010d", 4100000000+testdb.Next()))
	eng := d.Engagement(t, org, tp)
	// Statements join the rls.System transaction of sys (as in the worker).
	q := database.NewQueries(d.Pool)
	return &riskBook{
		t: t, d: d, q: q,
		subj: engine.Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp},
		r:    New(q, engine.New(repository.NewWarnings(q), repository.NewConfig(q))),
	}
}

// sys runs fn with the row level security bypass: fixtures, reads and the
// controls themselves.
func (b *riskBook) sys(fn func(ctx context.Context, tx pgx.Tx) error) {
	b.t.Helper()
	if err := rls.System(context.Background(), b.d.Pool, "test: invoice risk warnings", fn); err != nil {
		b.t.Fatal(err)
	}
}

// invoice writes an incoming invoice of party on day; arrived is created_at
// (zero: now).
func (b *riskBook) invoice(party, day, amount string, arrived time.Time) int64 {
	b.t.Helper()
	issue, err := time.Parse("2006-01-02", day)
	if err != nil {
		b.t.Fatal(err)
	}
	if arrived.IsZero() {
		arrived = time.Now()
	}
	n := testdb.Next()
	var id int64
	b.sys(func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number,
		direction, type, issue_date, period, currency, tax_summary_source, customer_tax_number, customer_name, payable_amount, tax_exclusive_amount, created_at)
		VALUES ($1, 'test', $2::text, 'manual', $2::text, 'incoming', 'ALIS', $3, $4, 'TRY', 'items', $5::text, $5::text, $6::numeric, $6::numeric, $7)
		RETURNING id`, b.subj.TaxpayerID, fmt.Sprintf("R-%d", n), issue, issue.Format("2006-01"), party, amount, arrived).Scan(&id)
	})
	return id
}

// exec runs a fixture statement.
func (b *riskBook) exec(sql string, args ...any) {
	b.t.Helper()
	b.sys(func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	})
}

func (b *riskBook) run(period string) {
	b.t.Helper()
	b.sys(func(ctx context.Context, _ pgx.Tx) error {
		_, err := b.r.Run(ctx, b.subj, period)
		return err
	})
}

type riskWarning struct {
	UUID   uuid.UUID
	Code   string
	Period string
	Status string
	Params map[string]any
}

// warnings returns the invoice's risk warnings by code.
func (b *riskBook) warnings(invoiceID int64) map[string]riskWarning {
	b.t.Helper()
	out := map[string]riskWarning{}
	b.sys(func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT uuid, code, COALESCE(period, ''), status, params FROM data_warnings
			WHERE engagement_id = $1 AND entity_type = 'invoice' AND entity_id = $2
			  AND code IN ('UNUSUAL_AMOUNT', 'NEW_PARTY_HIGH_AMOUNT', 'OUT_OF_PERIOD')
			ORDER BY id`, b.subj.EngagementID, invoiceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var w riskWarning
			var raw []byte
			if err := rows.Scan(&w.UUID, &w.Code, &w.Period, &w.Status, &raw); err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &w.Params); err != nil {
				return err
			}
			out[w.Code] = w
		}
		return rows.Err()
	})
	return out
}

func codesOf(m map[string]riskWarning) []string {
	var out []string
	for _, c := range []string{"UNUSUAL_AMOUNT", "NEW_PARTY_HIGH_AMOUNT", "OUT_OF_PERIOD"} {
		if w, ok := m[c]; ok && w.Status == string(catalog.StatusOpen) {
			out = append(out, c)
		}
	}
	return out
}

func wantCodes(t *testing.T, label string, got map[string]riskWarning, want ...string) {
	t.Helper()
	g := codesOf(got)
	if fmt.Sprint(g) != fmt.Sprint(want) {
		t.Errorf("%s: open risk warnings %v, want %v", label, g, want)
	}
}

// TestInvoiceRiskDB covers the invoice risk warnings with made-up books, one
// taxpayer per case in one database.
func TestInvoiceRiskDB(t *testing.T) {
	d := testdb.Open(t)
	at := func(t *testing.T, s string) time.Time {
		t.Helper()
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}

	// A supplier that invoices about 10.000 TL sends 1.000.000 TL:
	// UNUSUAL_AMOUNT with the comparison the text shows; the accountant
	// closes it as checked ("incelendi, doğru") and a re-run does not reopen it.
	t.Run("10.000 → 1.000.000", func(t *testing.T) {
		b := newRiskBook(t, d)
		const party = "1234567890"
		for i, a := range []string{"9000", "10000", "12400", "10000", "11000"} {
			b.invoice(party, fmt.Sprintf("2026-%02d-10", 5+i), a, time.Time{})
		}
		normal := b.invoice(party, "2026-10-03", "10500", time.Time{})
		big := b.invoice(party, "2026-10-05", "1000000", time.Time{})
		// Under 10× the median (10.500, the 1.000.000 now included): no warning.
		under := b.invoice(party, "2026-10-06", "90000", time.Time{})
		b.run("2026-10")

		wantCodes(t, "olağan fatura", b.warnings(normal))
		wantCodes(t, "eşik altı (9 kat)", b.warnings(under))
		got := b.warnings(big)
		wantCodes(t, "10.000 → 1.000.000", got, "UNUSUAL_AMOUNT")
		w := got["UNUSUAL_AMOUNT"]
		for k, v := range map[string]any{
			"amount": "1000000.00", "max": "12400.00", "min": "9000.00", "median": "10250.00", "count": float64(6),
			"basis": "party", "variant": "party_incoming", "multiplier": "10", "min_amount": "50000.00",
		} {
			if w.Params[k] != v {
				t.Errorf("param %s = %v want %v", k, w.Params[k], v)
			}
		}
		if w.Period != "2026-10" {
			t.Errorf("period %q", w.Period)
		}

		// "İncelendi, doğru": the existing acknowledge path closes it.
		b.sys(func(ctx context.Context, _ pgx.Tx) error {
			ok, err := repository.NewWarnings(b.q).Decide(ctx, b.subj.OrganizationID, b.subj.EngagementID, w.UUID,
				catalog.StatusAcknowledged, 0, "Belgeyle karşılaştırıldı, kayıt doğru.")
			if err == nil && !ok {
				err = fmt.Errorf("warning not decided")
			}
			return err
		})
		b.run("2026-10")
		again := b.warnings(big)
		wantCodes(t, "kapatıldıktan sonra", again)
		if again["UNUSUAL_AMOUNT"].Status != string(catalog.StatusAcknowledged) {
			t.Errorf("acknowledged warning reopened: %+v", again["UNUSUAL_AMOUNT"])
		}
	})

	// Two earlier invoices are too little history: no UNUSUAL_AMOUNT, even
	// for 1.000.000 TL (and the counterparty is not new).
	t.Run("az geçmiş", func(t *testing.T) {
		b := newRiskBook(t, d)
		const party = "2234567890"
		b.invoice(party, "2026-08-10", "10000", time.Time{})
		b.invoice(party, "2026-09-10", "10000", time.Time{})
		big := b.invoice(party, "2026-10-05", "1000000", time.Time{})
		b.run("2026-10")
		wantCodes(t, "az geçmiş", b.warnings(big))
	})

	// A counterparty seen for the first time with a high amount raises
	// NEW_PARTY_HIGH_AMOUNT; under the limit it does not.
	t.Run("yeni cari", func(t *testing.T) {
		b := newRiskBook(t, d)
		high := b.invoice("3234567890", "2026-10-05", "250000", time.Time{})
		low := b.invoice("3234567891", "2026-10-06", "80000", time.Time{})
		b.run("2026-10")
		got := b.warnings(high)
		wantCodes(t, "yeni cari, yüksek tutar", got, "NEW_PARTY_HIGH_AMOUNT")
		if p := got["NEW_PARTY_HIGH_AMOUNT"].Params; p["amount"] != "250000.00" || p["limit"] != "100000.00" {
			t.Errorf("params %v", p)
		}
		wantCodes(t, "yeni cari, eşik altı", b.warnings(low))
	})

	// A March invoice that arrives in May, after March was locked, is
	// OUT_OF_PERIOD in May; one that arrived before the lock is not, nor one
	// of an open month. Unlocking closes the warning by itself.
	t.Run("dönem dışı", func(t *testing.T) {
		b := newRiskBook(t, d)
		early := b.invoice("4234567890", "2026-03-20", "5000", at(t, "2026-04-02T09:00:00+03:00"))
		late := b.invoice("4234567890", "2026-03-25", "5000", at(t, "2026-05-03T09:00:00+03:00"))
		open := b.invoice("4234567890", "2026-02-25", "5000", at(t, "2026-05-03T09:00:00+03:00"))
		b.exec(`INSERT INTO period_locks (organization_id, engagement_id, taxpayer_id, period, locked_at)
			VALUES ($1, $2, $3, '2026-03', $4)`, b.subj.OrganizationID, b.subj.EngagementID, b.subj.TaxpayerID,
			at(t, "2026-04-20T10:00:00+03:00"))
		b.run("2026-03")
		b.run("2026-02")

		wantCodes(t, "kilitten önce gelen", b.warnings(early))
		wantCodes(t, "dönemi açık", b.warnings(open))
		got := b.warnings(late)
		wantCodes(t, "kilitten sonra gelen", got, "OUT_OF_PERIOD")
		w := got["OUT_OF_PERIOD"]
		if w.Period != "2026-05" {
			t.Errorf("warning period %q, want the period it fell into (2026-05)", w.Period)
		}
		for k, v := range map[string]any{
			"issue_date": "2026-03-25", "issue_period": "2026-03", "recorded_period": "2026-05",
			"closed_on": "2026-04-20", "reason": "lock", "variant": "lock",
		} {
			if w.Params[k] != v {
				t.Errorf("param %s = %v want %v", k, w.Params[k], v)
			}
		}

		b.exec(`DELETE FROM period_locks WHERE organization_id = $1 AND engagement_id = $2`, b.subj.OrganizationID, b.subj.EngagementID)
		b.run("2026-03")
		if s := b.warnings(late)["OUT_OF_PERIOD"].Status; s != string(catalog.StatusAutoResolved) {
			t.Errorf("after unlock: status %q", s)
		}
	})

	// The hourly sweep finds the periods that received invoices and runs the
	// controls there, without "kontrolleri çalıştır".
	t.Run("saatlik tarama", func(t *testing.T) {
		b := newRiskBook(t, d)
		const party = "5234567890"
		for i := 0; i < 4; i++ {
			b.invoice(party, fmt.Sprintf("2026-%02d-10", 6+i), "10000", time.Now().Add(-48*time.Hour))
		}
		big := b.invoice(party, "2026-10-05", "1000000", time.Time{})
		var opened int
		b.sys(func(ctx context.Context, _ pgx.Tx) error {
			var err error
			opened, err = b.r.Sweep(ctx, time.Now().Add(-SweepLookback))
			return err
		})
		if opened < 1 {
			t.Errorf("sweep opened %d warnings", opened)
		}
		wantCodes(t, "tarama", b.warnings(big), "UNUSUAL_AMOUNT")
	})
}
