package accountingexport

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"gopkg.in/yaml.v3"
)

// CSVTemplate is a delimited text layout of a program's import screen
// (Luca "Hesap Planı Aktar", "Banka Ekstreleri"), loaded from versioned YAML
// like Template. Column names, order, delimiter, quoting, encoding and line
// endings are data: when the real file is in hand only the YAML changes.
type CSVTemplate struct {
	Driver   string    `yaml:"driver"`
	Name     string    `yaml:"template"`
	Version  string    `yaml:"version"`
	Verified bool      `yaml:"verified"`
	CSV      CSVFormat `yaml:"csv"`
	Columns  []Column  `yaml:"columns"`
	Notes    string    `yaml:"notes"`
	// Options are template specific settings (bank statement amount
	// layout, …); the driver reads the keys it knows.
	Options map[string]string `yaml:"options"`
}

// CSVFormat is the file container of a CSVTemplate.
type CSVFormat struct {
	// Delimiter between fields (";" when empty).
	Delimiter string `yaml:"delimiter"`
	// Quote is the quote character; "none" disables quoting.
	Quote string `yaml:"quote"`
	// QuoteMode: minimal (only fields that need it), all, none.
	QuoteMode string `yaml:"quote_mode"`
	// Encoding: windows-1254 (Türkçe Windows; default) or utf-8.
	Encoding string `yaml:"encoding"`
	// BOM writes the UTF-8 byte order mark (utf-8 only).
	BOM bool `yaml:"bom"`
	// LineEnding: crlf (default) or lf.
	LineEnding string `yaml:"line_ending"`
	// Header writes the header row (default true).
	Header *bool `yaml:"header"`
	// HeaderLineEnd: the header row ends with a line ending when it is the
	// last line of the file (no data rows). FinalLineEnd: the last data row
	// ends with one. Rows in between are always separated.
	HeaderLineEnd bool `yaml:"header_line_end"`
	FinalLineEnd  bool `yaml:"final_line_end"`
	// TrailingDelimiter writes a delimiter after the last field of a row.
	TrailingDelimiter bool `yaml:"trailing_delimiter"`
}

// Encodings a CSV template may use.
const (
	EncodingWindows1254 = "windows-1254"
	EncodingUTF8        = "utf-8"
)

// ErrMissingField is returned when a required column is empty.
var ErrMissingField = errors.New("accountingexport: required field is empty")

// FieldError names the row (1-based, data rows) and column of a bad value.
type FieldError struct {
	Row    int
	Column string
	Err    error
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("row %d, %s: %v", e.Row, e.Column, e.Err)
}

func (e *FieldError) Unwrap() error { return e.Err }

// LoadCSVTemplate reads and checks one CSV template file.
func LoadCSVTemplate(fsys fs.FS, path string) (CSVTemplate, error) {
	raw, err := fs.ReadFile(fsys, path)
	if err != nil {
		return CSVTemplate{}, fmt.Errorf("%w: %v", ErrTemplate, err)
	}
	var t CSVTemplate
	dec := yaml.NewDecoder(bytesReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&t); err != nil {
		return CSVTemplate{}, fmt.Errorf("%w: %s: %v", ErrTemplate, path, err)
	}
	if err := t.check(); err != nil {
		return CSVTemplate{}, fmt.Errorf("%w: %s: %v", ErrTemplate, path, err)
	}
	return t, nil
}

func (t CSVTemplate) check() error {
	if t.Driver == "" || t.Name == "" || t.Version == "" {
		return errors.New("driver, template and version are required")
	}
	if len(t.Columns) == 0 {
		return errors.New("at least one column is required")
	}
	seen := map[string]bool{}
	for _, c := range t.Columns {
		if c.Key == "" || c.Header == "" {
			return errors.New("every column needs key and header")
		}
		if seen[c.Key] {
			return fmt.Errorf("duplicate column key %q", c.Key)
		}
		seen[c.Key] = true
	}
	f := t.CSV
	if utf8.RuneCountInString(f.delimiter()) != 1 {
		return fmt.Errorf("csv.delimiter %q must be one character", f.Delimiter)
	}
	if q := f.quote(); q != "" && utf8.RuneCountInString(q) != 1 {
		return fmt.Errorf("csv.quote %q must be one character or none", f.Quote)
	}
	switch f.QuoteMode {
	case "", "minimal", "all", "none":
	default:
		return fmt.Errorf("csv.quote_mode %q (minimal, all, none)", f.QuoteMode)
	}
	switch f.encoding() {
	case EncodingWindows1254, EncodingUTF8:
	default:
		return fmt.Errorf("csv.encoding %q (windows-1254, utf-8)", f.Encoding)
	}
	switch f.LineEnding {
	case "", "crlf", "lf":
	default:
		return fmt.Errorf("csv.line_ending %q (crlf, lf)", f.LineEnding)
	}
	return nil
}

func (f CSVFormat) delimiter() string {
	if f.Delimiter == "" {
		return ";"
	}
	return f.Delimiter
}

func (f CSVFormat) quote() string {
	switch f.Quote {
	case "":
		return `"`
	case "none":
		return ""
	}
	return f.Quote
}

func (f CSVFormat) encoding() string {
	if f.Encoding == "" {
		return EncodingWindows1254
	}
	return strings.ToLower(f.Encoding)
}

func (f CSVFormat) lineEnding() string {
	if f.LineEnding == "lf" {
		return "\n"
	}
	return "\r\n"
}

func (f CSVFormat) header() bool { return f.Header == nil || *f.Header }

// Headers returns the column headers in file order.
func (t CSVTemplate) Headers() []string {
	out := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		out[i] = c.Header
	}
	return out
}

// CSVResult is a written file.
type CSVResult struct {
	Data []byte
	Rows int
	// Replaced counts characters the encoding cannot hold, written as "?".
	Replaced int
}

// WriteCSV writes rows (column key → value) in the template's layout. A
// required column with an empty value is a *FieldError (ErrMissingField).
func (t CSVTemplate) WriteCSV(rows []map[string]string) (CSVResult, error) {
	f := t.CSV
	delim, quote, eol := f.delimiter(), f.quote(), f.lineEnding()
	var lines []string
	if f.header() {
		lines = append(lines, t.line(t.Headers(), delim, quote))
	}
	for i, r := range rows {
		vals := make([]string, len(t.Columns))
		for j, c := range t.Columns {
			v := strings.TrimSpace(r[c.Key])
			if c.Required && v == "" {
				return CSVResult{}, &FieldError{Row: i + 1, Column: c.Header, Err: ErrMissingField}
			}
			vals[j] = v
		}
		lines = append(lines, t.line(vals, delim, quote))
	}
	var sb strings.Builder
	for i, l := range lines {
		sb.WriteString(l)
		last := i == len(lines)-1
		switch {
		case !last:
			sb.WriteString(eol)
		case len(rows) == 0 && f.HeaderLineEnd:
			sb.WriteString(eol)
		case len(rows) > 0 && f.FinalLineEnd:
			sb.WriteString(eol)
		}
	}
	data, replaced, err := encode(sb.String(), f.encoding())
	if err != nil {
		return CSVResult{}, err
	}
	if f.encoding() == EncodingUTF8 && f.BOM {
		data = append([]byte{0xEF, 0xBB, 0xBF}, data...)
	}
	return CSVResult{Data: data, Rows: len(rows), Replaced: replaced}, nil
}

func (t CSVTemplate) line(vals []string, delim, quote string) string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = t.field(v, delim, quote)
	}
	s := strings.Join(out, delim)
	if t.CSV.TrailingDelimiter {
		s += delim
	}
	return s
}

func (t CSVTemplate) field(v, delim, quote string) string {
	mode := t.CSV.QuoteMode
	if quote == "" || mode == "none" {
		// No quoting: a delimiter or line break in the value would break
		// the row, so they become spaces.
		return strings.NewReplacer(delim, " ", "\r", " ", "\n", " ").Replace(v)
	}
	need := mode == "all" || strings.Contains(v, delim) || strings.Contains(v, quote) ||
		strings.ContainsAny(v, "\r\n")
	if !need {
		return v
	}
	return quote + strings.ReplaceAll(v, quote, quote+quote) + quote
}

// encode converts s to the file encoding. Characters windows-1254 cannot
// hold become "?" and are counted.
func encode(s, enc string) ([]byte, int, error) {
	if enc == EncodingUTF8 {
		return []byte(s), 0, nil
	}
	e := charmap.Windows1254
	var buf bytes.Buffer
	replaced := 0
	for _, r := range s {
		b, ok := e.EncodeRune(r)
		if !ok {
			buf.WriteByte('?')
			replaced++
			continue
		}
		buf.WriteByte(b)
	}
	return buf.Bytes(), replaced, nil
}
