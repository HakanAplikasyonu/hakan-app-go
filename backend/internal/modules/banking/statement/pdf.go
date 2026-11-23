package statement

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// ErrScannedPDF: the PDF has no text layer. Scanned statements need OCR
// (23 §5.2), which is not part of this channel yet.
var ErrScannedPDF = errors.New("pdf: no text layer (scanned statement)")

// ErrPDFUnreadable: the file starts like a PDF but cannot be opened
// (damaged, encrypted, or a structure the reader does not know). The office
// is told to download the statement as Excel or CSV instead (#780).
var ErrPDFUnreadable = errors.New("pdf: unreadable")

// ErrPDFBalance: a text PDF whose running balance does not add up is not
// imported (23 §5.3: it never reaches the preview).
var ErrPDFBalance = errors.New("pdf: running balance does not add up")

type pdfText struct {
	x, y float64
	// end: right edge of the run (glyph widths; estimated when the font
	// gives none).
	end  float64
	s    string
	size float64
}

// ReadPDFRows turns a text-based statement PDF into a virtual sheet: the
// table's header row fixes the column boundaries (23 §5.1) and every text run
// below it falls into the column whose range contains its start.
func ReadPDFRows(data []byte) ([][]string, error) {
	return ReadPDFRowsWith(data, nil)
}

// ReadPDFRowsWith is ReadPDFRows that also takes a saved or built-in
// template's header row as the table header (#383).
func ReadPDFRowsWith(data []byte, tpls []Template) ([][]string, error) {
	rows, err := pdfPageRows(data)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrScannedPDF
	}
	return tableFromRuns(rows, tpls), nil
}

// pdfPageRows reads the text rows of every page. The PDF reader panics on
// some damaged files; that is an unreadable PDF, not a server error.
func pdfPageRows(data []byte) (rows [][]pdfText, err error) {
	defer func() {
		if p := recover(); p != nil {
			rows, err = nil, fmt.Errorf("%w: %v", ErrPDFUnreadable, p)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPDFUnreadable, err)
	}
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		rows = append(rows, pageRows(p.Content().Text)...)
	}
	return rows, nil
}

// tableFromRuns lays the text runs out under the header columns.
//
// Below the first header row (#780):
//   - a repeated header (the table continues on the next page) only resets
//     the column bounds; it is not written again;
//   - a text-only row right under a movement (the description wrapped to a
//     second line) is joined to that movement's cells;
//   - other text-only rows (page footer, the bank title and customer block
//     repeated on the next page) are left out. Rows with an amount stay, so
//     totals end the reading and a row with a broken date is still reported.
func tableFromRuns(rows [][]pdfText, tpls []Template) [][]string {
	var out [][]string
	var bounds []float64 // start x of each header column
	var ends []float64   // right edge of each header cell
	seenHeader := false
	last := -1 // index in out of the last movement row
	var lastY float64
	var lastSize float64
	for _, row := range rows {
		if isHeaderRow(row) || isTemplateHeader(row, tpls) {
			bounds, ends = bounds[:0], ends[:0]
			for _, t := range row {
				bounds = append(bounds, t.x)
				ends = append(ends, t.end)
			}
			last = -1
			if seenHeader {
				continue
			}
			seenHeader = true
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
			if k := rightAligned(bounds, ends, t); k >= 0 {
				j = k
			}
			if cells[j] != "" {
				cells[j] += " "
			}
			cells[j] += t.s
		}
		if !rowHasDateOrAmount(row) {
			gap := lastY - row[0].y
			if last >= 0 && gap > 0 && gap < 2.2*math.Max(lastSize, row[0].size) {
				for j, c := range cells {
					if c == "" {
						continue
					}
					if out[last][j] != "" {
						out[last][j] += " "
					}
					out[last][j] += c
				}
				lastY = row[0].y
			}
			continue
		}
		out = append(out, cells)
		last, lastY, lastSize = len(out)-1, row[0].y, row[0].size
	}
	return out
}

// reDateRun: a run that starts with a date (dd.MM.yyyy, dd/MM/yy, yyyy-MM-dd).
var reDateRun = regexp.MustCompile(`^(?:\d{1,2}[./-]\d{1,2}[./-]\d{2,4}|\d{4}-\d{2}-\d{2})`)

func rowHasDateOrAmount(row []pdfText) bool {
	for _, t := range row {
		if reDateRun.MatchString(t.s) || reAmountRun.MatchString(t.s) {
			return true
		}
	}
	return false
}

// reAmountRun: a run that is an amount (decimal part, optional sign / currency).
var reAmountRun = regexp.MustCompile(`^[-+]?\d[\d.,' ]*[.,]\d{2}\s*(?:TL|TRY)?\s*[-+]?(?:\s*\((?:A|B|ALACAK|BORÇ|BORC)\))?$`)

// rightAligned: amounts are right-aligned under their header, so a wide
// amount can start left of its header (even under the previous column).
// An amount run goes to the header whose right edge is nearest to its own
// right edge (within 60pt); -1 when the run is not an amount or no header
// edge is near.
func rightAligned(bounds, ends []float64, t pdfText) int {
	if !reAmountRun.MatchString(t.s) || t.end <= t.x {
		return -1
	}
	best, bestD := -1, 60.0
	for i := range bounds {
		if i >= len(ends) || ends[i] <= bounds[i] || t.end < bounds[i]-12 {
			continue
		}
		if d := math.Abs(t.end - ends[i]); d <= bestD {
			best, bestD = i, d
		}
	}
	return best
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
			runs[n-1].end = math.Max(runs[n-1].end, g.X+g.W)
			lastX = g.X
			continue
		}
		runs = append(runs, pdfText{x: g.X, y: g.Y, end: g.X + g.W, s: g.S, size: g.FontSize})
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
		if r.end <= r.x {
			// Core fonts give no widths: about half an em per character.
			r.end = r.x + float64(utf8.RuneCountInString(r.s))*r.size*0.5
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

func isTemplateHeader(row []pdfText, tpls []Template) bool {
	cells := make([]string, len(row))
	for i, t := range row {
		cells[i] = t.s
	}
	_, m := matchTemplate(cells, "pdf", tpls)
	return m != nil
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
	return ParsePDFWith(data, nil)
}

// ParsePDFWith is ParsePDF with the office's templates.
func ParsePDFWith(data []byte, tpls []Template) (Statement, error) {
	rows, err := ReadPDFRowsWith(data, tpls)
	if err != nil {
		return Statement{}, err
	}
	st, err := ParseTabularWith(rows, "pdf", tpls)
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
