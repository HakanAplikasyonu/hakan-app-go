package build

import (
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
)

func setup(rows [][]string, fields map[string]string, dt string) []Row {
	s := parse.Sheet{Name: "S", Rows: rows}
	l, ok := parse.DetectLayout(s)
	if !ok {
		panic("no layout")
	}
	m := mapping.Mapping{DataType: dt, SourceSystem: "bizimhesap"}
	for _, c := range l.Columns {
		m.Columns = append(m.Columns, mapping.ColumnMap{Column: c.Letter, Header: c.Header, Field: fields[c.Letter]})
	}
	return Invoices(s, l, m, Options{Period: "2026-10"})
}

var fields = map[string]string{
	"A": "issue_date", "B": "invoice_number", "C": "customer_tax_number", "D": "customer_name",
	"E": "tax_summary[10].vat", "F": "tax_summary[20].vat", "G": "amounts.payable_amount", "H": "invoice_type",
}

func TestInvoicesTRNumbersRateSummaryAndChecks(t *testing.T) {
	rows := setup([][]string{
		{"Tarih", "No", "VKN", "Ünvan", "KDV %10", "KDV %20", "Toplam", "Tür", "Plasiyer"},
		{"15.10.2026", "A-1", "1234567890", "Demir", "10,00", "20,00", "230,00", "Satış", "Ali"},
		{"46311", "A-2", "12345678901", "Kaya", "", "1.000,00", "6.000,00", "Satış İade", ""},
		{"01.11.2026", "A-3", "123", "Boz", "", "2,00", "99,00", "", ""},
		{"02.10.2026", "A-1", "1234567890", "Demir", "", "1,00", "6,00", "", ""},
		{"xx", "", "", "", "", "", "", "", ""},
		{"Genel Toplam", "", "", "", "", "", "", "", ""},
	}, fields, mapping.OutgoingInvoices)
	if len(rows) != 5 {
		t.Fatalf("rows = %d", len(rows))
	}
	r := rows[0]
	inv := r.Invoice
	if r.Result != ResultOK || inv.Counterparty.TaxNumber != "1234567890" || inv.Amounts.TaxAmount.String() != "30" ||
		inv.Amounts.TaxExclusiveAmount.String() != "200" || len(inv.TaxSummaries) != 2 || inv.TaxSummarySource != "source_summary" ||
		inv.SourceSystem != "bizimhesap" || inv.CaptureMethod != "excel_import" || inv.Period != "2026-10" {
		t.Fatalf("row1 %+v %+v", r.Messages, inv)
	}
	if inv.RawPayload["columns"].(map[string]string)["Plasiyer"] != "Ali" {
		t.Fatal("unmapped column lost from raw_payload")
	}
	// Excel serial date, TR thousands, IADE type, TCKN.
	if inv2 := rows[1].Invoice; inv2.IssueDate.Format("2006-01-02") != "2026-10-16" || inv2.Type != "IADE" ||
		inv2.Amounts.PayableAmount.String() != "6000" || inv2.Counterparty.IdentityNumber == "" || rows[1].Result != ResultOK {
		t.Fatalf("row2 %+v %+v", rows[1].Messages, inv2)
	}
	// Out of period + invalid tax id + payable mismatch (base 10 + 2 != 99): warnings only.
	codes := map[string]bool{}
	for _, m := range rows[2].Messages {
		codes[m.Code] = true
	}
	if rows[2].Result != ResultWarning || !codes[CodeOutOfPeriod] || !codes[CodeInvalidTaxID] {
		t.Fatalf("row3 %+v", rows[2].Messages)
	}
	// Same number + counterparty again: error.
	if rows[3].Result != ResultError || rows[3].Messages[0].Code != CodeFileDuplicate {
		t.Fatalf("row4 %+v", rows[3].Messages)
	}
	if rows[4].Result != ResultError {
		t.Fatalf("row5 %+v", rows[4].Messages)
	}
	// Same file twice -> same source ids (no duplicates on re-import).
	again := setup([][]string{
		{"Tarih", "No", "VKN", "Ünvan", "KDV %10", "KDV %20", "Toplam", "Tür", "Plasiyer"},
		{"15.10.2026", "A-1", "1234567890", "Demir", "10,00", "20,00", "230,00", "Satış", "Ali"},
	}, fields, mapping.OutgoingInvoices)
	if again[0].Invoice.SourceID != inv.SourceID || again[0].Invoice.ContentHash != inv.ContentHash {
		t.Fatal("source id not stable")
	}
	s := Summarize(rows)
	if s.OK != 2 || s.Warnings != 1 || s.Errors != 2 || s.Total != "6329.00" || len(s.VATByRate) != 2 || s.VATByRate[1].VAT != "1022.00" {
		t.Fatalf("summary %+v", s)
	}
}

func TestHeaderEstimateAndEnglishNumbers(t *testing.T) {
	rows := setup([][]string{
		{"Date", "Invoice", "Customer", "Net", "VAT", "Total"},
		{"2026-10-15", "X1", "Acme", "1,000.00", "200.00", "1,200.00"},
		{"2026-10-16", "X2", "Beta", "50.00", "5.00", "60.00"},
	}, map[string]string{"A": "issue_date", "B": "invoice_number", "C": "customer_name",
		"D": "amounts.tax_exclusive_amount", "E": "amounts.tax_amount", "F": "amounts.payable_amount"}, mapping.IncomingInvoices)
	inv := rows[0].Invoice
	if inv.Direction != "incoming" || inv.Type != "ALIS" || inv.Amounts.TaxExclusiveAmount.String() != "1000" ||
		inv.TaxSummarySource != "header_estimate" || inv.TaxSummaries[0].TaxRate.String() != "20" {
		t.Fatalf("%+v", inv)
	}
	codes := map[string]bool{}
	for _, m := range rows[1].Messages {
		codes[m.Code] = true
	}
	if !codes[CodePayable] || !codes[CodeHeaderEst] || rows[1].Invoice.TaxSummaries[0].TaxRate.String() != "10" {
		t.Fatalf("row2 %+v", rows[1].Messages)
	}
}
