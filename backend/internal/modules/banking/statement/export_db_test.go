package statement

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/sheet"
)

// #395: banka hareketleri Excel'e süzgeçle aktarılır (tr-TR tarih, borç /
// alacak ayrı); dosya ekstre olarak geri yüklenince hareketler mükerrer olmaz.
func TestExportBankTransactionsAgainstPostgres(t *testing.T) {
	f := setupImport(t)
	ctx := context.Background()
	file := "Hesap IBAN;" + ibanZiraat + "\nİşlem Tarihi;Açıklama;Borç;Alacak;Bakiye\n" +
		"01.10.2026;EFT GELEN DEMIR GIDA;;1.250,00;11.250,00\n" +
		"02.10.2026;KIRA ODEMESI;3.000,00;;8.250,00\n" +
		"03.11.2026;POS;45,90;;8.204,10\n"
	if _, err := f.im.Import(ctx, Request{TaxpayerID: f.tp, OrganizationID: f.org, EngagementID: f.eng, FileName: "e.csv", Data: []byte(file)}); err != nil {
		t.Fatal(err)
	}
	rows, err := f.q.ExportBankTransactions(ctx, db.ExportBankTransactionsParams{
		TaxpayerID: f.tp, Period: pgtype.Text{String: "2026-10", Valid: true}, Lim: exportLimit,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := ExportRows(rows)
	if len(out) != 2 || out[0]["date"] != "01.10.2026" || out[0]["credit"] != "1250.00" || out[0]["debit"] != "" ||
		out[1]["debit"] != "3000.00" || out[1]["balance"] != "8250.00" || out[0]["account_iban"] != ibanZiraat || out[0]["match_status"] == "" {
		t.Fatalf("export rows %+v", out)
	}
	data, err := sheet.Write("Banka hareketleri", exportColumns, out)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := f.im.Import(ctx, Request{TaxpayerID: f.tp, OrganizationID: f.org, EngagementID: f.eng, FileName: "banka.xlsx", Data: data})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Accounts) != 1 || rep.Accounts[0].Inserted != 0 || f.count(t, f.box) != 3 {
		t.Fatalf("re-import %+v count=%d", rep.Accounts, f.count(t, f.box))
	}
	// Ortak Excel içe aktarmanın başlık eşlemesi dışa aktarılan kolonları tanır.
	for _, c := range exportColumns {
		want := map[string]string{"account_name": "", "currency": "", "kind": "", "match_status": ""}
		exp, skip := want[c.Key]
		got := HeaderField(c.Header)
		if skip && got != exp || !skip && got != c.Key {
			t.Fatalf("%s -> %q", c.Header, got)
		}
	}
}
