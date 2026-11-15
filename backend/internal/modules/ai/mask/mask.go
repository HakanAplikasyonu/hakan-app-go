// Package mask replaces personal and tax identifiers with stable placeholder
// tokens before text leaves the server for an AI provider, and restores them
// in the provider's answer (docs/01 G11, KVKK).
//
// Masked kinds:
//   - IBAN: country code + 2 check digits + 11–30 alphanumerics, spaces allowed
//     in groups of four (TR IBANs are 26 characters).
//   - TCKN: a standalone run of exactly 11 digits that does not start with 0.
//   - VKN: a standalone run of exactly 10 digits.
//   - Name: every name registered with AddNames (case-insensitive), e.g. the
//     taxpayer's title, its counterparties and their contacts.
//
// "Standalone" means not adjacent to another digit, so "VKN:1234567890" is
// masked while a 12-digit invoice number is not. Masking errs on the side of
// hiding: a 10-digit phone number written without its leading 0 is masked as a
// VKN too, which costs the model nothing.
//
// The same value always maps to the same token within one Masker, so the
// model can still reason about "the same party". A Masker is per call (or per
// conversation); it is safe for concurrent use.
package mask

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// Kind is a masked value class.
type Kind string

const (
	KindVKN  Kind = "VKN"
	KindTCKN Kind = "TCKN"
	KindIBAN Kind = "IBAN"
	KindName Kind = "AD"
)

// ibanRe matches IBAN-shaped strings, optionally grouped by spaces.
var ibanRe = regexp.MustCompile(`(?i)\b[A-Z]{2}\d{2}(?:[ ]?[A-Z0-9]{4}){2,7}(?:[ ]?[A-Z0-9]{1,4})?\b`)

// tokenRe matches the tokens this package produces.
var tokenRe = regexp.MustCompile(`\[(VKN|TCKN|IBAN|AD)_(\d+)\]`)

// Masker masks and unmasks text.
type Masker struct {
	mu       sync.Mutex
	forward  map[string]string // kind|normalized value -> token
	reverse  map[string]string // token -> original value (first spelling seen)
	counters map[Kind]int
	names    []string
	nameRe   *regexp.Regexp
}

// New returns a Masker that also masks the given names.
func New(names ...string) *Masker {
	m := &Masker{
		forward:  map[string]string{},
		reverse:  map[string]string{},
		counters: map[Kind]int{},
	}
	m.AddNames(names...)
	return m
}

// AddNames registers names (person or company titles) to mask. Names shorter
// than three letters are ignored: they would mask ordinary words.
func (m *Masker) AddNames(names ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[string]bool{}
	for _, n := range m.names {
		seen[strings.ToLower(n)] = true
	}
	for _, n := range names {
		n = strings.Join(strings.Fields(n), " ")
		if len([]rune(n)) < 3 || seen[strings.ToLower(n)] {
			continue
		}
		seen[strings.ToLower(n)] = true
		m.names = append(m.names, n)
	}
	if len(m.names) == 0 {
		m.nameRe = nil
		return
	}
	// Longest first so "Ayşe Yılmaz Ltd" wins over "Ayşe Yılmaz".
	sorted := append([]string(nil), m.names...)
	sort.SliceStable(sorted, func(i, j int) bool { return len(sorted[i]) > len(sorted[j]) })
	parts := make([]string, len(sorted))
	for i, n := range sorted {
		// Whitespace inside a name matches any run of whitespace.
		words := strings.Fields(n)
		for k, w := range words {
			words[k] = namePattern(w)
		}
		parts[i] = strings.Join(words, `\s+`)
	}
	m.nameRe = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])(` + strings.Join(parts, "|") + `)(?:[^\p{L}\p{N}]|$)`)
}

// Mask returns s with every identifier replaced by its token.
func (m *Masker) Mask(s string) string {
	if s == "" {
		return s
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s = ibanRe.ReplaceAllStringFunc(s, func(v string) string {
		norm := strings.ToUpper(strings.ReplaceAll(v, " ", ""))
		if !looksLikeIBAN(norm) {
			return v
		}
		return m.token(KindIBAN, norm, v)
	})
	s = m.maskDigitRuns(s)
	if m.nameRe != nil {
		s = m.replaceNames(s)
	}
	return s
}

// Unmask restores the original values of tokens in s. Unknown tokens are left
// as they are.
func (m *Masker) Unmask(s string) string {
	if s == "" || !strings.Contains(s, "[") {
		return s
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return tokenRe.ReplaceAllStringFunc(s, func(tok string) string {
		if v, ok := m.reverse[tok]; ok {
			return v
		}
		return tok
	})
}

// Counts returns how many distinct values of each kind were masked.
func (m *Masker) Counts() map[Kind]int {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[Kind]int, len(m.counters))
	for k, v := range m.counters {
		out[k] = v
	}
	return out
}

// Total returns the number of distinct masked values.
func (m *Masker) Total() int {
	n := 0
	for _, v := range m.Counts() {
		n += v
	}
	return n
}

func (m *Masker) token(kind Kind, norm, original string) string {
	key := string(kind) + "|" + norm
	if tok, ok := m.forward[key]; ok {
		return tok
	}
	m.counters[kind]++
	tok := fmt.Sprintf("[%s_%d]", kind, m.counters[kind])
	m.forward[key] = tok
	m.reverse[tok] = original
	return tok
}

// maskDigitRuns masks maximal digit runs of length 10 (VKN) or 11 (TCKN).
// Digits inside an existing token are skipped.
func (m *Masker) maskDigitRuns(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	rs := []rune(s)
	for i := 0; i < len(rs); {
		if !isDigit(rs[i]) || insideToken(rs, i) {
			b.WriteRune(rs[i])
			i++
			continue
		}
		j := i
		for j < len(rs) && isDigit(rs[j]) {
			j++
		}
		run := string(rs[i:j])
		switch {
		case len(run) == 10:
			b.WriteString(m.token(KindVKN, run, run))
		case len(run) == 11 && run[0] != '0':
			b.WriteString(m.token(KindTCKN, run, run))
		default:
			b.WriteString(run)
		}
		i = j
	}
	return b.String()
}

func (m *Masker) replaceNames(s string) string {
	// The regex consumes one boundary rune on each side; loop so adjacent
	// names separated by a single rune are all replaced.
	for {
		loc := m.nameRe.FindStringSubmatchIndex(s)
		if loc == nil {
			return s
		}
		start, end := loc[2], loc[3]
		v := s[start:end]
		norm := foldName(v)
		s = s[:start] + m.token(KindName, norm, v) + s[end:]
	}
}

// foldName normalizes a name for token lookup: single spaces, lower case,
// Turkish i variants folded together.
func foldName(v string) string {
	v = strings.Join(strings.Fields(v), " ")
	v = strings.NewReplacer("İ", "i", "I", "i", "ı", "i").Replace(v)
	return strings.ToLower(v)
}

// namePattern quotes w and makes the Turkish i / ı / İ / I letters match
// each other, which (?i) alone does not do.
func namePattern(w string) string {
	var b strings.Builder
	for _, r := range w {
		switch r {
		case 'i', 'ı', 'I', 'İ':
			b.WriteString(`[iıIİ]`)
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return b.String()
}

// insideToken reports whether position i sits inside "[KIND_n]".
func insideToken(rs []rune, i int) bool {
	j := i
	for j > 0 && isDigit(rs[j-1]) {
		j--
	}
	return j > 0 && rs[j-1] == '_'
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// looksLikeIBAN checks length and the ISO 13616 mod-97 checksum. A failed
// checksum still masks when the country is TR and the length is right: a
// mistyped IBAN is still personal data.
func looksLikeIBAN(v string) bool {
	if len(v) < 15 || len(v) > 34 {
		return false
	}
	if strings.HasPrefix(v, "TR") && len(v) == 26 {
		return true
	}
	return mod97(v) == 1
}

func mod97(v string) int {
	rearranged := v[4:] + v[:4]
	rem := 0
	for _, r := range rearranged {
		switch {
		case unicode.IsDigit(r):
			rem = (rem*10 + int(r-'0')) % 97
		case r >= 'A' && r <= 'Z':
			n := int(r-'A') + 10
			rem = (rem*100 + n) % 97
		default:
			return -1
		}
	}
	return rem
}
