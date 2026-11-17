// Package target is the contract between the common Excel import flow and
// the modules that own non-invoice records (#395): payroll, expenses, Z
// reports and bank movements. The flow uploads, maps (template fingerprint,
// header heuristic, AI), validates and previews; a Target writes the
// accepted rows with the module's own rules (duplicates, continuity, ...).
package target

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// ErrRejected: the module refused the import as a whole (missing period,
// unknown bank account ...). The message is shown to the accountant.
var ErrRejected = errors.New("target: import rejected")

// Rejectf builds an ErrRejected with a Turkish message.
func Rejectf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrRejected, fmt.Sprintf(format, args...))
}

// Row is one accepted row: field -> normalised value (dates YYYY-MM-DD,
// amounts as signed decimals, the rest trimmed text).
type Row struct {
	RowNo  int
	Values map[string]string
}

// Input is what a target writes.
type Input struct {
	Scope        engagementctx.Scope
	UserID       int64
	FileName     string
	Period       string
	SourceSystem string
	Rows         []Row
}

// RowError is a row the module did not write.
type RowError struct {
	RowNo  int    `json:"row_no"`
	Reason string `json:"reason"`
}

// Result is what the module wrote.
type Result struct {
	Added      int
	Duplicates int
	Errors     []RowError
	Note       string
}

// Target writes one data type.
type Target interface {
	// Heuristic maps the columns from their headers alone (no AI). Columns
	// it does not know stay unmapped.
	Heuristic(l parse.Layout) mapping.Mapping
	Commit(ctx context.Context, in Input) (Result, error)
}

// Column is one column of a canonical sheet.
type Column struct {
	Field  string
	Header string
}

// CSV writes rows as a ';' separated sheet with the given headers, after the
// optional preamble lines. lines maps a 1-based sheet line to its RowNo.
func CSV(preamble [][]string, cols []Column, rows []Row) (data []byte, lines map[int]int, err error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	for _, p := range preamble {
		if err := w.Write(p); err != nil {
			return nil, nil, err
		}
	}
	hdr := make([]string, len(cols))
	for i, c := range cols {
		hdr[i] = c.Header
	}
	if err := w.Write(hdr); err != nil {
		return nil, nil, err
	}
	lines = map[int]int{}
	line := len(preamble) + 1
	for _, r := range rows {
		rec := make([]string, len(cols))
		for i, c := range cols {
			rec[i] = r.Values[c.Field]
		}
		if err := w.Write(rec); err != nil {
			return nil, nil, err
		}
		line++
		lines[line] = r.RowNo
	}
	w.Flush()
	return buf.Bytes(), lines, w.Error()
}

// CSVName is the file name a module sees for the canonical sheet.
func CSVName(original string) string {
	base := path.Base(strings.ReplaceAll(original, "\\", "/"))
	if ext := path.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	if base == "" || base == "." || base == "/" {
		base = "excel-ice-aktarma"
	}
	return base + ".csv"
}

// MatchHeaders maps the layout's columns with match (header -> field, ""
// when unknown); a field goes to its first column only.
func MatchHeaders(dataType string, l parse.Layout, match func(header string) string) mapping.Mapping {
	mp := mapping.Mapping{DataType: dataType, SourceSystem: "unknown", HeaderRow: l.HeaderRow}
	used := map[string]bool{}
	for _, c := range l.Columns {
		cm := mapping.ColumnMap{Column: c.Letter, Header: c.Header, Origin: mapping.OriginHeader}
		if f := match(c.Header); f != "" && !used[f] {
			used[f] = true
			cm.Field, cm.Confidence = f, 0.9
		}
		mp.Columns = append(mp.Columns, cm)
	}
	return mp
}
