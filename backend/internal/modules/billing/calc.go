// Package billing is the price model of D9 (office base fee + per active
// taxpayer) without collection (D14): plans, monthly active-taxpayer count,
// statement drafts and the read-only AccessPolicy (G7 / A10).
package billing

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// Plan is the priced plan (billing_plans).
type Plan struct {
	Code              string          `json:"code"`
	Name              string          `json:"name"`
	Currency          string          `json:"currency"`
	BaseFee           decimal.Decimal `json:"base_fee"`
	PerTaxpayerFee    decimal.Decimal `json:"per_taxpayer_fee"`
	IncludedTaxpayers int             `json:"included_taxpayers"`
	VATRate           decimal.Decimal `json:"vat_rate"`
	Active            bool            `json:"active"`
}

// Line is one statement line.
type Line struct {
	Code        string          `json:"code"` // base | taxpayer
	Description string          `json:"description"`
	Quantity    int             `json:"quantity"`
	UnitPrice   decimal.Decimal `json:"unit_price"`
	Amount      decimal.Decimal `json:"amount"`
}

// Amounts is a computed statement.
type Amounts struct {
	ActiveTaxpayers   int             `json:"active_taxpayers"`
	BillableTaxpayers int             `json:"billable_taxpayers"`
	BaseFee           decimal.Decimal `json:"base_fee"`
	TaxpayerFeeTotal  decimal.Decimal `json:"taxpayer_fee_total"`
	Subtotal          decimal.Decimal `json:"subtotal"`
	VATRate           decimal.Decimal `json:"vat_rate"`
	VATAmount         decimal.Decimal `json:"vat_amount"`
	Total             decimal.Decimal `json:"total"`
	Lines             []Line          `json:"lines"`
}

var hundred = decimal.NewFromInt(100)

// Compute prices a month: the base fee covers IncludedTaxpayers, every active
// taxpayer above it costs PerTaxpayerFee; KDV on the subtotal (kuruş).
func Compute(p Plan, active int) Amounts {
	billable := active - p.IncludedTaxpayers
	if billable < 0 {
		billable = 0
	}
	a := Amounts{ActiveTaxpayers: active, BillableTaxpayers: billable, BaseFee: p.BaseFee.Round(2), VATRate: p.VATRate}
	a.TaxpayerFeeTotal = p.PerTaxpayerFee.Mul(decimal.NewFromInt(int64(billable))).Round(2)
	a.Subtotal = a.BaseFee.Add(a.TaxpayerFeeTotal)
	a.VATAmount = a.Subtotal.Mul(p.VATRate).Div(hundred).Round(2)
	a.Total = a.Subtotal.Add(a.VATAmount)
	a.Lines = []Line{{Code: "base", Description: p.Name + " planı ofis taban ücreti", Quantity: 1, UnitPrice: a.BaseFee, Amount: a.BaseFee}}
	desc := "Aktif mükellef"
	if p.IncludedTaxpayers > 0 {
		desc = fmt.Sprintf("Aktif mükellef (ilk %d mükellef tabana dahil, %d aktif)", p.IncludedTaxpayers, active)
	}
	a.Lines = append(a.Lines, Line{Code: "taxpayer", Description: desc, Quantity: billable, UnitPrice: p.PerTaxpayerFee.Round(2), Amount: a.TaxpayerFeeTotal})
	return a
}

// PeriodBounds returns [start, next month start) of "YYYY-MM" in Türkiye time.
func PeriodBounds(period string) (time.Time, time.Time, error) {
	t, err := time.ParseInLocation("2006-01", period, istanbul)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("billing: period %q: %w", period, err)
	}
	return t, t.AddDate(0, 1, 0), nil
}

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return time.FixedZone("TRT", 3*3600)
	}
	return loc
}()
