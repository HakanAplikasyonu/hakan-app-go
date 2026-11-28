package office

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/jung-kurt/gofpdf/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/view"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ioengine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// pdf serves the invoice as a PDF drawn from the stored record (#528): an
// office copy for the file, not the legal document (the GİB image is the
// UBL's own XSLT, shown in the browser).
func (h *Handler) pdf(w http.ResponseWriter, r *http.Request) {
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	d, err := view.LoadDetail(r.Context(), h.q, inv)
	if err != nil {
		response.InternalErr(w, r, err, "could not load invoice details")
		return
	}
	out, err := InvoicePDF(d)
	if err != nil {
		response.InternalErr(w, r, err, "could not render invoice PDF")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, pdfFilename(d.InvoiceNumber)))
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write(out)
}

var reUnsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// PDFFilename is the download name of an invoice PDF ("fatura-<no>.pdf");
// the mükellef panel's copy uses the same name (#780).
func PDFFilename(number string) string { return pdfFilename(number) }

func pdfFilename(number string) string {
	n := strings.Trim(reUnsafeName.ReplaceAllString(number, "-"), "-")
	if n == "" {
		n = "fatura"
	}
	return "fatura-" + n + ".pdf"
}

// trMoney writes "1234567.50" as "1.234.567,50".
func trMoney(s string) string {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, c := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	out := b.String()
	if frac != "" {
		out += "," + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

func trDate(iso string) string {
	if len(iso) >= 10 && iso[4] == '-' && iso[7] == '-' {
		return iso[8:10] + "." + iso[5:7] + "." + iso[0:4]
	}
	return iso
}

// trimQty drops trailing zeros of a quantity ("2.0000" → "2").
func trimQty(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	return strings.Replace(s, ".", ",", 1)
}

// InvoicePDF draws the invoice: header, counterparty, lines, KDV summary
// and totals, on A4 with the Turkish font.
func InvoicePDF(d view.Detail) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	if err := ioengine.RegisterPDFFonts(pdf); err != nil {
		return nil, err
	}
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 18)
	pdf.SetTitle("Fatura "+d.InvoiceNumber, true)
	pdf.AddPage()
	font := func(style string, size float64) { pdf.SetFont(ioengine.PDFFontFamily, style, size) }
	const width = 180.0

	font("I", 8)
	pdf.SetTextColor(100, 116, 139)
	pdf.MultiCell(width, 4, "Bu belge kayıttaki fatura bilgilerinden üretilmiş ofis kopyasıdır; resmî e-Fatura görüntüsü yerine geçmez.", "", "L", false)
	pdf.SetTextColor(15, 23, 43)
	pdf.Ln(2)

	kind := "Satış faturası"
	party := "Alıcı"
	if d.Direction == "incoming" {
		kind, party = "Alış faturası", "Satıcı"
	}
	font("B", 15)
	pdf.CellFormat(width, 8, kind, "", 1, "L", false, 0, "")
	font("", 10)
	meta := [][2]string{
		{"Fatura no", d.InvoiceNumber},
		{"Tarih", trDate(d.IssueDate)},
		{"Tür / senaryo", strings.Trim(d.Type+" / "+d.Profile, " /")},
		{"ETTN", d.ETTN},
		{"Para birimi", d.Currency},
	}
	for _, m := range meta {
		if m[1] == "" {
			continue
		}
		font("", 9)
		pdf.CellFormat(32, 5, m[0], "", 0, "L", false, 0, "")
		font("B", 9)
		pdf.CellFormat(width-32, 5, m[1], "", 1, "L", false, 0, "")
	}
	pdf.Ln(3)
	font("", 9)
	pdf.CellFormat(32, 5, party, "", 0, "L", false, 0, "")
	font("B", 10)
	pdf.MultiCell(width-32, 5, firstNonBlank(d.CounterpartyName, "—"), "", "L", false)
	if d.CounterpartyID != "" {
		font("", 9)
		pdf.CellFormat(32, 5, "VKN / TCKN", "", 0, "L", false, 0, "")
		pdf.CellFormat(width-32, 5, d.CounterpartyID, "", 1, "L", false, 0, "")
	}
	pdf.Ln(4)

	// Lines.
	cols := []struct {
		title string
		w     float64
		align string
	}{{"#", 8, "L"}, {"Mal / hizmet", 72, "L"}, {"Miktar", 24, "R"}, {"Birim fiyat", 26, "R"}, {"KDV %", 14, "R"}, {"Tutar", 36, "R"}}
	header := func() {
		font("B", 8)
		pdf.SetFillColor(241, 245, 249)
		for _, c := range cols {
			pdf.CellFormat(c.w, 6, c.title, "B", 0, c.align, true, 0, "")
		}
		pdf.Ln(-1)
	}
	header()
	font("", 8)
	for _, it := range d.Items {
		if pdf.GetY() > 265 {
			pdf.AddPage()
			header()
			font("", 8)
		}
		qty := trimQty(it.Quantity)
		if u := firstNonBlank(it.UnitName, it.UnitCode); u != "" {
			qty += " " + u
		}
		name := it.Name
		if len([]rune(name)) > 60 {
			name = string([]rune(name)[:59]) + "…"
		}
		vals := []string{fmt.Sprint(it.LineNo), name, qty, trMoney(it.UnitPrice), it.TaxRate, trMoney(it.TaxExclusive)}
		for i, c := range cols {
			pdf.CellFormat(c.w, 5.5, vals[i], "B", 0, c.align, false, 0, "")
		}
		pdf.Ln(-1)
	}
	pdf.Ln(4)

	// KDV summary and totals, right aligned.
	row := func(label, value string, bold bool) {
		if bold {
			font("B", 10)
		} else {
			font("", 9)
		}
		pdf.SetX(15 + width - 100)
		pdf.CellFormat(60, 5.5, label, "", 0, "L", false, 0, "")
		pdf.CellFormat(40, 5.5, value, "", 1, "R", false, 0, "")
	}
	for _, t := range d.TaxSummary {
		row("KDV matrahı (%"+t.TaxRate+")", trMoney(t.Base), false)
		row("Hesaplanan KDV (%"+t.TaxRate+")", trMoney(t.VAT), false)
	}
	row("Vergiler hariç toplam", trMoney(d.TaxExclusive), false)
	row("Toplam KDV", trMoney(d.TaxAmount), false)
	if d.Withholding != "" && d.Withholding != "0.00" {
		row("Tevkifat", trMoney(d.Withholding), false)
	}
	row("Ödenecek tutar", trMoney(d.Payable)+" "+d.Currency, true)
	if d.Notes != "" {
		pdf.Ln(4)
		font("", 8)
		pdf.MultiCell(width, 4, "Not: "+d.Notes, "", "L", false)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func firstNonBlank(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
