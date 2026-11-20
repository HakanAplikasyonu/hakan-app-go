package posting

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type env struct {
	d     *testdb.DB
	svc   *Service
	j     *journals.Service
	scope engagementctx.Scope
	actor int64
}

func setup(t *testing.T) env {
	t.Helper()
	d := testdb.Open(t)
	org := d.Organization(t)
	user := d.User(t)
	tp := d.Taxpayer(t, "legal", "1288331521")
	eng := d.Engagement(t, org, tp)
	j := journals.New(d.Pool, storage.NewMemory())
	return env{d: d, svc: NewService(db.New(d.Pool), j), j: j,
		scope: engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}, actor: user}
}

type line struct {
	net, vat, rate string
	account        string // "" = no approved account code
}

// invoice inserts an active TL invoice of the taxpayer with its lines and
// their approved account codes.
func (e env) invoice(t *testing.T, direction, number, issue, deduction string, lines ...line) db.Invoice {
	t.Helper()
	ctx := context.Background()
	var net, vat decimal.Decimal
	for _, l := range lines {
		net, vat = net.Add(d(l.net)), vat.Add(d(l.vat))
	}
	typ := "SATIS"
	if direction == "incoming" {
		typ = "ALIS"
	}
	var ded any
	if deduction != "" {
		ded = deduction
	}
	var id int64
	var u uuid.UUID
	if err := e.d.Pool.QueryRow(ctx, `
		INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number, direction, type,
			issue_date, period, currency, tax_summary_source, customer_name, customer_tax_number,
			tax_exclusive_amount, tax_amount, tax_inclusive_amount, payable_amount, vat_deduction_period)
		VALUES ($1, 'upload', $2, 'api', $3, $4, $5, $6::text::date, to_char($6::text::date, 'YYYY-MM'), 'TRY', 'items',
			'Karşı Taraf A.Ş.', '1234567890', $7::text::numeric, $8::text::numeric, $9::text::numeric, $9::text::numeric, $10)
		RETURNING id, uuid`, e.scope.TaxpayerID, uuid.NewString(), number, direction, typ, issue,
		net.String(), vat.String(), net.Add(vat).String(), ded).Scan(&id, &u); err != nil {
		t.Fatal(err)
	}
	for i, l := range lines {
		var item int64
		if err := e.d.Pool.QueryRow(ctx, `
			INSERT INTO invoice_items (taxpayer_id, invoice_id, line_no, name, tax_exclusive_amount, line_extension_amount,
				total_tax_amount, tax_rate)
			VALUES ($1, $2, $3, 'Kalem', $4::text::numeric, $4::text::numeric, $5::text::numeric, $6::text::numeric) RETURNING id`,
			e.scope.TaxpayerID, id, i+1, l.net, l.vat, l.rate).Scan(&item); err != nil {
			t.Fatal(err)
		}
		if l.account != "" {
			e.assign(t, item, l.account)
		}
	}
	inv, err := e.svc.Invoice(ctx, e.scope, u)
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func (e env) assign(t *testing.T, item int64, code string) {
	t.Helper()
	if _, err := e.d.Pool.Exec(context.Background(), `
		INSERT INTO account_assignments (organization_id, engagement_id, taxpayer_id, target_type, target_id, field, account_code)
		VALUES ($1, $2, $3, 'invoice_item', $4, 'account_code', $5)`,
		e.scope.OrganizationID, e.scope.ID, e.scope.TaxpayerID, item, code); err != nil {
		t.Fatal(err)
	}
}

type lineWant struct{ account, debit, credit string }

func (e env) lines(t *testing.T, draft uuid.UUID, inv db.Invoice, want []lineWant) journals.DraftDetail {
	t.Helper()
	got, err := e.j.GetDraft(context.Background(), e.scope, draft)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != len(want) {
		t.Fatalf("lines = %+v", got.Items)
	}
	for i, w := range want {
		l := got.Items[i]
		if l.Account != w.account || !l.Debit.Equal(d(w.debit)) || !l.Credit.Equal(d(w.credit)) {
			t.Fatalf("line %d = %s B%s A%s, want %+v", i+1, l.Account, l.Debit, l.Credit, w)
		}
		if l.SourceKind != SourceKind || l.SourceID != strconv.FormatInt(inv.ID, 10) || l.DocumentNo != inv.InvoiceNumber ||
			l.DocumentDate == nil || !l.DocumentDate.Equal(inv.IssueDate.Time) {
			t.Fatalf("line %d source = %+v", i+1, l)
		}
	}
	return got
}

// A purchase becomes 153 / 7xx B, 191 B, 320 A dated the document date; a
// second request returns the same draft (one entry per invoice).
func TestPostPurchaseOnce(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	inv := e.invoice(t, "incoming", "ALS2026000000001", "2026-10-14", "",
		line{"1000", "200", "20", "153.01"}, line{"500", "50", "10", "770.01"})

	res, err := e.svc.Post(ctx, e.scope, e.actor, inv)
	if err != nil || res.Outcome != OutcomeCreated || res.Draft == nil {
		t.Fatalf("post: %+v %v", res, err)
	}
	got := e.lines(t, res.Draft.UUID, inv, []lineWant{
		{"153.01", "1000", "0"}, {"770.01", "500", "0"}, {"191", "250", "0"}, {"320", "0", "1750"},
	})
	if got.Status != "draft" || got.Period != "2026-10" || !got.Date.Equal(time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)) ||
		got.DocumentType != "invoice_purchase" || got.Origin["invoice_uuid"] != inv.Uuid.String() {
		t.Fatalf("draft = %+v", got.Draft)
	}

	again, err := e.svc.Post(ctx, e.scope, e.actor, inv)
	if err != nil || again.Outcome != OutcomeExisting || again.Draft == nil || again.Draft.UUID != res.Draft.UUID {
		t.Fatalf("second post: %+v %v", again, err)
	}
	ref, err := e.svc.Link(ctx, e.scope, inv.ID)
	if err != nil || ref == nil || ref.UUID != res.Draft.UUID || ref.EntryNo != res.Draft.EntryNo {
		t.Fatalf("link: %+v %v", ref, err)
	}
}

// Concurrent requests for one invoice: exactly one draft.
func TestPostConcurrentOneDraft(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	inv := e.invoice(t, "outgoing", "SAT2026000000002", "2026-10-03", "", line{"100", "20", "20", "600.01"})
	var wg sync.WaitGroup
	results := make([]Result, 4)
	errs := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = e.svc.Post(ctx, e.scope, e.actor, inv)
		}(i)
	}
	wg.Wait()
	created := 0
	var draft uuid.UUID
	for i, r := range results {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if r.Draft == nil {
			t.Fatalf("result %d: %+v", i, r)
		}
		if r.Outcome == OutcomeCreated {
			created++
		}
		if draft == uuid.Nil {
			draft = r.Draft.UUID
		}
		if r.Draft.UUID != draft {
			t.Fatalf("two drafts: %s %s", draft, r.Draft.UUID)
		}
	}
	var n int
	if err := e.d.Pool.QueryRow(ctx, `SELECT count(*) FROM journal_drafts WHERE engagement_id = $1`, e.scope.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if created != 1 || n != 1 {
		t.Fatalf("created %d, drafts %d", created, n)
	}
}

// The period run: sales of the document month, purchases of their KDV
// deduction month (#640) dated its first day; refusals carry the reason
// and a later run books the invoice once its account is approved.
func TestPostPeriod(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	sale := e.invoice(t, "outgoing", "SAT2026000000010", "2026-10-03", "",
		line{"1200", "240", "20", "600.01"}, line{"800", "160", "20", "600.01"})
	late := e.invoice(t, "incoming", "ALS2026000000011", "2026-10-20", "2026-11", line{"300", "60", "20", "153.01"})
	missing := e.invoice(t, "incoming", "ALS2026000000012", "2026-10-21", "", line{"100", "20", "20", ""})

	oct, err := e.svc.PostPeriod(ctx, e.scope, e.actor, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	by := map[uuid.UUID]Result{}
	for _, r := range oct {
		by[r.InvoiceUUID] = r
	}
	if len(oct) != 2 || by[sale.Uuid].Outcome != OutcomeCreated || by[missing.Uuid].Outcome != OutcomeRefused ||
		by[missing.Uuid].Reason != ReasonItemAccount || by[missing.Uuid].LineNo != 1 {
		t.Fatalf("october: %+v", oct)
	}
	e.lines(t, by[sale.Uuid].Draft.UUID, sale, []lineWant{{"120", "2400", "0"}, {"600.01", "0", "2000"}, {"391", "0", "400"}})

	nov, err := e.svc.PostPeriod(ctx, e.scope, e.actor, "2026-11")
	if err != nil || len(nov) != 1 || nov[0].InvoiceUUID != late.Uuid || nov[0].Outcome != OutcomeCreated {
		t.Fatalf("november: %+v %v", nov, err)
	}
	got := e.lines(t, nov[0].Draft.UUID, late, []lineWant{{"153.01", "300", "0"}, {"191", "60", "0"}, {"320", "0", "360"}})
	if got.Period != "2026-11" || !got.Date.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("late purchase draft: %+v", got.Draft)
	}

	// The refused invoice is booked once its line has an account; booked
	// invoices are not listed again.
	var item int64
	if err := e.d.Pool.QueryRow(ctx, `SELECT id FROM invoice_items WHERE invoice_id = $1`, missing.ID).Scan(&item); err != nil {
		t.Fatal(err)
	}
	e.assign(t, item, "153.02")
	again, err := e.svc.PostPeriod(ctx, e.scope, e.actor, "2026-10")
	if err != nil || len(again) != 1 || again[0].InvoiceUUID != missing.Uuid || again[0].Outcome != OutcomeCreated {
		t.Fatalf("second october run: %+v %v", again, err)
	}
}
