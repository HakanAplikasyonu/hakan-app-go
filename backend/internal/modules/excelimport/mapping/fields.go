// Package mapping holds the target field catalog, column mappings, the
// template fingerprint match and the AI column mapper (04 §4.2, §4.3, §6).
package mapping

import (
	"regexp"
	"sort"
)

// Data types (04 §1). Invoices are committed by this module; payroll,
// expenses, Z reports and bank movements by their own modules (#395).
const (
	OutgoingInvoices = "outgoing_invoices"
	IncomingInvoices = "incoming_invoices"
)

// SupportedDataTypes can be committed.
var SupportedDataTypes = []string{OutgoingInvoices, IncomingInvoices, Payroll, Expenses, RetailSales, BankTransactions}

// Supported reports whether dt can be imported.
func Supported(dt string) bool {
	for _, s := range SupportedDataTypes {
		if s == dt {
			return true
		}
	}
	return false
}

// Field is a target field of the common model.
type Field struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Kind        string `json:"kind,omitempty"`
}

// Invoice fields (01 §4.4 names; tax_summary[<rate>].base|vat per 04 §4.3).
var InvoiceFields = []Field{
	{Key: "issue_date", Description: "fatura tarihi"},
	{Key: "invoice_number", Description: "fatura / belge numarası"},
	{Key: "ettn", Description: "ETTN (UUID)"},
	{Key: "customer_name", Description: "karşı taraf adı / unvanı"},
	{Key: "customer_tax_number", Description: "karşı taraf VKN veya TCKN"},
	{Key: "customer_tax_office", Description: "karşı taraf vergi dairesi"},
	{Key: "invoice_type", Description: "fatura türü metni (Satış, Alış, İade, Tevkifatlı)"},
	{Key: "profile", Description: "e-belge türü (e-Fatura, e-Arşiv, Kağıt)"},
	{Key: "currency", Description: "para birimi"},
	{Key: "due_date", Description: "vade tarihi"},
	{Key: "amounts.tax_exclusive_amount", Description: "KDV hariç tutar (matrah)"},
	{Key: "amounts.tax_amount", Description: "toplam KDV"},
	{Key: "amounts.withholding_amount", Description: "tevkifat tutarı"},
	{Key: "amounts.payable_amount", Description: "genel toplam / ödenecek"},
	{Key: "notes", Description: "açıklama"},
	{Key: "source_record_id", Description: "kaynak programın kayıt kimliği"},
}

var taxSummaryRe = regexp.MustCompile(`^tax_summary\[(\d{1,2}(?:\.\d{1,2})?)\]\.(base|vat)$`)

// TaxSummaryField parses "tax_summary[20].vat".
func TaxSummaryField(f string) (rate, part string, ok bool) {
	m := taxSummaryRe.FindStringSubmatch(f)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// ValidField reports whether f is a known target for dt.
func ValidField(dt, f string) bool {
	if !Supported(dt) {
		return false
	}
	if !IsInvoice(dt) {
		c, _ := CatalogFor(dt)
		return c.Has(f)
	}
	if _, _, ok := TaxSummaryField(f); ok {
		return true
	}
	for _, x := range InvoiceFields {
		if x.Key == f {
			return true
		}
	}
	return false
}

// Required groups: each group needs at least one mapped field (04 §1).
var invoiceRequired = [][]string{
	{"issue_date"},
	{"invoice_number"},
	{"customer_name", "customer_tax_number"},
	{"amounts.tax_exclusive_amount", "amounts.payable_amount"},
}

// MissingRequired lists the unmet required groups of a mapping.
func MissingRequired(dt string, m Mapping) [][]string {
	mapped := map[string]bool{}
	for _, c := range m.Columns {
		if c.Field != "" {
			mapped[c.Field] = true
		}
	}
	required := invoiceRequired
	if c, ok := CatalogFor(dt); ok {
		required = c.Required
	}
	var out [][]string
	for _, g := range required {
		hit := false
		for _, f := range g {
			if mapped[f] {
				hit = true
			}
		}
		if !hit {
			out = append(out, g)
		}
	}
	return out
}

// FieldKeys returns the plain field keys, sorted.
func FieldKeys() []string {
	out := make([]string, 0, len(InvoiceFields))
	for _, f := range InvoiceFields {
		out = append(out, f.Key)
	}
	sort.Strings(out)
	return out
}
