package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/shopspring/decimal"
)

// PlanAccount is one account under a main code in the target plan (#30).
type PlanAccount struct {
	Code   string
	Name   string
	IsLeaf bool
}

// CounterpartyProposal is a cari account to use or to open.
type CounterpartyProposal struct {
	AccountCode string
	Name        string
	// Existing: the plan already has an account with this name.
	Existing bool
}

// ProposeCounterparty finds the counterparty's cari account under main
// (120 alıcılar / 320 satıcılar) in the plan by name, or proposes the next
// free code next to the existing leaf accounts ("120.01.007" after
// "120.01.006"). With no sub-accounts it proposes main + ".01.001".
func ProposeCounterparty(main, name string, plan []PlanAccount) CounterpartyProposal {
	want := NameKey(name)
	parents := map[string]int{}  // leaf'lerin üst kodu -> leaf sayısı
	lastSeg := map[string]int{}  // üst kod -> en büyük son segment
	segWidth := map[string]int{} // üst kod -> son segment genişliği
	for _, a := range plan {
		if want != "" && a.IsLeaf && NameKey(a.Name) == want {
			return CounterpartyProposal{AccountCode: a.Code, Name: a.Name, Existing: true}
		}
		if !a.IsLeaf || !strings.HasPrefix(a.Code, main+".") {
			continue
		}
		i := strings.LastIndexByte(a.Code, '.')
		parent, seg := a.Code[:i], a.Code[i+1:]
		n, err := strconv.Atoi(seg)
		if err != nil {
			continue
		}
		parents[parent]++
		if n > lastSeg[parent] {
			lastSeg[parent] = n
		}
		if len(seg) > segWidth[parent] {
			segWidth[parent] = len(seg)
		}
	}
	best, bestN := "", 0
	for p, n := range parents {
		if n > bestN || (n == bestN && p < best) {
			best, bestN = p, n
		}
	}
	if best == "" {
		return CounterpartyProposal{AccountCode: main + ".01.001", Name: name}
	}
	return CounterpartyProposal{
		AccountCode: fmt.Sprintf("%s.%0*d", best, segWidth[best], lastSeg[best]+1),
		Name:        name,
	}
}

// NameKey is the form cari names are matched in (#310, #319): Turkish-aware
// lower case, words cut at every non letter / digit, the common company
// abbreviations expanded (nameWords: "San." = "Sanayi", "Tic." = "Ticaret",
// "Ltd. Şti." = "Limited Şirketi", "A.Ş." = "Anonim Şirketi"), then joined
// without separators. So the mizan's "KORKMAZ GIDA SAN. VE TİC. A.Ş."
// matches the invoice's "Korkmaz Gıda Sanayi ve Ticaret Anonim Şirketi" and
// "LTD.ŞTİ." matches "Limited Şirketi". Other words must be the same: a wrong
// cari is worse than a new-account proposal.
func NameKey(s string) string {
	words := strings.FieldsFunc(asciiFold.Replace(Fold(s)), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var b strings.Builder
	for i := 0; i < len(words); i++ {
		w := words[i]
		// "A.Ş." is two words after the cut.
		if w == "a" && i+1 < len(words) && words[i+1] == "s" {
			b.WriteString("anonimsirketi")
			i++
			continue
		}
		if full, ok := nameWords[w]; ok {
			w = full
		}
		b.WriteString(w)
	}
	return b.String()
}

// asciiFold drops the Turkish letters to ASCII after Fold, so a name typed
// without them ("DENIZ AMBALAJ LTD STI") matches the plan's ("DENİZ AMBALAJ
// LTD. ŞTİ."); upper-case ASCII "I" folds to "ı" and must meet "i".
var asciiFold = strings.NewReplacer("ı", "i", "ş", "s", "ğ", "g", "ü", "u", "ö", "o", "ç", "c")

// nameWords expands company-name abbreviations (ASCII-folded, without the
// dot). Only abbreviations that cannot be another word are listed.
var nameWords = map[string]string{
	"san": "sanayi", "sn": "sanayi", "sanayii": "sanayi",
	"tic": "ticaret",
	"ltd": "limited",
	"sti": "sirketi", "sirket": "sirketi",
	"as":   "anonimsirketi",
	"koll": "kollektif", "kom": "komandit",
	"ins": "insaat",
	"paz": "pazarlama", "nak": "nakliyat",
	"ith": "ithalat", "ihr": "ihracat",
	"muh": "muhendislik", "end": "endustri", "elk": "elektrik", "otom": "otomotiv",
}

// vatRateRe finds the rate in a VAT account name: "İNDİRİLECEK KDV %20",
// "KDV % 10", "%1 KDV", "KDV 20%", "yüzde 10".
var vatRateRe = regexp.MustCompile(`(?:%\s*([0-9]+(?:[.,][0-9]+)?))|(?:([0-9]+(?:[.,][0-9]+)?)\s*%)|(?:yüzde\s*([0-9]+(?:[.,][0-9]+)?))`)

// RateInName returns the VAT rate written in an account name.
func RateInName(name string) (decimal.Decimal, bool) {
	m := vatRateRe.FindStringSubmatch(Fold(name))
	if m == nil {
		return decimal.Zero, false
	}
	for _, g := range m[1:] {
		if g != "" {
			d, err := decimal.NewFromString(strings.ReplaceAll(g, ",", "."))
			return d, err == nil
		}
	}
	return decimal.Zero, false
}

// VATSubAccount maps a VAT main account (191 / 391, or a rule's group code)
// to the plan's leaf sub-account for rate (#319): "191" with 20% becomes
// "191.01 İNDİRİLECEK KDV %20". It answers only when exactly one leaf under
// code names that rate; otherwise the caller keeps code.
func VATSubAccount(code string, rate decimal.Decimal, plan []PlanAccount) (PlanAccount, bool) {
	var hit []PlanAccount
	for _, a := range plan {
		if !a.IsLeaf || !strings.HasPrefix(a.Code, code+".") {
			continue
		}
		if r, ok := RateInName(a.Name); ok && r.Equal(rate) {
			hit = append(hit, a)
		}
	}
	if len(hit) != 1 {
		return PlanAccount{}, false
	}
	return hit[0], true
}
