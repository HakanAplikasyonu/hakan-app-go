package mapping

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
)

// SampleRows is how many data rows go to the model (04 §4.3).
const SampleRows = 20

var (
	emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	ibanRe  = regexp.MustCompile(`(?i)\b[A-Z]{2}\d{2}(?:\s?[A-Z0-9]{4}){3,7}(?:\s?[A-Z0-9]{1,4})?\b`)
	// A run of 10+ digits, optionally broken by spaces, dashes, dots or
	// parentheses: VKN, TCKN, phone numbers.
	longDigitsRe = regexp.MustCompile(`\+?\(?\d(?:[\d\s\-().]{8,}\d)`)
)

// vocabulary survives masking: it tells the model what a column holds
// without being personal data.
var vocabulary = map[string]bool{}

func init() {
	for _, w := range []string{"satis", "alis", "iade", "satis iade", "alis iade", "tevkifatli", "tevkifat",
		"e fatura", "efatura", "e arsiv", "earsiv", "kagit", "tl", "try", "usd", "eur", "gbp",
		"evet", "hayir", "nakit", "kredi karti", "havale", "eft", "acik", "kapali", "iptal", "onayli"} {
		vocabulary[w] = true
	}
}

// MaskCell masks a sample cell (04 §4.3): VKN/TCKN, IBAN, phone and e-mail
// keep their shape ("1234******"); other free text (names, addresses)
// becomes a letter shape ("Xxxxx Xxxx"). Numbers, dates and the domain
// vocabulary stay, since the model needs them to recognise a column.
func MaskCell(s string) string {
	if s == "" {
		return s
	}
	s = emailRe.ReplaceAllStringFunc(s, func(e string) string {
		at := strings.IndexByte(e, '@')
		return e[:1] + strings.Repeat("*", at-1) + "@***"
	})
	s = ibanRe.ReplaceAllStringFunc(s, keepPrefix)
	s = longDigitsRe.ReplaceAllStringFunc(s, func(v string) string {
		if countDigits(v) < 10 {
			return v
		}
		return keepPrefix(v)
	})
	if strings.ContainsAny(s, "*") || isNumberOrDate(s) || vocabulary[parse.Normalize(s)] {
		return s
	}
	return shape(s)
}

// MaskSample returns the header and up to SampleRows masked data rows.
func MaskSample(sheet parse.Sheet, l parse.Layout) (headers []string, rows [][]string) {
	for _, c := range l.Columns {
		headers = append(headers, c.Letter+": "+c.Header)
	}
	for i := l.FirstDataRow - 1; i < l.LastDataRow && len(rows) < SampleRows; i++ {
		if i < 0 || i >= len(sheet.Rows) {
			break
		}
		r := sheet.Rows[i]
		out := make([]string, len(l.Columns))
		for k, c := range l.Columns {
			if c.Index < len(r) {
				out[k] = MaskCell(r[c.Index])
			}
		}
		rows = append(rows, out)
	}
	return headers, rows
}

// keepPrefix keeps the first four alphanumerics and stars the rest.
func keepPrefix(v string) string {
	var b strings.Builder
	kept := 0
	for _, r := range v {
		switch {
		case unicode.IsDigit(r) || unicode.IsLetter(r):
			if kept < 4 {
				b.WriteRune(r)
				kept++
			} else {
				b.WriteByte('*')
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func shape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsUpper(r):
			b.WriteByte('X')
		case unicode.IsLetter(r):
			b.WriteByte('x')
		case unicode.IsDigit(r):
			b.WriteByte('9')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func countDigits(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}

func isNumberOrDate(s string) bool {
	if _, ok := parse.ParseDate(s); ok {
		return true
	}
	bare := strings.TrimSpace(strings.NewReplacer("TL", "", "₺", "", "TRY", "").Replace(s))
	if strings.ContainsFunc(bare, unicode.IsLetter) {
		return false
	}
	_, _, ok := parse.ParseAmount(bare, parse.StyleAuto)
	return ok
}
