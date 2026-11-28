package rules

import (
	"sort"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/shopspring/decimal"
)

// Codes checked by the party rules.
const (
	CodeInvalidTaxID  = "INVALID_TAX_ID"
	CodeUnusualAmount = "UNUSUAL_AMOUNT"
	CodeNewPartyHigh  = "NEW_PARTY_HIGH_AMOUNT"
)

// TaxIDInput is a counterparty identifier on an invoice or customer.
type TaxIDInput struct {
	EntityType string // invoice or customer
	EntityID   int64
	Period     string
	TaxID      string
}

// TaxID checks INVALID_TAX_ID: a VKN/TCKN must be 10 or 11 digits (01 §7).
type TaxID struct{}

// Producer implements engine.Rule.
func (TaxID) Producer() string { return "control:tax_id" }

// Codes implements engine.Rule.
func (TaxID) Codes() []string { return []string{CodeInvalidTaxID} }

// Check implements engine.Rule.
func (TaxID) Check(in TaxIDInput, _ map[string]engine.Effective) ([]engine.Outcome, error) {
	id := in.EntityID
	o := engine.Outcome{
		Entity: engine.Entity{Type: in.EntityType, ID: &id, Period: in.Period},
		Owned:  []string{CodeInvalidTaxID},
	}
	if !validTaxID(in.TaxID) {
		o.Findings = []engine.Finding{{Code: CodeInvalidTaxID, Params: map[string]any{"length": len(in.TaxID)}}}
	}
	return []engine.Outcome{o}, nil
}

func validTaxID(s string) bool {
	if len(s) != 10 && len(s) != 11 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// AmountInput is an invoice amount with the counterparty's history.
type AmountInput struct {
	InvoiceID int64
	Period    string
	// Direction is incoming (alış) or outgoing (satış); the warning text
	// names it.
	Direction string
	Amount    decimal.Decimal
	// PartyHistory is the same counterparty's invoice amounts in the same
	// direction over the 12 months before the invoice.
	PartyHistory []decimal.Decimal
	// DirectionHistory is the taxpayer's own invoice amounts in the same
	// direction over the same 12 months: the basis when the counterparty has
	// too little history (05 §3.3, "yoksa mükellefin").
	DirectionHistory []decimal.Decimal
	// FirstSeen is true when the counterparty appears for the first time.
	FirstSeen bool
}

// Amounts checks UNUSUAL_AMOUNT and NEW_PARTY_HIGH_AMOUNT (06 T3).
//
// UNUSUAL_AMOUNT (params multiplier, min_history, min_amount): the amount is
// above multiplier × the median of the basis and above min_amount. The basis
// is the counterparty's last 12 months when it has at least min_history
// invoices, else the taxpayer's same-direction last 12 months; with less
// history than that the check does not run. The median, not the average, so
// one earlier outlier does not hide the next.
//
// NEW_PARTY_HIGH_AMOUNT (param limit): the counterparty is seen for the first
// time and the amount is above limit.
//
// A null threshold skips its check (multiplier, limit) or its condition
// (min_history, min_amount).
type Amounts struct{}

// Producer implements engine.Rule.
func (Amounts) Producer() string { return "control:amounts" }

// Codes implements engine.Rule.
func (Amounts) Codes() []string { return []string{CodeUnusualAmount, CodeNewPartyHigh} }

// Bases of an UNUSUAL_AMOUNT comparison (param "basis").
const (
	BasisParty     = "party"
	BasisDirection = "direction"
)

// Check implements engine.Rule.
func (Amounts) Check(in AmountInput, cfg map[string]engine.Effective) ([]engine.Outcome, error) {
	id := in.InvoiceID
	o := engine.Outcome{
		Entity: engine.Entity{Type: catalog.EntityInvoice, ID: &id, Period: in.Period},
		Owned:  []string{CodeUnusualAmount, CodeNewPartyHigh},
	}
	mult, multSet, err := paramDecimal(cfg, CodeUnusualAmount, "multiplier")
	if err != nil {
		return nil, err
	}
	minHist, minSet, err := paramDecimal(cfg, CodeUnusualAmount, "min_history")
	if err != nil {
		return nil, err
	}
	minAmount, minAmountSet, err := paramDecimal(cfg, CodeUnusualAmount, "min_amount")
	if err != nil {
		return nil, err
	}
	enough := func(h []decimal.Decimal) bool {
		if len(h) == 0 {
			return false
		}
		return !minSet || decimal.NewFromInt(int64(len(h))).GreaterThanOrEqual(minHist)
	}
	basis, hist := "", []decimal.Decimal(nil)
	switch {
	case enough(in.PartyHistory):
		basis, hist = BasisParty, in.PartyHistory
	case enough(in.DirectionHistory):
		basis, hist = BasisDirection, in.DirectionHistory
	}
	if multSet && basis != "" && (!minAmountSet || in.Amount.GreaterThan(minAmount)) {
		st := statsOf(hist)
		if st.median.IsPositive() && in.Amount.GreaterThan(st.median.Mul(mult)) {
			params := map[string]any{
				"amount": money(in.Amount), "median": money(st.median),
				"min": money(st.min), "max": money(st.max), "count": len(hist),
				"multiplier": mult.String(), "basis": basis, "direction": in.Direction,
				// variant picks the text: who was compared, in which direction.
				"variant": basis + "_" + in.Direction,
			}
			if minAmountSet {
				params["min_amount"] = money(minAmount)
			}
			o.Findings = append(o.Findings, engine.Finding{Code: CodeUnusualAmount, Params: params})
		}
	}
	if limit, ok, err := paramDecimal(cfg, CodeNewPartyHigh, "limit"); err != nil {
		return nil, err
	} else if ok && in.FirstSeen && in.Amount.GreaterThan(limit) {
		o.Findings = append(o.Findings, engine.Finding{Code: CodeNewPartyHigh, Params: map[string]any{
			"amount": money(in.Amount), "limit": money(limit), "direction": in.Direction,
		}})
	}
	return []engine.Outcome{o}, nil
}

type amountStats struct{ min, max, median decimal.Decimal }

// statsOf returns the minimum, maximum and median of a non-empty list.
func statsOf(h []decimal.Decimal) amountStats {
	s := make([]decimal.Decimal, len(h))
	copy(s, h)
	sort.Slice(s, func(i, j int) bool { return s[i].LessThan(s[j]) })
	n := len(s)
	med := s[n/2]
	if n%2 == 0 {
		med = s[n/2-1].Add(s[n/2]).Div(decimal.NewFromInt(2))
	}
	return amountStats{min: s[0], max: s[n-1], median: med}
}

// Compile-time checks.
var (
	_ engine.Rule[InvoiceTotalsInput] = InvoiceTotals{}
	_ engine.Rule[TaxIDInput]         = TaxID{}
	_ engine.Rule[AmountInput]        = Amounts{}
	_ engine.Rule[OutOfPeriodInput]   = OutOfPeriod{}
)
