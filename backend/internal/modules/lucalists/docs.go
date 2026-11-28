package lucalists

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	bankposting "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/posting"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	invoiceposting "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/posting"
	receiptposting "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/posting"
)

// InvoiceKey is an invoice's double export guard key, the same as the
// journal export's (#466): the ETTN in lower case, else "inv:<uuid>".
func InvoiceKey(id uuid.UUID, ettn *uuid.UUID) string {
	if ettn != nil {
		return strings.ToLower(ettn.String())
	}
	return "inv:" + strings.ToLower(id.String())
}

// BankKey is a bank movement's guard key: "bank:<uuid>".
func BankKey(id uuid.UUID) string { return "bank:" + strings.ToLower(id.String()) }

// ZKey is a Z report's guard key: "z:<uuid>".
func ZKey(id uuid.UUID) string { return "z:" + strings.ToLower(id.String()) }

// ItemSuggestion is the pending (not yet approved) account suggestion of an
// invoice line: line account and VAT account.
type ItemSuggestion struct {
	Account    string
	VATAccount string
}

// InvoiceDoc is the fiş of one invoice: the same lines as the invoice's
// journal draft (invoiceposting.Build: 153 / 7xx + 191 per rate against 320
// for a purchase, 120 against 600 + 391 per rate for a sale). A line
// without an approved account takes its pending suggestion, else the main
// account (153 / 600) marked ITEM_ACCOUNT; a cari without an account is the
// main 120 / 320 (COUNTERPARTY_ACCOUNT); a VAT line the plan has no rate
// account for stays on the main 191 / 391 (VAT_ACCOUNT). An invoice Build
// refuses (foreign currency, return, withholding …) comes back with the
// refusal code and no lines.
func InvoiceDoc(key string, id uuid.UUID, taxID string, in invoiceposting.Input, sug map[int]ItemSuggestion) (Doc, string) {
	inv := in.Invoice
	purchase := inv.Direction == string(model.DirectionIncoming)
	itemMain, cariMain, vatMain := "600", "120", "391"
	if purchase {
		itemMain, cariMain, vatMain = "153", "320", "191"
	}
	head := Doc{Key: key, UUID: id, No: inv.Number, Date: invoiceposting.EntryDate(inv), DocDate: inv.IssueDate, TaxID: taxID}
	checks := map[string]string{}
	items := make([]invoiceposting.Item, len(in.Items))
	copy(items, in.Items)
	for i := range items {
		it := &items[i]
		s := sug[it.LineNo]
		if strings.TrimSpace(it.Account) == "" {
			if s.Account != "" {
				it.Account = s.Account
			} else {
				it.Account = itemMain
				checks[itemMain] = CheckItemAccount
			}
		}
		if strings.TrimSpace(it.VATAccount) == "" && s.VATAccount != "" {
			it.VATAccount = s.VATAccount
		}
	}
	in.Items = items
	if strings.TrimSpace(in.Cari) == "" {
		checks[cariMain] = CheckCounterpartyAccount
	}
	e, err := invoiceposting.Build(in)
	var refusal *invoiceposting.Refusal
	if errors.As(err, &refusal) {
		return head, refusal.Code
	}
	if err != nil {
		return head, SkipUnbalanced
	}
	d := head
	d.No, d.Date, d.DocDate = e.DocumentNo, e.Date, e.DocumentDate
	d.Description, d.DocumentType = e.Description, e.DocumentType
	d.SourceKind, d.SourceID = invoiceposting.SourceKind, e.SourceID
	split := vatByRate(purchase, in)
	for _, l := range e.Lines {
		c := checks[l.Account]
		if c == "" && l.Account == vatMain {
			c = CheckVATAccount
		}
		if parts, ok := split[l.Account]; ok && isVATLine(l.Description) {
			d.Lines = append(d.Lines, splitVAT(l, parts, c)...)
			continue
		}
		d.Lines = append(d.Lines, Line{Account: l.Account, Description: l.Description, Debit: l.Debit, Credit: l.Credit, Check: c})
	}
	return d, ""
}

// vatPart is the VAT of one rate on one KDV account.
type vatPart struct {
	Label  string // "KDV %20"; "" = rate unknown
	Amount decimal.Decimal
}

// vatByRate groups the invoice's VAT by account and then by rate, in line
// order: Build books one line per account, and when the plan has no rate
// sub-account several rates share the main 191 / 391. Luca wants every
// rate on its own row (Oğuz'un geri bildirimi).
func vatByRate(purchase bool, in invoiceposting.Input) map[string][]vatPart {
	out := map[string][]vatPart{}
	for _, it := range in.Items {
		if it.VAT.IsZero() {
			continue
		}
		acc := invoiceposting.VATAccount(purchase, it, in.Plan)
		label := ""
		if it.Rate != nil {
			label = "KDV %" + it.Rate.Round(2).String()
		}
		parts := out[acc]
		found := false
		for i := range parts {
			if parts[i].Label == label {
				parts[i].Amount = parts[i].Amount.Add(it.VAT)
				found = true
				break
			}
		}
		if !found {
			parts = append(parts, vatPart{Label: label, Amount: it.VAT})
		}
		out[acc] = parts
	}
	return out
}

// isVATLine: Build's KDV line ("İndirilecek KDV …" / "Hesaplanan KDV …").
func isVATLine(desc string) bool {
	return strings.HasPrefix(desc, "İndirilecek KDV") || strings.HasPrefix(desc, "Hesaplanan KDV")
}

// splitVAT writes Build's KDV line as one row per rate, "KDV %20" in the
// description. The rows add up to the line exactly (the last row takes the
// rounding), so the fiş stays balanced.
func splitVAT(l invoiceposting.Line, parts []vatPart, check string) []Line {
	total := l.Debit.Add(l.Credit)
	if len(parts) < 2 {
		desc := l.Description
		if len(parts) == 1 && parts[0].Label != "" {
			desc = parts[0].Label
		}
		return []Line{{Account: l.Account, Description: desc, Debit: l.Debit, Credit: l.Credit, Check: check}}
	}
	out := make([]Line, 0, len(parts))
	rest := total
	for i, p := range parts {
		amt := p.Amount.Round(2)
		if i == len(parts)-1 {
			amt = rest
		}
		rest = rest.Sub(amt)
		desc := p.Label
		if desc == "" {
			desc = l.Description
		}
		r := Line{Account: l.Account, Description: desc, Check: check}
		if l.Debit.IsPositive() {
			r.Debit = amt
		} else {
			r.Credit = amt
		}
		out = append(out, r)
	}
	return out
}

// Movement is one bank movement of the period.
type Movement struct {
	UUID         uuid.UUID
	ID           int64
	Date         time.Time
	In           bool // money in (alacak in the bank's statement)
	Amount       decimal.Decimal
	Currency     string
	Description  string
	Counterparty string
	TaxID        string
	Detail       string // kind_detail
	// Account is the counter account (approved or suggested), BankAccount
	// the bank account's 102 code; "" = unknown.
	Account     string
	BankAccount string
}

// BankDoc is the fiş of one bank movement: the bank account (102) against
// the counter account; money in debits the bank. An unknown bank account
// is the main 102 (BANK_ACCOUNT); an unknown counter account is the usual
// account of the movement type, else 120 / 320 (COUNTER_ACCOUNT).
// Foreign-currency movements are left out (their TRY amount is not known),
// like the bank drafts; so is one booked to the bank account itself.
func BankDoc(m Movement) (Doc, string) {
	desc := strings.TrimSpace(firstNonEmpty(m.Counterparty, m.Description))
	d := Doc{Key: BankKey(m.UUID), UUID: m.UUID, Date: m.Date, DocDate: m.Date, TaxID: m.TaxID,
		Description: strings.TrimSpace("Banka hareketi " + desc), DocumentType: "bank_transaction",
		SourceKind: bankposting.DraftSourceKind, SourceID: strconv.FormatInt(m.ID, 10)}
	switch {
	case m.Date.IsZero():
		return d, SkipNoDate
	case m.Currency != "" && m.Currency != "TRY":
		return d, SkipForeignCurrency
	case !m.Amount.IsPositive():
		return d, SkipNoAmount
	}
	dir := banking.DirOut
	if m.In {
		dir = banking.DirIn
	}
	bank, bankCheck := strings.TrimSpace(m.BankAccount), ""
	if bank == "" {
		bank, bankCheck = bankposting.DefaultBankAccount, CheckBankAccount
	}
	counter, counterCheck := strings.TrimSpace(m.Account), ""
	if counter == "" {
		counter, counterCheck = bankposting.DefaultCounterAccount(banking.Detail(m.Detail), dir), CheckCounterAccount
	}
	if bankposting.SameAccount(bank, counter) {
		return d, SkipSameAccount
	}
	b := Line{Account: bank, Description: desc, Check: bankCheck}
	c := Line{Account: counter, Description: desc, Check: counterCheck}
	if m.In {
		b.Debit, c.Credit = m.Amount, m.Amount
	} else {
		c.Debit, b.Credit = m.Amount, m.Amount
	}
	d.Lines = []Line{b, c}
	return d, ""
}

// ZReport is one Z report of the period.
type ZReport struct {
	UUID       uuid.UUID
	Date       time.Time
	ZNumber    string
	OKC        string
	Currency   string
	Total, Tax decimal.Decimal
	Cash, Card *decimal.Decimal
	Lines      []receiptposting.ZVatLine
}

// ZDoc is the fiş of one Z report: the same lines as its journal draft
// (receiptposting.ZEntry): 100 Kasa / 108 POS debit, per VAT rate 600 and
// 391 credit. A line the plan has no sub account for stays on the main
// account (MAIN_ACCOUNT).
func ZDoc(z ZReport, plan []engine.PlanAccount) (Doc, string) {
	src := "retail_sale:" + z.UUID.String()
	d := Doc{Key: ZKey(z.UUID), UUID: z.UUID, No: z.ZNumber, Date: z.Date, DocDate: z.Date,
		DocumentType: "z_report", SourceKind: "receipt", SourceID: src}
	switch {
	case z.Date.IsZero():
		return d, SkipNoDate
	case z.Currency != "" && z.Currency != "TRY":
		return d, SkipForeignCurrency
	}
	e, err := receiptposting.ZEntry(receiptposting.ZInput{SourceID: src, Date: z.Date, ZNumber: z.ZNumber, OKC: z.OKC,
		Total: z.Total, Tax: z.Tax, Cash: z.Cash, Card: z.Card, Lines: z.Lines, Plan: plan})
	if errors.Is(err, receiptposting.ErrZBreakdown) {
		return d, SkipZBreakdown
	}
	if err != nil {
		return d, SkipNoAmount
	}
	d.Description = e.Description
	for _, l := range e.Lines {
		c := ""
		if isMain(l.Account) {
			c = CheckMainAccount
		}
		d.Lines = append(d.Lines, Line{Account: l.Account, Description: l.Description, Debit: l.Debit, Credit: l.Credit, Check: c})
	}
	return d, ""
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
