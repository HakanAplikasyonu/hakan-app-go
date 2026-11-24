package reviewlink

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	receipts "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	reviewrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	review "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// vatReader is a fake AI read: one %20 row (120,00 incl. 20,00 VAT).
type vatReader struct{}

func (vatReader) Read(_ context.Context, _ extract.Request) (extract.Result, error) {
	return extract.Parse(`{"kind":"expense_receipt","fields":{
"date":{"value":"14.10.2026","confidence":0.95},"vendor_name":{"value":"ÖRNEK MARKET","confidence":0.95},
"total_amount":{"value":"120,00","confidence":0.95},"tax_amount":{"value":"20,00","confidence":0.95},
"vat_20_total":{"value":"120,00","confidence":0.95},"vat_20_tax":{"value":"20,00","confidence":0.95}}}`)
}

// TestTaxpayerCorrectsVatRows: the mobile preview (Oğuz'un geri bildirimi)
// sends the corrected rate rows as taxpayer corrections; they reach the
// review queue as "Mükellef düzeltmesi" and, once applied, the expense's
// breakdown holds the taxpayer's rows (matrah = total - VAT).
func TestTaxpayerCorrectsVatRows(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t).Pool
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	var org, tp, eng, user int64
	tag := time.Now().Format("150405.000000")
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'R') RETURNING id`, "rv-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', lpad((floor(random() * 9000000000) + 1000000000)::bigint::text, 10, '0'), 'T') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, "rv"+tag+"@example.test").Scan(&user))
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, org)
		_, _ = pool.Exec(ctx, `DELETE FROM expenses WHERE taxpayer_id = $1`, tp)
		_, _ = pool.Exec(ctx, `DELETE FROM attachments WHERE taxpayer_id = $1`, tp)
		_, _ = pool.Exec(ctx, `DELETE FROM taxpayers WHERE id = $1`, tp)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user)
	})
	q := db.New(pool)
	rev := review.New(reviewrepo.New(pool, q)).Register(Kind, Applier{})
	svc := receipts.New(pool, nopStore{}, vatReader{}, NewReviewer(rev), nil, nil)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}

	v, err := svc.Capture(ctx, sc, user, "f.png", append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), ("vat"+tag)...), "")
	must(err)
	if v.Draft == nil || v.Draft.Type != "expense" {
		t.Fatalf("draft = %+v", v.Draft)
	}

	// An unknown rate is refused before anything is sent.
	if _, err := svc.Correct(ctx, sc, v.UUID, user, []receipts.Correction{{Field: "vat_7_total", Value: "10,00"}}); !errors.Is(err, receipts.ErrInvalid) {
		t.Fatalf("unknown rate: err = %v", err)
	}

	// The taxpayer fixes the %20 row and adds a %10 row (VAT before its
	// total on purpose: the service sends totals first).
	sent, err := svc.Correct(ctx, sc, v.UUID, user, []receipts.Correction{
		{Field: "vat_10_tax", Value: "2,00"},
		{Field: "vat_20_tax", Value: "16,00"},
		{Field: "vat_10_total", Value: "22,00"},
		{Field: "vat_20_total", Value: "96,00"},
		{Field: extract.FieldTotal, Value: "118,00"},
		{Field: extract.FieldTax, Value: "18,00"},
	})
	must(err)
	if len(sent) != 6 || sent[0] != extract.FieldTotal || sent[2] != "vat_10_total" || sent[5] != "vat_20_tax" {
		t.Fatalf("submitted = %v", sent)
	}

	// The office accepts whatever is still pending (the policy may have
	// applied the taxpayer's values already).
	sub := review.Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}
	rows, _, err := rev.List(ctx, review.Filter{OrganizationID: org, EngagementID: eng, AllEngagements: true, Status: "pending", Limit: 50})
	must(err)
	for _, f := range sent {
		for _, r := range rows {
			if r.Input.TargetField != f {
				continue
			}
			if r.Input.Provenance.Inputs["proposed_by"] != "taxpayer" {
				t.Fatalf("%s provenance = %+v", f, r.Input.Provenance)
			}
			_, err := rev.Accept(ctx, sub, r.UUID, user, "")
			must(err)
		}
	}

	lines, err := q.ListExpenseVatLines(ctx, db.ListExpenseVatLinesParams{TaxpayerID: tp, Ids: []int64{v.Draft.ID}})
	must(err)
	got := map[string][2]string{}
	for _, l := range lines {
		got[numDec(l.TaxRate).String()] = [2]string{numDec(l.TaxBase).StringFixed(2), numDec(l.TaxAmount).StringFixed(2)}
	}
	want := map[string][2]string{"10": {"20.00", "2.00"}, "20": {"80.00", "16.00"}}
	if len(got) != len(want) || got["10"] != want["10"] || got["20"] != want["20"] {
		t.Fatalf("vat lines = %v, want %v", got, want)
	}
	var amount, tax string
	must(pool.QueryRow(ctx, `SELECT amount::text, tax_amount::text FROM expenses WHERE id = $1`, v.Draft.ID).Scan(&amount, &tax))
	if amount != "118.0000" || tax != "18.0000" {
		t.Fatalf("expense amount=%s tax=%s", amount, tax)
	}

	// A zero total removes the rate.
	_, err = svc.Correct(ctx, sc, v.UUID, user, []receipts.Correction{{Field: "vat_10_total", Value: "0"}})
	must(err)
	rows, _, err = rev.List(ctx, review.Filter{OrganizationID: org, EngagementID: eng, AllEngagements: true, Status: "pending", Limit: 50})
	must(err)
	for _, r := range rows {
		if r.Input.TargetField == "vat_10_total" {
			_, err := rev.Accept(ctx, sub, r.UUID, user, "")
			must(err)
		}
	}
	lines, err = q.ListExpenseVatLines(ctx, db.ListExpenseVatLinesParams{TaxpayerID: tp, Ids: []int64{v.Draft.ID}})
	must(err)
	if len(lines) != 1 || numDec(lines[0].TaxRate).String() != "20" {
		t.Fatalf("after removing %%10: %+v", lines)
	}
}

func TestVatField(t *testing.T) {
	for f, want := range map[string][3]any{
		"vat_20_total": {"20", "total", true},
		"vat_1_tax":    {"1", "tax", true},
		"vat_7_total":  {"", "", false},
		"vat_20_base":  {"", "", false},
		"total_amount": {"", "", false},
	} {
		rate, part, ok := extract.VatField(f)
		if rate != want[0] || part != want[1] || ok != want[2] {
			t.Errorf("VatField(%q) = %q %q %v, want %v", f, rate, part, ok, want)
		}
	}
}
