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

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
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
	// Mizan tutarları (#393), yalnızca KindTrialBalance dosyalarında dolu:
	// dönem sonu borç / alacak toplamı ve bakiye (borç bakiye − alacak
	// bakiye). Hesap planı ucu bunları göstermez.
	Debit   decimal.Decimal `json:"-"`
	Credit  decimal.Decimal `json:"-"`
	Balance decimal.Decimal `json:"-"`
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
	// Period is the trial balance period ("YYYY-MM") read from the title
	// rows above the header (the end of "01.01.2026 - 30.09.2026") or from
	// the file name; empty when unknown.
	Period string
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
	cols := detectColumns(rows)
	codeCol, nameCol, header := cols.code, cols.name, cols.header
	p.HeaderRow, p.Kind = header, cols.kind
	p.Period = detectPeriod(rows, header, fileName)
	style := cols.numberStyle(rows)
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
		acc := Account{
			Code: code, Name: strings.TrimSpace(cell(row, nameCol)),
			MainCode: code[:3], Level: strings.Count(code, ".") + 1,
		}
		if p.Kind == KindTrialBalance {
			acc.Debit, acc.Credit, acc.Balance = cols.amounts(row, style)
		}
		p.Accounts = append(p.Accounts, acc)
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

type columns struct {
	code, name, header int
	kind               string
	// Mizan tutar sütunları (-1: yok).
	debit, credit, debitBal, creditBal, balance int
}

// detectColumns finds the header row in the first 30 rows. Without a header
// the first column is the code and the second the name.
func detectColumns(rows [][]string) columns {
	out := columns{code: 0, name: 1, kind: KindAccountPlan, debit: -1, credit: -1, debitBal: -1, creditBal: -1, balance: -1}
	for i := 0; i < len(rows) && i < 30; i++ {
		c, n := -1, -1
		tb := false
		amt := columns{debit: -1, credit: -1, debitBal: -1, creditBal: -1, balance: -1}
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
			amt.amountColumn(f, j)
		}
		if c >= 0 {
			if n < 0 {
				n = c + 1
			}
			out = amt
			out.code, out.name, out.header, out.kind = c, n, i+1, KindAccountPlan
			if tb {
				out.kind = KindTrialBalance
			}
			return out
		}
	}
	return out
}

// amountColumn records a Luca / Zirve / Logo mizan amount header: "Borç",
// "Alacak" (or "Borç Toplamı"), "Borç Bakiye(si)", "Alacak Bakiye(si)",
// "Bakiye".
func (c *columns) amountColumn(f string, j int) {
	set := func(dst *int) {
		if *dst < 0 {
			*dst = j
		}
	}
	switch {
	case strings.HasPrefix(f, "borcbakiye"):
		set(&c.debitBal)
	case strings.HasPrefix(f, "alacakbakiye"):
		set(&c.creditBal)
	case f == "borc" || f == "borctoplami" || f == "toplamborc":
		set(&c.debit)
	case f == "alacak" || f == "alacaktoplami" || f == "toplamalacak":
		set(&c.credit)
	case f == "bakiye" || f == "netbakiye":
		set(&c.balance)
	}
}

// numberStyle decides TR / EN decimals from every amount cell of the file.
func (c columns) numberStyle(rows [][]string) parse.NumberStyle {
	var vals []string
	for i := c.header; i < len(rows); i++ {
		for _, j := range []int{c.debit, c.credit, c.debitBal, c.creditBal, c.balance} {
			if v := strings.TrimSpace(cell(rows[i], j)); v != "" {
				vals = append(vals, v)
			}
		}
	}
	return parse.DetectNumberStyle(vals)
}

func amount(row []string, j int, style parse.NumberStyle) decimal.Decimal {
	d, neg, ok := parse.ParseAmount(cell(row, j), style)
	if !ok {
		return decimal.Zero
	}
	if neg {
		d = d.Neg()
	}
	return d.Round(2)
}

// amounts returns the debit and credit totals and the signed balance
// (debit balance − credit balance). Without balance columns the balance is
// debit − credit.
func (c columns) amounts(row []string, style parse.NumberStyle) (debit, credit, balance decimal.Decimal) {
	debit, credit = amount(row, c.debit, style), amount(row, c.credit, style)
	switch {
	case c.debitBal >= 0 || c.creditBal >= 0:
		balance = amount(row, c.debitBal, style).Sub(amount(row, c.creditBal, style))
	case c.balance >= 0:
		balance = amount(row, c.balance, style)
	default:
		balance = debit.Sub(credit)
	}
	return debit, credit, balance
}

var (
	titleDateRe  = regexp.MustCompile(`\b([0-3]?[0-9])[./]([01]?[0-9])[./](20[0-9]{2})\b`)
	namePeriodRe = regexp.MustCompile(`(20[0-9]{2})[-_. ]?(0[1-9]|1[0-2])(?:[^0-9]|$)`)
)

// detectPeriod reads the trial balance period: the last date in the title
// rows above the header ("01.01.2026 - 30.09.2026" → 2026-09), else a
// "2026-09" / "202609" in the file name.
func detectPeriod(rows [][]string, header int, fileName string) string {
	for i := 0; i < header-1 && i < len(rows); i++ {
		for _, v := range rows[i] {
			m := titleDateRe.FindAllStringSubmatch(v, -1)
			if len(m) == 0 {
				continue
			}
			last := m[len(m)-1]
			month := last[2]
			if len(month) == 1 {
				month = "0" + month
			}
			if month >= "01" && month <= "12" {
				return last[3] + "-" + month
			}
		}
	}
	if m := namePeriodRe.FindStringSubmatch(fileName); m != nil {
		return m[1] + "-" + m[2]
	}
	return ""
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
