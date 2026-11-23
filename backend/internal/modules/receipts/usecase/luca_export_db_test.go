package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warnrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// Masraf çift aktarım koruması (#780): ilk Luca dosyası masrafı işaretler,
// ikincisi onu dışarıda bırakır, "yeniden dahil et" yeniden alır.
func TestExpenseLucaDoubleExportAgainstPostgres(t *testing.T) {
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
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'R') RETURNING id`, "lx-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', '4730074556', 'T') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))
	var imp int64
	must(pool.QueryRow(ctx, `INSERT INTO account_plan_imports (organization_id, engagement_id, source_format, source_kind, sha256)
		VALUES ($1, $2, 'xlsx', 'trial_balance', repeat('b', 64)) RETURNING id`, org, eng).Scan(&imp))
	_, err := pool.Exec(ctx, `INSERT INTO account_plan_accounts (organization_id, engagement_id, import_id, code, name, main_code, level, is_leaf) VALUES
		($1, $2, $3, '770.01', 'GENEL GİDERLER', '770', 2, true),
		($1, $2, $3, '191.01', 'İNDİRİLECEK KDV %20', '191', 2, true),
		($1, $2, $3, '191.02', 'İNDİRİLECEK KDV %10', '191', 2, true),
		($1, $2, $3, '100.01', 'MERKEZ KASA', '100', 2, true)`, org, eng, imp)
	must(err)
	q := db.New(pool)
	svc := New(pool, nopStore{}, &fakeReader{answer: fuel}, nil, engine.New(warnrepo.NewWarnings(q), warnrepo.NewConfig(q)), nil)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}

	first, err := svc.CreateManualExpense(ctx, sc, user, ManualExpenseInput{Date: "2026-10-12", VendorName: "Örnek Kırtasiye",
		VatLines: []VatLine{{Rate: dd("20"), Base: dd("500"), Tax: dd("100")}, {Rate: dd("10"), Base: dd("200"), Tax: dd("20")}}})
	must(err)

	f, err := svc.ExportExpensesLuca(ctx, sc, user, LucaExportInput{Period: "2026-10"})
	must(err)
	if f.Exported != 1 || f.AlreadyExported != 0 || len(f.Data) == 0 {
		t.Fatalf("first export %+v", f)
	}
	v, err := svc.Expense(ctx, sc, first.UUID)
	must(err)
	if v.ExportedAt == nil || v.ExportedFile == nil || *v.ExportedFile != "masraf_luca_fis_2026-10.xlsx" {
		t.Fatalf("mark %+v %+v", v.ExportedAt, v.ExportedFile)
	}

	// İkinci çıktı: daha önce aktarılan dışarıda, yalnız yeni masraf girer.
	a := dd("250")
	second, err := svc.CreateManualExpense(ctx, sc, user, ManualExpenseInput{Date: "2026-10-13", Amount: &a})
	must(err)
	f, err = svc.ExportExpensesLuca(ctx, sc, user, LucaExportInput{Period: "2026-10"})
	must(err)
	if f.Exported != 1 || f.AlreadyExported != 1 {
		t.Fatalf("second export %+v", f)
	}
	// Hepsi aktarıldı: yeni masraf yok.
	if _, err := svc.ExportExpensesLuca(ctx, sc, user, LucaExportInput{Period: "2026-10"}); !errors.Is(err, ErrNothingToExport) {
		t.Fatalf("third export: %v", err)
	}
	// Yeniden dahil et: tek masraf (confirm) ya da hepsi.
	f, err = svc.ExportExpensesLuca(ctx, sc, user, LucaExportInput{Period: "2026-10", ConfirmDocuments: []string{journals.ExpenseDocumentKey(first.UUID)}})
	must(err)
	if f.Exported != 1 || f.AlreadyExported != 1 {
		t.Fatalf("confirm one %+v", f)
	}
	f, err = svc.ExportExpensesLuca(ctx, sc, user, LucaExportInput{Period: "2026-10", IncludeExported: true})
	must(err)
	if f.Exported != 2 || f.AlreadyExported != 0 {
		t.Fatalf("include all %+v", f)
	}
	var confirmed int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM expense_export_documents WHERE engagement_id = $1 AND expense_uuid = $2
		AND confirmed_codes = ARRAY['DOCUMENT_ALREADY_EXPORTED']`, eng, second.UUID).Scan(&confirmed))
	if confirmed != 1 {
		t.Fatalf("re-included rows of the second expense: %d", confirmed)
	}
}
