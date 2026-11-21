package taxreturns

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jung-kurt/gofpdf/v2"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ioengine"
)

// Cell kinds of the recorded form (book.record), for the PDF.
const (
	cellPlain = iota
	cellTitle
	cellHead
	cellBold
)

type recCell struct {
	text  string
	kind  int
	money bool
}

type recSheet struct {
	name   string
	widths []float64
	cells  map[int]map[int]recCell // row → column → cell
}

func (b *book) record(col int, v any, style int) {
	if len(b.sheets) == 0 {
		return
	}
	sh := b.sheets[len(b.sheets)-1]
	c := recCell{}
	switch style {
	case b.ti:
		c.kind = cellTitle
	case b.head:
		c.kind = cellHead
	case b.bold:
		c.kind = cellBold
	}
	switch t := v.(type) {
	case Money:
		c.text, c.money = trMoney(t.Dec()), true
	case nil:
	default:
		c.text = fmt.Sprint(t)
	}
	if sh.cells[b.row] == nil {
		sh.cells[b.row] = map[int]recCell{}
	}
	sh.cells[b.row][col] = c
}

// trMoney formats an amount the Turkish way: 1.234,56.
func trMoney(d decimal.Decimal) string {
	s := d.StringFixed(2)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, _ := strings.Cut(s, ".")
	var out []byte
	for i := range len(whole) {
		if i > 0 && (len(whole)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, whole[i])
	}
	r := string(out) + "," + frac
	if neg {
		r = "-" + r
	}
	return r
}

// ExportPDF writes the return summary as PDF (#669 H): the same form as the
// workbook (every part, the checklist, then the section summary, #710),
// one section per sheet.
func (s *Service) ExportPDF(ctx context.Context, sc engagementctx.Scope, kind, period string) ([]byte, string, error) {
	p, err := s.Preview(ctx, sc, kind, period)
	if err != nil {
		return nil, "", err
	}
	data, err := SummaryPDF(p)
	if err != nil {
		return nil, "", err
	}
	return data, fmt.Sprintf("%s-%s.pdf", kind, period), nil
}

// SummaryPDF renders a preview as a PDF: each workbook sheet on its own
// page (landscape when it has many columns), titles, header rows and
// amounts as in the workbook.
func SummaryPDF(p Preview) ([]byte, error) {
	w := newBook()
	defer func() { _ = w.f.Close() }()
	switch {
	case p.KDV1 != nil:
		w.kdv1(p, *p.KDV1)
	case p.KDV2 != nil:
		w.kdv2(p, *p.KDV2)
	case p.MUHSGK != nil:
		w.muhsgk(p, *p.MUHSGK)
	case p.BABS != nil:
		w.babs(p, *p.BABS)
	}
	w.checklist(p)
	w.sections(p)

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(false, 12)
	if err := ioengine.RegisterPDFFonts(pdf); err != nil {
		return nil, err
	}
	for _, sh := range w.sheets {
		drawSheet(pdf, sh)
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func drawSheet(pdf *gofpdf.Fpdf, sh *recSheet) {
	orientation, width, bottom := "P", 190.0, 282.0
	if len(sh.widths) > 7 {
		orientation, width, bottom = "L", 277.0, 195.0
	}
	pdf.AddPageFormat(orientation, gofpdf.SizeType{Wd: 210, Ht: 297})
	total := 0.0
	for _, x := range sh.widths {
		total += x
	}
	cols := make([]float64, len(sh.widths))
	for i, x := range sh.widths {
		cols[i] = x / total * width
	}
	rows := make([]int, 0, len(sh.cells))
	for r := range sh.cells {
		rows = append(rows, r)
	}
	sort.Ints(rows)

	y, prev := 12.0, 0
	for _, r := range rows {
		if prev != 0 && r > prev+1 {
			y += 3 // a blank workbook row
		}
		prev = r
		cells := sh.cells[r]
		h := 5.0
		for _, c := range cells {
			if c.kind == cellTitle {
				h = 7
			}
		}
		if y+h > bottom {
			pdf.AddPageFormat(orientation, gofpdf.SizeType{Wd: 210, Ht: 297})
			y = 12
		}
		x := 10.0
		for i := range cols {
			c, ok := cells[i+1]
			if !ok {
				x += cols[i]
				continue
			}
			style, size := "", 8.0
			switch c.kind {
			case cellTitle:
				style, size = "B", 11
			case cellHead, cellBold:
				style = "B"
			}
			pdf.SetFont(ioengine.PDFFontFamily, style, size)
			fill := c.kind == cellHead
			if fill {
				pdf.SetFillColor(238, 242, 247)
			}
			align := "L"
			if c.money {
				align = "R"
			}
			w := cols[i]
			if c.kind == cellTitle {
				w = width - (x - 10) // titles run over the empty cells
			}
			text := c.text
			for len(text) > 0 && pdf.GetStringWidth(text) > w-1 {
				rs := []rune(text)
				text = string(rs[:len(rs)-1])
			}
			pdf.SetXY(x, y)
			pdf.CellFormat(w, h, text, "", 0, align, fill, 0, "")
			if c.kind == cellTitle {
				break
			}
			x += cols[i]
		}
		y += h
	}
}
