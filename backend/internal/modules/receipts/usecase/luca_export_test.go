package usecase

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
)

// Luca'ya aktar (#780): fiş dosyasıyla aynı 15 sütunlu başlık; her KDV
// oranı kendi 191 satırında, kırılımı olmayan masraf tek ve işaretli KDV
// satırıyla. Uydurma veriler.
func TestLucaExpenseFile(t *testing.T) {
	plan := []engine.PlanAccount{
		{Code: "770.01", Name: "GENEL GİDERLER", IsLeaf: true},
		{Code: "191.01", Name: "İNDİRİLECEK KDV %20", IsLeaf: true},
		{Code: "191.02", Name: "İNDİRİLECEK KDV %10", IsLeaf: true},
		{Code: "100.01", Name: "MERKEZ KASA", IsLeaf: true},
	}
	day := func(n int) time.Time { return time.Date(2026, 10, n, 0, 0, 0, 0, time.UTC) }
	xs := []lucaExpense{
		{ID: "b", Date: day(15), Total: dd("250"), Tax: dd("0"), Deductible: true, ExpenseAccount: "770.01",
			VATAccount: "191", PaymentAccount: "100.01", VendorName: "Örnek Büfe", ReceiptNumber: "B-2"},
		{ID: "a", Date: day(3), Total: dd("820"), Tax: dd("120"), Deductible: true, ExpenseAccount: "770.01",
			VATAccount: "191", PaymentAccount: "100.01", VendorName: "Örnek Kırtasiye", VendorTaxID: "1234567890",
			ReceiptNumber: "A-1", VatLines: []VatLine{{Rate: dd("20"), Base: dd("500"), Tax: dd("100")}, {Rate: dd("10"), Base: dd("200"), Tax: dd("20")}}},
		{ID: "c", Date: day(20), Total: dd("120"), Tax: dd("20"), Deductible: true, ExpenseAccount: "770.01",
			VATAccount: "191", PaymentAccount: "100.01", ReceiptNumber: "C-3"},
		// KDV toplamdan büyük: dosyaya alınmaz.
		{ID: "d", Date: day(21), Total: dd("10"), Tax: dd("50"), Deductible: true, ExpenseAccount: "770.01",
			VATAccount: "191", PaymentAccount: "100.01", VatLines: []VatLine{{Rate: dd("20"), Base: dd("0"), Tax: dd("50")}}},
	}
	entries, noBreakdown, skipped := lucaEntries(xs, plan)
	if len(entries) != 3 || noBreakdown != 2 || skipped != 1 {
		t.Fatalf("entries %d, no breakdown %d, skipped %d", len(entries), noBreakdown, skipped)
	}
	if entries[0].ID != "a" || entries[0].No != "1" || entries[2].No != "3" {
		t.Fatalf("order %+v", entries)
	}
	f, err := lucaFile("2026-10", entries)
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "masraf_luca_fis_2026-10.xlsx" {
		t.Fatalf("name %q", f.Name)
	}
	x, err := excelize.OpenReader(bytes.NewReader(f.Data))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := x.GetRows(x.GetSheetName(0))
	if err != nil {
		t.Fatal(err)
	}
	header := []string{"Fiş No", "Fiş Tarihi", "Fiş Açıklama", "Hesap Kodu", "Evrak No", "Evrak Tarihi", "Detay Açıklama",
		"Borç", "Alacak", "Miktar", "Belge Türü", "Para Birimi", "Kur", "Döviz Tutar", "TC/Vergi No"}
	for i, h := range header {
		if rows[0][i] != h {
			t.Fatalf("header %d = %q, want %q", i, rows[0][i], h)
		}
	}
	type row struct{ no, account, debit, credit string }
	var got []row
	for _, r := range rows[1:] {
		for len(r) < len(header) {
			r = append(r, "")
		}
		got = append(got, row{r[0], r[3], r[7], r[8]})
	}
	want := []row{
		{"1", "770.01", "700.00", "0.00"}, {"1", "191.01", "100.00", "0.00"}, {"1", "191.02", "20.00", "0.00"}, {"1", "100.01", "0.00", "820.00"},
		{"2", "770.01", "250.00", "0.00"}, {"2", "100.01", "0.00", "250.00"},
		{"3", "770.01", "100.00", "0.00"}, {"3", "191", "20.00", "0.00"}, {"3", "100.01", "0.00", "120.00"},
	}
	if len(got) != len(want) {
		t.Fatalf("rows %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if rows[1][14] != "1234567890" || rows[1][4] != "A-1" {
		t.Errorf("tax id / evrak no: %q %q", rows[1][14], rows[1][4])
	}
	if d := rows[8][6]; d == "" || !bytes.Contains([]byte(d), []byte(NoBreakdownNote)) {
		t.Errorf("no-breakdown note missing: %q", d)
	}
}

// Çift aktarım koruması (#780): daha önce aktarılan masraf varsayılan olarak
// dışarıda kalır; tek tek (confirm) ya da hepsi birden yeniden dahil edilir.
func TestGuardExpenses(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	xs := []lucaExpense{{uuid: a, ID: a.String()}, {uuid: b, ID: b.String()}, {uuid: c, ID: c.String()}}
	prior := map[string]ExportMark{
		journals.ExpenseDocumentKey(a): {File: "masraf_luca_fis_2026-10.xlsx"},
		journals.ExpenseDocumentKey(b): {File: "luca_fis_2026-10.xlsx"},
	}
	out, confirmed, left := guardExpenses(xs, prior, nil, false)
	if len(out) != 1 || out[0].uuid != c || left != 2 || len(confirmed) != 0 {
		t.Fatalf("default: %d %v %d", len(out), confirmed, left)
	}
	out, confirmed, left = guardExpenses(xs, prior, []string{strings.ToUpper(journals.ExpenseDocumentKey(b))}, false)
	if len(out) != 2 || left != 1 || !confirmed[journals.ExpenseDocumentKey(b)] {
		t.Fatalf("confirm one: %d %v %d", len(out), confirmed, left)
	}
	out, confirmed, left = guardExpenses(xs, prior, nil, true)
	if len(out) != 3 || left != 0 || len(confirmed) != 2 {
		t.Fatalf("include all: %d %v %d", len(out), confirmed, left)
	}
}
