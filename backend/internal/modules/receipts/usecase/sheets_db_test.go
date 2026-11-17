package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/sheet"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warnrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// #384: masraf ve Z için Excel içe / dışa aktarma; dışa aktarılan dosya
// yeniden yüklenince yalnızca yeni satırlar eklenir.
func TestSheetsAgainstPostgres(t *testing.T) {
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
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'S') RETURNING id`, "sh-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', '5610023435', 'S') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))
	q := db.New(pool)
	svc := New(pool, nopStore{}, &fakeReader{}, nil, engine.New(warnrepo.NewWarnings(q), warnrepo.NewConfig(q)), nil)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}

	csv := "Tarih;Satıcı;VKN / TCKN;Fiş No;Kategori;KDV Oranı;KDV;Toplam;Ödeme\n" +
		"03.10.2026;Opet Petrolcülük;1234567890;0042;Akaryakıt;20;200;1.200;Kredi kartı\n" +
		"04.10.2026;Köşe Lokantası;;0007;;10;;550,00;nakit\n" +
		"05.10.2026;Hatalı;12;;;;;100;\n" +
		"kötü tarih;X;;;;;;100;\n"
	res, err := svc.ImportExpenses(ctx, sc, user, "masraf.csv", []byte(csv))
	must(err)
	if res.Rows != 4 || res.Inserted != 2 || len(res.Errors) != 2 || res.Errors[0].Line != 4 {
		t.Fatalf("import %+v", res)
	}
	page, err := svc.Expenses(ctx, sc, ExpenseFilter{Limit: 50})
	must(err)
	if page.Total != 2 {
		t.Fatalf("expenses %d", page.Total)
	}
	for _, x := range page.Items {
		switch *x.VendorName {
		case "Opet Petrolcülük":
			if x.Suggested || *x.CategoryCode != "akaryakit" || *x.PaymentMethod != "credit_card" || x.CaptureMethod != "excel_import" {
				t.Fatalf("opet %+v", x)
			}
		case "Köşe Lokantası":
			// Yalnızca oran: KDV toplamdan hesaplanır (550 / 1,10).
			if x.TaxAmount.String() != "50" || !x.Suggested || *x.CategoryCode != "yemek" {
				t.Fatalf("lokanta %+v tax=%v", x, x.TaxAmount)
			}
		}
	}
	// Dışa aktarılan dosya yeniden yüklenir: hepsi zaten var.
	xlsx, err := svc.ExportExpenses(ctx, sc, ExpenseFilter{})
	must(err)
	rows, err := sheet.Read("m.xlsx", xlsx, sheet.ExpenseColumns)
	must(err)
	if len(rows) != 2 || rows[0].Row["category"] == "" {
		t.Fatalf("export rows %+v", rows)
	}
	res, err = svc.ImportExpenses(ctx, sc, user, "m.xlsx", xlsx)
	must(err)
	if res.Inserted != 0 || res.Skipped != 2 {
		t.Fatalf("export re-import %+v", res)
	}
	res, err = svc.ImportExpenses(ctx, sc, user, "masraf.csv", []byte(csv))
	must(err)
	if res.Inserted != 0 || res.Skipped != 2 {
		t.Fatalf("second import %+v", res)
	}

	zcsv := "Tarih;ÖKC Seri No;Z No;Fiş Sayısı;Nakit;Kart;%1 Matrah;%1 KDV;%10 Matrah;%10 KDV;%20 Matrah;%20 KDV;Toplam\n" +
		"01.10.2026;JH20012345;0100;40;1000;1122,00;;;1000;100;851,67;170,33;2.122,00\n" +
		"02.10.2026;JH20012345;0101;35;500;0;;;;;416,67;83,33;500\n" +
		"05.10.2026;JH20012345;0103;20;200;0;198;2;;;;;200\n"
	res, err = svc.ImportRetailSales(ctx, sc, "z.csv", []byte(zcsv))
	must(err)
	if res.Inserted != 3 || len(res.Errors) != 0 {
		t.Fatalf("z import %+v", res)
	}
	zp, err := svc.RetailSales(ctx, sc, RetailFilter{Limit: 50})
	must(err)
	if zp.Total != 3 || len(zp.VatRates) != 3 || zp.Amount.String() != "2822" {
		t.Fatalf("z page %+v", zp)
	}
	var gapOn string
	for _, it := range zp.Items {
		if len(it.Warnings) > 0 {
			gapOn = *it.ZNumber
			if len(it.Warnings) != 2 {
				t.Fatalf("warnings %v", it.Warnings)
			}
		}
		if *it.ZNumber == "0100" && (it.Card.String() != "1122" || len(it.VatLines) != 2 || it.BreakdownMismatch) {
			t.Fatalf("0100 %+v", it)
		}
	}
	if gapOn != "0103" {
		t.Fatalf("gap on %q", gapOn)
	}
	zx, err := svc.ExportRetailSales(ctx, sc, RetailFilter{})
	must(err)
	zr, err := sheet.Read("z.xlsx", zx, sheet.ZColumns)
	must(err)
	if len(zr) != 3 || zr[0].Row["okc"] != "JH20012345" {
		t.Fatalf("z export %+v", zr)
	}
	res, err = svc.ImportRetailSales(ctx, sc, "z.xlsx", zx)
	must(err)
	if res.Inserted != 0 || res.Skipped != 3 {
		t.Fatalf("z re-import %+v", res)
	}
	if _, err := svc.ImportExpenses(ctx, sc, user, "x.csv", []byte("a;b\n1;2\n")); err == nil {
		t.Fatal("headerless file accepted")
	}
}
