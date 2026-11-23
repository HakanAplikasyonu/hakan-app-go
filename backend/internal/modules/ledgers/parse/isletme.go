package parse

import (
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	xparse "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
)

// Sides of an işletme defteri amount column.
const (
	SidePurchase = "purchase" // Mal alışı (satın alınan mallar)
	SideExpense  = "expense"  // Giderler
	SideSale     = "sale"     // Satışlar / hasılat
	SideIncome   = "income"   // Diğer gelirler
)

const (
	maxHeaderScan = 40
	topLines      = 10
)

// IsletmeSummary: totals of an işletme defteri export.
type IsletmeSummary struct {
	Sheet string `json:"sheet"`
	Rows  int    `json:"rows"`
	// Columns: the detected header of each role (tarih, açıklama, and the
	// amount sides), as written in the file.
	Columns   map[string][]string `json:"columns"`
	Purchases string              `json:"purchases"`
	Expenses  string              `json:"expenses"`
	Sales     string              `json:"sales"`
	Income    string              `json:"income"`
	// FirstDate / LastDate: the date range of the rows (YYYY-MM-DD).
	FirstDate string `json:"first_date,omitempty"`
	LastDate  string `json:"last_date,omitempty"`
	// Top: the largest lines.
	Top []IsletmeLine `json:"top"`
}

// IsletmeLine is one row of the book.
type IsletmeLine struct {
	Date        string `json:"date,omitempty"`
	Description string `json:"description"`
	Side        string `json:"side"`
	Amount      string `json:"amount"`
}

type column struct {
	idx   int
	role  string // date, desc, or a Side
	title string
}

// roleOf classifies a normalized header cell. VAT and total columns are
// not amounts of a side.
func roleOf(n string) string {
	has := func(ws ...string) bool {
		for _, w := range ws {
			if strings.Contains(n, w) {
				return true
			}
		}
		return false
	}
	word := func(ws ...string) bool {
		for _, t := range strings.Fields(n) {
			for _, w := range ws {
				if t == w {
					return true
				}
			}
		}
		return false
	}
	switch {
	case n == "":
		return ""
	case has("tarih"):
		return "date"
	case has("kdv", "vergi", "toplam", "bakiye", "stopaj", "tevkifat") || word("no", "sira", "oran", "orani", "nosu", "numarasi"):
		return ""
	case has("aciklama", "izahat", "unvan", "cari"):
		return "desc"
	case has("alis", "satin alinan", "satinalinan", "emtia", "mal alim"):
		return SidePurchase
	case has("satis", "satilan", "hasilat"):
		return SideSale
	case has("gider", "masraf"):
		return SideExpense
	case has("gelir"):
		return SideIncome
	}
	return ""
}

func detectColumns(row []string) []column {
	var cols []column
	for j, c := range row {
		if r := roleOf(xparse.Normalize(c)); r != "" {
			cols = append(cols, column{idx: j, role: r, title: c})
		}
	}
	return cols
}

// isHeader: a date or description column and at least one amount column.
func isHeader(cols []column) bool {
	amount, label := false, false
	for _, c := range cols {
		switch c.role {
		case SidePurchase, SideExpense, SideSale, SideIncome:
			amount = true
		case "date", "desc":
			label = true
		}
	}
	return amount && label
}

// Isletme reads an işletme defteri export (xlsx / csv). The reason is
// non-empty when it cannot.
func Isletme(name string, data []byte) (IsletmeSummary, string) {
	_, sheets, err := xparse.Read(name, data)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "legacy"):
			return IsletmeSummary{}, ReasonLegacyXLS
		case err == xparse.ErrEmpty:
			return IsletmeSummary{}, ReasonEmpty
		case err == xparse.ErrTooLarge:
			return IsletmeSummary{}, ReasonTooLarge
		}
		return IsletmeSummary{}, ReasonUnsupported
	}
	for _, sh := range sheets {
		if out, ok := isletmeSheet(sh); ok {
			return out, ""
		}
	}
	return IsletmeSummary{}, ReasonNoColumns
}

func isletmeSheet(sh xparse.Sheet) (IsletmeSummary, bool) {
	// The header may span two rows (Luca: "GİDER" over "Satın Alınan
	// Mallar"): a row with amount columns is the header; a group word in
	// the row above is prepended to empty / generic titles.
	hi := -1
	var cols []column
	for i := 0; i < len(sh.Rows) && i < maxHeaderScan; i++ {
		row := sh.Rows[i]
		if i > 0 {
			row = mergeHeader(sh.Rows[i-1], sh.Rows[i])
		}
		c := detectColumns(row)
		if isHeader(c) {
			hi, cols = i, c
			break
		}
	}
	if hi < 0 {
		return IsletmeSummary{}, false
	}
	out := IsletmeSummary{Sheet: sh.Name, Columns: map[string][]string{}}
	for _, c := range cols {
		out.Columns[c.role] = append(out.Columns[c.role], c.title)
	}
	totals := map[string]decimal.Decimal{}
	var lines []struct {
		IsletmeLine
		abs decimal.Decimal
	}
	// Number style per column: all values of the column decide.
	styles := map[int]xparse.NumberStyle{}
	for _, c := range cols {
		if c.role == "date" || c.role == "desc" {
			continue
		}
		var vals []string
		for _, r := range sh.Rows[hi+1:] {
			if c.idx < len(r) && r[c.idx] != "" {
				vals = append(vals, r[c.idx])
			}
		}
		styles[c.idx] = xparse.DetectNumberStyle(vals)
	}
	var first, last string
	for _, r := range sh.Rows[hi+1:] {
		if xparse.IsTotalRow(r) || empty(r) {
			continue
		}
		date, desc := "", ""
		counted := false
		for _, c := range cols {
			v := cell(r, c.idx)
			switch c.role {
			case "date":
				if date == "" {
					if t, ok := xparse.ParseDate(v); ok {
						date = t.Format("2006-01-02")
					}
				}
			case "desc":
				if desc == "" {
					desc = v
				} else if v != "" {
					desc += " · " + v
				}
			}
		}
		for _, c := range cols {
			if c.role == "date" || c.role == "desc" {
				continue
			}
			amt, neg, ok := xparse.ParseAmount(cell(r, c.idx), styles[c.idx])
			if !ok || amt.IsZero() {
				continue
			}
			if neg {
				amt = amt.Neg()
			}
			totals[c.role] = totals[c.role].Add(amt)
			counted = true
			if r := []rune(desc); len(r) > 120 {
				desc = string(r[:120])
			}
			lines = append(lines, struct {
				IsletmeLine
				abs decimal.Decimal
			}{IsletmeLine{Date: date, Description: desc, Side: c.role, Amount: amt.StringFixed(2)}, amt.Abs()})
		}
		if !counted {
			continue
		}
		out.Rows++
		if date != "" {
			if first == "" || date < first {
				first = date
			}
			if date > last {
				last = date
			}
		}
	}
	if out.Rows == 0 {
		return IsletmeSummary{}, false
	}
	out.FirstDate, out.LastDate = first, last
	out.Purchases = totals[SidePurchase].StringFixed(2)
	out.Expenses = totals[SideExpense].StringFixed(2)
	out.Sales = totals[SideSale].StringFixed(2)
	out.Income = totals[SideIncome].StringFixed(2)
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].abs.GreaterThan(lines[j].abs) })
	out.Top = []IsletmeLine{}
	for i := 0; i < len(lines) && i < topLines; i++ {
		out.Top = append(out.Top, lines[i].IsletmeLine)
	}
	return out, true
}

// mergeHeader joins a group row above the header into its cells: "Gider"
// over an empty cell or "Tutar" becomes "Gider Tutar".
func mergeHeader(above, row []string) []string {
	out := make([]string, len(row))
	group := ""
	for j := range row {
		if a := cell(above, j); a != "" {
			group = a
		}
		c := row[j]
		n := xparse.Normalize(c)
		if n != "" && group != "" && roleOf(n) == "" && roleOf(xparse.Normalize(group)) != "" {
			c = group + " " + c
		}
		out[j] = c
	}
	return out
}

func cell(r []string, j int) string {
	if j < len(r) {
		return strings.TrimSpace(r[j])
	}
	return ""
}

func empty(r []string) bool {
	for _, c := range r {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
