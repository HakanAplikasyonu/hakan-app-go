package model

import (
	"strings"
	"unicode"

	"github.com/shopspring/decimal"
)

// NormalizeCurrency applies `01` §7: TRL / TL / YTL → TRY, upper case.
func NormalizeCurrency(code string) string {
	c := strings.ToUpper(strings.TrimSpace(code))
	switch c {
	case "TRL", "TL", "YTL":
		return "TRY"
	}
	return c
}

// DigitsOnly strips everything but ASCII digits (VKN / TCKN normalisation).
func DigitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ClassifyTaxID splits a normalised id by length (`01` §7): 10 digits is a
// VKN, 11 a TCKN. ok is false for any other length.
func ClassifyTaxID(raw string) (vkn, tckn string, ok bool) {
	d := DigitsOnly(raw)
	switch len(d) {
	case 10:
		return d, "", true
	case 11:
		return "", d, true
	}
	return "", "", false
}

// NormalizeInvoiceNumber removes spaces and dashes and upper-cases (`01` §7).
func NormalizeInvoiceNumber(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsSpace(r) || r == '-' {
			continue
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

var hundred = decimal.NewFromInt(100)

// NormalizePercent turns a fractional rate (0.20) into a percent (20); values
// above 1 are already percents.
func NormalizePercent(d decimal.Decimal) decimal.Decimal {
	if d.IsPositive() && d.LessThan(decimal.NewFromInt(1)) {
		return d.Mul(hundred)
	}
	return d
}

// ValidVATRates are the KDV rates a header estimate may round to (`01` §4.5).
var ValidVATRates = []int64{0, 1, 8, 10, 18, 20}

// NearestVATRate rounds an estimated rate to the closest valid KDV rate.
func NearestVATRate(r decimal.Decimal) decimal.Decimal {
	best := decimal.NewFromInt(ValidVATRates[0])
	bestDiff := r.Sub(best).Abs()
	for _, v := range ValidVATRates[1:] {
		d := decimal.NewFromInt(v)
		if diff := r.Sub(d).Abs(); diff.LessThan(bestDiff) {
			best, bestDiff = d, diff
		}
	}
	return best
}
