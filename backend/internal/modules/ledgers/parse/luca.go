package parse

import (
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	xparse "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/xls"
)

// Helpers shared by the Luca mizan and ücret bordrosu readers.

// readSheets reads an xlsx / csv / legacy .xls into sheets; the reason is
// non-empty when it cannot. The format comes from the content (magic
// bytes), not the extension. A binary Excel 97-2003 (.xls) workbook is read
// by the BIFF8 reader into the same rows, so the header search is shared.
func readSheets(name string, data []byte) ([]xparse.Sheet, string) {
	switch {
	case len(data) == 0:
		return nil, ReasonEmpty
	case len(data) > MaxBytes:
		return nil, ReasonTooLarge
	}
	if DetectFormat(name, data) == "xls" {
		return readXLS(data)
	}
	_, sheets, err := xparse.Read(name, data)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "legacy"):
			return nil, ReasonLegacyXLS
		case err == xparse.ErrEmpty:
			return nil, ReasonEmpty
		case err == xparse.ErrTooLarge:
			return nil, ReasonTooLarge
		}
		return nil, ReasonUnsupported
	}
	return sheets, ""
}

// readXLS reads a binary Excel 97-2003 workbook. A damaged or password
// protected one gives ReasonLegacyXLS.
func readXLS(data []byte) ([]xparse.Sheet, string) {
	book, err := xls.Read(data)
	if err != nil {
		return nil, ReasonLegacyXLS
	}
	sheets := make([]xparse.Sheet, 0, len(book))
	total := 0
	for _, sh := range book {
		sheets = append(sheets, xparse.Sheet{Name: sh.Name, Rows: sh.Rows})
		total += len(sh.Rows)
	}
	switch {
	case total == 0:
		return nil, ReasonEmpty
	case total > xparse.MaxRows:
		return nil, ReasonUnsupported
	}
	return sheets, ""
}

var (
	plainNumberRe = regexp.MustCompile(`^[-+(]?[0-9][0-9.,]*(?:[eE][-+]?[0-9]+)?[)\-]?$`)
	dateRe        = regexp.MustCompile(`\b([0-3]?[0-9])[./]([01]?[0-9])[./]((?:19|20)[0-9]{2})\b`)
	nameMonthRe   = regexp.MustCompile(`((?:19|20)[0-9]{2})[-_. ]?(0[1-9]|1[0-2])(?:[^0-9]|$)`)
)

// isNumber: a plain number cell ("1234.5", "1.234,50", "(12)").
func isNumber(s string) bool {
	return plainNumberRe.MatchString(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

// number parses an amount cell (raw xlsx value or TR / EN text); zero when
// empty or not a number.
func number(s string) decimal.Decimal {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero
	}
	if strings.ContainsAny(s, "eE") {
		if d, err := decimal.NewFromString(s); err == nil {
			return d
		}
	}
	d, neg, ok := xparse.ParseAmount(s, xparse.StyleAuto)
	if !ok {
		return decimal.Zero
	}
	if neg {
		d = d.Neg()
	}
	return d
}

// NationalID cleans a T.C. kimlik no cell. Luca writes it as a number, so
// the sheet may hold "12345678901", "12345678901.0" or "1.2345678901E+10";
// the result is the 11 digits, or "" when it is not one.
func NationalID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if d, err := decimal.NewFromString(s); err == nil && d.IsInteger() && d.Sign() > 0 {
		s = d.String()
	} else {
		s = strings.TrimSuffix(strings.TrimSuffix(s, ".0"), ",0")
	}
	var b strings.Builder
	for _, r := range s {
		if r < '0' || r > '9' {
			return ""
		}
		b.WriteRune(r)
	}
	if b.Len() != 11 {
		return ""
	}
	return b.String()
}

// dateRange returns the first and last dd.mm.yyyy dates (YYYY-MM-DD) of the
// header info rows.
func dateRange(rows [][]string) (first, last string) {
	for _, r := range rows {
		for _, v := range r {
			for _, m := range dateRe.FindAllStringSubmatch(v, -1) {
				t, err := time.Parse("2.1.2006", m[1]+"."+m[2]+"."+m[3])
				if err != nil {
					continue
				}
				d := t.Format("2006-01-02")
				if first == "" {
					first = d
				}
				last = d
			}
		}
	}
	return first, last
}

// periodFromName reads "2026-10" / "202610" from a file name.
func periodFromName(name string) string {
	if m := nameMonthRe.FindStringSubmatch(name); m != nil {
		return m[1] + "-" + m[2]
	}
	return ""
}

var trMonths = map[string]string{
	"ocak": "01", "subat": "02", "mart": "03", "nisan": "04", "mayis": "05", "haziran": "06",
	"temmuz": "07", "agustos": "08", "eylul": "09", "ekim": "10", "kasim": "11", "aralik": "12",
}

// periodFromTitle reads "EKİM 2026" (a Turkish month name and a year) from a
// title cell.
func periodFromTitle(s string) string {
	ws := strings.Fields(xparse.Normalize(s))
	for i, w := range ws {
		m, ok := trMonths[w]
		if !ok {
			continue
		}
		for _, n := range []int{i + 1, i - 1} {
			if n >= 0 && n < len(ws) && len(ws[n]) == 4 && (strings.HasPrefix(ws[n], "19") || strings.HasPrefix(ws[n], "20")) {
				return ws[n] + "-" + m
			}
		}
	}
	return ""
}
