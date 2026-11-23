package statement

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
)

// ErrUnsupported: the file is none of MT940/MT942, Excel (.xlsx), CSV or PDF.
var ErrUnsupported = errors.New("statement: unsupported file")

// Detect names the format of a statement file.
func Detect(name string, data []byte) string {
	lower := strings.ToLower(name)
	switch {
	case bytes.HasPrefix(data, []byte("%PDF")):
		return "pdf"
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		return "excel"
	case isBIFF(data):
		// Binary Excel 97-2003 (.xls): read like .xlsx.
		return "excel"
	case isHTMLTable(data) || isSpreadsheetML(data):
		// The ".xls" most internet branches save: an HTML table or an
		// Excel 2003 XML workbook (#780). Read like .xlsx.
		return "excel"
	}
	text, _ := Decode(head(data, 8192))
	if strings.Contains(text, ":20:") && (strings.Contains(text, ":25:") || strings.Contains(text, ":61:")) {
		return "mt940"
	}
	if strings.HasSuffix(lower, ".sta") || strings.HasSuffix(lower, ".mt940") || strings.HasSuffix(lower, ".940") {
		return "mt940"
	}
	return "csv"
}

func head(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}

// Parse reads any supported statement file into statements (one per account
// block) and returns the detected format and text encoding.
func Parse(name string, data []byte) ([]Statement, string, error) {
	return ParseWith(name, data, nil)
}

// ParseWith is Parse with the office's saved templates (#383).
func ParseWith(name string, data []byte, tpls []Template) ([]Statement, string, error) {
	format := Detect(name, data)
	switch format {
	case "mt940":
		text, _ := Decode(data)
		sts, err := ParseMT940(text)
		return sts, format, err
	case "pdf", "excel", "csv":
		rows, err := readTable(format, data, tpls)
		if err != nil {
			return nil, format, err
		}
		st, err := statementFromRows(rows, format, tpls)
		if err != nil {
			return nil, format, err
		}
		return []Statement{st}, format, nil
	}
	return nil, format, fmt.Errorf("%w: %s (save it as .xlsx or CSV)", ErrUnsupported, format)
}
