package easytrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// label is the label of a {value, label} enum.
func (o obj) label(k string) string {
	if m, ok := o[k].(map[string]any); ok {
		if s, ok := m["label"].(string); ok {
			return s
		}
	}
	return ""
}

// has reports whether k holds a number or a non-empty string.
func (o obj) has(k string) bool {
	_, err := decimal.NewFromString(o.str(k))
	return err == nil
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
	ItemInvoices  = "invoices"
	ItemExpenses  = "expenses"
	ItemFinancial = "financial_transactions"
	ItemZReports  = "z_reports"
)

// Items lists what a sync reads, in order: invoices before the financial
// transactions that reference them.
var Items = []string{ItemInvoices, ItemExpenses, ItemFinancial, ItemZReports}

// DateField is the sync_windows.date_field of an item.
func DateField(item string) string {
	switch item {
	case ItemExpenses:
		return "expense"
	case ItemFinancial:
		return "transaction"
	case ItemZReports:
		return "report"
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
		for id, ws := range part.InvoiceWarnings {
			b.Warn(id, ws...)
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
		case ItemFinancial:
			err = c.readFinancial(ctx, &b, seen, from, to)
		case ItemZReports:
			err = c.readZReports(ctx, &b, seen, from, to)
		default:
			err = fmt.Errorf("easytrade: unknown item %q", item)
		}
		if err != nil {
			return b, err
		}
	}
	return b, nil
}

// invoiceLists are Easytrade's invoice lists with the direction a detail
// without one falls back to. The inbox holds the e-invoices received from
// GİB; Easytrade lists them apart from purchases (its own VAT report reads
// both), so a purchase that was never "imported" was missing here.
var invoiceLists = []struct {
	path string
	dir  model.Direction
}{
	{"/company/invoices/sales", model.DirectionOutgoing},
	{"/company/invoices/purchases", model.DirectionIncoming},
	{"/company/invoices/inbox", model.DirectionIncoming},
}

func (c *Client) readInvoices(ctx context.Context, b *sources.Batch, seen map[string]bool, from, to time.Time) error {
	for _, l := range invoiceLists {
		ids, err := c.listIDs(ctx, l.path, "issue_date", from, to)
		var he *HTTPError
		if l.path == "/company/invoices/inbox" && errors.As(err, &he) && (he.Status == 404 || he.Status == 405) {
			continue // no inbox for this company (no e-invoice integration)
		}
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
			inv, ws, skip := mapInvoice(d.Data, l.dir)
			if skip != "" {
				b.Excluded[skip]++
				continue
			}
			b.Invoices = append(b.Invoices, inv)
			b.Warn(inv.SourceID, ws...)
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

// voidStatuses never reach the declaration (Easytrade's own reports skip
// them): cancelled, and rejected / returned by the counterparty.
var voidStatuses = map[string]bool{"cancelled": true, "canceled": true, "rejected": true, "declined": true, "refused": true, "returned": true}

// cancelled: a void status value or label, or the "Fatura İptal Edildi"
// movement (09 §3.2).
func cancelled(d obj) bool {
	v := strings.ToLower(d.str("status"))
	if voidStatuses[v] || strings.Contains(v, "cancel") || strings.Contains(v, "reject") || strings.Contains(v, "iptal") {
		return true
	}
	// "İ" lower-cases to "i̇": look for "ptal".
	if l := strings.ToLower(d.label("status") + " " + d.str("status_label")); strings.Contains(l, "ptal") || strings.Contains(l, "redd") {
		return true
	}
	for _, m := range d.list("movements") {
		if strings.Contains(strings.ToLower(m.str("description")+" "+m.str("title")+" "+m.str("type")), "iptal") {
			return true
		}
	}
	return false
}

// Tax codes in an item's tax_breakdowns (GİB code list): 0015 is KDV, 9015
// KDV tevkifatı; a 6xx / 8xx code is the withholding (tevkifat) type itself.
const (
	codeVAT         = "0015"
	codeWithholding = "9015"
)

func withholdingCode(code string) bool {
	return code == codeWithholding || (len(code) == 3 && (code[0] == '6' || code[0] == '8'))
}

// invoiceDirection reads an invoice direction value; "" when unknown.
func invoiceDirection(v string) model.Direction {
	switch strings.ToLower(v) {
	case "incoming", "purchase", "purchases", "in", "inbound", "alis", "alış":
		return model.DirectionIncoming
	case "outgoing", "sale", "sales", "out", "outbound", "satis", "satış":
		return model.DirectionOutgoing
	}
	return ""
}

func itemWarn(line int, params map[string]any) model.Warning {
	return model.Warning{Code: model.WarnUnknownTaxCode, EntityType: model.EntityInvoiceItem, LineNo: line, Params: params}
}

// mapInvoice maps an invoice detail to the common model in the shape the
// VAT return reads (invoice_tax_summaries by rate, withholding and
// exemption code; period by issue date). A field Easytrade does not give is
// left empty and flagged ("kontrol et"), never guessed. fallback is the
// direction of the list the invoice came from.
func mapInvoice(d obj, fallback model.Direction) (model.Invoice, []model.Warning, string) {
	var inv model.Invoice
	var ws []model.Warning
	// The documented detail keeps the header under "details" (direction,
	// invoice_type, profile, invoice_date, currency, exchange_rate); the
	// mobile client's top-level names are still read as a fallback.
	dt := d.sub("details")
	issue, ok := d.date("issue_date")
	if !ok {
		issue, ok = dt.date("invoice_date")
	}
	if !ok {
		issue, ok = d.date("invoice_date")
	}
	if !ok {
		return inv, nil, "no_issue_date"
	}
	if cancelled(d) {
		return inv, nil, "cancelled" // status_normalized is not in the model yet
	}
	if s := strings.ToLower(d.str("status")); s == "draft" || s == "taslak" {
		return inv, nil, "draft"
	}
	inv.SourceSystem, inv.SourceID, inv.CaptureMethod = SourceSystem, d.str("id"), model.CaptureAPI
	inv.UUID = strings.ToLower(d.str("uuid"))
	inv.InvoiceNumber = model.NormalizeInvoiceNumber(d.str("invoice_number"))
	inv.Direction = invoiceDirection(firstNonEmpty(dt.str("direction"), d.str("direction")))
	if inv.Direction == "" {
		inv.Direction = fallback
	}
	if inv.Direction == "" {
		inv.Direction = model.DirectionOutgoing
	}
	// details.invoice_type: ALIS, SATIS, IADE, TEVKIFAT, ISTISNA,
	// OZELMATRAH, IHRACKAYITLI, SGK.
	inv.Type = strings.ToUpper(firstNonEmpty(dt.str("invoice_type"), d.str("invoice_type"), d.str("type")))
	if inv.Type == "" {
		inv.Type = map[model.Direction]string{model.DirectionOutgoing: model.TypeSatis, model.DirectionIncoming: model.TypeAlis}[inv.Direction]
	}
	// Profile tells the document kind: TEMELFATURA / TICARIFATURA are
	// e-fatura, EARSIVFATURA e-arşiv; none is paper.
	if p := strings.ToUpper(firstNonEmpty(dt.str("profile"), d.str("profile"))); p != "" {
		inv.Profile, inv.ProfileRaw = &p, p
	}
	inv.DocumentClass = "invoice"
	inv.IssueDate, inv.Period, inv.IssueTime = issue, issue.Format("2006-01"), d.str("issue_time")
	if due, ok := d.date("payment_due_date"); ok {
		inv.PaymentDueDate = &due
	}
	cur := d.sub("currency")
	inv.Currency = model.NormalizeCurrency(firstNonEmpty(dt.str("currency"), cur.str("code"), d.str("currency")))
	if inv.Currency == "" {
		inv.Currency = "TRY"
	}
	// A foreign-currency rate of 1 is an old mobile default, not a rate.
	if x := firstPositive(dt.dec("exchange_rate"), d.dec("exchange_rate"), cur.dec("exchange_rate"), cur.dec("rate"), d.dec("currency_rate")); inv.Currency != "TRY" && x.IsPositive() && !x.Equal(decimal.NewFromInt(1)) {
		inv.ExchangeRate = &x
	}
	inv.Notes = d.str("notes")
	inv.GlobalTaxExemptionCode = firstNonEmpty(d.sub("meta").str("global_tax_exemption_code"), d.str("global_tax_exemption_code"))

	cu := d.sub("customer")
	inv.Counterparty = model.Party{
		Name:      firstNonEmpty(cu.str("trade_name"), cu.str("full_name"), cu.str("name"), d.str("customer_name")),
		FirstName: cu.str("first_name"), LastName: cu.str("last_name"),
		TaxOffice: cu.str("tax_office"), Address: cu.str("address"),
		City: cu.str("city"), District: cu.str("district"), Email: cu.str("email"), Phone: cu.str("phone"),
	}
	if b, _ := cu["is_default_retail_customer"].(bool); b {
		inv.Counterparty.IsDefaultRetail = true
	}
	// The documented field is customer.tax_number; tax_or_identity_number
	// is what the mobile client's answers carry.
	rawID := firstNonEmpty(cu.str("tax_number"), cu.str("tax_or_identity_number"), cu.str("identity_number"),
		d.str("customer_tax_number"), d.str("customer_identity_number"))
	if vkn, tckn, ok := model.ClassifyTaxID(rawID); ok {
		inv.Counterparty.TaxNumber, inv.Counterparty.IdentityNumber = vkn, tckn
	} else {
		reason := "missing"
		if rawID != "" {
			reason = "invalid"
		}
		ws = append(ws, model.Warning{Code: model.WarnInvalidTaxID, EntityType: model.EntityInvoice,
			Params: map[string]any{"field": "customer_tax_number", "value": rawID, "reason": reason, "source": SourceSystem}})
	}
	if inv.Counterparty.Name == "" {
		ws = append(ws, model.Warning{Code: model.WarnInvalidTaxID, EntityType: model.EntityInvoice,
			Params: map[string]any{"field": "customer_name", "reason": "missing", "source": SourceSystem}})
	}

	lines := d.list("items")
	if len(lines) == 0 {
		lines = d.list("lines") // the documented name
	}
	for i, it := range lines {
		item, iws := mapItem(i+1, it, inv.GlobalTaxExemptionCode)
		inv.Items = append(inv.Items, item)
		ws = append(ws, iws...)
	}
	a := &inv.Amounts
	am := d.sub("amounts")
	a.TaxExclusiveAmount, a.TaxAmount, a.PayableAmount = am.dec("tax_exclusive_amount"), am.dec("tax_amount"), am.dec("payable_amount")
	// ⊕ header fields from the lines (09 §3.2).
	var itemBase, itemVAT decimal.Decimal
	for _, it := range inv.Items {
		a.LineExtensionAmount = a.LineExtensionAmount.Add(it.LineExtensionAmount)
		a.AllowanceTotalAmount = a.AllowanceTotalAmount.Add(it.AllowanceTotalAmount)
		a.WithholdingAmount = a.WithholdingAmount.Add(it.WithholdingAmount)
		itemBase, itemVAT = itemBase.Add(it.TaxExclusiveAmount), itemVAT.Add(it.TotalTaxAmount)
	}
	if !am.has("tax_exclusive_amount") && len(inv.Items) > 0 {
		a.TaxExclusiveAmount = itemBase
	}
	if !am.has("tax_amount") && len(inv.Items) > 0 {
		a.TaxAmount = itemVAT
	}
	a.TaxInclusiveAmount = a.TaxExclusiveAmount.Add(a.TaxAmount)
	if !am.has("payable_amount") {
		a.PayableAmount = a.TaxInclusiveAmount.Sub(a.WithholdingAmount)
	}
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
		ws = append(ws, model.Warning{Code: model.WarnHeaderEstimate, EntityType: model.EntityInvoice, Params: map[string]any{"source": SourceSystem}})
	}
	inv.ContentHash = hash(d)
	inv.RawPayload = map[string]any{"easytrade": map[string]any(d)}
	return inv, ws, ""
}

// mapItem maps one line. The VAT rate is the "0015" row of tax_breakdowns
// (Easytrade's mobile VAT report reads it there), else the line's tax_rate;
// a line with VAT but neither is derived from its amounts and flagged.
func mapItem(line int, it obj, globalExemption string) (model.InvoiceItem, []model.Warning) {
	var ws []model.Warning
	excl := it.dec("tax_exclusive_amount")
	if !it.has("tax_exclusive_amount") {
		excl = firstPositive(it.dec("line_extension_amount").Sub(it.dec("allowance_total_amount")), it.dec("subtotal"))
	}
	var (
		vatRate, vatSum, wRate, wAmount decimal.Decimal
		vatFound, wFound                bool
		wCode, exCode                   string
		bds                             []model.TaxBreakdown
	)
	for _, tb := range it.list("tax_breakdowns") {
		code := tb.str("code")
		kind := "tax"
		switch {
		case code == codeVAT:
			vatRate, vatFound = tb.dec("rate"), true
			vatSum = vatSum.Add(tb.dec("amount"))
			exCode = firstNonEmpty(exCode, tb.str("exemption_code"), tb.str("tax_exemption_code"))
		case withholdingCode(code):
			kind, wFound = "withholding", true
			if code != codeWithholding {
				wCode = code
			}
			wRate = model.NormalizePercent(tb.dec("rate"))
			wAmount = wAmount.Add(tb.dec("amount"))
		}
		bds = append(bds, model.TaxBreakdown{Kind: kind, TaxTypeCode: code, Name: tb.str("name"),
			TaxableAmount: tb.str("base_amount"), TaxAmount: tb.str("amount"), Percent: tb.str("rate"),
			ExemptionCode: firstNonEmpty(tb.str("exemption_code"), tb.str("tax_exemption_code")), Exemption: tb.str("exemption_reason")})
	}
	vat := it.dec("total_tax_amount")
	if !it.has("total_tax_amount") {
		vat = it.dec("tax_amount")
	}
	if vatFound && (vat.IsZero() || !vatSum.IsZero()) {
		vat = vatSum
	}
	rate := vatRate
	switch {
	case vatFound:
	case it.has("tax_rate"):
		rate = model.NormalizePercent(it.dec("tax_rate"))
	case vat.IsZero():
		rate = decimal.Zero // a line without VAT is the %0 row (Easytrade's own report)
	case excl.IsPositive():
		rate = model.NearestVATRate(vat.Div(excl).Mul(hundred))
		ws = append(ws, itemWarn(line, map[string]any{"field": "tax_rate", "reason": "derived", "source": SourceSystem}))
	default:
		ws = append(ws, itemWarn(line, map[string]any{"field": "tax_rate", "reason": "missing", "source": SourceSystem}))
	}

	// Tevkifat: code, rate and amount; a part Easytrade did not send stays
	// empty and is flagged.
	wCode = firstNonEmpty(it.str("withholding_code"), wCode)
	if it.has("withholding_rate") {
		wRate = model.NormalizePercent(it.dec("withholding_rate"))
	}
	if it.has("withholding_amount") {
		wAmount = it.dec("withholding_amount")
	}
	if wCode != "" || wFound || wRate.IsPositive() || wAmount.IsPositive() {
		var missing []string
		if wCode == "" {
			missing = append(missing, "withholding_code")
		}
		if !wRate.IsPositive() {
			missing = append(missing, "withholding_rate")
		}
		if !wAmount.IsPositive() {
			missing = append(missing, "withholding_amount")
		}
		if len(missing) > 0 {
			ws = append(ws, itemWarn(line, map[string]any{"field": "withholding", "missing": missing, "source": SourceSystem}))
		}
	}

	exCode = firstNonEmpty(it.str("tax_exemption_code"), exCode)
	if rate.IsZero() && excl.IsPositive() && exCode == "" && globalExemption == "" {
		ws = append(ws, itemWarn(line, map[string]any{"field": "tax_exemption_code", "reason": "missing", "source": SourceSystem}))
	}

	code := firstNonEmpty(it.sub("product").str("sku"), it.sub("product").str("barcode"), it.str("sku"))
	unit := firstNonEmpty(it.sub("unit").str("code"), it.str("unit_code"))
	name := firstNonEmpty(it.str("name"), it.sub("product").str("name"), it.str("product_name"), it.str("description"))
	return model.InvoiceItem{
		LineNo: line, Name: name, Description: it.str("description"),
		ProductCode: code, Quantity: it.dec("quantity"), UnitCode: unit, UnitPrice: it.dec("unit_price"),
		LineExtensionAmount: it.dec("line_extension_amount"), OwnAllowanceAmount: it.dec("own_allowance_amount"),
		AllowanceTotalAmount: it.dec("allowance_total_amount"), TaxExclusiveAmount: excl, TaxRate: rate,
		TotalTaxAmount: vat, TaxInclusiveAmount: firstPositive(it.dec("tax_inclusive_amount"), excl.Add(vat)),
		WithholdingCode: wCode, WithholdingRate: wRate, WithholdingAmount: wAmount, TaxExemptionCode: exCode,
		SpecialTaxBaseCode:   it.str("special_tax_base_code"),
		ExportRegisteredCode: it.str("export_registered_code"), GTIPCode: it.str("gtip_code"), LineType: model.LineUnknown,
		TaxBreakdowns: bds,
	}, ws
}

type summaryKey struct{ rate, wcode, excode string }

// summaries groups the lines by rate, withholding code and exemption code
// (`31` §6, like the UBL channel): the VAT return reads tevkifat and
// istisna per code from these rows.
func summaries(its []model.InvoiceItem) []model.TaxSummary {
	by := map[summaryKey]*model.TaxSummary{}
	var keys []summaryKey
	for _, it := range its {
		k := summaryKey{it.TaxRate.String(), it.WithholdingCode, it.TaxExemptionCode}
		s, ok := by[k]
		if !ok {
			s = &model.TaxSummary{TaxRate: it.TaxRate, WithholdingCode: it.WithholdingCode, TaxExemptionCode: it.TaxExemptionCode}
			by[k] = s
			keys = append(keys, k)
		}
		s.Base = s.Base.Add(it.TaxExclusiveAmount)
		s.VAT = s.VAT.Add(it.TotalTaxAmount)
		s.Withholding = s.Withholding.Add(it.WithholdingAmount)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		a, b := by[keys[i]], by[keys[j]]
		if !a.TaxRate.Equal(b.TaxRate) {
			return a.TaxRate.LessThan(b.TaxRate)
		}
		if a.WithholdingCode != b.WithholdingCode {
			return a.WithholdingCode < b.WithholdingCode
		}
		return a.TaxExemptionCode < b.TaxExemptionCode
	})
	out := make([]model.TaxSummary, 0, len(keys))
	for _, k := range keys {
		out = append(out, *by[k])
	}
	return out
}

var rateRe = regexp.MustCompile(`^(\d+)`)

// mapExpense: 09 §6. Templates of recurring expenses never reach the
// declaration (rule 2 ⚠️); amount is taken as KDV dahil (09 §9 soru 10 ⚠️).
// The rate is tax_rate ("20_kdv") or tax_rate_percent; without either the
// VAT stays empty (the office sees the receipt flagged "kontrol et").
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
	} else if d.has("tax_rate_percent") {
		r := d.dec("tax_rate_percent")
		rate = &r
	}
	switch {
	case d.has("tax_amount"):
		tax = d.dec("tax_amount")
	case rate != nil:
		tax = amount.Mul(*rate).Div(hundred.Add(*rate)).Round(2)
	}
	cur := model.NormalizeCurrency(firstNonEmpty(d.sub("currency").str("code"), d.str("currency")))
	if cur == "" {
		cur = "TRY"
	}
	return sources.Expense{
		SourceID: d.str("id"), Date: date, Period: date.Format("2006-01"), Amount: amount, TaxAmount: tax, TaxRate: rate,
		Currency: cur, Description: d.str("description"),
		VendorName:    firstNonEmpty(d.sub("customer").str("name"), d.str("vendor_name")),
		VendorTaxID:   vendorTaxID(d),
		PaymentMethod: expensePayment(d),
		ReceiptNumber: firstNonEmpty(d.str("receipt_number"), d.str("document_number"), d.str("reference_document_no")),
		VehiclePlate:  plate(d.str("description")), Raw: map[string]any{"easytrade": map[string]any(d)}, Hash: hash(d),
	}, ""
}

// vendorTaxID: expenses.vendor_tax_number holds a VKN (10) or TCKN (11).
func vendorTaxID(d obj) string {
	cu := d.sub("customer")
	return taxID(cu.str("tax_number"), cu.str("tax_or_identity_number"), cu.str("identity_number"), d.str("vendor_tax_number"))
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstPositive(vs ...decimal.Decimal) decimal.Decimal {
	for _, v := range vs {
		if v.IsPositive() {
			return v
		}
	}
	return decimal.Zero
}

var plateRe = regexp.MustCompile(`\b(0[1-9]|[1-7][0-9]|8[01])\s?[A-Z]{1,3}\s?\d{2,4}\b`)

func plate(s string) string {
	return strings.ReplaceAll(plateRe.FindString(strings.ToUpperSpecial(unicode.TurkishCase, s)), " ", "")
}
