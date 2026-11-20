package taxreturns

import (
	"bytes"
	"testing"
)

func TestTRMoney(t *testing.T) {
	for in, want := range map[string]string{
		"0": "0,00", "12.5": "12,50", "1234.56": "1.234,56", "-1234567.8": "-1.234.567,80", "100": "100,00",
	} {
		if got := trMoney(d(in)); got != want {
			t.Errorf("trMoney(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestSummaryPDFKDV1(t *testing.T) {
	k, items := ComputeKDV1(KDV1Input{
		Invoices: []InvoiceLine{
			{Direction: "outgoing", Type: "SATIS", Rate: d("20"), Count: 1, Base: d("1000"), VAT: d("200")},
			{Direction: "incoming", Type: "ALIS", Rate: d("20"), Count: 1, Base: d("400"), VAT: d("80")},
		},
		Carry: Carry{Source: CarryNone},
	})
	p := Preview{Kind: KindKDV1, Period: "2026-10", Status: StatusDraft, Calendar: Calendar{DueDate: "2026-11-30"}}
	p.KDV1, p.Checklist = &k, items
	data, err := SummaryPDF(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("not a PDF: %q", data[:8])
	}
	// The form and the checklist were both recorded for the PDF.
	w := newBook()
	defer func() { _ = w.f.Close() }()
	w.kdv1(p, k)
	w.checklist(p)
	if len(w.sheets) != 2 || w.sheets[0].name != "KDV1" || w.sheets[1].name != "Kontrol listesi" {
		t.Fatalf("sheets = %d", len(w.sheets))
	}
	found := false
	for _, row := range w.sheets[0].cells {
		for _, c := range row {
			if c.money && c.text == "1.000,00" {
				found = true
			}
		}
	}
	if !found {
		t.Error("KDV1 sheet has no 1.000,00 matrah cell")
	}
}

// The section summary (#710) closes the PDF and the workbook: per source
// total, processed, ignored and waiting; no summary, no sheet.
func TestSummarySections(t *testing.T) {
	p := Preview{Kind: KindKDV1, Period: "2026-10", Status: StatusFiled, Sections: []Section{
		{Key: "retail", Total: 31, Processed: 30, Ignored: 1},
		{Key: "invoices", Total: 12, Processed: 9, Ignored: 1},
		{Key: "bank", Total: 0},
		{Key: "expenses", Total: 4, Processed: 4},
	}}
	w := newBook()
	defer func() { _ = w.f.Close() }()
	w.sections(p)
	if len(w.sheets) != 1 || w.sheets[0].name != "Bölüm özeti" {
		t.Fatalf("sheets = %d", len(w.sheets))
	}
	rows := w.sheets[0].cells
	want := [][]string{
		{"Bölüm", "Toplam", "İşlenen", "Yok sayılan", "Bekleyen"},
		{"Z raporları", "31", "30", "1", "0"},
		{"Faturalar", "12", "9", "1", "2"},
		{"Banka hareketleri", "0", "0", "0", "0"},
		{"KDV'li fişler", "4", "4", "0", "0"},
	}
	for i, wr := range want {
		row := rows[i+1]
		for j, text := range wr {
			if row[j+1].text != text {
				t.Fatalf("row %d col %d = %q, want %q", i+1, j+1, row[j+1].text, text)
			}
		}
	}
	data, err := SummaryPDF(p)
	if err != nil || !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("pdf: %v", err)
	}

	empty := newBook()
	defer func() { _ = empty.f.Close() }()
	empty.sections(Preview{})
	if len(empty.sheets) != 0 {
		t.Fatal("no sections, no sheet")
	}
}
