package posting

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
)

// SourceKind is journal_draft_lines.source_kind of receipt lines.
const SourceKind = "receipt"

// Line is one journal line (journals.LineInput without the API details).
type Line struct {
	Account     string
	Description string
	Debit       decimal.Decimal
	Credit      decimal.Decimal
	SourceID    string
	Confidence  decimal.Decimal
}

// Entry is a balanced journal draft.
type Entry struct {
	Date         time.Time
	Description  string
	DocumentType string
	DocumentNo   string
	Lines        []Line
}

// PaymentMain is the main account a payment method is booked to. Unknown
// method: 100 Kasa (a receipt is most often paid in cash; the accountant
// changes it on the draft).
func PaymentMain(method string) (main, hint string) {
	switch method {
	case "bank_transfer":
		return "102", "banka"
	case "credit_card":
		return "309", "kredi kartı"
	case "check":
		return "103", "çek"
	case "promissory_note":
		return "321", "senet"
	case "credit":
		return "320", "satıcı"
	default:
		return "100", "kasa"
	}
}

// ExpenseInput is an approved expense.
type ExpenseInput struct {
	SourceID       string // "expense:<uuid>"
	Date           time.Time
	Total          decimal.Decimal
	Tax            decimal.Decimal // zero when unknown
	TaxRate        *decimal.Decimal
	Deductible     bool
	ExpenseAccount string
	VATAccount     string // main or full; resolved by rate
	PaymentAccount string // already resolved
	VendorName     string
	ReceiptNumber  string
	CategoryName   string
	Confidence     decimal.Decimal
	Plan           []engine.PlanAccount
}

// ExpenseEntry: gider (net) borç, indirilecek KDV borç, ödeme alacak.
// When the VAT is not deductible it is added to the expense line.
func ExpenseEntry(in ExpenseInput) (Entry, error) {
	if !in.Total.IsPositive() {
		return Entry{}, errNoAmount
	}
	tax := in.Tax
	if tax.IsNegative() || tax.GreaterThan(in.Total) {
		tax = decimal.Zero
	}
	desc := strings.TrimSpace(strings.Join(nonEmpty(in.CategoryName, in.VendorName, in.ReceiptNumber), " - "))
	if desc == "" {
		desc = "Masraf fişi"
	}
	net := in.Total.Sub(tax)
	var lines []Line
	if tax.IsPositive() && in.Deductible {
		vat := in.VATAccount
		if vat == "" {
			vat = "191"
		}
		if in.TaxRate != nil {
			if a, ok := engine.VATSubAccount(vat, *in.TaxRate, in.Plan); ok {
				vat = a.Code
			} else {
				vat = ResolveLeaf(vat, "kdv", in.Plan).Code
			}
		} else {
			vat = ResolveLeaf(vat, "kdv", in.Plan).Code
		}
		lines = append(lines,
			Line{Account: in.ExpenseAccount, Description: desc, Debit: net, SourceID: in.SourceID, Confidence: in.Confidence},
			Line{Account: vat, Description: "İndirilecek KDV " + rateText(in.TaxRate), Debit: tax, SourceID: in.SourceID, Confidence: in.Confidence})
	} else {
		lines = append(lines, Line{Account: in.ExpenseAccount, Description: desc, Debit: in.Total, SourceID: in.SourceID, Confidence: in.Confidence})
	}
	lines = append(lines, Line{Account: in.PaymentAccount, Description: desc, Credit: in.Total, SourceID: in.SourceID, Confidence: in.Confidence})
	return Entry{Date: in.Date, Description: "Masraf: " + desc, DocumentType: "receipt", DocumentNo: in.ReceiptNumber, Lines: trimLines(lines)}, nil
}

func rateText(r *decimal.Decimal) string {
	if r == nil {
		return ""
	}
	return "%" + r.String()
}

func nonEmpty(s ...string) []string {
	var out []string
	for _, v := range s {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// trimLines drops zero lines and rounds to kuruş.
func trimLines(in []Line) []Line {
	out := in[:0]
	for _, l := range in {
		l.Debit, l.Credit = l.Debit.Round(2), l.Credit.Round(2)
		if l.Debit.IsZero() && l.Credit.IsZero() {
			continue
		}
		out = append(out, l)
	}
	return out
}

// Balanced reports whether debit equals credit.
func (e Entry) Balanced() bool {
	var d, c decimal.Decimal
	for _, l := range e.Lines {
		d, c = d.Add(l.Debit), c.Add(l.Credit)
	}
	return d.Equal(c)
}

// String is for logs and tests.
func (l Line) String() string {
	return fmt.Sprintf("%s D%s C%s", l.Account, l.Debit.StringFixed(2), l.Credit.StringFixed(2))
}
