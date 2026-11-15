package parse

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// NumberStyle is how a column writes decimals.
type NumberStyle int

const (
	// StyleAuto decides per value (used when the column is ambiguous).
	StyleAuto NumberStyle = iota
	// StyleTR: 1.234,56
	StyleTR
	// StyleEN: 1,234.56 (also raw xlsx values: 1234.56)
	StyleEN
)

var numClean = strings.NewReplacer("₺", "", "TL", "", "TRY", "", "tl", "", " ", "", " ", "")

// DetectNumberStyle looks at every value of a column (04 §4.4): a comma
// after the last dot means TR, a dot after the last comma means EN.
func DetectNumberStyle(values []string) NumberStyle {
	tr, en := 0, 0
	for _, v := range values {
		v = numClean.Replace(v)
		c, d := strings.LastIndex(v, ","), strings.LastIndex(v, ".")
		switch {
		case c >= 0 && d >= 0 && c > d:
			tr += 2
		case c >= 0 && d >= 0 && d > c:
			en += 2
		case c >= 0 && d < 0:
			// "1234,56" is TR; "1,234" is ambiguous (EN thousands).
			if len(v)-c-1 != 3 {
				tr++
			}
		case d >= 0 && c < 0:
			if len(v)-d-1 != 3 || strings.Count(v, ".") > 1 {
				if strings.Count(v, ".") > 1 {
					tr++
				} else {
					en++
				}
			}
		}
	}
	switch {
	case tr > en:
		return StyleTR
	case en > tr:
		return StyleEN
	default:
		return StyleAuto
	}
}

// ParseAmount parses a money value: currency signs and spaces are dropped,
// parentheses or a leading/trailing minus mean negative. It returns the
// absolute value and whether it was negative.
func ParseAmount(s string, style NumberStyle) (abs decimal.Decimal, negative bool, ok bool) {
	v := numClean.Replace(strings.TrimSpace(s))
	if v == "" {
		return decimal.Zero, false, false
	}
	if strings.HasPrefix(v, "(") && strings.HasSuffix(v, ")") {
		negative, v = true, v[1:len(v)-1]
	}
	if strings.HasPrefix(v, "-") {
		negative, v = true, v[1:]
	} else if strings.HasSuffix(v, "-") {
		negative, v = true, v[:len(v)-1]
	}
	v = strings.TrimPrefix(v, "+")
	if style == StyleAuto {
		style = DetectNumberStyle([]string{v})
		if style == StyleAuto {
			style = StyleEN
		}
	}
	switch style {
	case StyleTR:
		v = strings.ReplaceAll(v, ".", "")
		v = strings.ReplaceAll(v, ",", ".")
	case StyleEN:
		v = strings.ReplaceAll(v, ",", "")
	}
	d, err := decimal.NewFromString(v)
	if err != nil {
		return decimal.Zero, false, false
	}
	if d.IsNegative() {
		negative, d = true, d.Neg()
	}
	return d, negative, true
}

var plainNumRe = regexp.MustCompile(`^[\(\-+]?[0-9.,]+[\)\-]?$`)

func parseNumberAuto(s string) (decimal.Decimal, bool) {
	v := numClean.Replace(s)
	if !plainNumRe.MatchString(v) {
		return decimal.Zero, false
	}
	d, _, ok := ParseAmount(v, StyleAuto)
	return d, ok
}

var dateLayouts = []string{"02.01.2006", "2.1.2006", "02/01/2006", "2/1/2006", "2006-01-02", "02-01-2006",
	"02.01.2006 15:04", "02.01.2006 15:04:05", "2006-01-02 15:04:05", "2006-01-02T15:04:05"}

var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// ParseDate reads dd.MM.yyyy, dd/MM/yyyy, yyyy-MM-dd (with optional time)
// and Excel serial dates (04 §4.4). The result is a UTC date.
func ParseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true
		}
	}
	// Excel serial: plausible range 1990..2100.
	if f, err := strconv.ParseFloat(s, 64); err == nil && f >= 32874 && f <= 73050 {
		t := excelEpoch.AddDate(0, 0, int(f))
		return t, true
	}
	return time.Time{}, false
}

// TaxID splits a VKN / TCKN (digits only): 10 → vkn, 11 → tckn.
func TaxID(s string) (vkn, tckn string, ok bool) {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	switch len(d) {
	case 10:
		return d, "", true
	case 11:
		return "", d, true
	}
	return "", "", false
}

// Rate normalizes a percent: "%20", "20", "0,20", "0.2" → 20.
func Rate(s string) (decimal.Decimal, bool) {
	v := strings.TrimSpace(strings.ReplaceAll(s, "%", ""))
	if v == "" {
		return decimal.Zero, false
	}
	if strings.Contains(v, "/") { // 5/10 → 50
		p := strings.SplitN(v, "/", 2)
		a, e1 := decimal.NewFromString(strings.TrimSpace(p[0]))
		b, e2 := decimal.NewFromString(strings.TrimSpace(p[1]))
		if e1 != nil || e2 != nil || b.IsZero() {
			return decimal.Zero, false
		}
		return a.Div(b).Mul(decimal.NewFromInt(100)).Round(2), true
	}
	d, _, ok := ParseAmount(v, StyleAuto)
	if !ok {
		return decimal.Zero, false
	}
	if d.LessThan(decimal.NewFromInt(1)) && d.IsPositive() {
		d = d.Mul(decimal.NewFromInt(100))
	}
	return d.Round(2), true
}

// Currency maps TL / TRL / ₺ to TRY; other ISO codes stay.
func Currency(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "", "TL", "TRL", "₺", "TRY", "YTL":
		return "TRY"
	default:
		return strings.ToUpper(strings.TrimSpace(s))
	}
}

// InvoiceKind is a parsed invoice type text (04 §4.4, 03 §2).
type InvoiceKind struct {
	Direction string // outgoing | incoming | ""
	Type      string // SATIS, ALIS, IADE, TEVKIFAT
}

// ParseInvoiceKind reads "Satış", "Satış İade", "Alış", "Alış İade", "Tevkifatlı".
func ParseInvoiceKind(s string) (InvoiceKind, bool) {
	n := Normalize(s)
	switch {
	case n == "":
		return InvoiceKind{}, false
	case strings.Contains(n, "tevkifat"):
		k := InvoiceKind{Type: "TEVKIFAT"}
		if strings.Contains(n, "alis") {
			k.Direction = "incoming"
		} else if strings.Contains(n, "satis") {
			k.Direction = "outgoing"
		}
		return k, true
	case strings.Contains(n, "iade") && strings.Contains(n, "alis"):
		return InvoiceKind{Direction: "incoming", Type: "IADE"}, true
	case strings.Contains(n, "iade") && strings.Contains(n, "satis"):
		return InvoiceKind{Direction: "outgoing", Type: "IADE"}, true
	case strings.Contains(n, "iade"):
		return InvoiceKind{Type: "IADE"}, true
	case strings.Contains(n, "satis"):
		return InvoiceKind{Direction: "outgoing", Type: "SATIS"}, true
	case strings.Contains(n, "alis"):
		return InvoiceKind{Direction: "incoming", Type: "ALIS"}, true
	}
	return InvoiceKind{}, false
}

// Profile maps "e-Fatura", "e-Arşiv", "Kağıt" (04 §4.4); "" for paper.
func Profile(s string) (string, bool) {
	n := strings.ReplaceAll(Normalize(s), " ", "")
	switch {
	case n == "":
		return "", false
	case strings.Contains(n, "arsiv"):
		return "EARSIVFATURA", true
	case strings.Contains(n, "efatura") || strings.Contains(n, "ticari"):
		return "TICARIFATURA", true
	case strings.Contains(n, "temel"):
		return "TEMELFATURA", true
	case strings.Contains(n, "kagit"):
		return "", true
	}
	return "", false
}
