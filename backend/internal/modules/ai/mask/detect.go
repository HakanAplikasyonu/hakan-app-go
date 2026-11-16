package mask

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Free-text detectors (issue #169). They are heuristics: they err on the side
// of hiding, and a missed name is still caught when it is registered with
// AddNames.

var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)

// phoneRe matches Turkish numbers written with +90 or a leading 0:
// "0532 123 45 67", "05321234567", "+90 (212) 555-12-34".
var phoneRe = regexp.MustCompile(`(?:\+90[\s-]?|0)\(?[2-5]\d{2}\)?[\s-]?\d{3}[\s-]?\d{2}[\s-]?\d{2}`)

func adjacentDigit(s string, start, end int) bool {
	return (start > 0 && isDigit(rune(s[start-1]))) || (end < len(s) && isDigit(rune(s[end])))
}

func phoneDigits(v string) string {
	var b strings.Builder
	for _, r := range v {
		if isDigit(r) {
			b.WriteRune(r)
		}
	}
	d := b.String()
	// +90 532… and 0532… are the same number.
	d = strings.TrimPrefix(d, "90")
	return strings.TrimPrefix(d, "0")
}

// trRe compiles a case-insensitive pattern in which every i-family letter
// (i ı I İ) matches the others; (?i) alone does not fold them.
func trRe(pattern string) *regexp.Regexp {
	var b strings.Builder
	for _, r := range pattern {
		switch r {
		case 'i', 'ı', 'I', 'İ':
			b.WriteString(`[iıIİ]`)
		default:
			b.WriteRune(r)
		}
	}
	return regexp.MustCompile(`(?i)` + b.String())
}

// legalFormRe matches the legal form that ends a company title.
var legalFormRe = trRe(`(?:ltd\.?\s*şti\.?|limited\s+şirketi|anonim\s+şirketi|koll\.?\s*şti\.?|kollektif\s+şirketi|a\.\s?ş\.?|aş\b|kooperatifi)`)

// labelRe matches a label whose value is a person or company name.
var labelRe = trRe(`(?:^|[^\p{L}])(?:adı?[\s/-]*soyadı?|adı?\s+ve\s+soyadı?|müşteri(?:\s+adı?)?|alıcı|teslim\s+alan|teslim\s+eden|kasiyer|yetkili|[uü]nvanı?|sahibi)\s*:[ \t]*`)

// sayinRe matches the honorific that precedes a name.
var sayinRe = trRe(`(?:^|[^\p{L}])sayın[ \t]+`)

// word is a run of letters in s (byte offsets).
type word struct{ start, end int }

func (w word) text(s string) string { return s[w.start:w.end] }

func words(s string) []word {
	var out []word
	start := -1
	for i, r := range s {
		if unicode.IsLetter(r) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			out = append(out, word{start, i})
			start = -1
		}
	}
	if start >= 0 {
		out = append(out, word{start, len(s)})
	}
	return out
}

func capitalized(w string) bool {
	r, _ := utf8.DecodeRuneInString(w)
	return unicode.IsUpper(r)
}

// gapIsSpace reports whether s[a:b] is only spaces or tabs (no line break).
func gapIsSpace(s string, a, b int) bool {
	if a >= b {
		return false
	}
	for _, r := range s[a:b] {
		if r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

var honorifics = map[string]bool{"bey": true, "hanım": true, "hanim": true, "bay": true, "bayan": true}

// detectNames returns byte ranges of names found by the heuristics.
func detectNames(s string) [][2]int {
	var out [][2]int
	ws := words(s)
	out = append(out, companyTitles(s, ws)...)
	out = append(out, labelledNames(s)...)
	out = append(out, personNames(s, ws)...)
	return out
}

// companyTitles: capitalized words (and "ve", "&", digits, dots) right before
// a legal form, on the same line: "Yılmaz Gıda San. ve Tic. Ltd. Şti.".
func companyTitles(s string, ws []word) [][2]int {
	var out [][2]int
	for _, loc := range legalFormRe.FindAllStringIndex(s, -1) {
		if loc[0] > 0 {
			if r, _ := utf8.DecodeLastRuneInString(s[:loc[0]]); unicode.IsLetter(r) {
				continue // "baş", "Kartaş"
			}
		}
		if loc[1] < len(s) {
			if r, _ := utf8.DecodeRuneInString(s[loc[1]:]); unicode.IsLetter(r) {
				continue
			}
		}
		start := loc[0]
		n := 0
		for i := len(ws) - 1; i >= 0 && n < 10; i-- {
			w := ws[i]
			if w.end > start {
				continue
			}
			gap := s[w.end:start]
			if strings.ContainsAny(gap, "\n\r:;,|") || len(strings.TrimSpace(strings.Trim(gap, ".&-' \t"))) > 0 {
				break
			}
			t := w.text(s)
			if !capitalized(t) && !strings.EqualFold(t, "ve") {
				break
			}
			start = w.start
			n++
		}
		if n > 0 {
			out = append(out, [2]int{start, loc[1]})
		}
	}
	return out
}

// labelledNames: the value after "Ad Soyad:", "Müşteri:" … up to the end of
// the line or a separator, when it starts with a capital letter.
func labelledNames(s string) [][2]int {
	var out [][2]int
	for _, re := range []*regexp.Regexp{labelRe, sayinRe} {
		for _, loc := range re.FindAllStringIndex(s, -1) {
			start := loc[1]
			if start >= len(s) || !capitalized(s[start:]) {
				continue
			}
			end := start
			limit := 80
			if re == sayinRe {
				limit = 3 // words
			}
			if re == labelRe {
				for i, r := range s[start:] {
					if strings.ContainsRune("\n\r,;|", r) || i >= limit {
						break
					}
					end = start + i + utf8.RuneLen(r)
				}
				end = start + len(strings.TrimRight(s[start:end], " \t."))
			} else {
				ws := words(s[start:])
				for k, w := range ws {
					if k >= limit || !capitalized(w.text(s[start:])) || honorifics[strings.ToLower(w.text(s[start:]))] {
						break
					}
					if k > 0 && !gapIsSpace(s[start:], ws[k-1].end, w.start) {
						break
					}
					end = start + w.end
				}
			}
			if end > start {
				out = append(out, [2]int{start, end})
			}
		}
	}
	return out
}

// personNames: a common first name followed by capitalized words on the
// same line ("Ayşe Yılmaz", "MEHMET ALİ KAYA"), or capitalized words right
// before "Bey" / "Hanım".
func personNames(s string, ws []word) [][2]int {
	var out [][2]int
	for i := 0; i < len(ws); i++ {
		t := ws[i].text(s)
		if !capitalized(t) {
			continue
		}
		// "… Ahmet Bey", "Ayşe Hanım"
		if i+1 < len(ws) && honorifics[foldName(ws[i+1].text(s))] && gapIsSpace(s, ws[i].end, ws[i+1].start) {
			start := ws[i].start
			if i > 0 && capitalized(ws[i-1].text(s)) && gapIsSpace(s, ws[i-1].end, ws[i].start) && firstNames[foldName(ws[i-1].text(s))] {
				start = ws[i-1].start
			}
			out = append(out, [2]int{start, ws[i].end})
			continue
		}
		if !firstNames[foldName(t)] {
			continue
		}
		j := i
		for j+1 < len(ws) && j-i < 3 {
			next := ws[j+1].text(s)
			if !capitalized(next) || honorifics[foldName(next)] || !gapIsSpace(s, ws[j].end, ws[j+1].start) {
				break
			}
			j++
		}
		if j > i {
			out = append(out, [2]int{ws[i].start, ws[j].end})
			i = j
		}
	}
	return out
}
