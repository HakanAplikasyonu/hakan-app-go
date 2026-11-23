package parse

import (
	"regexp"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	xparse "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
)

// maxMizanLines bounds the accounts kept in a mizan summary.
const maxMizanLines = 300

// MizanSummary: a Luca mizan (trial balance) export. Luca writes a few rows
// of header info (firm, period, date range) and then the header row
// HESAP KODU | HESAP ADI | (boş) | BORÇ | ALACAK | BORÇ BAKİYESİ |
// ALACAK BAKİYESİ. The header row is found by its text, not by its number.
type MizanSummary struct {
	Sheet string `json:"sheet"`
	// HeaderRow: 1-based row of the header in the sheet.
	HeaderRow int `json:"header_row"`
	// Period (YYYY-MM): the end of the date range in the header info
	// ("01.01.2026 - 31.10.2026" → 2026-10), else from the file name.
	Period      string `json:"period,omitempty"`
	PeriodStart string `json:"period_start,omitempty"`
	PeriodEnd   string `json:"period_end,omitempty"`
	// Accounts: every account row read; Lines: the main accounts (first
	// three digits) whose totals are summed below.
	Accounts      int    `json:"accounts"`
	DebitTotal    string `json:"debit_total"`
	CreditTotal   string `json:"credit_total"`
	DebitBalance  string `json:"debit_balance"`
	CreditBalance string `json:"credit_balance"`
	// Balanced: borç toplamı = alacak toplamı and borç bakiyesi = alacak
	// bakiyesi.
	Balanced bool        `json:"balanced"`
	Lines    []MizanLine `json:"lines"`
}

// MizanLine is one account row.
type MizanLine struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	Debit         string `json:"debit"`
	Credit        string `json:"credit"`
	DebitBalance  string `json:"debit_balance"`
	CreditBalance string `json:"credit_balance"`
}

type mizanCols struct {
	code, name, debit, credit, debitBal, creditBal int
}

// mizanHeader reports the columns of a mizan header row: an account code
// column and debit / credit columns.
func mizanHeader(row []string) (mizanCols, bool) {
	c := mizanCols{code: -1, name: -1, debit: -1, credit: -1, debitBal: -1, creditBal: -1}
	set := func(dst *int, j int) {
		if *dst < 0 {
			*dst = j
		}
	}
	for j, v := range row {
		n := xparse.Normalize(v)
		switch {
		case n == "hesap kodu" || n == "hesap no" || n == "hesap kod":
			set(&c.code, j)
		case n == "hesap adi" || n == "hesap ismi" || n == "hesap aciklamasi":
			set(&c.name, j)
		case strings.HasPrefix(n, "borc bakiye"):
			set(&c.debitBal, j)
		case strings.HasPrefix(n, "alacak bakiye"):
			set(&c.creditBal, j)
		case n == "borc" || n == "borc toplami" || n == "toplam borc":
			set(&c.debit, j)
		case n == "alacak" || n == "alacak toplami" || n == "toplam alacak":
			set(&c.credit, j)
		}
	}
	return c, c.code >= 0 && (c.debit >= 0 || c.debitBal >= 0) && (c.credit >= 0 || c.creditBal >= 0)
}

// findMizan returns the first sheet with a mizan header in its first rows.
func findMizan(sheets []xparse.Sheet) (xparse.Sheet, int, mizanCols, bool) {
	for _, sh := range sheets {
		for i := 0; i < len(sh.Rows) && i < maxHeaderScan; i++ {
			if c, ok := mizanHeader(sh.Rows[i]); ok {
				return sh, i, c, true
			}
		}
	}
	return xparse.Sheet{}, -1, mizanCols{}, false
}

var mizanCodeRe = regexp.MustCompile(`^[0-9]{3}([.\- ][0-9A-Za-z]+)*$`)

// Mizan reads a mizan export (xlsx / csv / .xls). The reason is non-empty when it
// cannot.
func Mizan(name string, data []byte) (MizanSummary, string) {
	sheets, reason := readSheets(name, data)
	if reason != "" {
		return MizanSummary{}, reason
	}
	sh, hi, c, ok := findMizan(sheets)
	if !ok {
		return MizanSummary{}, ReasonNoMizanHeader
	}
	out := MizanSummary{Sheet: sh.Name, HeaderRow: hi + 1, Lines: []MizanLine{}}
	out.PeriodStart, out.PeriodEnd = dateRange(sh.Rows[:hi])
	switch {
	case out.PeriodEnd != "":
		out.Period = out.PeriodEnd[:7]
	default:
		out.Period = periodFromName(name)
	}
	type row struct {
		code, name string
		v          [4]decimal.Decimal
	}
	var all []row
	for _, r := range sh.Rows[hi+1:] {
		code := strings.TrimSpace(cell(r, c.code))
		if !mizanCodeRe.MatchString(code) {
			continue // boş satır, ara başlık, toplam satırı
		}
		var v [4]decimal.Decimal
		for k, j := range []int{c.debit, c.credit, c.debitBal, c.creditBal} {
			v[k] = number(cell(r, j))
		}
		all = append(all, row{code: code, name: cell(r, c.name), v: v})
	}
	if len(all) == 0 {
		return MizanSummary{}, ReasonNoEntries
	}
	out.Accounts = len(all)
	// Totals over the main accounts when the file has them (sub-accounts
	// would count twice); otherwise over every row.
	mains := false
	for _, r := range all {
		if len(r.code) == 3 {
			mains = true
			break
		}
	}
	var tot [4]decimal.Decimal
	for _, r := range all {
		if mains && len(r.code) != 3 {
			continue
		}
		for k := range tot {
			tot[k] = tot[k].Add(r.v[k])
		}
		if len(out.Lines) < maxMizanLines {
			out.Lines = append(out.Lines, MizanLine{
				Code: r.code, Name: r.name, Debit: r.v[0].StringFixed(2), Credit: r.v[1].StringFixed(2),
				DebitBalance: r.v[2].StringFixed(2), CreditBalance: r.v[3].StringFixed(2),
			})
		}
	}
	sort.SliceStable(out.Lines, func(i, j int) bool { return out.Lines[i].Code < out.Lines[j].Code })
	out.DebitTotal, out.CreditTotal = tot[0].StringFixed(2), tot[1].StringFixed(2)
	out.DebitBalance, out.CreditBalance = tot[2].StringFixed(2), tot[3].StringFixed(2)
	out.Balanced = tot[0].Round(2).Equal(tot[1].Round(2)) && tot[2].Round(2).Equal(tot[3].Round(2))
	return out, ""
}
