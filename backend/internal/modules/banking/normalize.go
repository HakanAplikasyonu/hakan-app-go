package banking

import (
	"math/big"
	"strings"
	"unicode"
)

var asciiFold = strings.NewReplacer("İ", "I", "Ş", "S", "Ğ", "G", "Ü", "U", "Ö", "O", "Ç", "C", "Â", "A", "Î", "I", "Û", "U")

// NormalizeDescription produces the copy classification reads (21 §2.1):
// Turkish-locale upper case (i→İ, ı→I), then ASCII, punctuation to spaces,
// runs of spaces collapsed. The original description is stored untouched.
func NormalizeDescription(s string) string {
	up := asciiFold.Replace(strings.ToUpperSpecial(unicode.TurkishCase, s))
	var b strings.Builder
	b.Grow(len(up))
	space := true
	for _, r := range up {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// NormalizeIBAN removes spaces and upper-cases; it returns "" unless the
// result is a valid IBAN (country + check digits, mod 97 = 1). Turkish IBANs
// must be TR + 24 digits. Masked or partial IBANs never reach a column; they
// stay in raw_payload (21 §2.1).
func NormalizeIBAN(s string) string {
	v := strings.ToUpper(strings.Join(strings.Fields(s), ""))
	if len(v) < 15 || len(v) > 34 {
		return ""
	}
	if strings.HasPrefix(v, "TR") && len(v) != 26 {
		return ""
	}
	for i, r := range v {
		switch {
		case i < 2 && (r < 'A' || r > 'Z'):
			return ""
		case i >= 2 && i < 4 && (r < '0' || r > '9'):
			return ""
		case (r < 'A' || r > 'Z') && (r < '0' || r > '9'):
			return ""
		}
	}
	var digits strings.Builder
	for _, r := range v[4:] + v[:4] {
		if r >= 'A' && r <= 'Z' {
			digits.WriteString(big.NewInt(int64(r - 'A' + 10)).String())
		} else {
			digits.WriteRune(r)
		}
	}
	n, ok := new(big.Int).SetString(digits.String(), 10)
	if !ok || new(big.Int).Mod(n, big.NewInt(97)).Int64() != 1 {
		return ""
	}
	return v
}

// SplitTaxID keeps digits only; 10 digits is a VKN, 11 a TCKN. Filler values
// such as 0000000000 are dropped (21 §2.1).
func SplitTaxID(s string) (vkn, tckn string) {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if strings.Trim(d, "0") == "" {
		return "", ""
	}
	switch len(d) {
	case 10:
		return d, ""
	case 11:
		return "", d
	}
	return "", ""
}

// NormalizeCurrency maps TL / YTL / TRL to TRY and upper-cases the rest.
func NormalizeCurrency(s string) string {
	c := strings.ToUpper(strings.TrimSpace(s))
	switch c {
	case "", "TL", "YTL", "TRL":
		return "TRY"
	}
	return c
}
