package engine

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
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

// NameKey is the form cari names are matched in (#310): Turkish-aware lower
// case with only letters and digits kept, so the mizan's "KORKMAZ GIDA SAN.
// VE TİC. A.Ş." matches the invoice's "Korkmaz Gıda San. ve Tic. A.Ş." and
// "LTD. ŞTİ." matches "LTD.ŞTİ.". Abbreviation variants ("Sanayi" vs "San.")
// still differ on purpose: a wrong cari is worse than a new-account proposal.
func NameKey(s string) string {
	var b strings.Builder
	for _, r := range Fold(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
