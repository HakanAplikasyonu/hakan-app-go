package rules

import (
	"sort"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/shopspring/decimal"
)

// Codes checked by InvoiceTotals.
const (
	CodeLineTaxable = "LINE_TAXABLE_MISMATCH"
	CodeTaxTotal    = "TAX_TOTAL_MISMATCH"
	CodeBaseTotal   = "BASE_TOTAL_MISMATCH"
	CodePayable     = "PAYABLE_MISMATCH"
	CodeHeaderEst   = "HEADER_ESTIMATE"
)

// InvoiceLine is one invoice line's amounts (31 §5).
type InvoiceLine struct {
	ID            int64
	LineExtension decimal.Decimal
	// Taxable is the KDV TaxableAmount; nil when the line has none.
	Taxable *decimal.Decimal
	// Rate is the KDV rate as written ("20", "10", "1", "0").
	Rate string
	Tax  decimal.Decimal
}

// InvoiceTotalsInput is an invoice's header totals and lines (31 §6).
type InvoiceTotalsInput struct {
	InvoiceID int64
	Period    string
	Lines     []InvoiceLine
	// TaxExclusive is LegalMonetaryTotal/TaxExclusiveAmount (matrah).
	TaxExclusive decimal.Decimal
	// HeaderTax is the header KDV total; HeaderTaxByRate its subtotals.
	HeaderTax       decimal.Decimal
	HeaderTaxByRate map[string]decimal.Decimal
	Withholding     decimal.Decimal // tevkifat
	Stoppage        decimal.Decimal // stopaj
	Rounding        decimal.Decimal // PayableRoundingAmount
	Payable         decimal.Decimal
	// HeaderEstimate is true when the KDV breakdown was estimated from the
	// header because the invoice has no lines (tax_summary_source=header_estimate).
	HeaderEstimate bool
}

// InvoiceTotals checks line/header consistency: LINE_TAXABLE_MISMATCH (per
// line), TAX_TOTAL_MISMATCH, BASE_TOTAL_MISMATCH, PAYABLE_MISMATCH and
// HEADER_ESTIMATE (05 §3.1). Tolerances are the codes' "tolerance" params.
type InvoiceTotals struct{}

// Producer implements engine.Rule.
func (InvoiceTotals) Producer() string { return "control:invoice_totals" }

// Codes implements engine.Rule.
func (InvoiceTotals) Codes() []string {
	return []string{CodeLineTaxable, CodeTaxTotal, CodeBaseTotal, CodePayable, CodeHeaderEst}
}

// Check implements engine.Rule.
func (InvoiceTotals) Check(in InvoiceTotalsInput, cfg map[string]engine.Effective) ([]engine.Outcome, error) {
	id := in.InvoiceID
	inv := engine.Outcome{
		Entity: engine.Entity{Type: catalog.EntityInvoice, ID: &id, Period: in.Period},
		Owned:  []string{CodeTaxTotal, CodeBaseTotal, CodePayable, CodeHeaderEst},
	}
	var outs []engine.Outcome

	if in.HeaderEstimate || len(in.Lines) == 0 {
		inv.Findings = append(inv.Findings, engine.Finding{Code: CodeHeaderEst})
	}

	// Per-line: LineExtensionAmount vs TaxableAmount.
	lineTol, lineTolSet, err := paramDecimal(cfg, CodeLineTaxable, "tolerance")
	if err != nil {
		return nil, err
	}
	for _, l := range in.Lines {
		lid := l.ID
		o := engine.Outcome{
			Entity: engine.Entity{Type: catalog.EntityInvoiceItem, ID: &lid, Period: in.Period},
			Owned:  []string{CodeLineTaxable},
		}
		if lineTolSet && l.Taxable != nil && l.LineExtension.Sub(*l.Taxable).Abs().GreaterThan(lineTol) {
			o.Findings = append(o.Findings, engine.Finding{Code: CodeLineTaxable, Params: map[string]any{
				"line_extension": money(l.LineExtension), "taxable": money(*l.Taxable),
			}})
		}
		outs = append(outs, o)
	}

	if len(in.Lines) > 0 {
		var sumBase, sumTax decimal.Decimal
		byRate := map[string]decimal.Decimal{}
		for _, l := range in.Lines {
			base := l.LineExtension
			if l.Taxable != nil {
				base = *l.Taxable
			}
			sumBase = sumBase.Add(base)
			sumTax = sumTax.Add(l.Tax)
			byRate[l.Rate] = byRate[l.Rate].Add(l.Tax)
		}

		if tol, ok, err := paramDecimal(cfg, CodeTaxTotal, "tolerance"); err != nil {
			return nil, err
		} else if ok {
			params := map[string]any{}
			if sumTax.Sub(in.HeaderTax).Abs().GreaterThan(tol) {
				params["expected"], params["actual"] = money(in.HeaderTax), money(sumTax)
			}
			if rates := rateMismatches(byRate, in.HeaderTaxByRate, tol); len(rates) > 0 {
				params["rates"] = rates
			}
			if len(params) > 0 {
				inv.Findings = append(inv.Findings, engine.Finding{Code: CodeTaxTotal, Params: params})
			}
		}

		if tol, ok, err := paramDecimal(cfg, CodeBaseTotal, "tolerance"); err != nil {
			return nil, err
		} else if ok && sumBase.Sub(in.TaxExclusive).Abs().GreaterThan(tol) {
			inv.Findings = append(inv.Findings, engine.Finding{Code: CodeBaseTotal, Params: map[string]any{
				"expected": money(in.TaxExclusive), "actual": money(sumBase),
			}})
		}
	}

	// Matrah + KDV − tevkifat − stopaj (+ yuvarlama) = ödenecek (05, 31 §6).
	if tol, ok, err := paramDecimal(cfg, CodePayable, "tolerance"); err != nil {
		return nil, err
	} else if ok {
		expected := in.TaxExclusive.Add(in.HeaderTax).Sub(in.Withholding).Sub(in.Stoppage).Add(in.Rounding)
		if expected.Sub(in.Payable).Abs().GreaterThan(tol) {
			inv.Findings = append(inv.Findings, engine.Finding{Code: CodePayable, Params: map[string]any{
				"expected": money(expected), "actual": money(in.Payable),
			}})
		}
	}

	return append([]engine.Outcome{inv}, outs...), nil
}

// rateMismatches compares per-rate line KDV with the header subtotals.
func rateMismatches(lines, header map[string]decimal.Decimal, tol decimal.Decimal) []map[string]string {
	if len(header) == 0 {
		return nil
	}
	rates := map[string]bool{}
	for r := range lines {
		rates[r] = true
	}
	for r := range header {
		rates[r] = true
	}
	keys := make([]string, 0, len(rates))
	for r := range rates {
		keys = append(keys, r)
	}
	sort.Strings(keys)
	var out []map[string]string
	for _, r := range keys {
		if lines[r].Sub(header[r]).Abs().GreaterThan(tol) {
			out = append(out, map[string]string{"rate": r, "expected": money(header[r]), "actual": money(lines[r])})
		}
	}
	return out
}
