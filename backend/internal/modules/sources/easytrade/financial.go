package easytrade

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources"
)

// ErrNotSupported: Easytrade has no endpoint for the item (404 on its list).
// The window is done and the item is reported in Counters.Unsupported, so
// the run is not partial: nothing is missing that Easytrade has.
var ErrNotSupported = errors.New("easytrade: endpoint not offered by Easytrade")

// ZReportSource reads fiscal Z reports. Easytrade has none today: its
// day-end sessions (eod-sessions) are cash closings, not Z (09 §4 kural 4–5).
// HTTPZReports calls the endpoint the portal asks Easytrade for
// (docs/easytrade-senkron.md §4); tests and a future ÖKC/Paygo source can
// plug in their own.
type ZReportSource interface {
	ZReports(ctx context.Context, c *Client, from, to time.Time) ([]obj, error)
}

// HTTPZReports reads GET /company/z-reports (list, report_date desc) and
// GET /company/z-reports/{id} (detail), per branch like every list.
type HTTPZReports struct{}

// ZReports implements ZReportSource.
func (HTTPZReports) ZReports(ctx context.Context, c *Client, from, to time.Time) ([]obj, error) {
	ids, err := c.listIDs(ctx, "/company/z-reports", "report_date", from, to)
	var he *HTTPError
	if errors.As(err, &he) && he.Status == 404 {
		return nil, ErrNotSupported
	}
	if err != nil {
		return nil, err
	}
	out := make([]obj, 0, len(ids))
	for _, id := range ids {
		var d struct {
			Data obj `json:"data"`
		}
		if err := c.get(ctx, "/company/z-reports/"+id, nil, &d); err != nil {
			return nil, err
		}
		out = append(out, d.Data)
	}
	return out, nil
}

func (c *Client) readFinancial(ctx context.Context, b *sources.Batch, seen map[string]bool, from, to time.Time) error {
	ids, err := c.listIDs(ctx, "/company/financial-transactions", "transaction_date", from, to)
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
		if err := c.get(ctx, "/company/financial-transactions/"+id, nil, &d); err != nil {
			return err
		}
		f, skip := mapFinancial(d.Data)
		if skip == "" && !inWindow(f.Date, from, to) {
			// Listed without a readable date; its detail is outside.
			delete(seen, id)
			continue
		}
		if skip != "" {
			b.Excluded[skip]++
			continue
		}
		b.FinancialTransactions = append(b.FinancialTransactions, f)
	}
	return nil
}

func (c *Client) readZReports(ctx context.Context, b *sources.Batch, seen map[string]bool, from, to time.Time) error {
	src := c.ZReports
	if src == nil {
		src = HTTPZReports{}
	}
	ds, err := src.ZReports(ctx, c, from, to)
	if err != nil {
		return err
	}
	for _, d := range ds {
		id := d.str("id")
		if seen[id] {
			continue
		}
		seen[id] = true
		z, skip := mapZReport(d)
		if skip == "" && !inWindow(z.Date, from, to) {
			delete(seen, id)
			continue
		}
		if skip != "" {
			b.Excluded[skip]++
			continue
		}
		b.ZReports = append(b.ZReports, z)
	}
	return nil
}

// PaymentMethodType normalises Easytrade's payment_method_type (01 §4.9:
// cash, bank_transfer, credit_card, check, promissory_note, credit) and the
// Turkish labels seen in exports. Havale and EFT are both bank_transfer in
// Easytrade. Unknown values are "unknown"; the raw value stays in the
// payload.
func PaymentMethodType(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	s = strings.NewReplacer("ı", "i", "ş", "s", "ç", "c", "ğ", "g", "ö", "o", "ü", "u", "-", "_", " ", "_").Replace(s)
	switch s {
	case "cash", "nakit":
		return "cash"
	case "bank_transfer", "bank", "transfer", "wire", "havale", "eft", "havale_eft", "fast", "banka":
		return "bank_transfer"
	case "credit_card", "card", "kredi_karti", "kart", "pos", "debit_card", "banka_karti":
		return "credit_card"
	case "check", "cheque", "cek":
		return "check"
	case "promissory_note", "senet":
		return "promissory_note"
	case "credit", "veresiye", "acik_hesap", "on_account":
		return "credit"
	}
	return "unknown"
}

// direction: transaction_flow first, then transaction_type (09 §9 soru 8 ⚠️:
// the meanings are Easytrade's to confirm).
func direction(flow, typ string) string {
	switch strings.ToLower(flow) {
	case "in", "incoming", "inflow", "giris", "giriş", "collection", "tahsilat", "credit":
		return "in"
	case "out", "outgoing", "outflow", "cikis", "çıkış", "payment", "odeme", "ödeme", "debit":
		return "out"
	}
	switch strings.ToLower(typ) {
	case "income", "retail_sale", "collection", "tahsilat":
		return "in"
	case "expense", "payment", "odeme", "ödeme":
		return "out"
	}
	return "unknown"
}

// shortType turns Laravel morph types (App\Models\Invoice) into Invoice.
func shortType(s string) string {
	if i := strings.LastIndexAny(s, `\/`); i >= 0 {
		return s[i+1:]
	}
	return s
}

func taxID(vs ...string) string {
	for _, v := range vs {
		if vkn, tckn, ok := model.ClassifyTaxID(v); ok {
			return firstNonEmpty(vkn, tckn)
		}
	}
	return ""
}

func currencyOf(d obj) string {
	cur := model.NormalizeCurrency(firstNonEmpty(d.sub("currency").str("code"), d.str("currency")))
	if cur == "" {
		return "TRY"
	}
	return cur
}

// mapFinancial: 01 §4.9, 09 §5. Cancelled movements never reach the
// portal; opening balances are kept (direction unknown) for the cari view.
func mapFinancial(d obj) (sources.FinancialTransaction, string) {
	date, ok := d.date("transaction_date")
	if !ok {
		return sources.FinancialTransaction{}, "no_transaction_date"
	}
	status := d.str("status")
	if s := strings.ToLower(status); strings.Contains(s, "cancel") || strings.Contains(s, "iptal") {
		return sources.FinancialTransaction{}, "cancelled"
	}
	amount := d.dec("amount").Abs()
	cur := currencyOf(d)
	f := sources.FinancialTransaction{
		SourceID: d.str("id"), Date: date, Period: date.Format("2006-01"),
		TransactionType: d.str("transaction_type"), TransactionFlow: d.str("transaction_flow"),
		PaymentMethodType: PaymentMethodType(d.str("payment_method_type")),
		Amount:            amount, Currency: cur, AmountTRY: amount,
		Description: d.str("description"), ReferenceDocumentNo: d.str("reference_document_no"), Status: status,
		CustomerSourceID: firstNonEmpty(d.str("customer_id"), d.sub("customer").str("id")),
		CustomerName:     firstNonEmpty(d.sub("customer").str("name"), d.str("customer_name")),
		CustomerTaxNumber: taxID(d.sub("customer").str("tax_or_identity_number"), d.sub("customer").str("tax_number"), d.sub("customer").str("identity_number"),
			d.str("customer_tax_number")),
		CashBoxSourceID:   firstNonEmpty(d.str("cash_box_id"), d.sub("cash_box").str("id")),
		CashBoxName:       d.sub("cash_box").str("name"),
		CashBoxType:       d.sub("cash_box").str("type"),
		ReferenceableType: shortType(d.str("referenceable_type")),
		ReferenceableID:   d.str("referenceable_id"),
		Raw:               map[string]any{"easytrade": map[string]any(d)}, Hash: hash(d),
	}
	f.Direction = direction(f.TransactionFlow, f.TransactionType)
	if due, ok := d.date("due_date"); ok {
		f.DueDate = &due
	}
	if x := d.dec("exchange_rate"); x.IsPositive() && cur != "TRY" {
		f.ExchangeRate = &x
		f.AmountTRY = amount.Mul(x).Round(2)
	}
	return f, ""
}

// mapZReport maps the proposed Z report (docs/easytrade-senkron.md §4).
// The VAT breakdown and payment totals stay in the payload.
func mapZReport(d obj) (sources.ZReport, string) {
	date, ok := d.date("report_date")
	if !ok {
		if date, ok = d.date("z_date"); !ok {
			return sources.ZReport{}, "no_report_date"
		}
	}
	total, tax := d.dec("total_amount"), d.dec("tax_amount")
	sub := d.dec("subtotal_amount")
	if sub.IsZero() && total.IsPositive() {
		sub = total.Sub(tax)
	}
	return sources.ZReport{
		SourceID: "z:" + d.str("id"), Date: date, Period: date.Format("2006-01"),
		ZReportNumber: firstNonEmpty(d.str("z_report_number"), d.str("z_number")), OKCSerialNumber: d.str("okc_serial_number"),
		Subtotal: sub, Tax: tax, Total: total, Currency: currencyOf(d),
		Raw: map[string]any{"easytrade": map[string]any(d)}, Hash: hash(d),
	}, ""
}

// ZPayments reads the payment totals of a stored Z report payload:
// payments as {cash: "…", credit_card: "…"} or [{payment_method_type, amount}].
func ZPayments(raw map[string]any) map[string]decimal.Decimal {
	d := obj(raw).sub("easytrade")
	out := map[string]decimal.Decimal{}
	if m := d.sub("payments"); len(m) > 0 {
		for k := range m {
			out[PaymentMethodType(k)] = out[PaymentMethodType(k)].Add(m.dec(k))
		}
		return out
	}
	for _, p := range d.list("payments") {
		k := PaymentMethodType(p.str("payment_method_type"))
		out[k] = out[k].Add(p.dec("amount"))
	}
	return out
}

// expensePayment maps an expense's payment to expenses.payment_method
// (no "unknown" there: NULL).
func expensePayment(d obj) string {
	v := firstNonEmpty(d.str("payment_method_type"), d.str("payment_method"))
	if v == "" {
		for _, ft := range d.list("financial_transactions") {
			if v = ft.str("payment_method_type"); v != "" {
				break
			}
		}
	}
	if p := PaymentMethodType(v); p != "unknown" {
		return p
	}
	return ""
}
