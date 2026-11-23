package luca

import (
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
)

// The header row of Luca's own fiş Excel (Oğuz'un geri bildirimi), in order.
var lucaHeader = []string{
	"Fiş No", "Fiş Tarihi", "Fiş Açıklama", "Hesap Kodu", "Evrak No", "Evrak Tarihi",
	"Detay Açıklama", "Borç", "Alacak", "Miktar", "Belge Türü", "Para Birimi", "Kur",
	"Döviz Tutar", "TC/Vergi No",
}

func openDefault(t *testing.T) ae.Driver {
	t.Helper()
	drv, err := ae.Open(Name, ae.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return drv
}

// invoiceEntry is one made-up purchase invoice: expense and VAT on the debit
// side, the supplier on the credit side; every line carries the invoice.
func invoiceEntry(no, invNo, taxID string, date time.Time, net, vat string) ae.Entry {
	doc := date
	total := d(net).Add(d(vat)).StringFixed(2)
	src := ae.SourceRef{Kind: "invoice", ID: no, ETTN: fmt.Sprintf("00000000-0000-4000-8000-0000000000%s", no)}
	line := func(acc, desc string, dr, cr string) ae.Line {
		return ae.Line{Account: acc, Description: desc, Debit: d(dr), Credit: d(cr), DocNo: invNo, DocDate: &doc, TaxID: taxID, Source: src}
	}
	return ae.Entry{
		ID: "inv-" + no, No: no, Date: date, Type: ae.EntryMahsup, Description: "Alış faturası " + invNo, DocumentType: "Fatura",
		Lines: []ae.Line{
			line("770.01", "Kırtasiye", net, "0"),
			line("191.20", "KDV %20", vat, "0"),
			line("320.01.001", "Örnek Tedarikçi", "0", total),
		},
	}
}

func TestDefaultTemplateIsLucaLayout(t *testing.T) {
	info := openDefault(t).Info()
	if info.TemplateVer != DefaultTemplate || DefaultTemplate != "verified-1" || !info.Verified || info.FileTypes[0] != "xlsx" {
		t.Fatalf("info: %+v", info)
	}
}

// Header order, one fiş per invoice, VAT on its own row, every fiş balanced.
func TestLucaExportOneVoucherPerInvoice(t *testing.T) {
	b := ae.Batch{Period: "2026-10", Entries: []ae.Entry{
		invoiceEntry("1", "ABC2026000000001", "1234567890", day, "1000", "200"),
		invoiceEntry("2", "ABC2026000000002", "10000000146", day.AddDate(0, 0, 2), "250.50", "50.10"),
	}}
	res, w := run(t, openDefault(t), b)
	if len(res.Files) != 1 || res.Files[0].Name != "luca_fis_2026-10.xlsx" || res.Files[0].Entries != 2 {
		t.Fatalf("result: %+v", res)
	}
	for _, is := range res.Issues {
		if is.Code == ae.IssueTemplateUnverified {
			t.Fatalf("verified template warned: %+v", res.Issues)
		}
	}
	got := rows(t, w, "luca_fis_2026-10.xlsx")
	if fmt.Sprint(got[0]) != fmt.Sprint(lucaHeader) {
		t.Fatalf("header:\n got %v\nwant %v", got[0], lucaHeader)
	}
	if len(got) != 1+6 {
		t.Fatalf("rows: %d", len(got))
	}
	col := map[string]int{}
	for i, h := range lucaHeader {
		col[h] = i
	}
	at := func(r []string, h string) string {
		if i := col[h]; i < len(r) {
			return r[i]
		}
		return ""
	}
	type sums struct{ debit, credit decimal.Decimal }
	vouchers := map[string]*sums{}
	docs := map[string]map[string]bool{}
	vatRows := 0
	for _, r := range got[1:] {
		no := at(r, "Fiş No")
		if vouchers[no] == nil {
			vouchers[no], docs[no] = &sums{}, map[string]bool{}
		}
		vouchers[no].debit = vouchers[no].debit.Add(d(at(r, "Borç")))
		vouchers[no].credit = vouchers[no].credit.Add(d(at(r, "Alacak")))
		docs[no][at(r, "Evrak No")] = true
		if at(r, "Hesap Kodu") == "191.20" {
			vatRows++
			if !d(at(r, "Alacak")).IsZero() {
				t.Fatalf("VAT row on credit: %v", r)
			}
		}
		if at(r, "Fiş Açıklama") == "" || at(r, "Belge Türü") != "Fatura" || at(r, "Fiş Tarihi") == "" || at(r, "Evrak Tarihi") == "" {
			t.Fatalf("row: %v", r)
		}
	}
	if len(vouchers) != 2 || vatRows != 2 {
		t.Fatalf("vouchers %d, VAT rows %d", len(vouchers), vatRows)
	}
	for no, s := range vouchers {
		if !s.debit.Equal(s.credit) || s.debit.IsZero() {
			t.Fatalf("fiş %s not balanced: %s / %s", no, s.debit, s.credit)
		}
		if len(docs[no]) != 1 {
			t.Fatalf("fiş %s spans invoices: %v", no, docs[no])
		}
	}
	// Counterparty VKN / TCKN and the ETTN in the line description.
	first := got[1]
	if at(first, "TC/Vergi No") != "1234567890" || at(got[4], "TC/Vergi No") != "10000000146" {
		t.Fatalf("tax ids: %v / %v", first, got[4])
	}
	if want := "Kırtasiye ETTN:00000000-0000-4000-8000-00000000001"; at(first, "Detay Açıklama") != want {
		t.Fatalf("detail: %q", at(first, "Detay Açıklama"))
	}
}

// An unbalanced invoice entry blocks the file.
func TestLucaExportUnbalancedBlocks(t *testing.T) {
	e := invoiceEntry("1", "ABC2026000000001", "1234567890", day, "1000", "200")
	e.Lines[1].Debit = d("199")
	res, w := run(t, openDefault(t), ae.Batch{Entries: []ae.Entry{e}})
	if !ae.HasBlocker(res.Issues) || len(res.Files) != 0 || len(w.Names()) != 0 {
		t.Fatalf("result: %+v", res)
	}
}

// Foreign currency lines fill Para Birimi, Kur and Döviz Tutar.
func TestLucaExportForeignCurrency(t *testing.T) {
	rate, fx := d("35.5"), d("100")
	e := entry("9", day,
		ae.Line{Account: "120.01", Debit: d("3550"), Currency: "USD", ExchangeRate: &rate, ForeignAmount: &fx},
		cr("600.01", "3550"))
	_, w := run(t, openDefault(t), ae.Batch{Entries: []ae.Entry{e}})
	got := rows(t, w, "luca_fis.xlsx")
	r := got[1]
	if r[11] != "USD" || d(r[12]).Cmp(rate) != 0 || d(r[13]).Cmp(fx) != 0 {
		t.Fatalf("row: %v", r)
	}
}
