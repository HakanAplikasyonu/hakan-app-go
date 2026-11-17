package luca

// Luca "Banka Ekstreleri" CSV (issue #465): the portal's bank movements of
// one account and date range in a layout Luca's ekstre tanımı reads. The
// layout (columns, date and decimal format, sign rule, delimiter, quote,
// encoding) is data in templates/banka_ekstresi_<version>.yaml
// (unverified: another team's observation of the Luca screen).

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
)

// BankStatementTemplateDefault is the default banka ekstresi template.
const BankStatementTemplateDefault = "unverified-1"

// Amount layouts.
const (
	AmountSigned     = "signed"
	AmountTwoColumns = "two_columns"
)

// BankStatementTemplate loads templates/banka_ekstresi_<version>.yaml.
func BankStatementTemplate(version string) (ae.CSVTemplate, error) {
	if version == "" {
		version = BankStatementTemplateDefault
	}
	t, err := ae.LoadCSVTemplate(templates, fmt.Sprintf("templates/banka_ekstresi_%s.yaml", version))
	if err != nil {
		return ae.CSVTemplate{}, err
	}
	if t.Driver != Name || t.Name != "banka_ekstresi" {
		return ae.CSVTemplate{}, fmt.Errorf("%w: not a luca banka_ekstresi template", ae.ErrTemplate)
	}
	for _, c := range t.Columns {
		if !bankKeys[c.Key] {
			return ae.CSVTemplate{}, fmt.Errorf("%w: unknown column key %q", ae.ErrTemplate, c.Key)
		}
	}
	if _, err := (BankOptions{}).resolve(t); err != nil {
		return ae.CSVTemplate{}, err
	}
	return t, nil
}

var bankKeys = map[string]bool{
	"doc_date": true, "doc_no": true, "description": true, "amount": true, "debit": true, "credit": true,
	"tax_id": true, "iban": true, "currency": true,
}

// BankLine is one bank movement.
type BankLine struct {
	Date        time.Time
	DocNo       string
	Description string
	// Incoming: money into the account (ekstrede A / alacak).
	Incoming bool
	Amount   decimal.Decimal // ≥ 0
	TaxID    string          // counterparty VKN / TCKN
	IBAN     string          // counterparty IBAN
	Currency string
}

// BankOptions override the template defaults for one file. Empty fields
// keep the template value.
type BankOptions struct {
	AmountLayout string `json:"amount_layout,omitempty"`
	IncomingSign string `json:"incoming_sign,omitempty"`
	Delimiter    string `json:"delimiter,omitempty"`
	Quote        string `json:"quote,omitempty"`
}

type bankLayout struct {
	layout, incomingSign, thousands string
}

func (o BankOptions) resolve(t ae.CSVTemplate) (bankLayout, error) {
	l := bankLayout{layout: t.Options["amount_layout"], incomingSign: t.Options["incoming_sign"], thousands: t.Options["thousands"]}
	if o.AmountLayout != "" {
		l.layout = o.AmountLayout
	}
	if o.IncomingSign != "" {
		l.incomingSign = o.IncomingSign
	}
	if l.layout == "" {
		l.layout = AmountSigned
	}
	if l.incomingSign == "" {
		l.incomingSign = "+"
	}
	if l.layout != AmountSigned && l.layout != AmountTwoColumns {
		return l, fmt.Errorf("%w: amount_layout %q (signed, two_columns)", ae.ErrTemplate, l.layout)
	}
	if l.incomingSign != "+" && l.incomingSign != "-" {
		return l, fmt.Errorf("%w: incoming_sign %q (+, -)", ae.ErrTemplate, l.incomingSign)
	}
	return l, nil
}

// ApplyBankOptions returns the template with the options applied: the
// delimiter and quote, and the columns of the chosen amount layout (signed:
// Tutar; two_columns: Borç and Alacak).
func ApplyBankOptions(t ae.CSVTemplate, o BankOptions) (ae.CSVTemplate, error) {
	l, err := o.resolve(t)
	if err != nil {
		return ae.CSVTemplate{}, err
	}
	if o.Delimiter != "" {
		t.CSV.Delimiter = o.Delimiter
	}
	if o.Quote != "" {
		t.CSV.Quote = o.Quote
	}
	cols := make([]ae.Column, 0, len(t.Columns))
	for _, c := range t.Columns {
		switch {
		case l.layout == AmountSigned && (c.Key == "debit" || c.Key == "credit"):
			continue
		case l.layout == AmountTwoColumns && c.Key == "amount":
			continue
		}
		cols = append(cols, c)
	}
	t.Columns = cols
	t.Options = cloneOptions(t.Options)
	t.Options["amount_layout"], t.Options["incoming_sign"] = l.layout, l.incomingSign
	// Re-check the overridden container (delimiter / quote).
	if err := t.Check(); err != nil {
		return ae.CSVTemplate{}, err
	}
	return t, nil
}

func cloneOptions(m map[string]string) map[string]string {
	out := make(map[string]string, len(m)+2)
	for k, v := range m {
		out[k] = v
	}
	return out
}

// WriteBankStatement writes the lines in the template layout (apply the
// options first with ApplyBankOptions). Dates use the column's Go layout,
// amounts two decimals with the column's decimal separator; the sign rule
// comes from the template options.
func WriteBankStatement(t ae.CSVTemplate, lines []BankLine) (ae.CSVResult, error) {
	l, err := (BankOptions{}).resolve(t)
	if err != nil {
		return ae.CSVResult{}, err
	}
	byKey := map[string]ae.Column{}
	for _, c := range t.Columns {
		byKey[c.Key] = c
	}
	rows := make([]map[string]string, len(lines))
	for i, b := range lines {
		r := map[string]string{
			"doc_no": b.DocNo, "description": b.Description, "tax_id": b.TaxID, "iban": b.IBAN, "currency": b.Currency,
		}
		if c, ok := byKey["doc_date"]; ok && !b.Date.IsZero() {
			layout := c.Format
			if layout == "" {
				layout = "02.01.2006"
			}
			r["doc_date"] = b.Date.Format(layout)
		}
		amt := b.Amount.Abs()
		switch l.layout {
		case AmountSigned:
			v := amt
			if b.Incoming != (l.incomingSign == "+") {
				v = v.Neg()
			}
			r["amount"] = money(v, byKey["amount"].Decimal, l.thousands)
		case AmountTwoColumns:
			if b.Incoming {
				r["credit"] = money(amt, byKey["credit"].Decimal, l.thousands)
			} else {
				r["debit"] = money(amt, byKey["debit"].Decimal, l.thousands)
			}
		}
		rows[i] = r
	}
	return t.WriteCSV(rows)
}

// money formats v with two decimals, the decimal separator ("," when empty
// in the template means ".") and an optional thousands separator.
func money(v decimal.Decimal, dec, thousands string) string {
	s := v.StringFixed(2)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, _ := strings.Cut(s, ".")
	if thousands != "" && len(whole) > 3 {
		var b strings.Builder
		pre := len(whole) % 3
		if pre > 0 {
			b.WriteString(whole[:pre])
		}
		for i := pre; i < len(whole); i += 3 {
			if b.Len() > 0 {
				b.WriteString(thousands)
			}
			b.WriteString(whole[i : i+3])
		}
		whole = b.String()
	}
	if dec == "" {
		dec = "."
	}
	out := whole + dec + frac
	if neg {
		out = "-" + out
	}
	return out
}
