package usecase

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"
)

// Source kinds.
const (
	KindAccountPlan  = "account_plan"  // Luca hesap planı dökümü
	KindTrialBalance = "trial_balance" // mizan dökümü (borç / alacak / bakiye sütunlu)
)

// Formats.
const (
	FormatXLSX = "xlsx"
	FormatCSV  = "csv"
)

// Account is one parsed account.
type Account struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	MainCode string `json:"main_code"`
	Level    int    `json:"level"`
	IsLeaf   bool   `json:"is_leaf"`
}

// SkippedRow is a data row that carried no usable account code.
type SkippedRow struct {
	Row    int    `json:"row"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

// Parsed is a parsed account plan / trial balance file.
type Parsed struct {
	Format     string
	Kind       string
	HeaderRow  int // 1-based; 0 when no header was found (first two columns used)
	Accounts   []Account
	Skipped    []SkippedRow // first maxSkippedSamples rows only
	SkipCount  int
	Duplicates int
}

const maxSkippedSamples = 20

var (
	codeRe = regexp.MustCompile(`^[0-9]{3}(\.[0-9A-Za-z]+)*$`)
	sepRe  = regexp.MustCompile(`[\s\-_/,]+`)
	dotsRe = regexp.MustCompile(`\.{2,}`)
)

// NormalizeCode turns the spellings seen in Luca / Excel dumps
// ("120 01 001", "120-01-001", "120.01.001.") into the dotted form. It does
// not pad or strip segments: "120.1" and "120.01" stay different codes.
func NormalizeCode(s string) string {
	s = strings.TrimSpace(s)
	s = sepRe.ReplaceAllString(s, ".")
	s = dotsRe.ReplaceAllString(s, ".")
	return strings.Trim(s, ".")
}

// ValidCode reports whether a normalized code looks like a Tek Düzen hesap
// kodu: three-digit main account plus optional dotted sub-segments.
func ValidCode(code string) bool { return len(code) <= 64 && codeRe.MatchString(code) }

// Parse reads an account plan or trial balance export (.xlsx or .csv).
func Parse(fileName string, data []byte) (Parsed, error) {
	var (
		rows [][]string
		p    Parsed
		err  error
	)
	switch {
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		p.Format = FormatXLSX
		rows, err = readXLSX(data)
	case looksLikeOLE(data):
		return p, fmt.Errorf("%w: legacy .xls is not supported, save the file as .xlsx or .csv", ErrUnsupported)
	default:
		if strings.HasSuffix(strings.ToLower(fileName), ".xlsx") {
			return p, fmt.Errorf("%w: file is not a valid .xlsx", ErrUnsupported)
		}
		p.Format = FormatCSV
		rows, err = readCSV(data)
	}
	if err != nil {
		return p, err
	}
	codeCol, nameCol, header, kind := detectColumns(rows)
	p.HeaderRow, p.Kind = header, kind
	seen := map[string]bool{}
	for i := header; i < len(rows); i++ {
		row := rows[i]
		raw := cell(row, codeCol)
		if strings.TrimSpace(raw) == "" {
			continue // boş satır / ara başlık
		}
		code := NormalizeCode(raw)
		if !ValidCode(code) {
			p.skip(i+1, raw, "not an account code")
			continue
		}
		if seen[code] {
			p.Duplicates++
			continue
		}
		seen[code] = true
		p.Accounts = append(p.Accounts, Account{
			Code: code, Name: strings.TrimSpace(cell(row, nameCol)),
			MainCode: code[:3], Level: strings.Count(code, ".") + 1,
		})
	}
	markLeaves(p.Accounts)
	return p, nil
}

func (p *Parsed) skip(row int, raw, reason string) {
	p.SkipCount++
	if len(p.Skipped) < maxSkippedSamples {
		if len(raw) > 64 {
			raw = raw[:64]
		}
		p.Skipped = append(p.Skipped, SkippedRow{Row: row, Value: raw, Reason: reason})
	}
}

// markLeaves sets IsLeaf: a code with another code under it ("120" when
// "120.01" exists) is a group account. Sorting puts children right after.
func markLeaves(accs []Account) {
	sort.Slice(accs, func(i, j int) bool { return accs[i].Code < accs[j].Code })
	for i := range accs {
		accs[i].IsLeaf = true
	}
	for i := range accs {
		prefix := accs[i].Code + "."
		for j := i + 1; j < len(accs) && accs[j].Code[:3] == accs[i].Code[:3]; j++ {
			if strings.HasPrefix(accs[j].Code, prefix) {
				accs[i].IsLeaf = false
				break
			}
		}
	}
}

func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return row[i]
}

var trFold = strings.NewReplacer("ı", "i", "İ", "i", "ş", "s", "Ş", "s", "ğ", "g", "Ğ", "g",
	"ü", "u", "Ü", "u", "ö", "o", "Ö", "o", "ç", "c", "Ç", "c", " ", "", ".", "", "_", "")

func foldHeader(s string) string { return trFold.Replace(strings.ToLower(strings.TrimSpace(s))) }

var (
	codeHeaders = map[string]bool{"hesapkodu": true, "hesapno": true, "kod": true, "hesap": true, "kodu": true}
	nameHeaders = map[string]bool{"hesapadi": true, "hesapismi": true, "aciklama": true, "adi": true, "hesapaciklamasi": true, "unvan": true, "ad": true}
	tbHeaders   = []string{"borc", "alacak", "bakiye"}
)

// detectColumns finds the header row in the first 30 rows. Without a header
// the first column is the code and the second the name.
func detectColumns(rows [][]string) (codeCol, nameCol, headerRow int, kind string) {
	codeCol, nameCol, kind = 0, 1, KindAccountPlan
	for i := 0; i < len(rows) && i < 30; i++ {
		c, n := -1, -1
		tb := false
		for j, v := range rows[i] {
			f := foldHeader(v)
			switch {
			case c < 0 && codeHeaders[f]:
				c = j
			case n < 0 && nameHeaders[f]:
				n = j
			}
			for _, h := range tbHeaders {
				if strings.Contains(f, h) {
					tb = true
				}
			}
		}
		if c >= 0 {
			if n < 0 {
				n = c + 1
			}
			if tb {
				kind = KindTrialBalance
			}
			return c, n, i + 1, kind
		}
	}
	return codeCol, nameCol, 0, kind
}

func readXLSX(data []byte) ([][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	defer func() { _ = f.Close() }()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, ErrEmpty
	}
	// İlk dolu sayfa: Luca dökümleri tek sayfadır.
	for _, sh := range sheets {
		rows, err := f.GetRows(sh)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
		}
		if len(rows) > 0 {
			return rows, nil
		}
	}
	return nil, ErrEmpty
}

func readCSV(data []byte) ([][]string, error) {
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	if !utf8.Valid(data) {
		// Luca ve Excel'in Türkçe CSV'leri çoğunlukla Windows-1254.
		dec, err := charmap.Windows1254.NewDecoder().Bytes(data)
		if err != nil {
			return nil, fmt.Errorf("%w: unknown text encoding", ErrUnsupported)
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
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: csv: %v", ErrUnsupported, err)
		}
		rows = append(rows, rec)
	}
	return rows, nil
}

func sniffDelimiter(data []byte) rune {
	line := data
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		line = data[:i]
	}
	best, n := ';', -1
	for _, d := range []rune{';', '\t', ',', '|'} {
		if c := strings.Count(string(line), string(d)); c > n {
			best, n = d, c
		}
	}
	return best
}

func looksLikeOLE(data []byte) bool {
	return bytes.HasPrefix(data, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
}
