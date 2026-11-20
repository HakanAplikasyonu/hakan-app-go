// Package posting turns an invoice into its accounting entry (#669, Oğuz's
// decision 20.11: the app books the invoices and sends them to Luca with the
// other entries).
//
//	Alış: satır hesabı (153 / 150 / 7xx) B net, 191 B KDV, 320 cari A toplam.
//	Satış: 120 cari B toplam, satır hesabı (600 / 601 / 602) A net, 391 A KDV.
//
// It is pure: the use case loads the invoice, its lines' approved account
// codes, the cari and the plan. Invoices it cannot book correctly yet
// (foreign currency, return, withholding, exemption, special taxes) are
// refused with a reason: never a silently wrong entry.
package posting

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
)

// SourceKind is journal_draft_lines.source_kind of invoice lines; the
// source id is the invoice's internal id (reconciliation reads this link).
const SourceKind = "invoice"

// vatTypeCode is the UBL tax type code of KDV.
const vatTypeCode = "0015"

// Reasons an invoice is not booked (Refusal.Code); the screen shows them.
const (
	ReasonNotActive       = "NOT_ACTIVE"
	ReasonForeignCurrency = "FOREIGN_CURRENCY"
	ReasonReturn          = "RETURN_INVOICE"
	ReasonWithholding     = "WITHHOLDING"
	ReasonExemption       = "EXEMPTION"
	ReasonSpecialTax      = "SPECIAL_TAX"
	ReasonUnsupportedType = "UNSUPPORTED_TYPE"
	ReasonNoItems         = "NO_ITEMS"
	ReasonTotalsMismatch  = "TOTALS_MISMATCH"
	ReasonItemAccount     = "ITEM_ACCOUNT_MISSING"
)

// Refusal says why an invoice is not turned into an entry.
type Refusal struct {
	Code   string
	LineNo int // ITEM_ACCOUNT_MISSING
	Detail string
}

func (r *Refusal) Error() string {
	if r.LineNo > 0 {
		return fmt.Sprintf("posting: %s (line %d)", r.Code, r.LineNo)
	}
	if r.Detail != "" {
		return fmt.Sprintf("posting: %s (%s)", r.Code, r.Detail)
	}
	return "posting: " + r.Code
}

func refuse(code, detail string) *Refusal { return &Refusal{Code: code, Detail: detail} }

// Tax is one tax subtotal of a line (model.TaxBreakdown, amounts parsed).
type Tax struct {
	Kind          string // tax | withholding
	TypeCode      string
	Amount        decimal.Decimal
	ExemptionCode string
}

// Item is one invoice line with its approved account codes.
type Item struct {
	LineNo     int
	Name       string
	Net        decimal.Decimal // KDV hariç, iskonto sonrası
	VAT        decimal.Decimal
	Rate       *decimal.Decimal
	Account    string // approved (account_assignments); "" = not decided
	VATAccount string // approved vat_account_code; "" = 191 / 391 by rate
	Taxes      []Tax
	// Withholding, excise, communications and income tax amounts on the line.
	Withholding, Excise, Communications, IncomeTax decimal.Decimal
	ExemptionCode                                  string
}

// Invoice is the header.
type Invoice struct {
	ID          int64
	Number      string
	Direction   string // model.DirectionIncoming | model.DirectionOutgoing
	Type        string
	Currency    string
	Status      string // status_normalized
	IssueDate   time.Time
	Counterpart string
	// VATDeductionPeriod: the month a purchase's KDV is deducted in
	// (#640); "" = the document month.
	VATDeductionPeriod string
	TaxExclusive       decimal.Decimal
	Tax                decimal.Decimal
	Payable            decimal.Decimal
	// Header withholding, excise, communications and income tax amounts.
	Withholding, Excise, Communications, IncomeTax decimal.Decimal
	GlobalExemptionCode                            string
}

// Input is everything Build needs.
type Input struct {
	Invoice Invoice
	Items   []Item
	// Cari is the counterparty's account (120.x / 320.x), already resolved;
	// "" falls back to the main account (the draft check asks for a sub
	// account when a plan is loaded).
	Cari string
	Plan []engine.PlanAccount
}

// Line is one journal line.
type Line struct {
	Account     string
	Description string
	Debit       decimal.Decimal
	Credit      decimal.Decimal
}

// Entry is a balanced journal draft.
type Entry struct {
	Date         time.Time
	Description  string
	DocumentType string
	DocumentNo   string
	DocumentDate time.Time
	SourceID     string
	Lines        []Line
}

// Balanced reports whether debits equal credits.
func (e Entry) Balanced() bool {
	var d, c decimal.Decimal
	for _, l := range e.Lines {
		d, c = d.Add(l.Debit), c.Add(l.Credit)
	}
	return d.Equal(c) && d.IsPositive()
}

// Build makes the entry or says why it cannot.
func Build(in Input) (Entry, error) {
	inv := in.Invoice
	purchase := inv.Direction == string(model.DirectionIncoming)
	if !purchase && inv.Direction != string(model.DirectionOutgoing) {
		return Entry{}, refuse(ReasonUnsupportedType, inv.Direction)
	}
	if err := supported(in); err != nil {
		return Entry{}, err
	}

	var net, vat decimal.Decimal
	netBy := map[string]decimal.Decimal{}
	vatBy := map[string]decimal.Decimal{}
	var netOrder, vatOrder []string
	vatMain := "391"
	if purchase {
		vatMain = "191"
	}
	for _, it := range in.Items {
		if strings.TrimSpace(it.Account) == "" {
			return Entry{}, &Refusal{Code: ReasonItemAccount, LineNo: it.LineNo}
		}
		if _, ok := netBy[it.Account]; !ok {
			netOrder = append(netOrder, it.Account)
		}
		netBy[it.Account] = netBy[it.Account].Add(it.Net)
		net = net.Add(it.Net)
		if it.VAT.IsZero() {
			continue
		}
		acc := vatAccount(vatMain, it, in.Plan)
		if _, ok := vatBy[acc]; !ok {
			vatOrder = append(vatOrder, acc)
		}
		vatBy[acc] = vatBy[acc].Add(it.VAT)
		vat = vat.Add(it.VAT)
	}
	total := net.Add(vat)
	if !net.Round(2).Equal(inv.TaxExclusive.Round(2)) || !vat.Round(2).Equal(inv.Tax.Round(2)) ||
		!total.Round(2).Equal(inv.Payable.Round(2)) {
		return Entry{}, refuse(ReasonTotalsMismatch, fmt.Sprintf("satırlar %s + %s, fatura %s + %s = %s",
			net.StringFixed(2), vat.StringFixed(2), inv.TaxExclusive.StringFixed(2), inv.Tax.StringFixed(2), inv.Payable.StringFixed(2)))
	}

	cari := strings.TrimSpace(in.Cari)
	if cari == "" {
		cari = "320"
		if !purchase {
			cari = "120"
		}
	}
	docDate := inv.IssueDate
	kind, docType := "Satış faturası", "invoice_sales"
	if purchase {
		kind, docType = "Alış faturası", "invoice_purchase"
	}
	desc := strings.TrimSpace(fmt.Sprintf("%s %s (%s) %s", kind, inv.Number, docDate.Format("02.01.2006"), inv.Counterpart))

	var lines []Line
	if !purchase {
		lines = append(lines, Line{Account: cari, Description: desc, Debit: total})
	}
	for _, a := range netOrder {
		l := Line{Account: a, Description: desc}
		if purchase {
			l.Debit = netBy[a]
		} else {
			l.Credit = netBy[a]
		}
		lines = append(lines, l)
	}
	for _, a := range vatOrder {
		l := Line{Account: a, Description: vatText(purchase) + " " + inv.Number}
		if purchase {
			l.Debit = vatBy[a]
		} else {
			l.Credit = vatBy[a]
		}
		lines = append(lines, l)
	}
	if purchase {
		lines = append(lines, Line{Account: cari, Description: desc, Credit: total})
	}
	e := Entry{
		Date: EntryDate(inv), Description: desc, DocumentType: docType, DocumentNo: inv.Number,
		DocumentDate: docDate, SourceID: fmt.Sprint(inv.ID), Lines: round(lines),
	}
	if !e.Balanced() {
		return Entry{}, refuse(ReasonTotalsMismatch, "borç ve alacak tutmuyor")
	}
	return e, nil
}

// EntryDate is the entry's date: the document date; a purchase whose KDV
// was moved to a later month (#640) is booked on the first day of that
// month, so it falls in the declared month and before the month-end KDV
// offset entry (191 → 391), which offices date on the last day.
func EntryDate(inv Invoice) time.Time {
	if inv.Direction == string(model.DirectionIncoming) && inv.VATDeductionPeriod != "" {
		if t, err := time.Parse("2006-01", inv.VATDeductionPeriod); err == nil {
			return t
		}
	}
	return inv.IssueDate
}

// supported refuses what the first version cannot book correctly.
func supported(in Input) error {
	inv := in.Invoice
	if inv.Status != "active" {
		return refuse(ReasonNotActive, inv.Status)
	}
	if inv.Currency != "" && inv.Currency != "TRY" {
		return refuse(ReasonForeignCurrency, inv.Currency)
	}
	switch inv.Type {
	case model.TypeSatis, model.TypeAlis:
	case model.TypeIade:
		return refuse(ReasonReturn, "")
	case model.TypeTevkifat:
		return refuse(ReasonWithholding, "")
	case model.TypeIstisna, model.TypeIhracKayitli:
		return refuse(ReasonExemption, inv.Type)
	default:
		return refuse(ReasonUnsupportedType, inv.Type)
	}
	if !inv.Withholding.IsZero() {
		return refuse(ReasonWithholding, "")
	}
	if !inv.Excise.IsZero() || !inv.Communications.IsZero() || !inv.IncomeTax.IsZero() {
		return refuse(ReasonSpecialTax, "")
	}
	if inv.GlobalExemptionCode != "" {
		return refuse(ReasonExemption, inv.GlobalExemptionCode)
	}
	if len(in.Items) == 0 {
		return refuse(ReasonNoItems, "")
	}
	for _, it := range in.Items {
		if !it.Withholding.IsZero() {
			return refuse(ReasonWithholding, "")
		}
		if !it.Excise.IsZero() || !it.Communications.IsZero() || !it.IncomeTax.IsZero() {
			return refuse(ReasonSpecialTax, "")
		}
		if it.ExemptionCode != "" {
			return refuse(ReasonExemption, it.ExemptionCode)
		}
		for _, t := range it.Taxes {
			if t.Amount.IsZero() && t.ExemptionCode == "" {
				continue
			}
			if t.Kind == "withholding" {
				return refuse(ReasonWithholding, "")
			}
			if t.ExemptionCode != "" {
				return refuse(ReasonExemption, t.ExemptionCode)
			}
			if t.TypeCode != vatTypeCode {
				return refuse(ReasonSpecialTax, t.TypeCode)
			}
		}
	}
	return nil
}

// vatAccount: the approved KDV account of the line, else the plan's 191 /
// 391 sub-account for the rate, else the main account.
func vatAccount(main string, it Item, plan []engine.PlanAccount) string {
	if it.VATAccount != "" {
		return it.VATAccount
	}
	if it.Rate != nil {
		if a, ok := engine.VATSubAccount(main, *it.Rate, plan); ok {
			return a.Code
		}
	}
	return leaf(main, plan)
}

// leaf: the plan's only leaf under main when there is exactly one, else
// main (the draft check asks for a sub account).
func leaf(main string, plan []engine.PlanAccount) string {
	var found []string
	for _, a := range plan {
		if a.IsLeaf && strings.HasPrefix(a.Code, main+".") {
			found = append(found, a.Code)
		}
	}
	if len(found) == 1 {
		return found[0]
	}
	return main
}

func vatText(purchase bool) string {
	if purchase {
		return "İndirilecek KDV"
	}
	return "Hesaplanan KDV"
}

func round(in []Line) []Line {
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
