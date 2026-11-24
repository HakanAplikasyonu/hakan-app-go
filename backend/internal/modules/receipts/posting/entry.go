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
	// VatLines is the per-rate breakdown (#780). When present the VAT is
	// booked on one 191 line per rate (the plan's sub-account of that rate)
	// and Tax / TaxRate are not used for the VAT lines. Empty: the old single
	// VAT line ("kırılım yok": NoBreakdownNote marks it when set).
	VatLines []VatRate
	// NoBreakdownNote is added to the single VAT line's description when
	// the expense has no breakdown (Luca export, #780).
	NoBreakdownNote string
}

// VatRate is the VAT of one rate.
type VatRate struct {
	Rate decimal.Decimal
	Tax  decimal.Decimal
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
	if len(in.VatLines) > 0 {
		tax = decimal.Zero
		for _, l := range in.VatLines {
			if l.Tax.IsPositive() {
				tax = tax.Add(l.Tax)
			}
		}
		if tax.GreaterThan(in.Total) {
			return Entry{}, errVATOverTotal
		}
	}
	net := in.Total.Sub(tax)
	var lines []Line
	if tax.IsPositive() && in.Deductible && len(in.VatLines) > 0 {
		vat := in.VATAccount
		if vat == "" {
			vat = "191"
		}
		lines = append(lines, Line{Account: in.ExpenseAccount, Description: desc, Debit: net, SourceID: in.SourceID, Confidence: in.Confidence})
		for _, l := range in.VatLines {
			if !l.Tax.IsPositive() {
				continue
			}
			acc := ResolveLeaf(vat, "kdv", in.Plan).Code
			if a, ok := engine.VATSubAccount(vat, l.Rate, in.Plan); ok {
				acc = a.Code
			}
			r := l.Rate
			lines = append(lines, Line{Account: acc, Description: "İndirilecek KDV " + rateText(&r), Debit: l.Tax, SourceID: in.SourceID, Confidence: in.Confidence})
		}
	} else if tax.IsPositive() && in.Deductible {
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
		} else if in.NoBreakdownNote != "" {
			// Kırılım yok (#780): with several rate sub-accounts none is
			// picked (that would choose a rate); the main code stays and
			// the line is marked.
			if r := ResolveLeaf(vat, "kdv", in.Plan); !r.InPlan || r.Confidence.Equal(confExact) {
				vat = r.Code
			}
		} else {
			vat = ResolveLeaf(vat, "kdv", in.Plan).Code
		}
		lines = append(lines,
			Line{Account: in.ExpenseAccount, Description: desc, Debit: net, SourceID: in.SourceID, Confidence: in.Confidence},
			Line{Account: vat, Description: strings.TrimSpace("İndirilecek KDV " + rateText(in.TaxRate) + " " + in.NoBreakdownNote), Debit: tax, SourceID: in.SourceID, Confidence: in.Confidence})
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
