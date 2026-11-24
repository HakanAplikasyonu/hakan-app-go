package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warnrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// Elle masraf (#780): oran kırılımı expense_vat_lines'a yazılır ve listede
// görünür; kırılımsız masraf satırsız kalır ("kırılım yok").
func TestManualExpenseVatLinesAgainstPostgres(t *testing.T) {
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
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'R') RETURNING id`, "mx-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', '4730074556', 'T') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))
	q := db.New(pool)
	svc := New(pool, nopStore{}, &fakeReader{answer: fuel}, nil, engine.New(warnrepo.NewWarnings(q), warnrepo.NewConfig(q)), nil)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}

	v, err := svc.CreateManualExpense(ctx, sc, user, ManualExpenseInput{
		Date: "2026-10-12", VendorName: "Örnek Kırtasiye", VendorTaxNumber: "1234567890", ReceiptNumber: "A-1",
		VatLines: []VatLine{{Rate: dd("20"), Base: dd("500"), Tax: dd("100")}, {Rate: dd("10"), Base: dd("200"), Tax: dd("20")}},
	})
	must(err)
	if v.CaptureMethod != "manual" || v.ApprovalStatus != "pending" || len(v.VatLines) != 2 || v.Amount.String() != "820" || v.TaxRate != nil {
		t.Fatalf("%+v", v)
	}
	if !v.VatLines[0].Rate.Equal(dd("10")) || !v.VatLines[1].Base.Equal(dd("500")) {
		t.Fatalf("lines %+v", v.VatLines)
	}
	a := dd("250")
	v2, err := svc.CreateManualExpense(ctx, sc, user, ManualExpenseInput{Date: "2026-10-13", Amount: &a})
	must(err)
	if len(v2.VatLines) != 0 || v2.Amount.String() != "250" {
		t.Fatalf("kırılım yok: %+v", v2)
	}
	page, err := svc.Expenses(ctx, sc, ExpenseFilter{Period: "2026-10", Limit: 50})
	must(err)
	if page.Total != 2 || !page.TaxAmount.Equal(dd("120")) {
		t.Fatalf("page %+v", page)
	}
}
