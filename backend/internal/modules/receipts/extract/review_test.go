package extract

import (
	"slices"
	"testing"
)

// #780: masraf fişinde oran kırılımı okunur; okunamayan alan uydurulmaz,
// "kontrol et" listesine girer.
const expenseAnswer = `{"kind":"expense_receipt","kind_confidence":0.96,"fields":{
"date":{"value":"21.11.2026","confidence":0.95},
"vendor_tax_number":{"value":"4560012345","confidence":0.93},
"vendor_name":{"value":"DEMO MARKET","confidence":0.62},
"total_amount":{"value":"921,00","confidence":0.94},
"tax_amount":{"value":"","confidence":0},
"vat_1_total":{"value":"101,00","confidence":0.9},"vat_1_tax":{"value":"1,00","confidence":0.9},
"vat_10_total":{"value":"220,00","confidence":0.9},
"vat_20_total":{"value":"600,00","confidence":0.9},"vat_20_tax":{"value":"100,00","confidence":0.9}}}`

func TestExpenseVatBreakdownAndReview(t *testing.T) {
	r, err := Parse(expenseAnswer)
	if err != nil {
		t.Fatal(err)
	}
	lines := r.VatLines()
	if len(lines) != 3 {
		t.Fatalf("lines = %+v", lines)
	}
	for _, l := range lines {
		if l.Rate.String() == "10" && l.TaxRead {
			t.Fatal("the 10 percent VAT was not on the receipt: it must not read as zero")
		}
		if l.Rate.String() == "20" && (!l.TaxRead || l.Tax.String() != "100") {
			t.Fatalf("%%20 = %+v", l)
		}
	}
	if _, ok := r.Decimal(FieldTax); ok {
		t.Fatal("an empty tax_amount must stay empty")
	}
	got := ReviewFields(KindExpense, r.Fields, nil)
	for _, f := range []string{FieldReceiptNumber, FieldTax, FieldVendorName, "vat_10_tax"} {
		if !slices.Contains(got, f) {
			t.Errorf("%s not flagged: %v", f, got)
		}
	}
	for _, f := range []string{FieldDate, FieldTotal, FieldVendorTaxNumber, "vat_20_tax"} {
		if slices.Contains(got, f) {
			t.Errorf("%s flagged: %v", f, got)
		}
	}
	// The office filled the receipt number and corrected the vendor name.
	got = ReviewFields(KindExpense, r.Fields, map[string]string{FieldReceiptNumber: "0042", FieldVendorName: "DEMO MARKET GIDA"})
	if slices.Contains(got, FieldReceiptNumber) || slices.Contains(got, FieldVendorName) {
		t.Fatalf("filled / corrected fields still flagged: %v", got)
	}
	if ReviewFields(KindExpense, nil, nil) != nil {
		t.Fatal("a record without OCR fields has nothing to review")
	}
}

func TestZReviewFlagsUnreadPayments(t *testing.T) {
	r, err := Parse(`{"kind":"z_report","kind_confidence":0.9,"fields":{"date":{"value":"21.11.2026","confidence":0.9},
"z_report_number":{"value":"0780","confidence":0.9},"total_amount":{"value":"5.430,00","confidence":0.9},
"tax_amount":{"value":"812,00","confidence":0.9},"cash_amount":{"value":"2.130,00","confidence":0.9}}}`)
	if err != nil {
		t.Fatal(err)
	}
	got := ReviewFields(KindZReport, r.Fields, nil)
	if !slices.Equal(got, []string{FieldCardAmount}) {
		t.Fatalf("review = %v", got)
	}
}
