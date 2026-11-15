package rules

import (
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
	Amount    decimal.Decimal
	// History is the counterparty's earlier invoice amounts (or the
	// taxpayer's, when the counterparty has none: 05 §3.3).
	History []decimal.Decimal
	// FirstSeen is true when the counterparty appears for the first time.
	FirstSeen bool
}

// Amounts checks UNUSUAL_AMOUNT (params multiplier, min_history) and
// NEW_PARTY_HIGH_AMOUNT (param limit). A null threshold skips the check.
type Amounts struct{}

// Producer implements engine.Rule.
func (Amounts) Producer() string { return "control:amounts" }

// Codes implements engine.Rule.
func (Amounts) Codes() []string { return []string{CodeUnusualAmount, CodeNewPartyHigh} }

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
	if multSet && len(in.History) > 0 && (!minSet || decimal.NewFromInt(int64(len(in.History))).GreaterThanOrEqual(minHist)) {
		sum := decimal.Zero
		for _, h := range in.History {
			sum = sum.Add(h)
		}
		avg := sum.Div(decimal.NewFromInt(int64(len(in.History))))
		if avg.IsPositive() && in.Amount.GreaterThan(avg.Mul(mult)) {
			o.Findings = append(o.Findings, engine.Finding{Code: CodeUnusualAmount, Params: map[string]any{
				"amount": money(in.Amount), "average": money(avg), "multiplier": mult.String(),
			}})
		}
	}
	if limit, ok, err := paramDecimal(cfg, CodeNewPartyHigh, "limit"); err != nil {
		return nil, err
	} else if ok && in.FirstSeen && in.Amount.GreaterThan(limit) {
		o.Findings = append(o.Findings, engine.Finding{Code: CodeNewPartyHigh, Params: map[string]any{
			"amount": money(in.Amount), "limit": money(limit),
		}})
	}
	return []engine.Outcome{o}, nil
}

// Compile-time checks.
var (
	_ engine.Rule[InvoiceTotalsInput] = InvoiceTotals{}
	_ engine.Rule[TaxIDInput]         = TaxID{}
	_ engine.Rule[AmountInput]        = Amounts{}
)
