// Package missingdocs names the documents an office expects from a taxpayer
// every month (#780, ay başı belge toplama): bank statements, Z reports,
// expense receipts and purchase invoices. A kind is "missing" for a closed
// month when the taxpayer had it in one of the three months before and has
// none in that month. The package only builds the shared wording (the title
// of the document request shown to the taxpayer); the periods matrix and the
// document task module both use it, so the same item is never asked twice.
package missingdocs

import (
	"fmt"
	"strings"
)

// Kinds of expected documents.
const (
	KindBankStatement    = "bank_statement"
	KindZReports         = "z_reports"
	KindExpenses         = "expenses"
	KindPurchaseInvoices = "purchase_invoices"
)

// Kinds lists every kind in display order.
var Kinds = []string{KindBankStatement, KindZReports, KindExpenses, KindPurchaseInvoices}

// SourceRule tags the SMS sent with a document request in the taxpayer's
// communication history.
const SourceRule = "Belge isteği — eksik belge"

// ValidKind reports whether k is a known kind.
func ValidKind(k string) bool {
	for _, v := range Kinds {
		if v == k {
			return true
		}
	}
	return false
}

// Item is one missing document of a taxpayer for a period.
type Item struct {
	Kind string `json:"kind"`
	// Name: the bank account of a statement; empty for the other kinds.
	Name string `json:"name"`
	// Title: the request title shown to the taxpayer.
	Title string `json:"title"`
	// Requested: an open (or submitted) request with this title exists.
	Requested bool `json:"requested"`
}

var months = [...]string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}

// PeriodText is "Ekim 2026" for "2026-10"; anything else is returned as is.
func PeriodText(period string) string {
	var y, m int
	if _, err := fmt.Sscanf(period, "%d-%d", &y, &m); err != nil || m < 1 || m > 12 {
		return period
	}
	return fmt.Sprintf("%s %d", months[m-1], y)
}

// Title is the request title of a kind ("Ekim 2026 Ziraat banka ekstresi",
// "Ekim 2026 Z raporları"). The statement wording matches the one the tax
// returns screen used before (#568), so an earlier request is recognised.
func Title(kind, period, name string) string {
	p := PeriodText(period)
	switch kind {
	case KindBankStatement:
		return strings.TrimSpace(fmt.Sprintf("%s %s banka ekstresi", p, strings.TrimSpace(name)))
	case KindZReports:
		return p + " Z raporları"
	case KindExpenses:
		return p + " masraf fişleri"
	case KindPurchaseInvoices:
		return p + " alış faturaları"
	}
	return p + " belgeleri"
}

// Note is the explanation shown under the request.
func Note(kind string) string {
	switch kind {
	case KindBankStatement:
		return "Ayın tüm hareketlerini içeren ekstre (PDF, Excel ya da MT940) gerekiyor. İnternet bankacılığından indirip yükleyebilirsiniz."
	case KindZReports:
		return "Ayın tüm günlerine ait Z raporlarının fotoğrafını ya da PDF'ini yükleyebilirsiniz."
	case KindExpenses:
		return "Ay içinde aldığınız fiş ve masraf belgelerinin fotoğrafını yükleyebilirsiniz."
	case KindPurchaseInvoices:
		return "Ay içinde aldığınız kâğıt ya da e-faturaları (PDF) yükleyebilirsiniz."
	}
	return ""
}

// ShortLabel is the kind's name inside an SMS ("Z raporları").
func ShortLabel(kind, name string) string {
	switch kind {
	case KindBankStatement:
		return strings.TrimSpace(strings.TrimSpace(name) + " ekstresi")
	case KindZReports:
		return "Z raporları"
	case KindExpenses:
		return "masraf fişleri"
	case KindPurchaseInvoices:
		return "alış faturaları"
	}
	return "belgeler"
}

// IsRequested reports whether title is among the open request titles
// (case-insensitive, like the document task module's duplicate check).
func IsRequested(open []string, title string) bool {
	for _, t := range open {
		if strings.EqualFold(strings.TrimSpace(t), title) {
			return true
		}
	}
	return false
}

// Build lists a period's missing items in display order: statements by
// account, then Z reports, expenses and purchase invoices.
func Build(period string, statements []string, z, expenses, purchases bool, open []string) []Item {
	out := make([]Item, 0, len(statements)+3)
	add := func(kind, name string) {
		t := Title(kind, period, name)
		out = append(out, Item{Kind: kind, Name: name, Title: t, Requested: IsRequested(open, t)})
	}
	for _, n := range statements {
		add(KindBankStatement, n)
	}
	if z {
		add(KindZReports, "")
	}
	if expenses {
		add(KindExpenses, "")
	}
	if purchases {
		add(KindPurchaseInvoices, "")
	}
	return out
}
