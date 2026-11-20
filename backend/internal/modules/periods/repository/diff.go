package repository

import (
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	invoicerepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
)

// Diff kinds.
const (
	KindCreate = "create" // a new invoice for the locked period
	KindUpdate = "update" // new content for an existing invoice
)

// Diff is change_requests.diff: a summary of what applying would change
// (02 §3.4 "fark özeti"), not a full field-by-field comparison.
type Diff struct {
	Kind   string                `json:"kind"`
	Source DiffSource            `json:"source"`
	Fields map[string]FieldDelta `json:"fields"`
	// Summary names the incoming invoice for the plain sentence on screen
	// (#710: "Kapalı Ağustos 2026 dönemine yeni bir satış faturası geldi:
	// …"), whether or not these values changed.
	Summary DiffSummary `json:"summary"`
}

// DiffSummary is the incoming invoice in a few words.
type DiffSummary struct {
	InvoiceNumber string `json:"invoice_number"`
	Direction     string `json:"direction"`
	Counterparty  string `json:"counterparty"`
	PayableAmount string `json:"payable_amount"`
	Currency      string `json:"currency"`
}

// DiffSource is where the new content came from.
type DiffSource struct {
	System        string `json:"system"`
	CaptureMethod string `json:"capture_method"`
}

// FieldDelta is one changed field; Old is omitted for a new invoice.
type FieldDelta struct {
	Old any `json:"old,omitempty"`
	New any `json:"new"`
}

func invoiceDiff(old *db.Invoice, inv model.Invoice) Diff {
	d := Diff{Kind: KindCreate, Source: DiffSource{System: inv.SourceSystem, CaptureMethod: inv.CaptureMethod}, Fields: map[string]FieldDelta{},
		Summary: DiffSummary{
			InvoiceNumber: inv.InvoiceNumber, Direction: string(inv.Direction), Counterparty: inv.Counterparty.Name,
			PayableAmount: inv.Amounts.PayableAmount.String(), Currency: inv.Currency,
		}}
	issue := inv.IssueDate.Format("2006-01-02")
	newFields := []struct {
		name string
		val  string
	}{
		{"invoice_number", inv.InvoiceNumber},
		{"issue_date", issue},
		{"period", inv.Period},
		{"direction", string(inv.Direction)},
		{"type", inv.Type},
		{"currency", inv.Currency},
		{"customer_name", inv.Counterparty.Name},
		{"customer_tax_number", inv.Counterparty.TaxNumber},
		{"customer_identity_number", inv.Counterparty.IdentityNumber},
	}
	newAmounts := []struct {
		name string
		val  decimal.Decimal
	}{
		{"tax_exclusive_amount", inv.Amounts.TaxExclusiveAmount},
		{"tax_amount", inv.Amounts.TaxAmount},
		{"withholding_amount", inv.Amounts.WithholdingAmount},
		{"tax_inclusive_amount", inv.Amounts.TaxInclusiveAmount},
		{"payable_amount", inv.Amounts.PayableAmount},
	}
	if old == nil {
		for _, f := range newFields {
			if f.val != "" {
				d.Fields[f.name] = FieldDelta{New: f.val}
			}
		}
		for _, a := range newAmounts {
			d.Fields[a.name] = FieldDelta{New: a.val.String()}
		}
		d.Fields["item_count"] = FieldDelta{New: len(inv.Items)}
		return d
	}
	d.Kind = KindUpdate
	oldIssue := ""
	if old.IssueDate.Valid {
		oldIssue = old.IssueDate.Time.Format("2006-01-02")
	}
	oldFields := map[string]string{
		"invoice_number": old.InvoiceNumber, "issue_date": oldIssue, "period": old.Period,
		"direction": old.Direction, "type": old.Type, "currency": old.Currency,
		"customer_name": old.CustomerName.String, "customer_tax_number": old.CustomerTaxNumber.String,
		"customer_identity_number": old.CustomerIdentityNumber.String,
	}
	for _, f := range newFields {
		if o := oldFields[f.name]; o != f.val {
			d.Fields[f.name] = FieldDelta{Old: o, New: f.val}
		}
	}
	oldAmounts := map[string]decimal.Decimal{
		"tax_exclusive_amount": invoicerepo.Dec(old.TaxExclusiveAmount), "tax_amount": invoicerepo.Dec(old.TaxAmount),
		"withholding_amount": invoicerepo.Dec(old.WithholdingAmount), "tax_inclusive_amount": invoicerepo.Dec(old.TaxInclusiveAmount),
		"payable_amount": invoicerepo.Dec(old.PayableAmount),
	}
	for _, a := range newAmounts {
		if o := oldAmounts[a.name]; !o.Equal(a.val) {
			d.Fields[a.name] = FieldDelta{Old: o.String(), New: a.val.String()}
		}
	}
	if old.SourceSystem != inv.SourceSystem || old.CaptureMethod != inv.CaptureMethod {
		d.Fields["source"] = FieldDelta{Old: old.SourceSystem + "/" + old.CaptureMethod, New: inv.SourceSystem + "/" + inv.CaptureMethod}
	}
	return d
}
