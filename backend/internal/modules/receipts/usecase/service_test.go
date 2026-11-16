package usecase

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warnrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type nopStore struct{}

func (nopStore) Upload(_ context.Context, f storage.File, _ string) error {
	_, _ = io.ReadAll(f.Body)
	return nil
}

type fakeReader struct {
	calls  int
	answer string
}

func (f *fakeReader) Read(_ context.Context, _ extract.Request) (extract.Result, error) {
	f.calls++
	r, err := extract.Parse(f.answer)
	r.Model = "claude-test"
	return r, err
}

type recReviewer struct{ got []FieldReview }

func (r *recReviewer) SubmitField(_ context.Context, f FieldReview) error {
	r.got = append(r.got, f)
	return nil
}

// png returns a distinct, sniffable PNG-looking payload.
func png(tag string) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), []byte(tag)...)
}

const fuel = `{"kind":"fuel_receipt","kind_confidence":0.95,"fields":{
"date":{"value":"14.10.2026","confidence":0.95},"vendor_tax_number":{"value":"1234567890","confidence":0.92},
"vendor_name":{"value":"Petrol A.Ş.","confidence":0.9},"receipt_number":{"value":"0042","confidence":0.85},
"total_amount":{"value":"1.200,00","confidence":0.93},"tax_amount":{"value":"200,00","confidence":0.6},
"tax_rate":{"value":"20","confidence":0.9},"vehicle_plate":{"value":"34 ABC 123","confidence":0.5}}}`

func TestCaptureAgainstPostgres(t *testing.T) {
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
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'R') RETURNING id`, "rc-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', lpad((floor(random() * 9000000000) + 1000000000)::bigint::text, 10, '0'), 'T') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, org)
		for _, tbl := range []string{"expenses", "retail_sales", "taxpayer_vehicles", "attachments"} {
			_, _ = pool.Exec(ctx, `DELETE FROM `+tbl+` WHERE taxpayer_id = $1`, tp)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM taxpayers WHERE id = $1`, tp)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user)
	})
	q := db.New(pool)
	reader := &fakeReader{answer: fuel}
	rev := &recReviewer{}
	svc := New(pool, nopStore{}, reader, rev, engine.New(warnrepo.NewWarnings(q), warnrepo.NewConfig(q)), nil)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}
	openPlateWarnings := func() int {
		var n int
		must(pool.QueryRow(ctx, `SELECT count(*) FROM data_warnings WHERE engagement_id = $1 AND code = 'PLATE_NOT_REGISTERED' AND status = 'open'`, eng).Scan(&n))
		return n
	}

	v, err := svc.Capture(ctx, sc, user, "fis.png", png("a"), extract.KindFuel)
	must(err)
	if v.Status != StatusDraft || v.Draft == nil || v.Draft.Type != "expense" || v.PlateCheck != "not_registered" {
		t.Fatalf("capture %+v", v)
	}
	if openPlateWarnings() != 1 {
		t.Fatal("PLATE_NOT_REGISTERED not raised")
	}
	if len(v.InReview) != 2 || len(rev.got) != 2 || rev.got[0].TargetType != "expense" || rev.got[0].TargetID == 0 {
		t.Fatalf("in review %v / %+v", v.InReview, rev.got)
	}
	var amount, taxExcl, status, plate string
	must(pool.QueryRow(ctx, `SELECT amount::text, tax_exclusive_amount::text, approval_status, vehicle_plate FROM expenses WHERE uuid = $1`, v.Draft.UUID).Scan(&amount, &taxExcl, &status, &plate))
	if amount != "1200.0000" || taxExcl != "1000.0000" || status != "pending" || plate != "34ABC123" {
		t.Fatalf("expense %s %s %s %s", amount, taxExcl, status, plate)
	}

	// Same image: duplicate, no AI call.
	d, err := svc.Capture(ctx, sc, user, "fis.png", png("a"), "")
	must(err)
	if d.Status != StatusDuplicate || reader.calls != 1 || d.DuplicateOf == nil || *d.DuplicateOf != v.UUID {
		t.Fatalf("same image: %+v calls=%d", d, reader.calls)
	}
	// Other photo of the same receipt: duplicate by fields.
	d, err = svc.Capture(ctx, sc, user, "fis2.png", png("b"), "")
	must(err)
	if d.Status != StatusDuplicate || reader.calls != 2 || d.Draft != nil {
		t.Fatalf("same fields: %+v", d)
	}

	// Register the vehicle; a new receipt with that plate passes.
	_, err = q.InsertTaxpayerVehicle(ctx, db.InsertTaxpayerVehicleParams{TaxpayerID: tp, Plate: "34ABC123", Description: "binek"})
	must(err)
	reader.answer = `{"kind":"fuel_receipt","fields":{"date":{"value":"15.10.2026","confidence":0.9},"vendor_tax_number":{"value":"1234567890","confidence":0.9},"total_amount":{"value":"500,00","confidence":0.9},"vehicle_plate":{"value":"34ABC123","confidence":0.9}}}`
	v2, err := svc.Capture(ctx, sc, user, "fis3.png", png("c"), "")
	must(err)
	if v2.PlateCheck != "registered" || len(v2.InReview) != 0 {
		t.Fatalf("registered plate: %+v", v2)
	}

	// Z report -> retail_sale draft.
	reader.answer = `{"kind":"z_report","fields":{"date":{"value":"15.10.2026","confidence":0.9},"z_report_number":{"value":"0123","confidence":0.95},"total_amount":{"value":"10.500,00","confidence":0.9},"tax_amount":{"value":"1.750,00","confidence":0.9}}}`
	z, err := svc.Capture(ctx, sc, user, "z.png", png("z"), extract.KindZReport)
	must(err)
	if z.Draft == nil || z.Draft.Type != "retail_sale" {
		t.Fatalf("z report %+v", z)
	}

	// Accountant approval.
	a, err := svc.Decide(ctx, sc, v.UUID, user, true)
	must(err)
	must(pool.QueryRow(ctx, `SELECT approval_status FROM expenses WHERE uuid = $1`, v.Draft.UUID).Scan(&status))
	if a.Status != StatusApproved || status != "approved" {
		t.Fatalf("approve %s %s", a.Status, status)
	}
	if _, err := svc.Decide(ctx, sc, v.UUID, user, false); err != ErrState {
		t.Fatalf("second decision: %v", err)
	}
	if _, err := svc.Capture(ctx, sc, user, "x.txt", []byte("hello"), ""); err == nil {
		t.Fatal("text accepted as image")
	}
	_, total, err := svc.List(ctx, sc, "", "", 50, 0)
	must(err)
	if total != 5 {
		t.Fatalf("captures = %d", total)
	}
}
