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
