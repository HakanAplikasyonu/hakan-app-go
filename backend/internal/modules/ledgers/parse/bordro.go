package parse

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	xparse "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
)

// maxBordroEmployees bounds the people kept in a bordro summary.
const maxBordroEmployees = 500

// BordroSummary: a Luca "ÜCRET BORDROSU" export. The title row carries the
// month ("EKİM 2026 ÜCRET BORDROSU"); a header row (ADI SOYADI … T.C.
// KİMLİK NO … NET İSTİHKAK) is followed by three rows per person; at the
// bottom "BRÜT ÖDEMELER" and "NET ÖDEMELER" blocks hold the totals the
// accountant books. Rows and columns are found by their text.
type BordroSummary struct {
	Sheet  string `json:"sheet"`
	Period string `json:"period,omitempty"`
	// Persons: "Toplam Kişi Sayısı" (else "Listelenen N Personel", else the
	// people read).
	Persons int `json:"persons"`
	// FromTotals: the amounts come from the bottom summary blocks; false
	// when the file has none and the person rows were summed (gross, net).
	FromTotals           bool   `json:"from_totals"`
	Gross                string `json:"gross"`
	SGKBase              string `json:"sgk_base"`
	SGKEmployee          string `json:"sgk_employee"`
	SGKEmployer          string `json:"sgk_employer"`
	SGKIncentive         string `json:"sgk_incentive"`
	UnemploymentEmployee string `json:"unemployment_employee"`
	UnemploymentEmployer string `json:"unemployment_employer"`
	IncomeTax            string `json:"income_tax"`
	StampTax             string `json:"stamp_tax"`
	Net                  string `json:"net"`
	// EmployerCost: the TOPLAM of the BRÜT ÖDEMELER block (brüt + işveren
	// payları).
	EmployerCost string           `json:"employer_cost"`
	Employees    []BordroEmployee `json:"employees"`
}

// BordroEmployee is the main row of one person.
type BordroEmployee struct {
	Name       string `json:"name"`
	NationalID string `json:"national_id"`
	Days       string `json:"days,omitempty"`
	Gross      string `json:"gross"`
	Net        string `json:"net"`
}

type bordroCols struct{ name, id, days, gross, net int }

func bordroHeader(row []string) (bordroCols, bool) {
	c := bordroCols{name: -1, id: -1, days: -1, gross: -1, net: -1}
	for j, v := range row {
		n := xparse.Normalize(v)
		switch {
		case c.name < 0 && (n == "adi soyadi" || n == "ad soyad" || n == "adi soyad"):
			c.name = j
		case c.id < 0 && strings.Contains(n, "kimlik"):
			c.id = j
		case c.days < 0 && n == "gun":
			c.days = j
		case c.gross < 0 && n == "toplam kazanc":
			c.gross = j
		case c.net < 0 && strings.HasPrefix(n, "net istihkak"):
			c.net = j
		}
	}
	return c, c.name >= 0 && c.id >= 0 && (c.gross >= 0 || c.net >= 0)
}

// bordroLabels: bottom summary labels (normalized) → field. A label
// matches exactly; prefix labels end with a space.
var bordroLabels = map[string]string{
	"brut kazanclar toplami":      "gross",
	"toplam kisi sayisi":          "persons",
	"toplam sgk matrahi":          "sgk_base",
	"sigorta isveren hissesi":     "sgk_employer_gross",
	"sgk isveren tesvikleri":      "sgk_incentive",
	"net sigorta isveren hissesi": "sgk_employer",
	"issizlik isveren hissesi":    "unemployment_employer",
	"gelir vergisi":               "income_tax_gross",
	"net gv":                      "income_tax",
	"damga vergisi":               "stamp_tax_gross",
	"net dv":                      "stamp_tax",
	"sigorta toplam prim ":        "sgk_employee",
	"sigorta toplam prim":         "sgk_employee",
	"issizlik sigortasi":          "unemployment_employee",
	"net odemeler toplam":         "net",
	"net odemeler toplami":        "net",
}

func bordroLabel(n string) string {
	if f, ok := bordroLabels[n]; ok {
		return f
	}
	for k, f := range bordroLabels {
		if strings.HasSuffix(k, " ") && strings.HasPrefix(n, k) {
			return f
		}
	}
	return ""
}

var listedRe = regexp.MustCompile(`listelenen ([0-9]+) personel`)

// isBordroSheet reports whether a sheet looks like a Luca ücret bordrosu.
func isBordroSheet(sh xparse.Sheet) bool {
	for i := 0; i < len(sh.Rows) && i < maxHeaderScan; i++ {
		for _, v := range sh.Rows[i] {
			if strings.Contains(xparse.Normalize(v), "ucret bordrosu") {
				return true
			}
		}
		if _, ok := bordroHeader(sh.Rows[i]); ok {
			return true
		}
	}
	return false
}

// Bordro reads a Luca ücret bordrosu (Excel 97-2003 .xls, xlsx or csv). The reason is
// non-empty when it cannot.
func Bordro(name string, data []byte) (BordroSummary, string) {
	sheets, reason := readSheets(name, data)
	if reason != "" {
		return BordroSummary{}, reason
	}
	for _, sh := range sheets {
		if out, ok := bordroSheet(sh, name); ok {
			return out, ""
		}
	}
	return BordroSummary{}, ReasonNoBordro
}

func bordroSheet(sh xparse.Sheet, fileName string) (BordroSummary, bool) {
	out := BordroSummary{Sheet: sh.Name, Employees: []BordroEmployee{}}
	hi := -1
	var cols bordroCols
	for i := 0; i < len(sh.Rows) && i < maxHeaderScan; i++ {
		for _, v := range sh.Rows[i] {
			if out.Period == "" && strings.Contains(xparse.Normalize(v), "ucret bordrosu") {
				out.Period = periodFromTitle(v)
			}
		}
		if c, ok := bordroHeader(sh.Rows[i]); ok {
			hi, cols = i, c
			break
		}
	}
	if out.Period == "" {
		out.Period = periodFromName(fileName)
	}
	// The bottom summary: from the row with "BRÜT ÖDEMELER" / "NET
	// ÖDEMELER"; the NET ÖDEMELER column splits the left and right blocks.
	sumRow, netCol := -1, -1
	listed := 0
	start := hi + 1
	for i := start; i < len(sh.Rows); i++ {
		for j, v := range sh.Rows[i] {
			n := xparse.Normalize(v)
			if m := listedRe.FindStringSubmatch(n); m != nil {
				listed, _ = strconv.Atoi(m[1])
			}
			if n == "brut odemeler" || n == "net odemeler" {
				if sumRow < 0 {
					sumRow = i
				}
				if n == "net odemeler" && netCol < 0 {
					netCol = j
				}
			}
		}
		if sumRow >= 0 {
			break
		}
	}
	// People: rows between the header and the summary with an 11-digit
	// T.C. kimlik no (Luca writes it as a number: "12345678901.0").
	var gross, net decimal.Decimal
	if hi >= 0 {
		end := len(sh.Rows)
		if sumRow >= 0 {
			end = sumRow
		}
		for _, r := range sh.Rows[hi+1 : end] {
			id := NationalID(cell(r, cols.id))
			if id == "" {
				continue
			}
			g, n := number(cell(r, cols.gross)), number(cell(r, cols.net))
			gross, net = gross.Add(g), net.Add(n)
			if len(out.Employees) < maxBordroEmployees {
				out.Employees = append(out.Employees, BordroEmployee{
					Name: cell(r, cols.name), NationalID: id, Days: cell(r, cols.days),
					Gross: g.StringFixed(2), Net: n.StringFixed(2),
				})
			}
		}
	}
	if sumRow < 0 && hi < 0 {
		return BordroSummary{}, false
	}
	vals := map[string]decimal.Decimal{}
	if sumRow >= 0 {
		vals = bordroTotals(sh.Rows[sumRow:], netCol)
	}
	pick := func(fields ...string) (decimal.Decimal, bool) {
		for _, f := range fields {
			if v, ok := vals[f]; ok {
				return v, true
			}
		}
		return decimal.Zero, false
	}
	if g, ok := pick("gross"); ok {
		out.FromTotals = true
		gross = g
	}
	if n, ok := pick("net"); ok {
		out.FromTotals = true
		net = n
	}
	if hi < 0 && !out.FromTotals {
		return BordroSummary{}, false
	}
	str := func(fields ...string) string {
		v, _ := pick(fields...)
		return v.StringFixed(2)
	}
	out.Gross, out.Net = gross.StringFixed(2), net.StringFixed(2)
	out.SGKBase = str("sgk_base")
	out.SGKEmployee = str("sgk_employee")
	out.SGKEmployer = str("sgk_employer", "sgk_employer_gross")
	out.SGKIncentive = str("sgk_incentive")
	out.UnemploymentEmployee = str("unemployment_employee")
	out.UnemploymentEmployer = str("unemployment_employer")
	out.IncomeTax = str("income_tax", "income_tax_gross")
	out.StampTax = str("stamp_tax", "stamp_tax_gross")
	out.EmployerCost = str("employer_cost")
	switch p, ok := pick("persons"); {
	case ok && p.IsPositive():
		out.Persons = int(p.IntPart())
	case listed > 0:
		out.Persons = listed
	default:
		out.Persons = len(out.Employees)
	}
	return out, true
}

// bordroTotals reads the label → amount pairs of the summary blocks. The
// amount of a label is the "Tutar" column of its block when the block has
// one, else the last number before the next label on the row. The first
// TOPLAM of the left block is the employer cost.
func bordroTotals(rows [][]string, netCol int) map[string]decimal.Decimal {
	out := map[string]decimal.Decimal{}
	tutar := map[int]bool{}
	for _, r := range rows {
		for j, v := range r {
			if xparse.Normalize(v) == "tutar" {
				tutar[j] = true
			}
		}
	}
	for _, r := range rows {
		for j := 0; j < len(r); j++ {
			n := xparse.Normalize(r[j])
			if n == "" || isNumber(r[j]) {
				continue
			}
			field := bordroLabel(n)
			if n == "toplam" && (netCol < 0 || j < netCol) {
				field = "employer_cost"
			}
			if field == "" {
				continue
			}
			// The segment up to the next text cell.
			var last, inTutar *decimal.Decimal
			for k := j + 1; k < len(r); k++ {
				v := strings.TrimSpace(r[k])
				if v == "" {
					continue
				}
				if !isNumber(v) {
					break
				}
				d := number(v)
				last = &d
				if tutar[k] {
					inTutar = &d
				}
			}
			val := last
			if inTutar != nil {
				val = inTutar
			}
			if val == nil {
				continue
			}
			if _, seen := out[field]; !seen {
				out[field] = *val
			}
		}
	}
	return out
}
