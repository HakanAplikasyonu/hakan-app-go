package demo

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources"
)

// Easytrade demo (issue #385): Atlas Gıda's Easytrade company with October
// 2026 expenses, financial transactions (collections and payments of
// people and accounts by cash, card and bank transfer) and Z reports. The
// seed writes them through sources.Write like a sync would; the connection
// has no credentials, so the worker never calls Easytrade for it.
const (
	EasytradeTaxpayer  = "atlas"
	EasytradeCompanyID = "demo-42"
)

// EasytradeBranches are the demo company's branches.
var EasytradeBranches = []struct{ ID, Name string }{{"1", "Merkez"}, {"2", "Kadıköy Şube"}}

func oct(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }

func amt(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// EasytradeBatch is what a sync of October 2026 would have written.
func EasytradeBatch() sources.Batch {
	b := sources.Batch{Excluded: sources.Excluded{}}
	rate := amt("20")
	for _, e := range []struct {
		id, desc, total, vendor, vkn, pay, receipt string
		day                                        int
		kind                                       string
	}{
		{"et-exp-1", "Akaryakıt 34 ATL 101", "1500.00", "Boğaz Petrol A.Ş.", "1111111111", "credit_card", "F-1042", 6, "fuel_receipt"},
		{"et-exp-2", "Kırtasiye", "360.00", "Kadıköy Kırtasiye", "2222222222", "cash", "0031", 9, "okc_receipt"},
		{"et-exp-3", "Depo kirası Ekim", "24000.00", "Moda Gayrimenkul Ltd.", "3333333333", "bank_transfer", "", 15, "expense_voucher"},
	} {
		total := amt(e.total)
		tax := total.Mul(rate).Div(amt("120")).Round(2)
		b.Expenses = append(b.Expenses, sources.Expense{
			SourceID: e.id, Date: oct(e.day), Period: "2026-10", Amount: total, TaxAmount: tax, TaxRate: &rate,
			Currency: "TRY", Description: e.desc, ReceiptType: e.kind, VendorName: e.vendor, VendorTaxID: e.vkn,
			PaymentMethod: e.pay, ReceiptNumber: e.receipt,
			VatLines: []sources.ExpenseVatLine{{Rate: rate, Base: total.Sub(tax), Tax: tax}},
			Raw:      map[string]any{"easytrade": map[string]any{"id": e.id, "description": e.desc, "amount": e.total}}, Hash: "demo-" + e.id,
		})
	}
	for _, f := range []struct {
		id, typ, pay, amount, customer, tax, box, boxType, desc string
		day                                                     int
	}{
		{"et-fin-1", "income", "cash", "4500.00", "Ahmet Yılmaz", "10000000078", "Merkez Kasa", "cash", "Ekim tahsilatı", 3},
		{"et-fin-2", "income", "credit_card", "12800.00", "Mehmet Demir", "", "Garanti POS", "valor", "Toptan satış tahsilatı", 7},
		{"et-fin-3", "income", "bank_transfer", "38250.00", "Yıldız Market Ltd.", "9100000021", "İş Bankası TL", "bank_account", "Havale · fatura ödemesi", 10},
		{"et-fin-4", "expense", "bank_transfer", "24000.00", "Moda Gayrimenkul Ltd.", "3333333333", "İş Bankası TL", "bank_account", "Depo kirası Ekim", 15},
		{"et-fin-5", "expense", "cash", "2750.00", "Ahmet Yılmaz", "10000000078", "Merkez Kasa", "cash", "Nakliye ödemesi", 18},
		{"et-fin-6", "retail_sale", "credit_card", "6420.00", "Perakende müşteri", "", "Garanti POS", "valor", "Perakende satış", 21},
		{"et-fin-7", "income", "cash", "3100.00", "Mehmet Demir", "", "Merkez Kasa", "cash", "Ekim tahsilatı", 24},
		{"et-fin-8", "expense", "credit_card", "1500.00", "Boğaz Petrol A.Ş.", "1111111111", "Şirket kartı", "credit_card", "Akaryakıt", 6},
		// Çek ve senetle ödeme (#565): banka hareketi bunlara bağlanır.
		{"et-fin-9", "expense", "check", "18600.00", "Anadolu Un Sanayi A.Ş.", "4444444444", "Çek portföyü", "check", "Un alımı · çek no 0047812", 20},
		{"et-fin-10", "expense", "promissory_note", "9400.00", "Ege Ambalaj Ltd.", "5555555555", "Senet portföyü", "promissory_note", "Ambalaj alımı · senet no S-2026-118", 23},
	} {
		dir := map[string]string{"income": "in", "retail_sale": "in", "expense": "out"}[f.typ]
		a := amt(f.amount)
		b.FinancialTransactions = append(b.FinancialTransactions, sources.FinancialTransaction{
			SourceID: f.id, Date: oct(f.day), Period: "2026-10", TransactionType: f.typ, Direction: dir,
			PaymentMethodType: f.pay, Amount: a, Currency: "TRY", AmountTRY: a, Description: f.desc, Status: "completed",
			CustomerName: f.customer, CustomerTaxNumber: f.tax, CashBoxName: f.box, CashBoxType: f.boxType,
			Raw: map[string]any{"easytrade": map[string]any{"id": f.id, "transaction_type": f.typ, "payment_method_type": f.pay}}, Hash: "demo-" + f.id,
		})
	}
	for i, z := range []struct{ no, cash, card string }{
		{"0181", "4210.50", "9380.00"}, {"0182", "3875.00", "11240.75"}, {"0183", "5120.25", "8790.00"},
		{"0184", "2980.00", "7410.50"}, {"0185", "6040.75", "12890.25"},
	} {
		cash, card := amt(z.cash), amt(z.card)
		total := cash.Add(card)
		tax := total.Mul(rate).Div(amt("120")).Round(2)
		b.ZReports = append(b.ZReports, sources.ZReport{
			SourceID: "z:et-z-" + z.no, Date: oct(27 + i), Period: "2026-10", ZReportNumber: z.no, OKCSerialNumber: "JH40012345",
			Subtotal: total.Sub(tax), Tax: tax, Total: total, Currency: "TRY",
			Raw: map[string]any{"easytrade": map[string]any{"z_report_number": z.no,
				"payments": map[string]any{"cash": z.cash, "credit_card": z.card}}},
			Hash: "demo-z-" + z.no,
		})
	}
	return b
}
