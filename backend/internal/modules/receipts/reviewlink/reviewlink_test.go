package reviewlink

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	receipts "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	reviewrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	review "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type nopStore struct{}

func (nopStore) Upload(_ context.Context, f storage.File, _ string) error {
	_, _ = io.ReadAll(f.Body)
	return nil
}

type reader struct{}

func (reader) Read(_ context.Context, _ extract.Request) (extract.Result, error) {
	return extract.Parse(`{"kind":"expense_receipt","fields":{
"date":{"value":"14.10.2026","confidence":0.95},"vendor_tax_number":{"value":"1234567890","confidence":0.9},
"total_amount":{"value":"120,00","confidence":0.95},"tax_amount":{"value":"20,00","confidence":0.4},
"receipt_number":{"value":"O0I2","confidence":0.3}}}`)
}

// TestLowConfidenceFieldsReachQueueAndCorrectionsApply covers the #23
// criterion "düşük güvenli alan inceleme kuyruğuna düşüyor" end to end.
func TestLowConfidenceFieldsReachQueueAndCorrectionsApply(t *testing.T) {
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
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'R') RETURNING id`, "rl-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', lpad((floor(random() * 9000000000) + 1000000000)::bigint::text, 10, '0'), 'T') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, org)
		_, _ = pool.Exec(ctx, `DELETE FROM expenses WHERE taxpayer_id = $1`, tp)
		_, _ = pool.Exec(ctx, `DELETE FROM attachments WHERE taxpayer_id = $1`, tp)
		_, _ = pool.Exec(ctx, `DELETE FROM taxpayers WHERE id = $1`, tp)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user)
	})
	q := db.New(pool)
	rev := review.New(reviewrepo.New(pool, q)).Register(Kind, Applier{})
	svc := receipts.New(pool, nopStore{}, reader{}, NewReviewer(rev), nil, nil)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}

	v, err := svc.Capture(ctx, sc, user, "f.png", append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), tag...), "")
	must(err)
	if len(v.InReview) != 2 {
		t.Fatalf("in review = %v", v.InReview)
	}
	sub := review.Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}
	rows, total, err := rev.List(ctx, review.Filter{OrganizationID: org, EngagementID: eng, AllEngagements: true, Status: "pending", Limit: 10})
	must(err)
	if total != 2 || rows[0].Input.Kind != Kind || rows[0].Input.Provenance.Model == "" {
		t.Fatalf("queue = %d %+v", total, rows)
	}
	var taxSug, numSug review.ListRow
	for _, r := range rows {
		switch r.Input.TargetField {
		case extract.FieldTax:
			taxSug = r
		case extract.FieldReceiptNumber:
			numSug = r
		}
	}
	// The accountant corrects the KDV: amount and matrah follow.
	_, err = rev.Correct(ctx, sub, taxSug.UUID, user, json.RawMessage(`"18,00"`), "fişte 18,00 yazıyor")
	must(err)
	var tax, excl string
	must(pool.QueryRow(ctx, `SELECT tax_amount::text, tax_exclusive_amount::text FROM expenses WHERE uuid = $1`, v.Draft.UUID).Scan(&tax, &excl))
	if tax != "18.0000" || excl != "102.0000" {
		t.Fatalf("after correct tax=%s excl=%s", tax, excl)
	}
	// Accept the receipt number as read; then revert the KDV correction.
	_, err = rev.Accept(ctx, sub, numSug.UUID, user, "")
	must(err)
	_, err = rev.Revert(ctx, sub, taxSug.UUID, user, "yanlış düzeltme")
	must(err)
	must(pool.QueryRow(ctx, `SELECT tax_amount::text FROM expenses WHERE uuid = $1`, v.Draft.UUID).Scan(&tax))
	if tax != "20.0000" {
		t.Fatalf("after revert tax=%s", tax)
	}
	// An invalid correction is refused and nothing changes.
	again, err := rev.Submit(ctx, sub, review.Input{Kind: Kind, TargetType: "expense", TargetID: taxSug.Input.TargetID, TargetField: extract.FieldTax,
		Value: json.RawMessage(`"20,00"`), Confidence: taxSug.Input.Confidence, Rationale: "yeniden", Provenance: review.Provenance{Origin: review.OriginInput}})
	must(err)
	if _, err := rev.Correct(ctx, sub, again.UUID, user, json.RawMessage(`"yirmi"`), "x"); err == nil {
		t.Fatal("invalid amount applied")
	}
	must(pool.QueryRow(ctx, `SELECT tax_amount::text FROM expenses WHERE uuid = $1`, v.Draft.UUID).Scan(&tax))
	if tax != "20.0000" {
		t.Fatalf("after refused correction tax=%s", tax)
	}
	// #318: a taxpayer's total below the VAT is the taxpayer's input error
	// (receipts.ErrInvalid → 400), not a check constraint failure (500).
	if _, err := svc.Correct(ctx, sc, v.UUID, user, []receipts.Correction{{Field: extract.FieldTotal, Value: "10,00"}}); !errors.Is(err, receipts.ErrInvalid) {
		t.Fatalf("total below VAT: err = %v, want receipts.ErrInvalid", err)
	}
	var amount string
	must(pool.QueryRow(ctx, `SELECT amount::text FROM expenses WHERE uuid = $1`, v.Draft.UUID).Scan(&amount))
	if amount != "120.0000" {
		t.Fatalf("after refused total amount=%s", amount)
	}
}
