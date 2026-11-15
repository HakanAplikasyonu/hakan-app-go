package extract

import (
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Receipt values are short, single values; these helpers are enough here
// (the Excel channel, #22, has column-wide number style detection).

var dateLayouts = []string{"02.01.2006", "2.1.2006", "02/01/2006", "2/1/2006", "2006-01-02", "02-01-2006",
	"02.01.06", "02.01.2006 15:04", "02.01.2006 15:04:05", "2006-01-02 15:04:05"}

func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true
		}
	}
	return time.Time{}, false
}

// parseAmount reads "1.234,56", "1234.56", "*123,45", "₺12,00" (Turkish
// receipts print a leading "*" before amounts).
func parseAmount(s string) (decimal.Decimal, bool) {
	v := strings.NewReplacer("*", "", "₺", "", "TL", "", " ", "", " ", "").Replace(strings.TrimSpace(s))
	if v == "" {
		return decimal.Zero, false
	}
	c, d := strings.LastIndex(v, ","), strings.LastIndex(v, ".")
	switch {
	case c > d: // TR: 1.234,56
		v = strings.ReplaceAll(v, ".", "")
		v = strings.Replace(v, ",", ".", 1)
	case d > c && c >= 0: // EN: 1,234.56
		v = strings.ReplaceAll(v, ",", "")
	case d >= 0 && strings.Count(v, ".") > 1: // 1.234.567
		v = strings.ReplaceAll(v, ".", "")
	}
	out, err := decimal.NewFromString(v)
	if err != nil {
		return decimal.Zero, false
	}
	return out.Abs(), true
}

func parseRate(s string) (decimal.Decimal, bool) {
	v := strings.TrimSpace(strings.NewReplacer("%", "", "KDV", "", "kdv", "").Replace(s))
	d, ok := parseAmount(v)
	if !ok {
		return decimal.Zero, false
	}
	if d.IsPositive() && d.LessThan(decimal.NewFromInt(1)) {
		d = d.Mul(decimal.NewFromInt(100))
	}
	return d.Round(2), true
}

func parseTaxID(s string) (string, bool) {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if len(d) == 10 || len(d) == 11 {
		_, err := strconv.ParseInt(d, 10, 64)
		return d, err == nil
	}
	return "", false
}

func normText(s string) string {
	s = strings.NewReplacer("İ", "i", "I", "ı").Replace(s)
	s = strings.ToLower(s)
	return strings.NewReplacer("ı", "i", "ğ", "g", "ü", "u", "ş", "s", "ö", "o", "ç", "c").Replace(s)
}
