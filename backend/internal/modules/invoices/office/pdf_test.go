package office

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/view"
)

func TestTrMoney(t *testing.T) {
	for in, want := range map[string]string{
		"0.00": "0,00", "999.10": "999,10", "1234.50": "1.234,50", "1234567.00": "1.234.567,00", "-1500.25": "-1.500,25",
	} {
		if got := trMoney(in); got != want {
			t.Errorf("trMoney(%q) = %q, want %q", in, got, want)
		}
	}
	if got := trimQty("2.5000"); got != "2,5" {
		t.Errorf("trimQty = %q", got)
	}
	if got := pdfFilename("ABC2026/0001 ü"); got != "fatura-ABC2026-0001.pdf" {
		t.Errorf("pdfFilename = %q", got)
	}
}

// TestInvoicePDF: Turkish text and many lines render to a multi-page PDF.
func TestInvoicePDF(t *testing.T) {
	d := view.Detail{Invoice: view.Invoice{
		InvoiceNumber: "ABC2026000000001", Direction: "incoming", Type: "SATIS", Profile: "TICARIFATURA",
		IssueDate: "2026-10-15", CounterpartyName: "Çağlar Şişecam Ürünleri İğdır Ltd. Şti.", CounterpartyID: "1234567890",
		Currency: "TRY", TaxExclusive: "10000.00", TaxAmount: "2000.00", Withholding: "0.00", Payable: "12000.00",
	}, TaxSummary: []view.TaxSummaryRow{{TaxRate: "20", Base: "10000.00", VAT: "2000.00"}}}
	for i := 1; i <= 80; i++ {
		d.Items = append(d.Items, view.Item{LineNo: int32(i), Name: fmt.Sprintf("Kırtasiye ürünü %d", i), Quantity: "1.0000",
			UnitName: "adet", UnitPrice: "125.00", TaxRate: "20", TaxExclusive: "125.00", TaxAmount: "25.00"})
	}
	out, err := InvoicePDF(d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatal("not a PDF")
	}
	if n := bytes.Count(out, []byte("/Type /Page\n")); n < 2 {
		t.Fatalf("pages = %d, want ≥ 2 for 80 lines", n)
	}
}
