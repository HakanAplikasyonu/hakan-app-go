package usecase

import (
	"bytes"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
)

// Dört ondalıklı kaynak tutarlar (NUMERIC(18,4)) Luca dosyasından düşmez:
// veri → fiş → dosya byte'ları → geri okuma. Her fiş kendi içinde dengeli,
// %20 / %10 / %1 ayrı 191 satırında. Uydurma veriler.
func TestLucaExpenseFileFourDecimals(t *testing.T) {
	plan := []engine.PlanAccount{
		{Code: "770.01", Name: "GENEL GİDERLER", IsLeaf: true},
		{Code: "191.01", Name: "İNDİRİLECEK KDV %20", IsLeaf: true},
		{Code: "191.02", Name: "İNDİRİLECEK KDV %10", IsLeaf: true},
		{Code: "191.03", Name: "İNDİRİLECEK KDV %1", IsLeaf: true},
		{Code: "100.01", Name: "MERKEZ KASA", IsLeaf: true},
	}
	day := func(n int) time.Time { return time.Date(2026, 10, n, 0, 0, 0, 0, time.UTC) }
	xs := []lucaExpense{
		{ID: "a", Date: day(4), Total: dd("150.0000"), Tax: dd("13.2650"), Deductible: true, ExpenseAccount: "770.01",
			VATAccount: "191", PaymentAccount: "100.01", VendorName: "Deneme Market", VendorTaxID: "9990001112",
			ReceiptNumber: "D-10", VatLines: []VatLine{
				{Rate: dd("20"), Base: dd("41.6750"), Tax: dd("8.3350")},
				{Rate: dd("10"), Base: dd("44.5500"), Tax: dd("4.4550")},
				{Rate: dd("1"), Base: dd("47.5000"), Tax: dd("0.4750")},
			}},
		{ID: "b", Date: day(7), Total: dd("100.0000"), Tax: dd("8.3350"), Deductible: true, ExpenseAccount: "770.01",
			VATAccount: "191", PaymentAccount: "100.01", ReceiptNumber: "D-11"},
	}
	entries, noBreakdown, skipped := lucaEntries(xs, plan)
	if len(entries) != 2 || skipped != 0 || noBreakdown != 1 {
		t.Fatalf("entries %d, no breakdown %d, skipped %d", len(entries), noBreakdown, skipped)
	}
	f, err := lucaFile("2026-10", entries)
	if err != nil {
		t.Fatal(err)
	}
	x, err := excelize.OpenReader(bytes.NewReader(f.Data))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = x.Close() }()
	rows, err := x.GetRows(x.GetSheetName(0), excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 || rows[0][0] != "Fiş No" || rows[0][3] != "Hesap Kodu" || rows[0][7] != "Borç" || rows[0][8] != "Alacak" {
		t.Fatalf("header %v", rows[0])
	}
	sums := map[string][2]decimal.Decimal{}
	vat := map[string]string{}
	for _, r := range rows[1:] {
		for len(r) < 9 {
			r = append(r, "")
		}
		num := func(s string) decimal.Decimal {
			if s == "" {
				return decimal.Zero
			}
			return dd(s)
		}
		s := sums[r[0]]
		s[0], s[1] = s[0].Add(num(r[7])), s[1].Add(num(r[8]))
		sums[r[0]] = s
		if r[0] == "1" && len(r[3]) > 3 && r[3][:3] == "191" {
			vat[r[3]] = num(r[7]).StringFixed(2)
		}
	}
	if len(sums) != 2 {
		t.Fatalf("vouchers %v", sums)
	}
	for no, s := range sums {
		if !s[0].Equal(s[1]) {
			t.Errorf("fiş %s: borç %s alacak %s", no, s[0], s[1])
		}
	}
	if vat["191.01"] != "8.34" || vat["191.02"] != "4.46" || vat["191.03"] != "0.48" {
		t.Errorf("vat rows %v", vat)
	}
	if !sums["1"][1].Equal(dd("150")) || !sums["2"][1].Equal(dd("100")) {
		t.Errorf("totals %v", sums)
	}
}
