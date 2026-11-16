package petitions

import (
	"bytes"
	"strings"

	"github.com/jung-kurt/gofpdf/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ioengine"
)

// PDF prints a rendered petition on A4. A draft carries DraftNotice at the
// top and bottom of the page.
func PDF(r Rendered) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	if err := ioengine.RegisterPDFFonts(pdf); err != nil {
		return nil, err
	}
	pdf.SetMargins(25, 22, 25)
	pdf.SetAutoPageBreak(true, 28)
	pdf.SetTitle(r.Title, true)
	pdf.SetCreator("HAKAN dilekçe", true)
	font := func(style string, size float64) { pdf.SetFont(ioengine.PDFFontFamily, style, size) }
	draft := func() {
		if r.DraftNotice == "" {
			return
		}
		pdf.SetTextColor(180, 20, 20)
		font("B", 8)
		pdf.MultiCell(0, 4, r.DraftNotice, "", "C", false)
		pdf.SetTextColor(0, 0, 0)
	}
	pdf.SetFooterFunc(func() {
		if r.DraftNotice == "" {
			return
		}
		pdf.SetY(-18)
		draft()
	})
	pdf.AddPage()
	draft()
	pdf.Ln(4)

	font("", 11)
	pdf.CellFormat(0, 6, r.Date, "", 1, "R", false, 0, "")
	pdf.Ln(10)
	font("B", 12)
	pdf.MultiCell(0, 7, r.Recipient, "", "C", false)
	pdf.Ln(10)
	if r.Subject != "" {
		font("B", 11)
		pdf.CellFormat(18, 6, "Konu:", "", 0, "L", false, 0, "")
		font("", 11)
		pdf.MultiCell(0, 6, r.Subject, "", "L", false)
		pdf.Ln(6)
	}
	font("", 11)
	for _, para := range strings.Split(r.Body, "\n") {
		if strings.TrimSpace(para) == "" {
			pdf.Ln(3)
			continue
		}
		pdf.MultiCell(0, 6.5, "      "+strings.TrimSpace(para), "", "J", false)
		pdf.Ln(2)
	}
	pdf.Ln(16)
	font("B", 11)
	pdf.CellFormat(0, 6, r.Petitioner, "", 1, "R", false, 0, "")
	font("", 10)
	pdf.CellFormat(0, 6, "Kaşe / İmza", "", 1, "R", false, 0, "")
	if r.Signer != "" {
		pdf.Ln(10)
		font("I", 9)
		pdf.MultiCell(0, 5, "Hazırlayan: "+r.Signer, "", "L", false)
	}
	if err := pdf.Error(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
