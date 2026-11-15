package statement

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// ErrScannedPDF: the PDF has no text layer. Scanned statements need OCR
// (23 §5.2), which is not part of this channel yet.
var ErrScannedPDF = errors.New("pdf: no text layer (scanned statement)")

// ErrPDFBalance: a text PDF whose running balance does not add up is not
// imported (23 §5.3: it never reaches the preview).
var ErrPDFBalance = errors.New("pdf: running balance does not add up")

type pdfText struct {
	x, y float64
	s    string
}

// ReadPDFRows turns a text-based statement PDF into a virtual sheet: the
// table's header row fixes the column boundaries (23 §5.1) and every text run
// below it falls into the column whose range contains its start.
func ReadPDFRows(data []byte) ([][]string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("pdf: %w", err)
	}
	var rows [][]pdfText
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		rows = append(rows, pageRows(p.Content().Text)...)
	}
	if len(rows) == 0 {
		return nil, ErrScannedPDF
	}

	var out [][]string
	var bounds []float64 // start x of each header column
	for _, row := range rows {
		if isHeaderRow(row) {
			bounds = bounds[:0]
			for _, t := range row {
				bounds = append(bounds, t.x)
			}
			cells := make([]string, len(row))
			for j, t := range row {
				cells[j] = t.s
			}
			out = append(out, cells)
			continue
		}
		if len(bounds) == 0 {
			// Header area (bank name, IBAN, period): kept as one cell per run.
			cells := make([]string, len(row))
			for j, t := range row {
				cells[j] = t.s
			}
			out = append(out, cells)
			continue
		}
		cells := make([]string, len(bounds))
		for _, t := range row {
			j := columnOf(bounds, t.x)
			if cells[j] != "" {
				cells[j] += " "
			}
			cells[j] += t.s
		}
		out = append(out, cells)
	}
	return out, nil
}

// pageRows groups glyphs into text runs and runs into rows. Glyphs of one
// run share a baseline; their X either advances (embedded fonts with widths)
// or stays at the run start (core fonts), so a glyph joins the current run
// when it is on the same baseline and not far to the right of it.
func pageRows(glyphs []pdf.Text) [][]pdfText {
	var runs []pdfText
	var lastX float64
	for _, g := range glyphs {
		n := len(runs)
		if n > 0 && math.Abs(g.Y-runs[n-1].y) < 1 && (g.X == lastX || (g.X > lastX && g.X-lastX < g.FontSize*1.2)) {
			runs[n-1].s += g.S
			lastX = g.X
			continue
		}
		runs = append(runs, pdfText{x: g.X, y: g.Y, s: g.S})
		lastX = g.X
	}
	// Rows top to bottom (PDF y grows upwards), runs left to right.
	sort.SliceStable(runs, func(i, j int) bool {
		if math.Abs(runs[i].y-runs[j].y) >= 2 {
			return runs[i].y > runs[j].y
		}
		return runs[i].x < runs[j].x
	})
	var rows [][]pdfText
	for _, r := range runs {
		r.s = strings.Join(strings.Fields(r.s), " ")
		if r.s == "" {
			continue
		}
		n := len(rows)
		if n > 0 && math.Abs(rows[n-1][0].y-r.y) < 2 {
			rows[n-1] = append(rows[n-1], r)
			continue
		}
		rows = append(rows, []pdfText{r})
	}
	return rows
}

func isHeaderRow(row []pdfText) bool {
	var hasDate, hasDesc, hasAmt bool
	for _, t := range row {
		n := banking.NormalizeDescription(t.s)
		for _, name := range headerNames[colDate] {
			hasDate = hasDate || n == name
		}
		for _, name := range headerNames[colDescription] {
			hasDesc = hasDesc || n == name
		}
		for _, c := range []column{colAmount, colDebit, colCredit} {
			for _, name := range headerNames[c] {
				hasAmt = hasAmt || n == name
			}
		}
	}
	return hasDate && hasDesc && hasAmt
}

// columnOf: the last header column that starts at or before x, with a small
// tolerance for right-aligned numbers that start a little left of the header.
func columnOf(bounds []float64, x float64) int {
	j := 0
	for i, b := range bounds {
		if x+12 >= b {
			j = i
		}
	}
	return j
}

// ParsePDF reads a text PDF statement. The running balance must add up.
func ParsePDF(data []byte) (Statement, error) {
	rows, err := ReadPDFRows(data)
	if err != nil {
		return Statement{}, err
	}
	st, err := ParseTabular(rows, "pdf")
	if err != nil {
		return st, err
	}
	for i := range st.Lines {
		st.Lines[i].CaptureMethod = banking.CapturePDF
	}
	if st.Lines[0].BalanceAfter == nil {
		return st, fmt.Errorf("%w: no balance column", ErrPDFBalance)
	}
	if g := CheckRunning(st.Lines); g != nil {
		return st, fmt.Errorf("%w: %s", ErrPDFBalance, g.Reason)
	}
	return st, nil
}
