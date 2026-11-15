package kolaybi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources"
)

// SourceSystem of everything read from KolayBi.
const SourceSystem = "kolaybi"

var hundred = decimal.NewFromInt(100)

type obj map[string]any

func (o obj) sub(k string) obj {
	if m, ok := o[k].(map[string]any); ok {
		return obj(m)
	}
	return obj{}
}

func (o obj) list(k string) []obj {
	xs, _ := o[k].([]any)
	return objs(xs)
}

func (o obj) str(k string) string {
	switch v := o[k].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return decimal.NewFromFloat(v).String()
	case bool:
		if v {
			return "true"
		}
		return "false"
	}
	return ""
}

// pick returns the first non-empty of several candidate names (⚠️ 11 §2).
func (o obj) pick(keys ...string) string {
	for _, k := range keys {
		if v := o.str(k); v != "" {
			return v
		}
	}
	return ""
}

func (o obj) dec(keys ...string) decimal.Decimal {
	d, err := decimal.NewFromString(strings.ReplaceAll(o.pick(keys...), ",", ""))
	if err != nil {
		return decimal.Zero
	}
	return d
}

func (o obj) date(keys ...string) (time.Time, bool) {
	s := o.pick(keys...)
	if len(s) < 10 {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s[:10])
	return t, err == nil
}

func hash(o obj) string {
	b, _ := json.Marshal(o)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// kinds read per call (11 §3.2); waybills are not read.
var kinds = []string{"sale_invoice", "sale_return_invoice", "purchase_invoice", "purchase_return_invoice", "general_expense"}

// Read fetches the documents issued in [from, to] (dates; the filter is
// documented, 11 §3.1) and maps them. A list row without items is read in
// detail (11 §9 ⚠️).
func (c *Client) Read(ctx context.Context, from, to time.Time) (sources.Batch, error) {
	b := sources.Batch{Excluded: sources.Excluded{}}
	for _, kind := range kinds {
		q := url.Values{"type": {kind}, "min_issue_date": {from.Format("2006-01-02")}, "max_issue_date": {to.Format("2006-01-02")}}
		if c.CompanyID != "" {
			q.Set("company_id", c.CompanyID)
		}
		err := c.pages(ctx, "/invoices", q, func(r obj) error {
			if _, ok := r["items"]; !ok && kind != "general_expense" {
				raw, err := c.get(ctx, "/invoices/"+r.pick("id", "document_id"), nil)
				if err != nil {
					return err
				}
				var d struct {
					Data obj `json:"data"`
				}
				if json.Unmarshal(raw, &d) == nil && len(d.Data) > 0 {
					r = d.Data
				} else {
					_ = json.Unmarshal(raw, &r)
				}
			}
			if kind == "general_expense" {
				e, skip := mapExpense(r)
				if skip != "" {
					b.Excluded[skip]++
				} else {
					b.Expenses = append(b.Expenses, e)
				}
				return nil
			}
			inv, skip := mapInvoice(kind, r)
			if skip != "" {
				b.Excluded[skip]++
				return nil
			}
			b.Invoices = append(b.Invoices, inv)
			return nil
		})
		if err != nil {
			return b, err
		}
	}
	return b, nil
}

func issueDate(r obj) (time.Time, bool) { return r.date("issue_date", "order_date", "date") }

func mapInvoice(kind string, r obj) (model.Invoice, string) {
	var inv model.Invoice
	d, ok := issueDate(r)
	if !ok {
		return inv, "no_issue_date"
	}
	if strings.EqualFold(r.pick("status"), "draft") || r.str("is_draft") == "true" {
		return inv, "draft"
	}
	if strings.EqualFold(r.pick("status"), "cancelled") || r.str("is_cancelled") == "true" {
		return inv, "cancelled"
	}
	dt := strings.ToUpper(r.pick("document_type"))
	if dt == "SEVK" {
		return inv, "not_an_invoice" // 11 §3.4
	}
	inv.SourceSystem, inv.SourceID, inv.CaptureMethod = SourceSystem, kind+":"+r.pick("id", "document_id"), model.CaptureAPI
	inv.InvoiceNumber = model.NormalizeInvoiceNumber(r.pick("serial_no", "invoice_number", "document_no"))
	inv.UUID = strings.ToLower(r.pick("uuid", "ettn", "e_document_uuid"))
	switch kind {
	case "sale_invoice", "sale_return_invoice":
		inv.Direction = model.DirectionOutgoing
	default:
		inv.Direction = model.DirectionIncoming
	}
	switch {
	case strings.HasSuffix(kind, "return_invoice") || dt == "IADE" || dt == "TEVKIFATIADE":
		inv.Type = model.TypeIade
	case dt == "TEVKIFAT", dt == "ISTISNA", dt == "OZELMATRAH", dt == "IHRACKAYITLI":
		inv.Type = dt
	case inv.Direction == model.DirectionIncoming:
		inv.Type = model.TypeAlis
	default:
		inv.Type = model.TypeSatis
	}
	// 11 §3.5: scenario → profile; MATBUDAN or empty → paper.
	if sc := strings.ToUpper(r.pick("document_scenario")); sc != "" && dt != "MATBUDAN" {
		inv.Profile, inv.ProfileRaw = &sc, sc
	}
	inv.DocumentClass = "invoice"
	inv.IssueDate, inv.Period = d, d.Format("2006-01")
	if due, ok := r.date("due_date"); ok {
		inv.PaymentDueDate = &due
	}
	inv.Currency = model.NormalizeCurrency(r.pick("currency"))
	if inv.Currency == "" {
		inv.Currency = "TRY"
	}
	if x := r.dec("exchange_rate"); x.IsPositive() && inv.Currency != "TRY" {
		inv.ExchangeRate = &x
	}
	inv.Notes = r.pick("description")
	inv.GlobalTaxExemptionCode = r.pick("vat_exemption_reason_code")
	a := r.sub("associate")
	if len(a) == 0 {
		a = r.sub("contact")
	}
	name := a.pick("name", "title")
	if s := a.pick("surname"); s != "" && a.str("is_corporate") != "true" {
		name += " " + s
	}
	inv.Counterparty = model.Party{Name: name, TaxOffice: a.pick("tax_office")}
	if vkn, tckn, ok := model.ClassifyTaxID(a.pick("identity_no", "tax_number")); ok {
		inv.Counterparty.TaxNumber, inv.Counterparty.IdentityNumber = vkn, tckn
	}
	if ad := a.list("addresses"); len(ad) > 0 {
		inv.Counterparty.City, inv.Counterparty.District, inv.Counterparty.Address = ad[0].str("city"), ad[0].str("district"), ad[0].str("address")
	}

	for i, it := range r.list("items") {
		qty, unit := it.dec("quantity"), it.dec("unit_price")
		lineExt := qty.Mul(unit).Round(2)
		disc := it.dec("discount_amount")
		net := it.dec("net_total", "total_excluding_vat", "tax_exclusive_amount")
		if net.IsZero() {
			net = lineExt.Sub(disc)
		}
		rate := it.dec("vat_rate")
		vat := it.dec("vat_amount", "total_vat", "vat")
		if vat.IsZero() {
			vat = net.Mul(rate).Div(hundred).Round(2)
		}
		wRate, wAmt := decimal.Zero, decimal.Zero
		if strings.EqualFold(it.pick("withholding_type"), "NUMERIC") {
			wAmt = it.dec("withholding_value")
		} else if v := it.dec("withholding_value"); v.IsPositive() {
			wRate = model.NormalizePercent(v)
			wAmt = vat.Mul(wRate).Div(hundred).Round(2)
		}
		inv.Items = append(inv.Items, model.InvoiceItem{
			LineNo: i + 1, Name: it.pick("description", "name", "product_name"), Description: it.pick("description"),
			ProductCode: it.pick("product_code", "product_id"), Quantity: qty, UnitPrice: unit, LineExtensionAmount: lineExt,
			OwnAllowanceAmount: disc, AllowanceTotalAmount: disc, TaxExclusiveAmount: net, TaxRate: rate, TotalTaxAmount: vat,
			TaxInclusiveAmount: net.Add(vat), WithholdingCode: it.pick("withholding_code"), WithholdingRate: wRate,
			WithholdingAmount: wAmt, GTIPCode: it.pick("gtip_no"), LineType: model.LineUnknown,
		})
	}
	am := &inv.Amounts
	for _, it := range inv.Items {
		am.LineExtensionAmount = am.LineExtensionAmount.Add(it.LineExtensionAmount)
		am.AllowanceTotalAmount = am.AllowanceTotalAmount.Add(it.AllowanceTotalAmount)
		am.TaxExclusiveAmount = am.TaxExclusiveAmount.Add(it.TaxExclusiveAmount)
		am.TaxAmount = am.TaxAmount.Add(it.TotalTaxAmount)
		am.WithholdingAmount = am.WithholdingAmount.Add(it.WithholdingAmount)
	}
	if sd := r.dec("subtotal_discount_amount"); sd.IsPositive() {
		am.AllowanceTotalAmount = am.AllowanceTotalAmount.Add(sd)
	}
	// Header totals win when present ⚠️ (names from the response).
	if v := r.dec("total_vat", "vat_total", "total_vat_amount"); v.IsPositive() || len(inv.Items) == 0 {
		am.TaxAmount = v
	}
	if v := r.dec("total_excluding_vat", "subtotal", "net_total"); v.IsPositive() || len(inv.Items) == 0 {
		am.TaxExclusiveAmount = v
	}
	am.PayableAmount = r.dec("grand_total", "total", "payable_amount", "total_amount")
	if am.PayableAmount.IsZero() {
		am.PayableAmount = am.TaxExclusiveAmount.Add(am.TaxAmount).Sub(am.WithholdingAmount)
	}
	am.TaxInclusiveAmount = am.TaxExclusiveAmount.Add(am.TaxAmount)
	am.PayableAmountTRY = am.PayableAmount
	if inv.ExchangeRate != nil {
		am.PayableAmountTRY = am.PayableAmount.Mul(*inv.ExchangeRate).Round(2)
	}
	if len(inv.Items) > 0 {
		inv.HasItems, inv.TaxSummarySource = true, model.TaxSummaryFromItems
		inv.TaxSummaries = summaries(inv.Items)
	} else if am.TaxExclusiveAmount.IsPositive() {
		inv.TaxSummarySource = model.TaxSummaryFromHeaderEstimate
		rt := model.NearestVATRate(am.TaxAmount.Div(am.TaxExclusiveAmount).Mul(hundred))
		inv.TaxSummaries = []model.TaxSummary{{TaxRate: rt, Base: am.TaxExclusiveAmount, VAT: am.TaxAmount}}
	}
	inv.ContentHash = hash(r)
	inv.RawPayload = map[string]any{"kolaybi": map[string]any(r)}
	return inv, ""
}

func summaries(its []model.InvoiceItem) []model.TaxSummary {
	by := map[string]*model.TaxSummary{}
	var keys []string
	for _, it := range its {
		k := it.TaxRate.String()
		s, ok := by[k]
		if !ok {
			s = &model.TaxSummary{TaxRate: it.TaxRate}
			by[k] = s
			keys = append(keys, k)
		}
		s.Base, s.VAT, s.Withholding = s.Base.Add(it.TaxExclusiveAmount), s.VAT.Add(it.TotalTaxAmount), s.Withholding.Add(it.WithholdingAmount)
	}
	sort.Strings(keys)
	out := make([]model.TaxSummary, 0, len(keys))
	for _, k := range keys {
		out = append(out, *by[k])
	}
	return out
}

// mapExpense: 11 §6 (general_expense; items optional).
func mapExpense(r obj) (sources.Expense, string) {
	d, ok := issueDate(r)
	if !ok {
		return sources.Expense{}, "no_issue_date"
	}
	if strings.EqualFold(r.pick("status"), "draft") || strings.EqualFold(r.pick("status"), "cancelled") {
		return sources.Expense{}, "draft_or_cancelled"
	}
	amount := r.dec("grand_total", "total", "total_amount", "amount")
	vat := r.dec("total_vat", "vat_total", "vat_amount")
	var rate *decimal.Decimal
	if its := r.list("items"); len(its) > 0 {
		net, v := decimal.Zero, decimal.Zero
		for _, it := range its {
			n := it.dec("net_total", "total_excluding_vat")
			if n.IsZero() {
				n = it.dec("quantity").Mul(it.dec("unit_price")).Sub(it.dec("discount_amount"))
			}
			net = net.Add(n)
			v = v.Add(n.Mul(it.dec("vat_rate")).Div(hundred).Round(2))
		}
		if vat.IsZero() {
			vat = v
		}
		if amount.IsZero() {
			amount = net.Add(vat)
		}
	}
	if excl := amount.Sub(vat); excl.IsPositive() && vat.IsPositive() {
		x := model.NearestVATRate(vat.Div(excl).Mul(hundred))
		rate = &x
	}
	rt := "other"
	if strings.Contains(strings.ToLower(r.pick("financial_action_type", "expense_type")), "fuel") {
		rt = "fuel_receipt"
	}
	a := r.sub("associate")
	cur := model.NormalizeCurrency(r.pick("currency"))
	if cur == "" {
		cur = "TRY"
	}
	return sources.Expense{
		SourceID: "general_expense:" + r.pick("id", "document_id"), Date: d, Period: d.Format("2006-01"), Amount: amount,
		TaxAmount: vat, TaxRate: rate, Currency: cur, Description: r.pick("description"), ReceiptType: rt,
		VendorName: a.pick("name", "title"), VendorTaxID: a.pick("identity_no"), VehiclePlate: plate(r.pick("description")),
		Raw: map[string]any{"kolaybi": map[string]any(r)}, Hash: hash(r),
	}, ""
}

var plateRe = regexp.MustCompile(`\b(0[1-9]|[1-7][0-9]|8[01])\s?[A-Z]{1,3}\s?\d{2,4}\b`)

func plate(s string) string {
	return strings.ReplaceAll(plateRe.FindString(strings.ToUpperSpecial(unicode.TurkishCase, s)), " ", "")
}
