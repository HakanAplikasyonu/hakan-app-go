// Package engine is the account code rule engine (issue #25, docs/00 K6 T1):
// it picks the account code of an invoice line, its VAT or its counterparty
// from the engagement's versioned rules, then from the counterparty's
// history (kural hafızası), then from platform defaults. Every answer
// carries a confidence, a rationale and the rule version that produced it.
// The engine has no SQL; the usecase loads rules and history.
package engine

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ErrInvalid is returned for a malformed rule.
var ErrInvalid = errors.New("accountrules: invalid rule")

// Role is what a rule assigns.
type Role string

const (
	RoleLine         Role = "line"         // satırın gider / gelir / stok hesabı
	RoleVAT          Role = "vat"          // KDV hesabı (191 / 391)
	RoleCounterparty Role = "counterparty" // cari hesap (120 / 320 altı)
)

// Valid reports whether r is known.
func (r Role) Valid() bool { return r == RoleLine || r == RoleVAT || r == RoleCounterparty }

// Origins.
const (
	OriginManual    = "manual"
	OriginCandidate = "candidate"
	OriginTemplate  = "template"
)

// Conditions are ANDed; an empty field matches anything.
type Conditions struct {
	Direction                string   `json:"direction,omitempty"` // incoming / outgoing
	InvoiceType              string   `json:"invoice_type,omitempty"`
	CounterpartyTaxID        string   `json:"counterparty_tax_id,omitempty"`
	CounterpartyNameContains string   `json:"counterparty_name_contains,omitempty"`
	ItemNameContains         []string `json:"item_name_contains,omitempty"` // any of
	LineType                 string   `json:"line_type,omitempty"`
	TaxRate                  string   `json:"tax_rate,omitempty"`
}

var (
	codeRe  = regexp.MustCompile(`^[0-9]{3}(\.[0-9A-Za-z]+)*$`)
	taxIDRe = regexp.MustCompile(`^[0-9]{10,11}$`)
)

// ValidCode reports whether code is a dotted account code.
func ValidCode(code string) bool { return len(code) <= 64 && codeRe.MatchString(code) }

// Validate checks c.
func (c *Conditions) Validate() error {
	bad := func(m string) error { return fmt.Errorf("%w: %s", ErrInvalid, m) }
	c.CounterpartyNameContains = strings.TrimSpace(c.CounterpartyNameContains)
	switch c.Direction {
	case "", "incoming", "outgoing":
	default:
		return bad("direction must be incoming or outgoing")
	}
	if c.CounterpartyTaxID != "" && !taxIDRe.MatchString(c.CounterpartyTaxID) {
		return bad("counterparty_tax_id must be a VKN or TCKN")
	}
	if c.TaxRate != "" {
		if _, err := decimal.NewFromString(c.TaxRate); err != nil {
			return bad("tax_rate must be a number")
		}
	}
	kw := c.ItemNameContains[:0]
	for _, k := range c.ItemNameContains {
		if k = strings.TrimSpace(k); k != "" {
			kw = append(kw, k)
		}
	}
	c.ItemNameContains = kw
	if len(c.ItemNameContains) > 20 {
		return bad("at most 20 keywords")
	}
	return nil
}

// Count is the number of set conditions (specificity).
func (c Conditions) Count() int {
	n := 0
	for _, s := range []string{c.Direction, c.InvoiceType, c.CounterpartyTaxID, c.CounterpartyNameContains, c.LineType, c.TaxRate} {
		if s != "" {
			n++
		}
	}
	if len(c.ItemNameContains) > 0 {
		n++
	}
	return n
}

// Context is what a rule is matched against: one invoice line.
type Context struct {
	Direction         string
	InvoiceType       string
	CounterpartyTaxID string
	CounterpartyName  string
	ItemName          string
	LineType          string
	TaxRate           decimal.Decimal
}

// Match reports whether c holds for x and which conditions matched.
func (c Conditions) Match(x Context) (bool, []string) {
	var why []string
	if c.Direction != "" {
		if c.Direction != x.Direction {
			return false, nil
		}
		why = append(why, "direction="+c.Direction)
	}
	if c.InvoiceType != "" {
		if !strings.EqualFold(c.InvoiceType, x.InvoiceType) {
			return false, nil
		}
		why = append(why, "invoice_type="+c.InvoiceType)
	}
	if c.CounterpartyTaxID != "" {
		if c.CounterpartyTaxID != x.CounterpartyTaxID {
			return false, nil
		}
		why = append(why, "counterparty_tax_id="+c.CounterpartyTaxID)
	}
	if c.CounterpartyNameContains != "" {
		if !strings.Contains(Fold(x.CounterpartyName), Fold(c.CounterpartyNameContains)) {
			return false, nil
		}
		why = append(why, "counterparty_name~"+c.CounterpartyNameContains)
	}
	if len(c.ItemNameContains) > 0 {
		name, hit := Fold(x.ItemName), ""
		for _, k := range c.ItemNameContains {
			if strings.Contains(name, Fold(k)) {
				hit = k
				break
			}
		}
		if hit == "" {
			return false, nil
		}
		why = append(why, "item_name~"+hit)
	}
	if c.LineType != "" {
		if c.LineType != x.LineType {
			return false, nil
		}
		why = append(why, "line_type="+c.LineType)
	}
	if c.TaxRate != "" {
		r, _ := decimal.NewFromString(c.TaxRate)
		if !r.Equal(x.TaxRate) {
			return false, nil
		}
		why = append(why, "tax_rate="+c.TaxRate)
	}
	return true, why
}

var fold = strings.NewReplacer("İ", "i", "I", "ı", "Ş", "ş", "Ğ", "ğ", "Ü", "ü", "Ö", "ö", "Ç", "ç")

// Fold lower-cases with Turkish dotted / dotless i.
func Fold(s string) string { return strings.ToLower(fold.Replace(strings.TrimSpace(s))) }

// Rule is one active rule version.
type Rule struct {
	ID          int64 // 0 for platform defaults
	Key         uuid.UUID
	Version     int
	Role        Role
	Priority    int
	Name        string
	Conditions  Conditions
	AccountCode string
	Confidence  decimal.Decimal
	Origin      string
	// Ref names the rule version in provenance ("account_rule:42@v3",
	// "global:line_sales").
	Ref string
}

// Assignment is the engine's answer.
type Assignment struct {
	Role        Role
	AccountCode string
	Confidence  decimal.Decimal
	Rationale   string
	RuleID      int64
	RuleRef     string
	Matched     []string
	// Tier of the rule that answered (Rule.Tier).
	Tier int
}

// Tier orders the rule levels (docs: "mükellef > şablon > varsayılan",
// #319): the engagement's own rules (written or promoted for this taxpayer)
// always win over the office template copied into the engagement, which
// wins over the platform defaults. Priority only orders rules of one tier.
func (r Rule) Tier() int {
	switch {
	case r.ID == 0:
		return 2
	case r.Origin == OriginTemplate:
		return 1
	default:
		return 0
	}
}

// Pick returns the best matching rule of role: the engagement's own rules
// before the office template before platform defaults (Tier), then lowest
// priority, then the most specific.
func Pick(rules []Rule, role Role, x Context) (Assignment, bool) {
	type cand struct {
		r   Rule
		why []string
	}
	var cs []cand
	for _, r := range rules {
		if r.Role != role {
			continue
		}
		if ok, why := r.Conditions.Match(x); ok {
			cs = append(cs, cand{r, why})
		}
	}
	if len(cs) == 0 {
		return Assignment{}, false
	}
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i].r, cs[j].r
		if at, bt := a.Tier(), b.Tier(); at != bt {
			return at < bt
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if ac, bc := a.Conditions.Count(), b.Conditions.Count(); ac != bc {
			return ac > bc
		}
		return false
	})
	best := cs[0]
	label := best.r.Name
	if label == "" {
		label = best.r.Ref
	}
	why := "always"
	if len(best.why) > 0 {
		why = strings.Join(best.why, ", ")
	}
	return Assignment{
		Role: role, AccountCode: best.r.AccountCode, Confidence: best.r.Confidence,
		RuleID: best.r.ID, RuleRef: best.r.Ref, Matched: best.why, Tier: best.r.Tier(),
		Rationale: fmt.Sprintf("Kural %q (%s) → %s; koşullar: %s", label, best.r.Ref, best.r.AccountCode, why),
	}, true
}
