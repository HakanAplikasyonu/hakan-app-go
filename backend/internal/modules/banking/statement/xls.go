package statement

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// ".xls" statements (#780). Most Turkish internet branches' "Excel'e aktar"
// button saves an .xls that is not a binary Excel workbook: either an HTML
// table (Ziraat, Halkbank, Yapı Kredi and others) or an Excel 2003 XML
// workbook (SpreadsheetML). Both are read here into the same cell table as
// .xlsx and CSV. A real binary Excel 97-2003 file is refused with
// ErrLegacyXLS: it has to be saved as .xlsx or downloaded as CSV.

// ErrLegacyXLS: a binary Excel 97-2003 workbook.
var ErrLegacyXLS = fmt.Errorf("%w: binary Excel 97-2003 (.xls); save it as .xlsx or download CSV", ErrUnsupported)

// maxXLSCells bounds a hostile file (a statement has a few thousand cells).
const maxXLSCells = 500_000

// isHTMLTable: text that starts like markup and holds a <table>.
func isHTMLTable(data []byte) bool {
	text, _ := Decode(head(data, 8192))
	t := strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(t, "<") && strings.Contains(t, "<table") && !isSpreadsheetMLText(t)
}

// isSpreadsheetML: an Excel 2003 XML workbook.
func isSpreadsheetML(data []byte) bool {
	text, _ := Decode(head(data, 8192))
	return isSpreadsheetMLText(strings.ToLower(text))
}

func isSpreadsheetMLText(lower string) bool {
	return strings.Contains(lower, "urn:schemas-microsoft-com:office:spreadsheet") && strings.Contains(lower, "<workbook")
}

// ReadHTMLTable returns the rows of the HTML table with the most rows
// (banks put a small title table above the movements; the title lines are
// kept by reading every table's rows in document order, so the IBAN above
// the movement table is still found).
func ReadHTMLTable(data []byte) ([][]string, error) {
	text, _ := Decode(data)
	z := html.NewTokenizer(strings.NewReader(text))
	var rows [][]string
	var row []string
	var cell *strings.Builder
	cells := 0
	endCell := func() {
		if cell != nil {
			row = append(row, strings.Join(strings.Fields(cell.String()), " "))
			cell = nil
			cells++
		}
	}
	endRow := func() {
		endCell()
		if row != nil {
			rows = append(rows, row)
			row = nil
		}
	}
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if errors.Is(z.Err(), io.EOF) {
				endRow()
				if len(rows) == 0 {
					return nil, fmt.Errorf("html: no table rows")
				}
				return rows, nil
			}
			return nil, fmt.Errorf("html: %w", z.Err())
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch string(name) {
			case "tr":
				endRow()
				row = []string{}
			case "td", "th":
				endCell()
				if row == nil {
					row = []string{}
				}
				cell = &strings.Builder{}
				// colspan: keep the following columns aligned.
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					if string(k) == "colspan" {
						if n, err := strconv.Atoi(string(v)); err == nil && n > 1 && n < 50 {
							for i := 1; i < n; i++ {
								row = append(row, "")
							}
						}
					}
				}
			case "br":
				if cell != nil {
					cell.WriteByte(' ')
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "td", "th":
				endCell()
			case "tr", "table":
				endRow()
			}
		case html.TextToken:
			if cell != nil {
				cell.Write(z.Text())
			}
		}
		if cells > maxXLSCells {
			return nil, fmt.Errorf("html: too many cells")
		}
	}
}

// ReadSpreadsheetML returns the first worksheet of an Excel 2003 XML
// workbook. ss:Index on a cell skips to that 1-based column.
func ReadSpreadsheetML(data []byte) ([][]string, error) {
	text, _ := Decode(data)
	// The decoder must not re-decode: the text is UTF-8 now.
	d := xml.NewDecoder(strings.NewReader(text))
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	d.Strict = false
	var rows [][]string
	var row []string
	inRow, inData, sheets, cells := false, false, 0, 0
	var buf bytes.Buffer
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xml: %w", err)
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "Worksheet":
				sheets++
			case "Row":
				if sheets == 1 {
					inRow, row = true, []string{}
				}
			case "Cell":
				if !inRow {
					continue
				}
				for _, a := range e.Attr {
					if a.Name.Local == "Index" {
						if n, err := strconv.Atoi(a.Value); err == nil && n > len(row)+1 && n < 1000 {
							for len(row) < n-1 {
								row = append(row, "")
							}
						}
					}
				}
				row = append(row, "")
				cells++
			case "Data":
				if inRow && len(row) > 0 {
					inData = true
					buf.Reset()
				}
			}
		case xml.CharData:
			if inData {
				buf.Write(e)
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "Data":
				if inData {
					row[len(row)-1] += strings.TrimSpace(buf.String())
					inData = false
				}
			case "Row":
				if inRow {
					rows = append(rows, row)
					inRow = false
				}
			case "Worksheet":
				if sheets == 1 {
					if len(rows) == 0 {
						return nil, fmt.Errorf("xml: empty worksheet")
					}
					return rows, nil
				}
			}
		}
		if cells > maxXLSCells {
			return nil, fmt.Errorf("xml: too many cells")
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("xml: no worksheet rows")
	}
	return rows, nil
}
