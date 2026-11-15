// Package build turns mapped sheet rows into common-model invoices and
// validates them (04 §4.4, §4.5, §5). Deterministic: the AI never touches
// values.
package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/shopspring/decimal"
)

// Row results.
const (
	ResultOK      = "ok"
	ResultWarning = "warning"
	ResultError   = "error"
	ResultSkipped = "skipped"
)

// Message codes (row level; data_warning codes where one exists).
const (
	CodeRequired      = "REQUIRED_MISSING"
	CodeBadDate       = "INVALID_DATE"
	CodeBadNumber     = "INVALID_NUMBER"
	CodeOutOfPeriod   = "OUT_OF_PERIOD"
	CodeInvalidTaxID  = "INVALID_TAX_ID"
	CodePayable       = "PAYABLE_MISMATCH"
	CodeTaxTotal      = "TAX_TOTAL_MISMATCH"
	CodeFileDuplicate = "FILE_DUPLICATE"
	CodeNegative      = "NEGATIVE_AMOUNT"
	CodeKindConflict  = "DIRECTION_CONFLICT"
	CodeHeaderEst     = "HEADER_ESTIMATE"
)

var tolerance = decimal.RequireFromString("0.05")

// Message is one row finding.
type Message struct {
	Code     string `json:"code"`
	Field    string `json:"field,omitempty"`
	Severity string `json:"severity"` // error | warning
	Detail   string `json:"detail,omitempty"`
}

// Row is one data row after conversion.
type Row struct {
	RowNo    int               `json:"row_no"` // 1-based sheet row
	Raw      map[string]string `json:"raw"`
	Invoice  *model.Invoice    `json:"invoice,omitempty"`
	Result   string            `json:"result"`
	Messages []Message         `json:"messages"`
}

// Options of a build.
type Options struct {
	// Period (YYYY-MM) chosen for the import; rows outside it get a warning.
	Period string
}

// Invoices converts every data row of the layout.
func Invoices(sheet parse.Sheet, l parse.Layout, m mapping.Mapping, opt Options) []Row {
	cols := m.FieldColumns()
	idx := map[string]int{}
	headers := map[int]string{}
	for _, c := range l.Columns {
		idx[c.Letter] = c.Index
		headers[c.Index] = c.Header
	}
	value := func(r []string, field string) string {
		letter, ok := cols[field]
		if !ok {
			return ""
		}
		i := idx[letter]
		if i < len(r) {
			return r[i]
		}
		return ""
	}
	dataRows := func() [][]string {
		var out [][]string
		for i := l.FirstDataRow - 1; i < l.LastDataRow && i < len(sheet.Rows); i++ {
			if i >= 0 {
				out = append(out, sheet.Rows[i])
			}
		}
		return out
	}()
	// Number style per amount column, decided from all its values.
	styles := map[string]parse.NumberStyle{}
	for field := range cols {
		var vals []string
		for _, r := range dataRows {
			if v := value(r, field); v != "" {
				vals = append(vals, v)
			}
		}
		styles[field] = parse.DetectNumberStyle(vals)
	}
	var rateFields []string
	for field := range cols {
		if _, _, ok := mapping.TaxSummaryField(field); ok {
			rateFields = append(rateFields, field)
		}
	}
	sort.Strings(rateFields)

	direction := model.DirectionOutgoing
	defType := model.TypeSatis
	if m.DataType == mapping.IncomingInvoices {
		direction, defType = model.DirectionIncoming, model.TypeAlis
	}
	source := strings.TrimSpace(m.SourceSystem)
	if source == "" || source == "unknown" {
		source = "excel_unknown"
	}

	seen := map[string]int{}
	out := make([]Row, 0, len(dataRows))
	for k, r := range dataRows {
		row := Row{RowNo: l.FirstDataRow + k, Raw: map[string]string{}}
		for i, v := range r {
			if h, ok := headers[i]; ok && v != "" {
				row.Raw[h] = v
			}
		}
		if len(row.Raw) == 0 {
			continue
		}
		b := builder{row: &row, value: func(f string) string { return value(r, f) }, styles: styles}
		inv := model.Invoice{
			SourceSystem: source, CaptureMethod: model.CaptureExcelImport, Direction: direction, Type: defType,
			DocumentClass: "invoice", Currency: "TRY", TaxSummarySource: model.TaxSummaryFromHeaderEstimate,
		}
		// Date and number.
		if d, ok := b.date("issue_date", true); ok {
			inv.IssueDate, inv.Period = d, d.Format("2006-01")
			if opt.Period != "" && inv.Period != opt.Period {
				b.warn(CodeOutOfPeriod, "issue_date", inv.Period)
			}
		}
		if due, ok := b.date("due_date", false); ok {
			inv.PaymentDueDate = &due
		}
		inv.InvoiceNumber = model.NormalizeInvoiceNumber(b.value("invoice_number"))
		if inv.InvoiceNumber == "" {
			b.err(CodeRequired, "invoice_number", "")
		}
		inv.UUID = strings.ToLower(strings.TrimSpace(b.value("ettn")))
		// Counterparty.
		cp := model.Party{Name: strings.TrimSpace(b.value("customer_name")), TaxOffice: strings.TrimSpace(b.value("customer_tax_office"))}
		if raw := b.value("customer_tax_number"); raw != "" {
			if vkn, tckn, ok := parse.TaxID(raw); ok {
				cp.TaxNumber, cp.IdentityNumber = vkn, tckn
			} else {
				b.warn(CodeInvalidTaxID, "customer_tax_number", "")
			}
		}
		if cp.Name == "" && cp.TaxNumber == "" && cp.IdentityNumber == "" {
			b.err(CodeRequired, "customer_name", "")
		}
		if cp.TaxNumber != "" {
			cp.LegalType = "corporate"
		} else if cp.IdentityNumber != "" {
			cp.LegalType = "individual"
		}
		inv.Counterparty = cp
		// Kind and profile.
		if t := b.value("invoice_type"); t != "" {
			if kind, ok := parse.ParseInvoiceKind(t); ok {
				inv.Type = kind.Type
				if kind.Direction != "" && kind.Direction != string(direction) {
					b.warn(CodeKindConflict, "invoice_type", t)
				}
			}
		}
		if p, ok := parse.Profile(b.value("profile")); ok && p != "" {
			pp := p
			inv.Profile = &pp
		}
		if c := b.value("currency"); c != "" {
			inv.Currency = parse.Currency(c)
		}
		inv.Notes = strings.TrimSpace(b.value("notes"))

		// Amounts.
		excl, hasExcl := b.amount("amounts.tax_exclusive_amount")
		tax, hasTax := b.amount("amounts.tax_amount")
		wh, _ := b.amount("amounts.withholding_amount")
		payable, hasPayable := b.amount("amounts.payable_amount")
		var sums []model.TaxSummary
		byRate := map[string]*model.TaxSummary{}
		for _, f := range rateFields {
			rate, part, _ := mapping.TaxSummaryField(f)
			v, ok := b.amount(f)
			if !ok {
				continue
			}
			s := byRate[rate]
			if s == nil {
				s = &model.TaxSummary{TaxRate: decimal.RequireFromString(rate)}
				byRate[rate] = s
			}
			if part == "base" {
				s.Base = v
			} else {
				s.VAT = v
			}
		}
		var sumVAT, sumBase decimal.Decimal
		for _, rate := range sortedKeys(byRate) {
			s := byRate[rate]
			if s.Base.IsZero() && s.VAT.IsZero() {
				continue
			}
			if s.Base.IsZero() && !s.TaxRate.IsZero() {
				s.Base = s.VAT.Mul(decimal.NewFromInt(100)).Div(s.TaxRate).Round(2)
			}
			sumVAT, sumBase = sumVAT.Add(s.VAT), sumBase.Add(s.Base)
			sums = append(sums, *s)
		}
		if len(sums) > 0 {
			inv.TaxSummarySource = model.TaxSummaryFromSource
			if !hasTax {
				tax = sumVAT
			} else if sumVAT.Sub(tax).Abs().GreaterThan(tolerance) {
				b.warn(CodeTaxTotal, "amounts.tax_amount", fmt.Sprintf("%s ≠ %s", sumVAT.StringFixed(2), tax.StringFixed(2)))
			}
			if !hasExcl {
				excl, hasExcl = sumBase, true
			}
		}
		switch {
		case hasExcl && !hasPayable:
			payable, hasPayable = excl.Add(tax).Sub(wh), true
		case !hasExcl && hasPayable:
			excl, hasExcl = payable.Sub(tax).Add(wh), true
		case !hasExcl && !hasPayable:
			b.err(CodeRequired, "amounts.payable_amount", "")
		}
		if hasExcl && hasPayable && excl.Add(tax).Sub(wh).Sub(payable).Abs().GreaterThan(tolerance) {
			b.warn(CodePayable, "amounts.payable_amount", fmt.Sprintf("%s + %s - %s ≠ %s",
				excl.StringFixed(2), tax.StringFixed(2), wh.StringFixed(2), payable.StringFixed(2)))
		}
		if len(sums) == 0 && hasExcl && excl.IsPositive() {
			rate := model.NearestVATRate(tax.Mul(decimal.NewFromInt(100)).Div(excl))
			sums = []model.TaxSummary{{TaxRate: rate, Base: excl, VAT: tax}}
			b.warn(CodeHeaderEst, "", rate.String())
		}
		if wh.IsPositive() && len(sums) > 0 {
			sums[0].Withholding = wh
		}
		inv.Amounts = model.Amounts{
			LineExtensionAmount: excl, TaxExclusiveAmount: excl, TaxAmount: tax, TaxInclusiveAmount: excl.Add(tax),
			WithholdingAmount: wh, PayableAmount: payable,
		}
		if inv.Currency == "TRY" {
			inv.Amounts.PayableAmountTRY = payable
		}
		inv.TaxSummaries = sums

		// Source id (04 §5) and content hash.
		party := cp.TaxNumber + cp.IdentityNumber
		if party == "" {
			party = parse.Normalize(cp.Name)
		}
		if id := strings.TrimSpace(b.value("source_record_id")); id != "" {
			inv.SourceID = id
		} else {
			inv.SourceID = inv.InvoiceNumber + "|" + party + "|" + inv.IssueDate.Format("2006-01-02")
		}
		rawJSON, _ := json.Marshal(row.Raw)
		sum := sha256.Sum256(rawJSON)
		inv.ContentHash = hex.EncodeToString(sum[:])
		inv.RawPayload = map[string]any{"excel_row": row.RowNo, "columns": row.Raw}

		// In-file duplicate: same number + counterparty (04 §4.5).
		dupKey := inv.InvoiceNumber + "|" + party
		if inv.InvoiceNumber != "" {
			if first, ok := seen[dupKey]; ok {
				b.err(CodeFileDuplicate, "invoice_number", fmt.Sprintf("row %d", first))
			} else {
				seen[dupKey] = row.RowNo
			}
		}
		row.Invoice = &inv
		row.Result = ResultOK
		for _, msg := range row.Messages {
			if msg.Severity == ResultError {
				row.Result = ResultError
				break
			}
			row.Result = ResultWarning
		}
		if row.Messages == nil {
			row.Messages = []Message{}
		}
		out = append(out, row)
	}
	return out
}

type builder struct {
	row    *Row
	value  func(string) string
	styles map[string]parse.NumberStyle
}

func (b builder) err(code, field, detail string) {
	b.row.Messages = append(b.row.Messages, Message{Code: code, Field: field, Severity: ResultError, Detail: detail})
}

func (b builder) warn(code, field, detail string) {
	b.row.Messages = append(b.row.Messages, Message{Code: code, Field: field, Severity: ResultWarning, Detail: detail})
}

func (b builder) date(field string, required bool) (time.Time, bool) {
	v := b.value(field)
	if v == "" {
		if required {
			b.err(CodeRequired, field, "")
		}
		return time.Time{}, false
	}
	d, ok := parse.ParseDate(v)
	if !ok {
		b.err(CodeBadDate, field, v)
	}
	return d, ok
}

func (b builder) amount(field string) (decimal.Decimal, bool) {
	v := b.value(field)
	if v == "" {
		return decimal.Zero, false
	}
	d, neg, ok := parse.ParseAmount(v, b.styles[field])
	if !ok {
		b.err(CodeBadNumber, field, v)
		return decimal.Zero, false
	}
	if neg {
		b.warn(CodeNegative, field, v)
	}
	return d, true
}

func sortedKeys(m map[string]*model.TaxSummary) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, _ := decimal.NewFromString(keys[i])
		b, _ := decimal.NewFromString(keys[j])
		return a.LessThan(b)
	})
	return keys
}

// Summary is the preview (04 §5) before database checks.
type Summary struct {
	Rows      int         `json:"rows"`
	OK        int         `json:"ok"`
	Warnings  int         `json:"warnings"`
	Errors    int         `json:"errors"`
	Skipped   int         `json:"skipped"`
	Total     string      `json:"total_payable"`
	VATByRate []RateTotal `json:"vat_by_rate"`
}

// RateTotal is one rate of the VAT summary.
type RateTotal struct {
	Rate        string `json:"rate"`
	Base        string `json:"base"`
	VAT         string `json:"vat"`
	Withholding string `json:"withholding"`
}

// Summarize totals the importable (ok / warning) rows.
func Summarize(rows []Row) Summary {
	s := Summary{Rows: len(rows), VATByRate: []RateTotal{}}
	total := decimal.Zero
	type agg struct{ base, vat, wh decimal.Decimal }
	rates := map[string]*agg{}
	for _, r := range rows {
		switch r.Result {
		case ResultError:
			s.Errors++
			continue
		case ResultSkipped:
			s.Skipped++
			continue
		case ResultWarning:
			s.Warnings++
		default:
			s.OK++
		}
		if r.Invoice == nil {
			continue
		}
		total = total.Add(r.Invoice.Amounts.PayableAmount)
		for _, t := range r.Invoice.TaxSummaries {
			k := t.TaxRate.String()
			a := rates[k]
			if a == nil {
				a = &agg{}
				rates[k] = a
			}
			a.base, a.vat, a.wh = a.base.Add(t.Base), a.vat.Add(t.VAT), a.wh.Add(t.Withholding)
		}
	}
	s.Total = total.StringFixed(2)
	keys := make([]string, 0, len(rates))
	for k := range rates {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, _ := decimal.NewFromString(keys[i])
		b, _ := decimal.NewFromString(keys[j])
		return a.LessThan(b)
	})
	for _, k := range keys {
		a := rates[k]
		s.VATByRate = append(s.VATByRate, RateTotal{Rate: k, Base: a.base.StringFixed(2), VAT: a.vat.StringFixed(2), Withholding: a.wh.StringFixed(2)})
	}
	return s
}
