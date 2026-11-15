// Package facts computes the numbers of a period summary (issue #35) from
// the portal data and renders / verifies text that uses them. Every number
// in a summary comes from a Fact; Verify rejects text with a number that is
// not one.
package facts

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// Units.
const (
	UnitTRY     = "TRY"
	UnitCount   = "count"
	UnitPercent = "percent"
)

// Source says where a fact comes from (06 T15: "rakamlar kaynağa atıflı").
type Source struct {
	Table   string `json:"table"`
	Filter  string `json:"filter"`
	Records int64  `json:"records"`
}

// Fact is one number of the summary.
type Fact struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Value    string `json:"value"` // decimal, non-negative
	Unit     string `json:"unit"`
	Rendered string `json:"rendered"`
	Source   Source `json:"source"`
}

// New builds a fact and renders its value.
func New(id, label string, v decimal.Decimal, unit string, src Source) Fact {
	v = v.Abs()
	return Fact{ID: id, Label: label, Value: v.String(), Unit: unit, Rendered: Render(v, unit), Source: src}
}

// Render writes a value the Turkish way: 1.234.567,89 TL · 12 · %12,5.
func Render(v decimal.Decimal, unit string) string {
	switch unit {
	case UnitTRY:
		return groupTR(v.StringFixed(2)) + " TL"
	case UnitPercent:
		return "%" + groupTR(v.StringFixed(1))
	default:
		return groupTR(v.Round(0).String())
	}
}

func groupTR(s string) string {
	intPart, frac, hasFrac := strings.Cut(s, ".")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	if hasFrac {
		b.WriteByte(',')
		b.WriteString(frac)
	}
	return b.String()
}

var placeholderRe = regexp.MustCompile(`\{\{(F\d+)\}\}`)

// Fill replaces {{Fn}} with the rendered facts; unknown ids stay as they are.
func Fill(template string, fs []Fact) string {
	byID := index(fs)
	return placeholderRe.ReplaceAllStringFunc(template, func(m string) string {
		if f, ok := byID[m[2:len(m)-2]]; ok {
			return f.Rendered
		}
		return m
	})
}

// CheckTemplate validates a model-written template: no digits outside
// placeholders, only known placeholders, at least one.
func CheckTemplate(template string, fs []Fact) error {
	byID := index(fs)
	ids := placeholderRe.FindAllStringSubmatch(template, -1)
	if len(ids) == 0 {
		return fmt.Errorf("no {{F..}} placeholder")
	}
	for _, m := range ids {
		if _, ok := byID[m[1]]; !ok {
			return fmt.Errorf("unknown placeholder %s", m[0])
		}
	}
	rest := placeholderRe.ReplaceAllString(template, "")
	if strings.ContainsFunc(rest, func(r rune) bool { return r >= '0' && r <= '9' }) {
		return fmt.Errorf("text has digits outside placeholders")
	}
	return nil
}

var numberRe = regexp.MustCompile(`\d{1,3}(?:\.\d{3})+(?:,\d+)?|\d+(?:,\d+)?`)

// Verify checks that every number in text is one of the facts (or the
// period's year / month, e.g. "Ekim 2026"). It returns the numbers that
// match no fact.
func Verify(text string, fs []Fact, period string) []string {
	allowed := map[string]bool{}
	for _, f := range fs {
		if d, err := decimal.NewFromString(f.Value); err == nil {
			allowed[norm(d)] = true
			// A percent may be written rounded the way it was rendered.
			if f.Unit == UnitPercent {
				allowed[norm(d.Round(1))] = true
			}
		}
	}
	if y, m, ok := strings.Cut(period, "-"); ok {
		allowed[strings.TrimLeft(y, "0")] = true
		allowed[strings.TrimLeft(m, "0")] = true
	}
	var bad []string
	for _, n := range numberRe.FindAllString(text, -1) {
		d, err := decimal.NewFromString(strings.ReplaceAll(strings.ReplaceAll(n, ".", ""), ",", "."))
		if err != nil || !allowed[norm(d)] {
			bad = append(bad, n)
		}
	}
	return bad
}

func norm(d decimal.Decimal) string {
	s := d.String()
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

func index(fs []Fact) map[string]Fact {
	m := make(map[string]Fact, len(fs))
	for _, f := range fs {
		m[f.ID] = f
	}
	return m
}

// Sorted returns facts ordered by numeric id (F1, F2, ... F10).
func Sorted(fs []Fact) []Fact {
	out := append([]Fact(nil), fs...)
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(strings.TrimPrefix(out[i].ID, "F"))
		b, _ := strconv.Atoi(strings.TrimPrefix(out[j].ID, "F"))
		return a < b
	})
	return out
}
