// Package parse turns an uploaded spreadsheet into a grid, finds the header
// row and the data block, and converts cell values (04-excel-ice-aktarma.md
// §2, §4.1, §4.4). Everything here is deterministic: no AI.
package parse

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"
)

// Limits (04 §2).
const (
	MaxBytes = 20 << 20
	MaxRows  = 100_000
)

// Errors.
var (
	ErrUnsupported = errors.New("parse: unsupported file type (xlsx or csv)")
	ErrTooLarge    = errors.New("parse: file larger than 20 MB")
	ErrTooManyRows = errors.New("parse: more than 100000 rows")
	ErrEmpty       = errors.New("parse: file has no data")
)

// Format of an upload.
type Format string

const (
	FormatXLSX Format = "xlsx"
	FormatCSV  Format = "csv"
)

// Sheet is one worksheet as a grid of trimmed cell texts.
type Sheet struct {
	Name string     `json:"name"`
	Rows [][]string `json:"-"`
}

// DetectFormat sniffs the content (xlsx is a ZIP) and falls back to the name.
func DetectFormat(name string, data []byte) (Format, error) {
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return FormatXLSX, nil
	}
	if bytes.HasPrefix(data, []byte{0xD0, 0xCF, 0x11, 0xE0}) {
		// Legacy BIFF .xls: excelize cannot read it.
		return "", fmt.Errorf("%w: legacy .xls, save as .xlsx", ErrUnsupported)
	}
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".csv") || strings.HasSuffix(lower, ".txt") || looksLikeText(data) {
		return FormatCSV, nil
	}
	return "", ErrUnsupported
}

// Read parses the file into sheets.
func Read(name string, data []byte) (Format, []Sheet, error) {
	if len(data) == 0 {
		return "", nil, ErrEmpty
	}
	if len(data) > MaxBytes {
		return "", nil, ErrTooLarge
	}
	f, err := DetectFormat(name, data)
	if err != nil {
		return "", nil, err
	}
	var sheets []Sheet
	switch f {
	case FormatXLSX:
		sheets, err = readXLSX(data)
	case FormatCSV:
		sheets, err = readCSV(data)
	}
	if err != nil {
		return "", nil, err
	}
	total := 0
	for _, s := range sheets {
		total += len(s.Rows)
	}
	if total == 0 {
		return "", nil, ErrEmpty
	}
	if total > MaxRows {
		return "", nil, ErrTooManyRows
	}
	return f, sheets, nil
}

func readXLSX(data []byte) ([]Sheet, error) {
	book, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		// Not a workbook (a zip of something else, a damaged file): the
		// caller's input, not a server fault (#453).
		return nil, fmt.Errorf("%w: xlsx: %v", ErrUnsupported, err)
	}
	defer func() { _ = book.Close() }()
	var out []Sheet
	for _, name := range book.GetSheetList() {
		// Raw values: dates stay Excel serials, numbers keep their digits.
		rows, err := book.GetRows(name, excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, fmt.Errorf("%w: sheet %q: %v", ErrUnsupported, name, err)
		}
		if len(rows) > MaxRows {
			return nil, ErrTooManyRows
		}
		out = append(out, Sheet{Name: name, Rows: trimGrid(rows)})
	}
	return out, nil
}

func readCSV(data []byte) ([]Sheet, error) {
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	if !utf8.Valid(data) {
		dec, err := charmap.Windows1254.NewDecoder().Bytes(data)
		if err != nil {
			return nil, fmt.Errorf("parse: csv encoding: %w", err)
		}
		data = dec
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = sniffDelimiter(data)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var rows [][]string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse: csv: %w", err)
		}
		rows = append(rows, rec)
		if len(rows) > MaxRows {
			return nil, ErrTooManyRows
		}
	}
	return []Sheet{{Name: "csv", Rows: trimGrid(rows)}}, nil
}

// sniffDelimiter picks ';' or ',' (or tab) by counting in the first lines.
func sniffDelimiter(data []byte) rune {
	head := string(data)
	if len(head) > 8192 {
		head = head[:8192]
	}
	best, bestN := ';', -1
	for _, c := range []rune{';', ',', '\t'} {
		if n := strings.Count(head, string(c)); n > bestN {
			best, bestN = c, n
		}
	}
	return best
}

func looksLikeText(data []byte) bool {
	n := len(data)
	if n > 4096 {
		n = 4096
	}
	return bytes.IndexByte(data[:n], 0) < 0
}

func trimGrid(rows [][]string) [][]string {
	out := make([][]string, len(rows))
	for i, r := range rows {
		row := make([]string, len(r))
		for j, c := range r {
			row[j] = strings.TrimSpace(strings.ReplaceAll(c, " ", " "))
		}
		out[i] = row
	}
	return out
}
