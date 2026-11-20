// Package matching matches bank movements to invoices and runs the bank
// controls (issue #21, docs/kaynak/musavir-entegrasyon-paketi/21-banka-veri-modeli.md §6).
// Candidates come from SQL; scoring and the decision are here, in pure Go.
package matching

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// Thresholds of 21 §6.2.
const (
	AutoScore    = 80 // and a single candidate, 20 ahead of the second
	AutoLead     = 20
	SuggestScore = 50
	// MaxCombination: a payment may close at most this many invoices of one
	// counterparty when their sum is exact.
	MaxCombination = 5
)

var cent = decimal.RequireFromString("0.01")

// Movement is the bank side of a match.
type Movement struct {
	ID          int64
	Date        time.Time
	Direction   banking.Direction
	Amount      decimal.Decimal
	TaxID       string // VKN or TCKN
	IBAN        string
	Name        string
	Description string
}

// Candidate is an open invoice of the same taxpayer and direction.
type Candidate struct {
	ID        int64
	Number    string
	IssueDate time.Time
	DueDate   *time.Time
	// Open is what is still unpaid, in the movement's currency.
	Open  decimal.Decimal
	TaxID string
	IBANs []string
	Name  string
}

// Score is a candidate's total and its parts (kept as evidence, G12).
type Score struct {
	Candidate Candidate
	Total     int
	Parts     map[string]int
}

// Options are office settings.
type Options struct {
	// FeeTolerance: an incoming payment may be short by the sender's EFT
	// fee; up to this much less than the invoice scores 25 (office setting).
	FeeTolerance decimal.Decimal
}

var reInvoiceNo = regexp.MustCompile(`\b[A-Z0-9]{3}20\d{2}\d{9}\b`)

// InvoiceNumbers returns the GİB invoice numbers written in a description.
func InvoiceNumbers(description string) []string {
	return reInvoiceNo.FindAllString(strings.ToUpper(description), -1)
}

// ScoreCandidate applies the 21 §6.2 table.
func ScoreCandidate(m Movement, c Candidate, opt Options) Score {
	s := Score{Candidate: c, Parts: map[string]int{}}

	diff := c.Open.Sub(m.Amount)
	switch {
	case diff.Abs().LessThanOrEqual(cent):
		s.Parts["amount_exact"] = 40
	case m.Direction == banking.DirIn && diff.IsPositive() && diff.LessThanOrEqual(opt.FeeTolerance):
		s.Parts["amount_less_fee"] = 25
	}

	party := 0
	if m.TaxID != "" && m.TaxID == c.TaxID {
		party = 30
		s.Parts["tax_id"] = 30
	}
	if m.IBAN != "" {
		for _, i := range c.IBANs {
			if banking.NormalizeIBAN(i) == m.IBAN {
				if party == 0 {
					s.Parts["iban"] = 30
				} else {
					s.Parts["iban"] = 0 // with the tax id at most 30 together
				}
				break
			}
		}
	}

	for _, n := range InvoiceNumbers(m.Description) {
		if n == strings.ToUpper(c.Number) {
			s.Parts["invoice_number"] = 30
			break
		}
	}

	date := 0
	days := absDays(m.Date, c.IssueDate)
	switch {
	case days == 0:
		date = 10
	case days <= 3:
		date = 6
	case days <= 15:
		date = 3
	}
	if c.DueDate != nil && absDays(m.Date, *c.DueDate) <= 3 && date < 8 {
		date = 8
	}
	if date > 0 {
		s.Parts["date"] = date
	}

	if m.Name != "" && c.Name != "" && NameSimilarity(m.Name, c.Name) >= 0.8 {
		s.Parts["name"] = 10
	}
	for _, v := range s.Parts {
		s.Total += v
	}
	return s
}

// Related: the candidate shares something with the movement beyond amount
// and date (tax id, IBAN, invoice number in the description or the name).
// Only such candidates are suggested (#710).
func Related(s Score) bool {
	for _, k := range []string{"tax_id", "iban", "invoice_number", "name"} {
		if _, ok := s.Parts[k]; ok {
			return true
		}
	}
	return false
}

// Relevant: worth showing on the movement's screen — related, or of the
// same amount. A candidate that only falls in the date window is noise.
func Relevant(s Score) bool {
	if Related(s) {
		return true
	}
	_, exact := s.Parts["amount_exact"]
	_, fee := s.Parts["amount_less_fee"]
	return exact || fee
}

func absDays(a, b time.Time) int {
	d := a.Sub(b).Hours() / 24
	if d < 0 {
		d = -d
	}
	return int(d + 0.5)
}

// legalWords are dropped before comparing names.
var legalWords = map[string]bool{
	"LTD": true, "STI": true, "SIRKETI": true, "AS": true, "A": true, "S": true, "SAN": true, "VE": true,
	"TIC": true, "TICARET": true, "SANAYI": true, "LIMITED": true, "ANONIM": true, "INS": true, "PAZ": true,
}

// NameSimilarity is the token overlap of two normalized names (Jaccard),
// legal-form words ignored: "YILDIZ MARKET LTD STI" ~ "Yıldız Market Ltd. Şti." = 1.
func NameSimilarity(a, b string) float64 {
	ta, tb := tokens(a), tokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	inter := 0
	for t := range ta {
		if tb[t] {
			inter++
		}
	}
	return float64(inter) / float64(len(ta)+len(tb)-inter)
}

func tokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(banking.NormalizeDescription(s)) {
		if !legalWords[w] {
			out[w] = true
		}
	}
	return out
}

// Outcome of a decision.
type Outcome string

const (
	OutcomeAuto    Outcome = "auto"
	OutcomeSuggest Outcome = "suggest"
	OutcomeNone    Outcome = "none"
)

// Decision is what to do with a movement.
type Decision struct {
	Outcome Outcome
	// Picks are the invoices to match: one, or several for a combination.
	Picks  []Score
	Ranked []Score
	// Combination: Picks close several invoices whose sum is the amount.
	Combination bool
}

// Decide ranks the candidates and applies 21 §6.2:
// auto: ≥ 80, a single candidate at that level and ≥ 20 ahead of the second;
// suggest: 50–79, or several strong candidates, or an exact combination;
// none: < 50.
func Decide(m Movement, cands []Candidate, opt Options) Decision {
	ranked := make([]Score, 0, len(cands))
	for _, c := range cands {
		if !c.Open.IsPositive() {
			continue
		}
		ranked = append(ranked, ScoreCandidate(m, c, opt))
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Total != ranked[j].Total {
			return ranked[i].Total > ranked[j].Total
		}
		return ranked[i].Candidate.IssueDate.Before(ranked[j].Candidate.IssueDate) // oldest open first
	})
	d := Decision{Outcome: OutcomeNone, Ranked: ranked}
	if len(ranked) > 0 {
		best := ranked[0]
		second := 0
		if len(ranked) > 1 {
			second = ranked[1].Total
		}
		switch {
		case best.Total >= AutoScore && best.Total-second >= AutoLead:
			d.Outcome, d.Picks = OutcomeAuto, []Score{best}
			return d
		case best.Total >= SuggestScore && Related(best):
			// Amount and date alone are a coincidence (#710): another
			// company's invoice of the same amount is not suggested.
			d.Outcome, d.Picks = OutcomeSuggest, []Score{best}
			return d
		}
	}
	// A bulk payment: invoices of one counterparty whose open amounts add
	// up exactly to the movement.
	if combo := findCombination(m, ranked); combo != nil {
		d.Outcome, d.Picks, d.Combination = OutcomeSuggest, combo, true
	}
	return d
}

func findCombination(m Movement, ranked []Score) []Score {
	groups := map[string][]Score{}
	for _, s := range ranked {
		key := s.Candidate.TaxID
		if key == "" {
			continue
		}
		// Only the movement's own counterparty when it is known.
		if m.TaxID != "" && key != m.TaxID {
			continue
		}
		groups[key] = append(groups[key], s)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g := groups[k]
		if len(g) < 2 {
			continue
		}
		sort.SliceStable(g, func(i, j int) bool { return g[i].Candidate.IssueDate.Before(g[j].Candidate.IssueDate) })
		if len(g) > 12 {
			g = g[:12] // bounds the search: C(12,≤5) is small
		}
		if pick := subsetSum(g, m.Amount, MaxCombination); pick != nil {
			return pick
		}
	}
	return nil
}

func subsetSum(g []Score, target decimal.Decimal, maxN int) []Score {
	var best []Score
	var walk func(i int, acc decimal.Decimal, cur []Score)
	walk = func(i int, acc decimal.Decimal, cur []Score) {
		if best != nil {
			return
		}
		if len(cur) >= 2 && acc.Sub(target).Abs().LessThanOrEqual(cent) {
			best = append([]Score(nil), cur...)
			return
		}
		if i == len(g) || len(cur) == maxN || acc.GreaterThan(target.Add(cent)) {
			return
		}
		walk(i+1, acc.Add(g[i].Candidate.Open), append(cur, g[i]))
		walk(i+1, acc, cur)
	}
	walk(0, decimal.Zero, nil)
	return best
}
