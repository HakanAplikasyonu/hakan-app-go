package usecase

import (
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/posting"
)

// Tek oranlı fiş (Luca uçtan uca denetimi): telefondan ya da WhatsApp'tan
// gelen akaryakıt fişinde yapay zekâ oran kırılımı değil tek KDV oranı
// (%20) okur. Onayın yevmiye fişi KDV'yi %20'nin 191 alt hesabına yazar;
// Masraflar'ın Luca dosyası da aynı hesabı yazmalı, satırı "kırılım yok"
// diye işaretlememeli (oran okundu, tahmin değil). Oranı bilinmeyen masraf
// yine işaretli tek satırla gider. Uydurma veriler.
func TestLucaExpenseSingleReadRate(t *testing.T) {
	plan := []engine.PlanAccount{
		{Code: "770.01", Name: "GENEL GİDERLER", IsLeaf: true},
		{Code: "191.01", Name: "İNDİRİLECEK KDV %20", IsLeaf: true},
		{Code: "191.02", Name: "İNDİRİLECEK KDV %10", IsLeaf: true},
		{Code: "100.01", Name: "MERKEZ KASA", IsLeaf: true},
	}
	day := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	rate := dd("20")
	fuel := lucaExpense{ID: "f", Date: day, Total: dd("1250"), Tax: dd("208.33"), TaxRate: &rate, Deductible: true,
		ExpenseAccount: "770.01", VATAccount: "191", PaymentAccount: "100.01", VendorName: "Örnek Akaryakıt", ReceiptNumber: "0042"}
	unknown := fuel
	unknown.ID, unknown.TaxRate, unknown.ReceiptNumber = "u", nil, "0043"

	entries, noBreakdown, skipped := lucaEntries([]lucaExpense{fuel, unknown}, plan)
	if len(entries) != 2 || noBreakdown != 1 || skipped != 0 {
		t.Fatalf("entries %d, no breakdown %d, skipped %d", len(entries), noBreakdown, skipped)
	}
	// Onayın yevmiye fişi (postExpense) ile aynı satırlar.
	journal, err := posting.ExpenseEntry(posting.ExpenseInput{SourceID: "expense:f", Date: day, Total: dd("1250"), Tax: dd("208.33"),
		TaxRate: &rate, Deductible: true, ExpenseAccount: "770.01", VATAccount: "191", PaymentAccount: "100.01",
		VendorName: "Örnek Akaryakıt", ReceiptNumber: "0042", Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	got := entries[0].Lines
	if len(got) != len(journal.Lines) {
		t.Fatalf("lines %+v, journal %+v", got, journal.Lines)
	}
	for i, l := range journal.Lines {
		if got[i].Account != l.Account || !got[i].Debit.Equal(l.Debit) || !got[i].Credit.Equal(l.Credit) {
			t.Errorf("line %d = %s %s/%s, journal %s %s/%s", i, got[i].Account, got[i].Debit, got[i].Credit, l.Account, l.Debit, l.Credit)
		}
	}
	if got[1].Account != "191.01" || !got[1].Debit.Equal(dd("208.33")) {
		t.Errorf("VAT line = %s %s, want 191.01 208.33", got[1].Account, got[1].Debit)
	}
	if strings.Contains(got[1].Description, NoBreakdownNote) {
		t.Errorf("read rate marked as no breakdown: %q", got[1].Description)
	}
	// Oranı bilinmeyen: ana 191, işaretli.
	vat := entries[1].Lines[1]
	if vat.Account != "191" || !strings.Contains(vat.Description, NoBreakdownNote) {
		t.Errorf("unknown rate VAT line = %s %q", vat.Account, vat.Description)
	}
}
