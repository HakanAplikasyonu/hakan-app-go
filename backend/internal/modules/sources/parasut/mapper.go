package parasut

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

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
)

// SourceSystem of everything read from Paraşüt.
const SourceSystem = "parasut"

var hundred = decimal.NewFromInt(100)

// Expense is a kalemsiz, e-belgesiz purchase bill (10 §4.3, Kalem 5).
type Expense struct {
	SourceID     string
	Date         time.Time
	Period       string
	Amount       decimal.Decimal // net_total, KDV dahil
	TaxAmount    decimal.Decimal
	TaxRate      *decimal.Decimal // header estimate
	Currency     string
	Description  string
	VendorName   string
	VendorTaxID  string
	VehiclePlate string
	Raw          map[string]any
	Hash         string
}

// Excluded counts what was read but not written, by reason.
type Excluded map[string]int

// Batch is one window's mapped records.
type Batch struct {
	Invoices []model.Invoice
	Expenses []Expense
	Excluded Excluded
}

const salesInclude = "contact,details,details.product,active_e_document"
const purchaseInclude = "supplier,details,details.product,active_e_document"

// Read fetches sales invoices and purchase bills issued in [from, to]
// (dates, inclusive) and maps them to the common model (10 §3–4, §7.1).
func (c *Client) Read(ctx context.Context, from, to time.Time) (Batch, error) {
	b := Batch{Excluded: Excluded{}}
	in := func(r resource) (bool, bool) { // in window, older than window
		d, err := time.Parse("2006-01-02", r.str("issue_date"))
		if err != nil {
			return false, false
		}
		return !d.Before(from) && !d.After(to), d.Before(from)
	}
	q := url.Values{"sort": {"-issue_date"}, "include": {salesInclude}}
	err := c.pages(ctx, "/sales_invoices", q, func(r resource, ix index) (bool, error) {
		ok, older := in(r)
		if older {
			return true, nil
		}
		if !ok {
			return false, nil
		}
		inv, skip := mapSales(r, ix)
		if skip != "" {
			b.Excluded[skip]++
			return false, nil
		}
		b.Invoices = append(b.Invoices, inv)
		return false, nil
	})
	if err != nil {
		return b, err
	}
	q = url.Values{"sort": {"-issue_date"}, "include": {purchaseInclude}}
	err = c.pages(ctx, "/purchase_bills", q, func(r resource, ix index) (bool, error) {
		ok, older := in(r)
		if older {
			return true, nil
		}
		if !ok {
			return false, nil
		}
		inv, exp, skip := mapPurchase(r, ix)
		switch {
		case skip != "":
			b.Excluded[skip]++
		case exp != nil:
			b.Expenses = append(b.Expenses, *exp)
		default:
			b.Invoices = append(b.Invoices, inv)
		}
		return false, nil
	})
	return b, err
}

func hash(r resource) string {
	raw, _ := json.Marshal(r)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func date(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	return t, err == nil
}

// currency: Paraşüt writes TRL (10 §3.3).
func currency(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" || s == "TRL" || s == "TL" {
		return "TRY"
	}
	return model.NormalizeCurrency(s)
}

func party(r resource, ok bool, invTaxNumber, invTaxOffice string) model.Party {
	var p model.Party
	if ok {
		p.Name = r.str("name")
		p.TaxOffice = r.str("tax_office")
		p.City, p.District, p.Address = r.str("city"), r.str("district"), r.str("address")
		p.Email, p.Phone = r.str("email"), r.str("phone")
		if r.str("contact_type") == "person" {
			p.LegalType = "individual"
		} else if r.str("contact_type") == "company" {
			p.LegalType = "corporate"
		}
		invTaxNumber = firstNonEmpty(invTaxNumber, r.str("tax_number"))
	}
	if invTaxOffice != "" {
		p.TaxOffice = invTaxOffice
	}
	// The invoice's own tax number wins (10 §3.3).
	if vkn, tckn, ok := model.ClassifyTaxID(invTaxNumber); ok {
		p.TaxNumber, p.IdentityNumber = vkn, tckn
	}
	return p
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// profile from the active e-document (10 §3.6); TEMEL/TICARI is not
// visible on the relation ⚠️, so e-invoices keep profile_raw only.
func profile(r resource) (*string, string) {
	d, ok := r.rel("active_e_document").one()
	if !ok {
		return nil, ""
	}
	switch d.Type {
	case "e_archives":
		p := model.ProfileEArsiv
		return &p, d.Type
	default:
		return nil, d.Type
	}
}

func items(r resource, ix index) []model.InvoiceItem {
	var out []model.InvoiceItem
	for i, d := range r.rel("details").many() {
		det, ok := ix.get(d)
		if !ok {
			continue
		}
		qty, unit := det.dec("quantity"), det.dec("unit_price")
		net := det.dec("net_total").Round(2) // KDV hariç ⚠️ (10 §3.4)
		rate := det.dec("vat_rate")
		lineExt := qty.Mul(unit).Round(2)
		allow := lineExt.Sub(net)
		if allow.IsNegative() {
			allow = decimal.Zero
		}
		vat := net.Mul(rate).Div(hundred).Round(2)
		name := det.str("description")
		code := ""
		if p, ok := det.rel("product").one(); ok {
			if pr, ok := ix.get(p); ok {
				code = pr.str("code")
				if name == "" {
					name = pr.str("name")
				}
			}
		}
		out = append(out, model.InvoiceItem{
			LineNo: i + 1, Name: name, ProductCode: code, Quantity: qty, UnitPrice: unit,
			LineExtensionAmount: lineExt, OwnAllowanceAmount: allow, AllowanceTotalAmount: allow,
			TaxExclusiveAmount: net, TaxRate: rate, TotalTaxAmount: vat, TaxInclusiveAmount: net.Add(vat),
			ExciseRate: det.dec("excise_duty_value"), CommunicationsTaxRate: det.dec("communications_tax_rate"),
			LineType: model.LineUnknown,
		})
	}
	return out
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
	}
	sort.Strings(keys)
	out := make([]model.TaxSummary, 0, len(keys))
	for _, k := range keys {
		out = append(out, *by[k])
	}
	return out
}

// headerEstimate: no lines → one summary with the nearest legal rate (01 §4.5).
func headerEstimate(base, vat decimal.Decimal) ([]model.TaxSummary, *decimal.Decimal) {
	if base.IsZero() {
		return nil, nil
	}
	rate := model.NearestVATRate(vat.Div(base).Mul(hundred))
	return []model.TaxSummary{{TaxRate: rate, Base: base, VAT: vat}}, &rate
}

func finish(inv *model.Invoice, r resource, ix index, its []model.InvoiceItem) {
	inv.Items = its
	if len(its) > 0 {
		inv.HasItems, inv.TaxSummarySource = true, model.TaxSummaryFromItems
		inv.TaxSummaries = summaries(its)
	} else {
		inv.TaxSummarySource = model.TaxSummaryFromHeaderEstimate
		inv.TaxSummaries, _ = headerEstimate(inv.Amounts.TaxExclusiveAmount, inv.Amounts.TaxAmount)
	}
	inv.Amounts.TaxInclusiveAmount = inv.Amounts.TaxExclusiveAmount.Add(inv.Amounts.TaxAmount)
	inv.Amounts.PayableAmountTRY = inv.Amounts.PayableAmount
	if inv.Currency != "TRY" && inv.ExchangeRate != nil {
		inv.Amounts.PayableAmountTRY = inv.Amounts.PayableAmount.Mul(*inv.ExchangeRate).Round(2)
	}
	inv.ContentHash = hash(r)
	inv.RawPayload = map[string]any{"parasut": r.attrs()}
	_ = ix
}

func base(inv *model.Invoice, r resource) bool {
	d, ok := date(r.str("issue_date"))
	if !ok {
		return false
	}
	inv.SourceSystem, inv.CaptureMethod = SourceSystem, model.CaptureAPI
	inv.IssueDate, inv.Period = d, d.Format("2006-01")
	if due, ok := date(r.str("due_date")); ok {
		inv.PaymentDueDate = &due
	}
	inv.Currency = currency(r.str("currency"))
	if x := r.dec("exchange_rate"); !x.IsZero() && inv.Currency != "TRY" {
		inv.ExchangeRate = &x
	}
	inv.Notes = r.str("description")
	inv.DocumentClass = "invoice"
	inv.Profile, inv.ProfileRaw = profile(r)
	return true
}

// mapSales: 10 §3. skip names the reason a record is not written.
func mapSales(r resource, ix index) (model.Invoice, string) {
	var inv model.Invoice
	switch r.str("item_type") {
	case "invoice":
		inv.Type = model.TypeSatis
	case "refund":
		inv.Type = model.TypeIade
	case "cancelled":
		return inv, "cancelled" // status_normalized=cancelled is not in the model yet
	default:
		return inv, "not_an_invoice" // estimate, recurring_* (10 §3.2)
	}
	if !base(&inv, r) {
		return inv, "no_issue_date"
	}
	inv.SourceID = "sales_invoices:" + r.ID
	inv.Direction = model.DirectionOutgoing
	inv.InvoiceNumber = model.NormalizeInvoiceNumber(r.str("invoice_series") + r.str("invoice_id"))
	c, ok := r.rel("contact").one()
	var cr resource
	if ok {
		cr, ok = ix.get(c)
	}
	inv.Counterparty = party(cr, ok, r.str("tax_number"), r.str("tax_office"))
	a := &inv.Amounts
	a.LineExtensionAmount = r.dec("gross_total")
	a.AllowanceTotalAmount = r.dec("total_discount").Add(r.dec("total_invoice_discount"))
	a.TaxExclusiveAmount = r.dec("before_taxes_total")
	if a.TaxExclusiveAmount.IsZero() {
		a.TaxExclusiveAmount = a.LineExtensionAmount.Sub(a.AllowanceTotalAmount)
	}
	a.TaxAmount = r.dec("total_vat")
	a.WithholdingAmount = r.dec("vat_withholding")
	a.IncomeTaxWithholdingAmount = r.dec("withholding")
	a.ExciseAmount = r.dec("total_excise_duty")
	a.CommunicationsTaxAmount = r.dec("total_communications_tax")
	a.PayableAmount = r.dec("net_total")
	if inv.Type == model.TypeSatis && a.WithholdingAmount.IsPositive() {
		inv.Type = model.TypeTevkifat
	}
	finish(&inv, r, ix, items(r, ix))
	return inv, ""
}

// mapPurchase: 10 §4. A kalemsiz bill without an e-document is an expense
// (10 §4.3).
func mapPurchase(r resource, ix index) (model.Invoice, *Expense, string) {
	var inv model.Invoice
	switch r.str("item_type") {
	case "purchase_bill":
		inv.Type = model.TypeAlis
	case "refund":
		inv.Type = model.TypeIade
	case "cancelled":
		return inv, nil, "cancelled"
	default:
		return inv, nil, "not_an_invoice" // recurring_purchase_bill
	}
	if !base(&inv, r) {
		return inv, nil, "no_issue_date"
	}
	s, ok := r.rel("supplier").one()
	var sr resource
	if ok {
		sr, ok = ix.get(s)
	}
	vendor := party(sr, ok, r.str("tax_number"), "")
	_, hasEDoc := r.rel("active_e_document").one()
	gross := r.dec("gross_total")
	allow := r.dec("total_discount").Add(r.dec("total_invoice_discount"))
	vat := r.dec("total_vat")
	net := r.dec("net_total")

	if !r.boolean("is_detailed") && !hasEDoc {
		excl := net.Sub(vat)
		_, rate := headerEstimate(excl, vat)
		e := &Expense{
			SourceID: "purchase_bills:" + r.ID, Date: inv.IssueDate, Period: inv.Period, Amount: net, TaxAmount: vat,
			TaxRate: rate, Currency: inv.Currency, Description: r.str("description"),
			VendorName: vendor.Name, VendorTaxID: firstNonEmpty(vendor.TaxNumber, vendor.IdentityNumber),
			VehiclePlate: plate(r.str("description")), Raw: map[string]any{"parasut": r.attrs()}, Hash: hash(r),
		}
		return inv, e, ""
	}

	inv.SourceID = "purchase_bills:" + r.ID
	inv.Direction = model.DirectionIncoming
	inv.InvoiceNumber = model.NormalizeInvoiceNumber(r.str("invoice_no"))
	inv.Counterparty = vendor
	a := &inv.Amounts
	a.LineExtensionAmount = gross
	a.AllowanceTotalAmount = allow
	a.TaxExclusiveAmount = gross.Sub(allow) // no before_taxes_total on bills (10 §4.4)
	a.TaxAmount = vat
	a.ExciseAmount = r.dec("total_excise_duty")
	a.CommunicationsTaxAmount = r.dec("total_communications_tax")
	a.PayableAmount = net
	its := items(r, ix)
	// Tevkifat: Σ(line VAT) × rate (10 §4.4); the rate format is ⚠️
	// (50 or 0.5), both read as percent.
	if wr := model.NormalizePercent(r.dec("vat_withholding_rate")); wr.IsPositive() {
		a.WithholdingAmount = vat.Mul(wr).Div(hundred).Round(2)
		if inv.Type == model.TypeAlis {
			inv.Type = model.TypeTevkifat
		}
	}
	finish(&inv, r, ix, its)
	return inv, nil, ""
}

// plate: 10 §7.4 regex on the upper-cased (Turkish) description.
var plateRe = regexp.MustCompile(`\b(0[1-9]|[1-7][0-9]|8[01])\s?[A-Z]{1,3}\s?\d{2,4}\b`)

func plate(s string) string {
	m := plateRe.FindString(strings.ToUpperSpecial(trCase, s))
	return strings.ReplaceAll(m, " ", "")
}
