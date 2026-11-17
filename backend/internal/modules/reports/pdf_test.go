package reports

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ledongthuc/pdf"
)

func sampleReport() Report {
	r := Report{Kind: KindMonth, Period: "2026-10", Label: "Ekim 2026", Currency: "TRY",
		Taxpayer: Taxpayer{Title: "Örnek Gıda Ticaret Ltd. Şti.", TaxID: "1234567890"},
		Totals: Totals{Revenue: "95000.00", Expenses: "29500.00", Net: "65500.00", VATCalculated: "19000.00",
			VATDeductible: "5800.00", VATPayable: "13200.00"},
		Categories: []Category{
			{Key: "purchase_invoices", Label: "Alış faturaları", Amount: "28000.00", Records: 1, Share: "70.0"},
			{Key: "fuel_receipt", Label: "Akaryakıt", Amount: "6000.00", Records: 4, Share: "15.0"},
			{Key: "okc_receipt", Label: "Yazar kasa fişleri", Amount: "6000.00", Records: 9, Share: "15.0"},
		},
		Commentary: &Commentary{Status: "approved", Body: "Satışlarınız önceki aya göre arttı.\n\nMasraflar incelendi."},
		Notes:      []string{"Tutarlar KDV hariç ve TL'dir."},
	}
	up := "18.8"
	r.Change.Revenue = &up
	for i, m := range Months("2025-11", "2026-10") {
		rev, exp := 60000+i*3500+(i%3)*8000, 30000+(i%4)*6000-i*900
		r.Months = append(r.Months, Month{Period: m, Label: MonthShort(m), Revenue: fmt.Sprint(rev), Expenses: fmt.Sprint(exp),
			Net: fmt.Sprint(rev - exp), VATCalculated: fmt.Sprint(rev / 5), VATDeductible: fmt.Sprint(exp / 5)})
	}
	return r
}

func pdfText(t *testing.T, data []byte) string {
	t.Helper()
	rd, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := 1; i <= rd.NumPage(); i++ {
		p := rd.Page(i)
		if p.V.IsNull() {
			continue
		}
		rows, err := p.GetTextByRow()
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			for _, w := range row.Content {
				b.WriteString(w.S)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func TestRenderPDF(t *testing.T) {
	data, err := RenderPDF(sampleReport(), PDFOptions{OfficeName: "Demo Mali Müşavirlik", Now: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
	if p := os.Getenv("REPORT_PDF_OUT"); p != "" {
		_ = os.WriteFile(p, data, 0o600)
	}
	text := pdfText(t, data)
	for _, want := range []string{"Demo Mali Müşavirlik", "Örnek Gıda", "Ekim 2026", "₺95.000,00", "Müşavir yorumu", "Gider kategorileri", "17 Kasım 2026"} {
		if !strings.Contains(text, want) {
			t.Errorf("PDF text has no %q", want)
		}
	}
	draft := sampleReport()
	draft.Commentary.Status = "draft"
	data, err = RenderPDF(draft, PDFOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(pdfText(t, data), "Müşavir yorumu") {
		t.Error("a draft comment must not be printed")
	}
}

func TestTRFormat(t *testing.T) {
	cases := map[string]string{"95250.75": "₺95.250,75", "0": "₺0,00", "-1234567.5": "−₺1.234.567,50", "999.999": "₺1.000,00"}
	for in, want := range cases {
		if got := TRMoney(in); got != want {
			t.Errorf("TRMoney(%s) = %s, want %s", in, got, want)
		}
	}
	if TRCompact(1250000) != "1,3 Mn" && TRCompact(1250000) != "1,2 Mn" {
		t.Errorf("compact: %s", TRCompact(1250000))
	}
	if TRCompact(45000) != "45 B" || TRCompact(-900) != "−900" {
		t.Errorf("compact: %s %s", TRCompact(45000), TRCompact(-900))
	}
}
