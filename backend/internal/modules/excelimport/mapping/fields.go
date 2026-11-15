// Package mapping holds the target field catalog, column mappings, the
// template fingerprint match and the AI column mapper (04 §4.2, §4.3, §6).
package mapping

import (
	"regexp"
	"sort"
)

// Data types (04 §1). Only the invoice types are committed in this version;
// the rest are recognised so a mapping can be stored and reviewed.
const (
	OutgoingInvoices = "outgoing_invoices"
	IncomingInvoices = "incoming_invoices"
)

// SupportedDataTypes can be committed.
var SupportedDataTypes = []string{OutgoingInvoices, IncomingInvoices}

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
}

// Invoice fields (01 §4.4 names; tax_summary[<rate>].base|vat per 04 §4.3).
var InvoiceFields = []Field{
	{"issue_date", "fatura tarihi"},
	{"invoice_number", "fatura / belge numarası"},
	{"ettn", "ETTN (UUID)"},
	{"customer_name", "karşı taraf adı / unvanı"},
	{"customer_tax_number", "karşı taraf VKN veya TCKN"},
	{"customer_tax_office", "karşı taraf vergi dairesi"},
	{"invoice_type", "fatura türü metni (Satış, Alış, İade, Tevkifatlı)"},
	{"profile", "e-belge türü (e-Fatura, e-Arşiv, Kağıt)"},
	{"currency", "para birimi"},
	{"due_date", "vade tarihi"},
	{"amounts.tax_exclusive_amount", "KDV hariç tutar (matrah)"},
	{"amounts.tax_amount", "toplam KDV"},
	{"amounts.withholding_amount", "tevkifat tutarı"},
	{"amounts.payable_amount", "genel toplam / ödenecek"},
	{"notes", "açıklama"},
	{"source_record_id", "kaynak programın kayıt kimliği"},
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
	var out [][]string
	for _, g := range invoiceRequired {
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
