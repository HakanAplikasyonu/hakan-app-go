package easytrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources"
)

// SourceSystem of everything read from Easytrade.
const SourceSystem = "easytrade"

var hundred = decimal.NewFromInt(100)

// obj is a decoded JSON object with lenient accessors: Easytrade returns
// {value, label} enums and amounts as strings (09 §3.2).
type obj map[string]any

func (o obj) sub(k string) obj {
	if m, ok := o[k].(map[string]any); ok {
		return obj(m)
	}
	return obj{}
}

func (o obj) list(k string) []obj {
	xs, _ := o[k].([]any)
	out := make([]obj, 0, len(xs))
	for _, x := range xs {
		if m, ok := x.(map[string]any); ok {
			out = append(out, obj(m))
		}
	}
	return out
}

func (o obj) str(k string) string {
	switch v := o[k].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return decimal.NewFromFloat(v).String()
	case json.Number:
		return v.String()
	case map[string]any: // {value, label}
		if s, ok := v["value"].(string); ok {
			return s
		}
		if s, ok := v["code"].(string); ok {
			return s
		}
	}
	return ""
}

func (o obj) dec(k string) decimal.Decimal {
	d, err := decimal.NewFromString(o.str(k))
	if err != nil {
		return decimal.Zero
	}
	return d
}

func (o obj) date(k string) (time.Time, bool) {
	s := o.str(k)
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

// Items the adapter reads; each is one sync_windows.item (issue #385).
const (
	ItemInvoices = "invoices"
	ItemExpenses = "expenses"
)

// Items lists what a sync reads, in order.
var Items = []string{ItemInvoices, ItemExpenses}

// DateField is the sync_windows.date_field of an item.
func DateField(item string) string {
	if item == ItemExpenses {
		return "expense"
	}
	return "issue"
}

// Read fetches sales and purchase invoices and expenses of [from, to] for
// every branch and maps them (09 §3, §6).
func (c *Client) Read(ctx context.Context, from, to time.Time) (sources.Batch, error) {
	branches, err := c.ActiveBranches(ctx)
	if err != nil {
		return sources.Batch{Excluded: sources.Excluded{}}, err
	}
	b := sources.Batch{Excluded: sources.Excluded{}}
	for _, item := range Items {
		part, err := c.ReadItem(ctx, item, branches, from, to)
		if err != nil {
			return b, err
		}
		b.Invoices = append(b.Invoices, part.Invoices...)
		b.Expenses = append(b.Expenses, part.Expenses...)
		for k, v := range part.Excluded {
			b.Excluded[k] += v
		}
	}
	return b, nil
}

// ActiveBranches lists the branches; a company without any is read once
// without X-Branch-ID.
func (c *Client) ActiveBranches(ctx context.Context) ([]Branch, error) {
	branches, err := c.Branches(ctx)
	if err != nil {
		return nil, err
	}
	if len(branches) == 0 {
		branches = []Branch{{}}
	}
	return branches, nil
}

// ReadItem reads one item of [from, to] over branches.
func (c *Client) ReadItem(ctx context.Context, item string, branches []Branch, from, to time.Time) (sources.Batch, error) {
	b := sources.Batch{Excluded: sources.Excluded{}}
	seen := map[string]bool{}
	defer func() { c.BranchID = "" }()
	for _, br := range branches {
		c.BranchID = br.ID
		var err error
		switch item {
		case ItemInvoices:
			err = c.readInvoices(ctx, &b, seen, from, to)
		case ItemExpenses:
			err = c.readExpenses(ctx, &b, seen, from, to)
		default:
			err = fmt.Errorf("easytrade: unknown item %q", item)
		}
		if err != nil {
			return b, err
		}
	}
	return b, nil
}

func (c *Client) readInvoices(ctx context.Context, b *sources.Batch, seen map[string]bool, from, to time.Time) error {
	for _, path := range []string{"/company/invoices/sales", "/company/invoices/purchases"} {
		ids, err := c.listIDs(ctx, path, "issue_date", from, to)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			var d struct {
				Data obj `json:"data"`
			}
			if err := c.get(ctx, "/company/invoices/"+id, nil, &d); err != nil {
				return err
			}
			inv, skip := mapInvoice(d.Data)
			if skip != "" {
				b.Excluded[skip]++
				continue
			}
			b.Invoices = append(b.Invoices, inv)
		}
	}
	return nil
}

func (c *Client) readExpenses(ctx context.Context, b *sources.Batch, seen map[string]bool, from, to time.Time) error {
	ids, err := c.listIDs(ctx, "/company/expenses", "expense_date", from, to)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		var d struct {
			Data obj `json:"data"`
		}
		if err := c.get(ctx, "/company/expenses/"+id, nil, &d); err != nil {
			return err
		}
		e, skip := mapExpense(d.Data)
		if skip != "" {
			b.Excluded[skip]++
			continue
		}
		b.Expenses = append(b.Expenses, e)
	}
	return nil
}

// cancelled: status value or the "Fatura İptal Edildi" movement (09 §3.2).
// The full status list is 09 §9 soru 6 ⚠️.
func cancelled(d obj) bool {
	if strings.Contains(strings.ToLower(d.str("status")), "cancel") || strings.Contains(strings.ToLower(d.str("status")), "iptal") {
		return true
	}
	for _, m := range d.list("movements") {
		if strings.Contains(strings.ToLower(m.str("description")+" "+m.str("title")+" "+m.str("type")), "iptal") {
			return true
		}
	}
	return false
}

func mapInvoice(d obj) (model.Invoice, string) {
	var inv model.Invoice
	issue, ok := d.date("issue_date")
	if !ok {
		return inv, "no_issue_date"
	}
	if cancelled(d) {
		return inv, "cancelled" // status_normalized is not in the model yet
	}
	if s := strings.ToLower(d.str("status")); s == "draft" || s == "taslak" {
		return inv, "draft"
	}
	inv.SourceSystem, inv.SourceID, inv.CaptureMethod = SourceSystem, d.str("id"), model.CaptureAPI
	inv.UUID = strings.ToLower(d.str("uuid"))
	inv.InvoiceNumber = model.NormalizeInvoiceNumber(d.str("invoice_number"))
	switch d.str("direction") {
	case "incoming", "purchase", "in":
		inv.Direction = model.DirectionIncoming
	default:
		inv.Direction = model.DirectionOutgoing
	}
	inv.Type = strings.ToUpper(d.str("type"))
	if inv.Type == "" {
		inv.Type = map[model.Direction]string{model.DirectionOutgoing: model.TypeSatis, model.DirectionIncoming: model.TypeAlis}[inv.Direction]
	}
	if p := strings.ToUpper(d.str("profile")); p != "" {
		inv.Profile, inv.ProfileRaw = &p, p
	}
	inv.DocumentClass = "invoice"
	inv.IssueDate, inv.Period, inv.IssueTime = issue, issue.Format("2006-01"), d.str("issue_time")
	if due, ok := d.date("payment_due_date"); ok {
		inv.PaymentDueDate = &due
	}
	inv.Currency = model.NormalizeCurrency(d.sub("currency").str("code"))
	if inv.Currency == "" {
		inv.Currency = "TRY"
	}
	if x := d.dec("exchange_rate"); x.IsPositive() && inv.Currency != "TRY" {
		inv.ExchangeRate = &x
	}
	inv.Notes = d.str("notes")
	cu := d.sub("customer")
	inv.Counterparty = model.Party{
		Name: cu.str("name"), TaxOffice: cu.str("tax_office"), Address: cu.str("address"),
		City: cu.str("city"), District: cu.str("district"), Email: cu.str("email"), Phone: cu.str("phone"),
	}
	if vkn, tckn, ok := model.ClassifyTaxID(firstNonEmpty(cu.str("tax_number"), cu.str("identity_number"), d.str("customer_tax_number"), d.str("customer_identity_number"))); ok {
		inv.Counterparty.TaxNumber, inv.Counterparty.IdentityNumber = vkn, tckn
	}
	if inv.Counterparty.Name == "" {
		inv.Counterparty.Name = d.str("customer_name")
	}

	for i, it := range d.list("items") {
		excl := it.dec("tax_exclusive_amount")
		vat := it.dec("total_tax_amount")
		rate := it.dec("tax_rate")
		if rate.IsZero() && excl.IsPositive() { // rate lives in tax_breakdowns ⚠️ (09 §9 soru 7)
			rate = model.NearestVATRate(vat.Div(excl).Mul(hundred))
		}
		code := firstNonEmpty(it.sub("product").str("sku"), it.sub("product").str("barcode"))
		inv.Items = append(inv.Items, model.InvoiceItem{
			LineNo: i + 1, Name: firstNonEmpty(it.str("name"), it.str("description")), Description: it.str("description"),
			ProductCode: code, Quantity: it.dec("quantity"), UnitCode: it.sub("unit").str("code"), UnitPrice: it.dec("unit_price"),
			LineExtensionAmount: it.dec("line_extension_amount"), OwnAllowanceAmount: it.dec("own_allowance_amount"),
			AllowanceTotalAmount: it.dec("allowance_total_amount"), TaxExclusiveAmount: excl, TaxRate: rate,
			TotalTaxAmount: vat, TaxInclusiveAmount: it.dec("tax_inclusive_amount"),
			WithholdingCode: it.str("withholding_code"), WithholdingRate: model.NormalizePercent(it.dec("withholding_rate")),
			WithholdingAmount: it.dec("withholding_amount"), TaxExemptionCode: it.str("tax_exemption_code"),
			ExportRegisteredCode: it.str("export_registered_code"), GTIPCode: it.str("gtip_code"), LineType: model.LineUnknown,
		})
	}
	a := &inv.Amounts
	am := d.sub("amounts")
	a.TaxExclusiveAmount, a.TaxAmount, a.PayableAmount = am.dec("tax_exclusive_amount"), am.dec("tax_amount"), am.dec("payable_amount")
	// ⊕ header fields from the lines (09 §3.2).
	for _, it := range inv.Items {
		a.LineExtensionAmount = a.LineExtensionAmount.Add(it.LineExtensionAmount)
		a.AllowanceTotalAmount = a.AllowanceTotalAmount.Add(it.AllowanceTotalAmount)
		a.WithholdingAmount = a.WithholdingAmount.Add(it.WithholdingAmount)
	}
	a.TaxInclusiveAmount = a.TaxExclusiveAmount.Add(a.TaxAmount)
	a.PayableAmountTRY = a.PayableAmount
	if inv.ExchangeRate != nil {
		a.PayableAmountTRY = a.PayableAmount.Mul(*inv.ExchangeRate).Round(2)
	}
	if len(inv.Items) > 0 {
		inv.HasItems, inv.TaxSummarySource = true, model.TaxSummaryFromItems
		inv.TaxSummaries = summaries(inv.Items)
	} else if a.TaxExclusiveAmount.IsPositive() {
		inv.TaxSummarySource = model.TaxSummaryFromHeaderEstimate
		r := model.NearestVATRate(a.TaxAmount.Div(a.TaxExclusiveAmount).Mul(hundred))
		inv.TaxSummaries = []model.TaxSummary{{TaxRate: r, Base: a.TaxExclusiveAmount, VAT: a.TaxAmount}}
	}
	inv.ContentHash = hash(d)
	inv.RawPayload = map[string]any{"easytrade": map[string]any(d)}
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
		s.Base = s.Base.Add(it.TaxExclusiveAmount)
		s.VAT = s.VAT.Add(it.TotalTaxAmount)
		s.Withholding = s.Withholding.Add(it.WithholdingAmount)
	}
	sort.Strings(keys)
	out := make([]model.TaxSummary, 0, len(keys))
	for _, k := range keys {
		out = append(out, *by[k])
	}
	return out
}

var rateRe = regexp.MustCompile(`^(\d+)`)

// mapExpense: 09 §6. Templates of recurring expenses never reach the
// declaration (rule 2 ⚠️); amount is taken as KDV dahil (09 §9 soru 10 ⚠️).
func mapExpense(d obj) (sources.Expense, string) {
	date, ok := d.date("expense_date")
	if !ok {
		return sources.Expense{}, "no_expense_date"
	}
	if b, _ := d["is_main_expense"].(bool); b {
		return sources.Expense{}, "recurring_template"
	}
	if s := strings.ToLower(d.str("status")); strings.Contains(s, "reject") || strings.Contains(s, "red") || strings.Contains(s, "cancel") {
		return sources.Expense{}, "rejected"
	}
	amount := d.dec("amount")
	var rate *decimal.Decimal
	tax := decimal.Zero
	if m := rateRe.FindString(d.str("tax_rate")); m != "" { // "20_kdv" → 20
		r := decimal.RequireFromString(m)
		rate = &r
		tax = amount.Mul(r).Div(hundred.Add(r)).Round(2)
	}
	cur := model.NormalizeCurrency(d.sub("currency").str("code"))
	if cur == "" {
		cur = "TRY"
	}
	return sources.Expense{
		SourceID: d.str("id"), Date: date, Period: date.Format("2006-01"), Amount: amount, TaxAmount: tax, TaxRate: rate,
		Currency: cur, Description: d.str("description"), VendorName: d.sub("customer").str("name"),
		VehiclePlate: plate(d.str("description")), Raw: map[string]any{"easytrade": map[string]any(d)}, Hash: hash(d),
	}, ""
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

var plateRe = regexp.MustCompile(`\b(0[1-9]|[1-7][0-9]|8[01])\s?[A-Z]{1,3}\s?\d{2,4}\b`)

func plate(s string) string {
	return strings.ReplaceAll(plateRe.FindString(strings.ToUpperSpecial(unicode.TurkishCase, s)), " ", "")
}
