package statement

import (
	"fmt"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// Tablo önizleme ve düzeltmeler: Excel, CSV ve metin PDF önce aynı hücre
// tablosuna çevrilir. Müşavir tabloyu Excel gibi görür, kolon eşlemesini
// değiştirir, gereksiz satırları dışarıda bırakır, hücre düzeltir; içe
// aktarma bu düzeltilmiş tabloyu okur. Seçilen eşleme ofis şablonu olarak
// kaydedilir ve aynı düzende bir dahaki dosyada kendiliğinden gelir.

// MaxPreviewRows: önizlemede gösterilen en çok satır (başlığın altı).
const MaxPreviewRows = 2000

// Row kinds of the preview.
const (
	RowKindMovement = "movement" // a movement the import reads
	RowKindError    = "error"    // not readable with the mapping (fix a cell or leave it out)
	RowKindSkip     = "skip"     // after the totals line; the import stops there
)

// PreviewRow is one table row below the header.
type PreviewRow struct {
	// Row: 1-based row of the file table (edits and skips name it).
	Row int `json:"row"`
	// Cells: one per Analysis.Columns entry, in that order.
	Cells  []string `json:"cells"`
	Kind   string   `json:"kind"`
	Reason string   `json:"reason,omitempty"`
}

// CellEdit replaces one cell. Column is the file column
// (ColumnSuggestion.Index). Header and rows above it cannot be edited.
type CellEdit struct {
	Row    int    `json:"row"`
	Column int    `json:"column"`
	Value  string `json:"value"`
}

// Overrides is what the accountant decided on the preview.
type Overrides struct {
	// Columns: header → field, in header order (as a template's columns).
	Columns  []TemplateColumn `json:"columns"`
	SkipRows []int            `json:"skip_rows,omitempty"`
	Edits    []CellEdit       `json:"edits,omitempty"`
}

// ErrInvalidOverrides: a mapping that is not a valid template, or an edit
// outside the movement rows.
var ErrInvalidOverrides = fmt.Errorf("%w: invalid preview overrides", ErrInvalidTemplate)

// readTable turns a tabular statement file into its cell table.
func readTable(format string, data []byte, tpls []Template) ([][]string, error) {
	switch format {
	case "pdf":
		return ReadPDFRowsWith(data, tpls)
	case "excel":
		switch {
		case isBIFF(data):
			return ReadBIFF8(data)
		case isHTMLTable(data):
			return ReadHTMLTable(data)
		case isSpreadsheetML(data):
			return ReadSpreadsheetML(data)
		}
		return ReadXLSX(data)
	case "csv":
		text, _ := Decode(data)
		rows, err := ReadCSV(text)
		if err != nil {
			return nil, fmt.Errorf("csv: %w", err)
		}
		return rows, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrUnsupported, format)
}

// statementFromRows reads the movements of a cell table (PDF keeps its
// capture method and running-balance checks).
func statementFromRows(rows [][]string, format string, tpls []Template) (Statement, error) {
	st, err := ParseTabularWith(rows, format, tpls)
	if err != nil || format != "pdf" {
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

// template is the mapping as a one-off template for the file's format.
func (o Overrides) template(format string) (Template, error) {
	t := Template{Format: format, Columns: o.Columns, Source: SourceOffice}
	if err := t.Validate(); err != nil {
		return t, fmt.Errorf("%w: %v", ErrInvalidOverrides, err)
	}
	return t, nil
}

// apply returns a copy of rows with the edits made and the left-out rows
// emptied (an empty row is not read; row numbers stay as in the file).
func (o Overrides) apply(rows [][]string, hdr int) ([][]string, error) {
	out := make([][]string, len(rows))
	for i, r := range rows {
		out[i] = append([]string(nil), r...)
	}
	for _, e := range o.Edits {
		i := e.Row - 1
		if i <= hdr || i >= len(out) || e.Column < 0 || e.Column > 200 {
			return nil, fmt.Errorf("%w: cell %d:%d is not in the movement rows", ErrInvalidOverrides, e.Row, e.Column)
		}
		for len(out[i]) <= e.Column {
			out[i] = append(out[i], "")
		}
		out[i][e.Column] = strings.TrimSpace(e.Value)
	}
	for _, r := range o.SkipRows {
		i := r - 1
		if i <= hdr || i >= len(out) {
			return nil, fmt.Errorf("%w: row %d is not a movement row", ErrInvalidOverrides, r)
		}
		for j := range out[i] {
			out[i][j] = ""
		}
	}
	return out, nil
}

// parseWithOverrides reads a tabular file with the accountant's mapping,
// edits and left-out rows.
func parseWithOverrides(format string, data []byte, tpls []Template, o Overrides) (Statement, error) {
	t, err := o.template(format)
	if err != nil {
		return Statement{}, err
	}
	rows, err := readTable(format, data, append([]Template{t}, tpls...))
	if err != nil {
		return Statement{}, err
	}
	hdr, _, _ := findHeader(rows, format, []Template{t})
	if hdr < 0 {
		return Statement{}, fmt.Errorf("%w (%s: the mapped header row is not in the file)", ErrLayoutUnknown, format)
	}
	rows, err = o.apply(rows, hdr)
	if err != nil {
		return Statement{}, err
	}
	return statementFromRows(rows, format, []Template{t})
}

// previewRows classifies the rows below the header with the proposed
// mapping, as the import would read them.
func previewRows(rows [][]string, hdr int, format string, cs []ColumnSuggestion, t Template) []PreviewRow {
	cols, _ := matchTemplate(rows[hdr], format, []Template{t})
	out := []PreviewRow{}
	ended := false
	for r := hdr + 1; r < len(rows) && len(out) < MaxPreviewRows; r++ {
		row := rows[r]
		if blank(row) {
			continue
		}
		p := PreviewRow{Row: r + 1, Cells: make([]string, len(cs)), Kind: RowKindMovement}
		for k, c := range cs {
			if c.Index < len(row) {
				p.Cells[k] = strings.TrimSpace(row[c.Index])
			}
		}
		switch {
		case ended:
			p.Kind = RowKindSkip
		case cols == nil:
			p.Kind = ""
		default:
			if _, err := readRow(row, cols); err != nil {
				if isFooter(row) {
					ended = true
					p.Kind, p.Reason = RowKindSkip, "toplam / kapanış satırı; okuma burada biter"
				} else {
					p.Kind, p.Reason = RowKindError, err.Error()
				}
			}
		}
		out = append(out, p)
	}
	return out
}
