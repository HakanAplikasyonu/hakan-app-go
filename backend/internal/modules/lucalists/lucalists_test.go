package lucalists

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	invoiceposting "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/posting"
	receiptposting "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/posting"
)

// Luca'ya aktar (Excel) listeleri (Oğuz'un geri bildirimi): gelen / giden
// fatura, banka hareketi ve Z raporu fiş dosyası. Tüm veriler uydurmadır.

func dd(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func rate(s string) *decimal.Decimal { r := dd(s); return &r }

func day(n int) time.Time { return time.Date(2026, 10, n, 0, 0, 0, 0, time.UTC) }

var lucaHeader = []string{"Fiş No", "Fiş Tarihi", "Fiş Açıklama", "Hesap Kodu", "Evrak No", "Evrak Tarihi", "Detay Açıklama",
	"Borç", "Alacak", "Miktar", "Belge Türü", "Para Birimi", "Kur", "Döviz Tutar", "TC/Vergi No"}

var plan = []engine.PlanAccount{
	{Code: "191", Name: "İNDİRİLECEK KDV"},
	{Code: "191.01", Name: "İNDİRİLECEK KDV %20", IsLeaf: true},
	{Code: "191.02", Name: "İNDİRİLECEK KDV %10", IsLeaf: true},
	{Code: "391", Name: "HESAPLANAN KDV"},
	{Code: "391.01", Name: "HESAPLANAN KDV %20", IsLeaf: true},
	{Code: "391.02", Name: "HESAPLANAN KDV %10", IsLeaf: true},
	{Code: "100.01", Name: "MERKEZ KASA", IsLeaf: true},
	{Code: "108.01", Name: "POS KREDİ KARTI", IsLeaf: true},
	{Code: "600.01", Name: "YURT İÇİ SATIŞLAR", IsLeaf: true},
}

type row struct{ no, account, debit, credit string }

// fileRows writes the vouchers and returns the sheet's rows; the first row
// must be the Luca header, column by column.
func fileRows(t *testing.T, list List, entries []Doc) ([][]string, []row) {
	t.Helper()
	es, _, _, _ := Entries(entries)
	f, err := WriteFile(list, "2026-10", es)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(f.Name, list.filePrefix()) || !strings.HasSuffix(f.Name, ".xlsx") {
		t.Fatalf("file name %q", f.Name)
	}
	x, err := excelize.OpenReader(bytes.NewReader(f.Data))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := x.GetRows(x.GetSheetName(0))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 || len(rows[0]) < len(lucaHeader) {
		t.Fatalf("header row %v", rows)
	}
	for i, h := range lucaHeader {
		if rows[0][i] != h {
			t.Fatalf("header %d = %q, want %q", i, rows[0][i], h)
		}
	}
	var got []row
	for _, r := range rows[1:] {
		for len(r) < len(lucaHeader) {
			r = append(r, "")
		}
		got = append(got, row{r[0], r[3], r[7], r[8]})
	}
	return rows, got
}

func sameRows(t *testing.T, got, want []row) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("rows %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func purchaseInput() invoiceposting.Input {
	return invoiceposting.Input{
		Invoice: invoiceposting.Invoice{
			ID: 42, Number: "ABC2026000000123", Direction: "incoming", Type: "SATIS", Currency: "TRY", Status: "active",
			IssueDate: day(14), Counterpart: "Örnek Tedarik A.Ş.",
			TaxExclusive: dd("1500"), Tax: dd("250"), Payable: dd("1750"),
		},
		Items: []invoiceposting.Item{
			{LineNo: 1, Name: "Mal", Net: dd("1000"), VAT: dd("200"), Rate: rate("20"), Account: "153.01"},
			{LineNo: 2, Name: "Hizmet", Net: dd("500"), VAT: dd("50"), Rate: rate("10"), Account: "770.01"},
		},
		Cari: "320.01.005",
		Plan: plan,
	}
}

func salesInput() invoiceposting.Input {
	return invoiceposting.Input{
		Invoice: invoiceposting.Invoice{
			ID: 43, Number: "SAT2026000000007", Direction: "outgoing", Type: "SATIS", Currency: "TRY", Status: "active",
			IssueDate: day(3), Counterpart: "Örnek Müşteri Ltd.",
			TaxExclusive: dd("2000"), Tax: dd("300"), Payable: dd("2300"),
		},
		Items: []invoiceposting.Item{
			{LineNo: 1, Net: dd("1000"), VAT: dd("200"), Rate: rate("20"), Account: "600.01"},
			{LineNo: 2, Net: dd("1000"), VAT: dd("100"), Rate: rate("10"), Account: "600.01"},
		},
		Cari: "120.01.002",
		Plan: plan,
	}
}

// Gelen fatura: bir fatura bir fiş, her KDV oranı kendi 191 satırında,
// borç = alacak; hesabı bilinmeyen satır 153 ile ve "kontrol edin" notuyla.
func TestIncomingInvoices(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	first, reason := InvoiceDoc(InvoiceKey(a, nil), a, "1234567890", purchaseInput(), nil)
	if reason != "" {
		t.Fatal(reason)
	}
	in := purchaseInput()
	in.Invoice.Number, in.Invoice.IssueDate = "ABC2026000000124", day(20)
	in.Items[1].Account = "" // onaylı kod ve öneri yok
	in.Cari = ""             // cari hesabı bilinmiyor
	second, reason := InvoiceDoc(InvoiceKey(b, nil), b, "1234567890", in, nil)
	if reason != "" {
		t.Fatal(reason)
	}
	rows, got := fileRows(t, IncomingInvoices, []Doc{second, first})
	sameRows(t, got, []row{
		{"1", "153.01", "1000.00", "0.00"}, {"1", "770.01", "500.00", "0.00"},
		{"1", "191.01", "200.00", "0.00"}, {"1", "191.02", "50.00", "0.00"}, {"1", "320.01.005", "0.00", "1750.00"},
		{"2", "153.01", "1000.00", "0.00"}, {"2", "153", "500.00", "0.00"},
		{"2", "191.01", "200.00", "0.00"}, {"2", "191.02", "50.00", "0.00"}, {"2", "320", "0.00", "1750.00"},
	})
	if rows[1][4] != "ABC2026000000123" || rows[1][14] != "1234567890" {
		t.Errorf("evrak no / vkn: %q %q", rows[1][4], rows[1][14])
	}
	if !strings.Contains(rows[7][6], CheckNote) || !strings.Contains(rows[10][6], CheckNote) || strings.Contains(rows[6][6], CheckNote) {
		t.Errorf("check notes: %q / %q / %q", rows[7][6], rows[10][6], rows[6][6])
	}
	_, _, check, _ := Entries([]Doc{first, second})
	if len(check) != 2 || check[0].Reason != CheckItemAccount || check[0].Account != "153" || check[1].Reason != CheckCounterpartyAccount {
		t.Fatalf("check rows %+v", check)
	}
}

// Bekleyen öneri onaylı kod yoksa kullanılır; kontrol notu düşmez.
func TestIncomingInvoiceUsesSuggestion(t *testing.T) {
	in := purchaseInput()
	in.Items[1].Account = ""
	id := uuid.New()
	d, reason := InvoiceDoc(InvoiceKey(id, nil), id, "", in, map[int]ItemSuggestion{2: {Account: "770.02"}})
	if reason != "" {
		t.Fatal(reason)
	}
	for _, l := range d.Lines {
		if l.Check != "" {
			t.Fatalf("unexpected check %+v", l)
		}
	}
	if d.Lines[1].Account != "770.02" {
		t.Fatalf("suggestion not used: %+v", d.Lines)
	}
}

// Giden fatura: 120 borç, 600 alacak, her oran kendi 391 satırında; planda
// oran hesabı yoksa ana 391 ve "kontrol edin".
func TestOutgoingInvoices(t *testing.T) {
	id := uuid.New()
	ettn := uuid.MustParse("6F1B2C3D-0000-4000-8000-00000000ABCD")
	d, reason := InvoiceDoc(InvoiceKey(id, &ettn), id, "11111111111", salesInput(), nil)
	if reason != "" {
		t.Fatal(reason)
	}
	if d.Key != "6f1b2c3d-0000-4000-8000-00000000abcd" {
		t.Fatalf("key %q", d.Key)
	}
	_, got := fileRows(t, OutgoingInvoices, []Doc{d})
	sameRows(t, got, []row{
		{"1", "120.01.002", "2300.00", "0.00"}, {"1", "600.01", "0.00", "2000.00"},
		{"1", "391.01", "0.00", "200.00"}, {"1", "391.02", "0.00", "100.00"},
	})
	in := salesInput()
	in.Plan = []engine.PlanAccount{{Code: "391.01", Name: "HESAPLANAN KDV A", IsLeaf: true}, {Code: "391.02", Name: "HESAPLANAN KDV B", IsLeaf: true}}
	d, _ = InvoiceDoc(InvoiceKey(id, nil), id, "", in, nil)
	_, _, check, _ := Entries([]Doc{d})
	if len(check) != 1 || check[0].Reason != CheckVATAccount || check[0].Account != "391" {
		t.Fatalf("vat check %+v", check)
	}
	// Dövizli fatura dosyaya girmez, gerekçesiyle döner.
	in = salesInput()
	in.Invoice.Currency = "USD"
	if _, reason := InvoiceDoc(InvoiceKey(id, nil), id, "", in, nil); reason != invoiceposting.ReasonForeignCurrency {
		t.Fatalf("foreign: %q", reason)
	}
}

// Planda oran alt hesabı yoksa iki oran aynı ana 191'e düşer; yine de her
// oran ayrı satırdır ("KDV %20" / "KDV %10") ve fiş dengededir.
func TestVATRatesSameAccountSeparateRows(t *testing.T) {
	id := uuid.New()
	in := purchaseInput()
	in.Plan = nil
	d, reason := InvoiceDoc(InvoiceKey(id, nil), id, "", in, nil)
	if reason != "" {
		t.Fatal(reason)
	}
	rows, _ := fileRows(t, IncomingInvoices, []Doc{d})
	var vat [][]string
	var debit, credit decimal.Decimal
	for _, r := range rows[1:] {
		for len(r) < len(lucaHeader) {
			r = append(r, "")
		}
		if r[3] == "191" {
			vat = append(vat, r)
		}
		debit = debit.Add(decimal.RequireFromString(r[7]))
		credit = credit.Add(decimal.RequireFromString(r[8]))
	}
	if len(vat) != 2 {
		t.Fatalf("191 rows %v", vat)
	}
	if !strings.HasPrefix(vat[0][6], "KDV %20") || vat[0][7] != "200.00" ||
		!strings.HasPrefix(vat[1][6], "KDV %10") || vat[1][7] != "50.00" {
		t.Fatalf("191 rows %v", vat)
	}
	if !debit.Equal(credit) || !debit.Equal(decimal.RequireFromString("1750")) {
		t.Fatalf("unbalanced %s / %s", debit, credit)
	}
}

// Banka hareketi: 102 karşı hesaba; giriş bankayı borçlandırır. Banka
// hesabı ya da karşı hesap bilinmiyorsa ana hesap ve "kontrol edin".
// Dövizli hareket ve bankanın kendisine giden hareket alınmaz.
func TestBankTransactions(t *testing.T) {
	in, reason := BankDoc(Movement{UUID: uuid.New(), ID: 1, Date: day(5), In: true, Amount: dd("1180"), Currency: "TRY",
		Counterparty: "Örnek Müşteri Ltd.", Account: "120.01.002", BankAccount: "102.01"})
	if reason != "" {
		t.Fatal(reason)
	}
	out, reason := BankDoc(Movement{UUID: uuid.New(), ID: 2, Date: day(6), Amount: dd("45.50"), Currency: "TRY",
		Description: "HESAP İŞLETİM ÜCRETİ", Detail: "BANKA_MASRAFI"})
	if reason != "" {
		t.Fatal(reason)
	}
	rows, got := fileRows(t, BankTransactions, []Doc{out, in})
	sameRows(t, got, []row{
		{"1", "102.01", "1180.00", "0.00"}, {"1", "120.01.002", "0.00", "1180.00"},
		{"2", "102", "0.00", "45.50"}, {"2", out.Lines[1].Account, "45.50", "0.00"},
	})
	if out.Lines[1].Account == "" || out.Lines[1].Check != CheckCounterAccount || out.Lines[0].Check != CheckBankAccount {
		t.Fatalf("fallback lines %+v", out.Lines)
	}
	if !strings.Contains(rows[3][6], CheckNote) || !strings.Contains(rows[4][6], CheckNote) || strings.Contains(rows[1][6], CheckNote) {
		t.Errorf("check notes %q %q %q", rows[3][6], rows[4][6], rows[1][6])
	}
	_, _, check, _ := Entries([]Doc{out})
	if len(check) != 2 {
		t.Fatalf("check rows %+v", check)
	}
	if _, r := BankDoc(Movement{UUID: uuid.New(), Date: day(7), Amount: dd("10"), Currency: "USD"}); r != SkipForeignCurrency {
		t.Fatalf("foreign: %q", r)
	}
	if _, r := BankDoc(Movement{UUID: uuid.New(), Date: day(7), Amount: dd("10"), Currency: "TRY", Account: "102.01", BankAccount: "102.01"}); r != SkipSameAccount {
		t.Fatalf("same account: %q", r)
	}
}

// POS tahsilatı (#565): banka net tutarı aldı, kestiği komisyon 653'e
// gider; karşı hesap brüt tutarla alacaklanır, fiş dengede kalır. Çıkan
// harekette komisyon alanı yok sayılır.
func TestBankPOSCommission(t *testing.T) {
	in, reason := BankDoc(Movement{UUID: uuid.New(), ID: 3, Date: day(8), In: true, Amount: dd("970.40"), Currency: "TRY",
		Counterparty: "POS Tahsilat Örnek", Account: "108.01", BankAccount: "102.01", Commission: dd("29.60"), CommissionAccount: "653.01"})
	if reason != "" {
		t.Fatal(reason)
	}
	out, reason := BankDoc(Movement{UUID: uuid.New(), ID: 4, Date: day(9), Amount: dd("50"), Currency: "TRY",
		Account: "320.01", BankAccount: "102.01", Commission: dd("5"), CommissionAccount: "653"})
	if reason != "" {
		t.Fatal(reason)
	}
	_, got := fileRows(t, BankTransactions, []Doc{in, out})
	sameRows(t, got, []row{
		{"1", "102.01", "970.40", "0.00"}, {"1", "653.01", "29.60", "0.00"}, {"1", "108.01", "0.00", "1000.00"},
		{"2", "320.01", "50.00", "0.00"}, {"2", "102.01", "0.00", "50.00"},
	})
	debit, credit := decimal.Zero, decimal.Zero
	for _, l := range in.Lines {
		debit, credit = debit.Add(l.Debit), credit.Add(l.Credit)
	}
	if !debit.Equal(credit) || !strings.Contains(in.Lines[1].Description, "POS komisyonu") || in.Lines[1].Check != "" {
		t.Fatalf("commission lines %+v", in.Lines)
	}
}

// Z raporu: 100 / 108 borç, her oran kendi 600 ve 391 satırında.
func TestZReports(t *testing.T) {
	card := dd("400")
	z, reason := ZDoc(ZReport{UUID: uuid.New(), Date: day(9), ZNumber: "0042", OKC: "AB12345678", Currency: "TRY",
		Total: dd("1100"), Tax: dd("150"), Card: &card,
		Lines: []receiptposting.ZVatLine{{Rate: dd("20"), Base: dd("500"), Tax: dd("100")}, {Rate: dd("10"), Base: dd("450"), Tax: dd("50")}}}, plan)
	if reason != "" {
		t.Fatal(reason)
	}
	_, got := fileRows(t, ZReports, []Doc{z})
	sameRows(t, got, []row{
		{"1", "100.01", "700.00", "0.00"}, {"1", "108.01", "400.00", "0.00"},
		{"1", "600.01", "0.00", "500.00"}, {"1", "391.01", "0.00", "100.00"},
		{"1", "600.01", "0.00", "450.00"}, {"1", "391.02", "0.00", "50.00"},
	})
	// Planı olmayan mükellef: ana hesaplar "kontrol edin" ile.
	z, _ = ZDoc(ZReport{UUID: uuid.New(), Date: day(10), ZNumber: "0043", Total: dd("120"), Tax: dd("20")}, nil)
	_, _, check, _ := Entries([]Doc{z})
	if len(check) == 0 || check[0].Reason != CheckMainAccount {
		t.Fatalf("check rows %+v", check)
	}
	// Kırılım toplamı tutmuyor: alınmaz.
	_, reason = ZDoc(ZReport{UUID: uuid.New(), Date: day(11), Total: dd("1000"), Tax: dd("100"),
		Lines: []receiptposting.ZVatLine{{Rate: dd("20"), Base: dd("100"), Tax: dd("20")}}}, plan)
	if reason != SkipZBreakdown {
		t.Fatalf("breakdown: %q", reason)
	}
}

// Borç ≠ alacak olan fiş dosyaya girmez ve raporlanır; fiş numaraları
// boşluksuz kalır.
func TestUnbalancedLeftOut(t *testing.T) {
	ok := Doc{Key: "z:a", No: "1", Date: day(1), Lines: []Line{{Account: "100.01", Debit: dd("10")}, {Account: "600.01", Credit: dd("10")}}}
	bad := Doc{Key: "z:b", No: "2", Date: day(2), Lines: []Line{{Account: "100.01", Debit: dd("10")}, {Account: "600.01", Credit: dd("9.99")}}}
	zero := Doc{Key: "z:c", No: "3", Date: day(3), Lines: []Line{{Account: "100.01"}, {Account: "600.01"}}}
	ok2 := Doc{Key: "z:d", No: "4", Date: day(4), Lines: []Line{{Account: "100.01", Debit: dd("5")}, {Account: "600.01", Credit: dd("5")}}}
	entries, kept, _, skipped := Entries([]Doc{ok2, zero, bad, ok})
	if len(entries) != 2 || len(kept) != 2 || entries[0].No != "1" || entries[1].No != "2" || entries[1].ID != "z:d" {
		t.Fatalf("entries %+v", entries)
	}
	if len(skipped) != 2 || skipped[0].Reason != SkipUnbalanced || skipped[0].DocumentNo != "2" || skipped[1].DocumentNo != "3" {
		t.Fatalf("skipped %+v", skipped)
	}
	for _, e := range entries {
		if !e.Balanced() {
			t.Fatalf("unbalanced voucher in the file: %+v", e)
		}
	}
}

// Çift aktarım koruması: daha önce aktarılan belge varsayılan olarak
// dışarıda; tek tek (confirm) ya da hepsi birden yeniden dahil edilir.
func TestGuard(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	docs := []Doc{{Key: InvoiceKey(a, nil)}, {Key: BankKey(b)}, {Key: ZKey(c)}}
	prior := map[string]bool{InvoiceKey(a, nil): true, BankKey(b): true}
	out, confirmed, left := Guard(docs, prior, nil, false)
	if len(out) != 1 || out[0].Key != ZKey(c) || len(left) != 2 || len(confirmed) != 0 {
		t.Fatalf("default: %d %v %d", len(out), confirmed, len(left))
	}
	out, confirmed, left = Guard(docs, prior, []string{strings.ToUpper(BankKey(b))}, false)
	if len(out) != 2 || len(left) != 1 || !confirmed[BankKey(b)] {
		t.Fatalf("confirm one: %d %v %d", len(out), confirmed, len(left))
	}
	out, confirmed, left = Guard(docs, prior, nil, true)
	if len(out) != 3 || len(left) != 0 || len(confirmed) != 2 {
		t.Fatalf("include all: %d %v %d", len(out), confirmed, len(left))
	}
}

func TestListValid(t *testing.T) {
	for _, l := range Lists {
		if !l.Valid() {
			t.Fatalf("%s not valid", l)
		}
	}
	if List("expenses").Valid() {
		t.Fatal("expenses is not a list here")
	}
	rep := encodeReport(Report{Exported: 1})
	if rep == "" || strings.ContainsAny(rep, "+/=") {
		t.Fatalf("report header %q", rep)
	}
}
