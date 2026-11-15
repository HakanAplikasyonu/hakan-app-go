package parse

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

// Layout is the detected structure of one sheet (04 §4.1).
type Layout struct {
	Sheet string `json:"sheet"`
	// HeaderRow is 1-based; for a two-row header it is the lower row.
	HeaderRow int `json:"header_row"`
	// Columns are the merged header texts, indexed like the grid columns.
	Columns []Column `json:"columns"`
	// FirstDataRow and LastDataRow are 1-based, inclusive.
	FirstDataRow int    `json:"first_data_row"`
	LastDataRow  int    `json:"last_data_row"`
	Fingerprint  string `json:"fingerprint"`
}

// Column is one header cell.
type Column struct {
	Index  int    `json:"index"`
	Letter string `json:"column"`
	Header string `json:"header"`
}

const scanRows = 50

// DetectLayout finds the header row, merges a two-row header and finds the
// data block. ok=false when no plausible header is found.
func DetectLayout(s Sheet) (Layout, bool) {
	n := len(s.Rows)
	if n > scanRows {
		n = scanRows
	}
	best, bestScore := -1, 0
	for i := 0; i < n; i++ {
		filled, text := countCells(s.Rows[i])
		if filled < 2 || text*2 < filled {
			continue
		}
		// The rows below must carry dates or numbers.
		if !hasValuesBelow(s.Rows, i) {
			continue
		}
		score := filled*10 + text
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 {
		return Layout{}, false
	}
	header := append([]string(nil), s.Rows[best]...)
	headerRow := best
	// Two-row header: the row below is mostly short text like "%1 / %10".
	if best+1 < len(s.Rows) && isSubHeader(s.Rows[best+1]) {
		sub := s.Rows[best+1]
		parent := ""
		width := len(header)
		if len(sub) > width {
			width = len(sub)
		}
		merged := make([]string, width)
		for j := 0; j < width; j++ {
			top := cell(header, j)
			if top != "" {
				parent = top
			}
			bottom := cell(sub, j)
			switch {
			case bottom == "":
				merged[j] = top
			case top == "" && parent != "":
				merged[j] = parent + " " + bottom
			case top != "":
				merged[j] = top + " " + bottom
			default:
				merged[j] = bottom
			}
		}
		header, headerRow = merged, best+1
	}
	l := Layout{Sheet: s.Name, HeaderRow: headerRow + 1, FirstDataRow: headerRow + 2}
	for j, h := range header {
		if strings.TrimSpace(h) == "" {
			continue
		}
		l.Columns = append(l.Columns, Column{Index: j, Letter: ColumnLetter(j), Header: collapse(h)})
	}
	l.LastDataRow = headerRow + 1
	for i := headerRow + 1; i < len(s.Rows); i++ {
		if isEmpty(s.Rows[i]) || IsTotalRow(s.Rows[i]) {
			break
		}
		l.LastDataRow = i + 1
	}
	l.Fingerprint = Fingerprint(l.Columns)
	return l, true
}

// Fingerprint hashes the ordered, normalized header names (04 §4.2).
func Fingerprint(cols []Column) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = Normalize(c.Header)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// Normalize lower-cases, folds Turkish letters and drops punctuation.
func Normalize(s string) string {
	s = strings.NewReplacer("İ", "i", "I", "ı").Replace(s)
	s = strings.ToLower(s)
	s = strings.NewReplacer("ı", "i", "ğ", "g", "ü", "u", "ş", "s", "ö", "o", "ç", "c").Replace(s)
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '%':
			b.WriteRune(r)
			space = false
		default:
			if !space && b.Len() > 0 {
				b.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

var totalWords = []string{"genel toplam", "ara toplam", "toplam", "total", "grand total", "subtotal"}

// IsTotalRow reports a "Toplam" style summary row (04 §2): a total word in
// one of the first text cells.
func IsTotalRow(row []string) bool {
	seen := 0
	for _, c := range row {
		if c == "" {
			continue
		}
		seen++
		if seen > 3 {
			break
		}
		n := Normalize(c)
		for _, w := range totalWords {
			if n == w || strings.HasPrefix(n, w+" ") {
				return true
			}
		}
	}
	return false
}

// ColumnLetter turns a 0-based index into A, B, ..., Z, AA ...
func ColumnLetter(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('A'+(i-1)%26)) + s
	}
	return s
}

func countCells(row []string) (filled, text int) {
	for _, c := range row {
		if c == "" {
			continue
		}
		filled++
		if !looksNumericOrDate(c) {
			text++
		}
	}
	return
}

func hasValuesBelow(rows [][]string, i int) bool {
	for k := i + 1; k < len(rows) && k <= i+3; k++ {
		for _, c := range rows[k] {
			if c != "" && looksNumericOrDate(c) {
				return true
			}
		}
	}
	return false
}

func isSubHeader(row []string) bool {
	filled, short := 0, 0
	for _, c := range row {
		if c == "" {
			continue
		}
		filled++
		if len([]rune(c)) <= 8 && (strings.Contains(c, "%") || !looksNumericOrDate(c)) {
			short++
		}
	}
	return filled >= 2 && short == filled && hasPercent(row)
}

func hasPercent(row []string) bool {
	for _, c := range row {
		if strings.Contains(c, "%") {
			return true
		}
	}
	return false
}

func looksNumericOrDate(s string) bool {
	if _, ok := ParseDate(s); ok {
		return true
	}
	_, ok := parseNumberAuto(s)
	return ok
}

func isEmpty(row []string) bool {
	for _, c := range row {
		if c != "" {
			return false
		}
	}
	return true
}

func cell(row []string, j int) string {
	if j < len(row) {
		return row[j]
	}
	return ""
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }
