package helper

import (
	"regexp"
	"strings"
)

var (
	taxIDRe = regexp.MustCompile(`(^|[^0-9])(\d{10,11})($|[^0-9])`)
	// Maskeleme kalıpları src-tauri/src/logmask.rs ile aynı amaçta: Luca
	// mesajı sunucuya giderken VKN/TCKN, IBAN, e-posta, telefon ve tutar
	// içermez (tasarım §2.4, §3.1).
	maskEmail  = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	maskIBAN   = regexp.MustCompile(`(?i)\bTR\d{2}(?:\s?\d{4}){5}\s?\d{2}\b`)
	maskPhone  = regexp.MustCompile(`(\+90\s?|\b0)5\d{2}\s?\d{3}\s?\d{2}\s?\d{2}\b`)
	maskTaxID  = regexp.MustCompile(`(^|[^A-Za-z0-9-])(\d{10,11})($|[^A-Za-z0-9-])`)
	maskAmount = regexp.MustCompile(`\b\d{1,3}(?:\.\d{3})+(?:,\d{1,2})?\b|\b\d+,\d{2}\b`)
	spaceRe    = regexp.MustCompile(`\s+`)
)

// digits yalnızca rakamları bırakır.
func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// firstTaxID metindeki ilk 10 ya da 11 haneli sayı (VKN / TCKN).
func firstTaxID(s string) string {
	m := taxIDRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[2]
}

// mask kişisel ve parasal bilgiyi yer tutucuyla değiştirir ve kısaltır.
func mask(s string, limit int) string {
	s = spaceRe.ReplaceAllString(strings.TrimSpace(s), " ")
	s = maskEmail.ReplaceAllString(s, "<e-posta>")
	s = maskIBAN.ReplaceAllString(s, "<iban>")
	s = maskPhone.ReplaceAllString(s, "<telefon>")
	s = maskTaxID.ReplaceAllString(s, "${1}<vkn>${3}")
	s = maskAmount.ReplaceAllString(s, "<tutar>")
	if r := []rune(s); limit > 0 && len(r) > limit {
		s = string(r[:limit])
	}
	return s
}

// clip metni rune sınırında kısaltır (maskelemeden).
func clip(s string, limit int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > limit {
		return string(r[:limit])
	}
	return s
}
