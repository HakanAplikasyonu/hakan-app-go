package taxreturns

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

// StampLine is one manual line of the damga vergisi beyannamesi: a taxable
// document (kağıt) of the period. Nispi: Base × RatePermille / 1000; maktu:
// Tax is entered (Base and RatePermille stay empty).
type StampLine struct {
	Document string `json:"document"`
	Count    int64  `json:"count"`
	Base     *Money `json:"base,omitempty"`
	// RatePermille is the rate in binde ("9.48" = ‰9,48).
	RatePermille *string `json:"rate_permille,omitempty"`
	Tax          *Money  `json:"tax,omitempty"`
}

// StampAdjustments are the manual fields of a damga draft.
type StampAdjustments struct {
	Lines []StampLine `json:"stamp_lines,omitempty"`
}

// Limits of the manual lines.
const (
	maxStampLines    = 50
	maxStampDocument = 120
	maxStampCount    = 100000
)

var maxStampRate = decimal.NewFromInt(20)

// ParseStampAdjustments validates the adjustments of a damga draft.
func ParseStampAdjustments(raw []byte) (StampAdjustments, error) {
	var a StampAdjustments
	if len(raw) == 0 || string(raw) == "null" {
		return a, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return a, fmt.Errorf("%w: adjustments: %v", ErrInvalidInput, err)
	}
	if len(a.Lines) > maxStampLines {
		return a, fmt.Errorf("%w: at most %d stamp lines", ErrInvalidInput, maxStampLines)
	}
	for i := range a.Lines {
		l := &a.Lines[i]
		l.Document = strings.TrimSpace(l.Document)
		if l.Document == "" || utf8.RuneCountInString(l.Document) > maxStampDocument {
			return a, fmt.Errorf("%w: stamp line %d: document", ErrInvalidInput, i+1)
		}
		if l.Count < 1 || l.Count > maxStampCount {
			return a, fmt.Errorf("%w: stamp line %d: count", ErrInvalidInput, i+1)
		}
		if l.RatePermille != nil && strings.TrimSpace(*l.RatePermille) == "" {
			l.RatePermille = nil
		}
		nispi := l.Base != nil || l.RatePermille != nil
		switch {
		case nispi && l.Tax != nil:
			return a, fmt.Errorf("%w: stamp line %d: either base and rate or tax", ErrInvalidInput, i+1)
		case nispi:
			if l.Base == nil || l.RatePermille == nil {
				return a, fmt.Errorf("%w: stamp line %d: base and rate are both required", ErrInvalidInput, i+1)
			}
			r, err := decimal.NewFromString(strings.TrimSpace(*l.RatePermille))
			if err != nil || !r.IsPositive() || r.GreaterThan(maxStampRate) || r.Exponent() < -3 {
				return a, fmt.Errorf("%w: stamp line %d: rate", ErrInvalidInput, i+1)
			}
			s := r.String()
			l.RatePermille = &s
			if l.Base.Dec().IsNegative() {
				return a, fmt.Errorf("%w: stamp line %d: base", ErrInvalidInput, i+1)
			}
		case l.Tax == nil:
			return a, fmt.Errorf("%w: stamp line %d: tax or base and rate", ErrInvalidInput, i+1)
		default:
			if l.Tax.Dec().IsNegative() {
				return a, fmt.Errorf("%w: stamp line %d: tax", ErrInvalidInput, i+1)
			}
		}
	}
	return a, nil
}

// StampInput is everything ComputeStamp needs.
type StampInput struct {
	Adjustments StampAdjustments
	// PayrollStampTax is the period's payroll stamp tax (net of exemption):
	// declared on MUHSGK, never here.
	PayrollStampTax decimal.Decimal
}

// StampRow is one computed line.
type StampRow struct {
	Document     string  `json:"document"`
	Count        int64   `json:"count"`
	Base         *Money  `json:"base"`
	RatePermille *string `json:"rate_permille"`
	// Fixed: maktu (the tax was entered).
	Fixed bool  `json:"fixed"`
	Tax   Money `json:"tax"`
}

// StampTotals are the declaration totals.
type StampTotals struct {
	Count   int64 `json:"count"`
	Base    Money `json:"base"`
	Payable Money `json:"payable"`
}

// Stamp is the damga vergisi beyannamesi preview (#575).
type Stamp struct {
	Rows   []StampRow  `json:"rows"`
	Totals StampTotals `json:"totals"`
}

var thousand = decimal.NewFromInt(1000)

// ComputeStamp builds the damga vergisi beyannamesi from the manual lines.
// Rules (488 sayılı DVK):
//
//   - Nispi: tax = matrah × binde oran / 1000, rounded to kuruş per line.
//   - Maktu: the entered tax.
//   - Payable = Σ tax. The return is due on the 26th of the following month.
//   - Muhtasar filers declare damga vergisi on MUHSGK; this return is for
//     taxpayers who do not file MUHSGK (or for documents outside it).
func ComputeStamp(in StampInput) (Stamp, []CheckItem) {
	out := Stamp{Rows: []StampRow{}}
	var count int64
	var base, total decimal.Decimal
	for _, l := range in.Adjustments.Lines {
		r := StampRow{Document: l.Document, Count: l.Count}
		var tax decimal.Decimal
		if l.Tax != nil {
			r.Fixed, tax = true, l.Tax.Dec()
		} else if l.Base != nil && l.RatePermille != nil {
			rate, _ := decimal.NewFromString(*l.RatePermille)
			b := M(l.Base.Dec())
			r.Base, r.RatePermille = &b, l.RatePermille
			tax = l.Base.Dec().Mul(rate).Div(thousand).Round(2)
			base = base.Add(l.Base.Dec())
		}
		r.Tax = M(tax)
		count += l.Count
		total = total.Add(tax.Round(2))
		out.Rows = append(out.Rows, r)
	}
	out.Totals = StampTotals{Count: count, Base: M(base), Payable: M(total)}

	var c []CheckItem
	if len(out.Rows) == 0 {
		c = append(c, item("NO_STAMP_DOCUMENTS", SeverityInfo,
			"Bu dönem için damga vergisine tabi kağıt girilmedi; satır eklemezseniz beyan verilmez.", 0))
	}
	if in.PayrollStampTax.IsPositive() {
		c = append(c, itemAmount("PAYROLL_STAMP_IN_MUHSGK", SeverityWarning,
			"Bu dönem bordroda damga vergisi var; muhtasar veren mükellef damga vergisini MUHSGK'da beyan eder. Bu beyanı yalnızca MUHSGK dışındaki kağıtlar için verin.", 0, in.PayrollStampTax))
	}
	return out, c
}
