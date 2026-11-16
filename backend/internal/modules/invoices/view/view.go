// Package view is the JSON shape of an invoice shared by the taxpayer panel
// (#39) and the office panel (#142): header, lines, KDV summary and sources.
// Amounts are decimal strings.
package view

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ublunit"
)

// Invoice is one invoice row.
type Invoice struct {
	UUID             uuid.UUID `json:"uuid"`
	ETTN             string    `json:"ettn,omitempty"`
	InvoiceNumber    string    `json:"invoice_number"`
	Direction        string    `json:"direction"`
	Type             string    `json:"type"`
	Profile          string    `json:"profile,omitempty"`
	IssueDate        string    `json:"issue_date"`
	Period           string    `json:"period"`
	CounterpartyName string    `json:"counterparty_name,omitempty"`
	CounterpartyID   string    `json:"counterparty_tax_id,omitempty"`
	Currency         string    `json:"currency"`
	TaxExclusive     string    `json:"tax_exclusive_amount"`
	TaxAmount        string    `json:"tax_amount"`
	Withholding      string    `json:"withholding_amount"`
	Payable          string    `json:"payable_amount"`
	Status           string    `json:"status"`
	SourceSystem     string    `json:"source_system"`
	CaptureMethod    string    `json:"capture_method"`
}

// Detail is one invoice with lines, KDV summary and sources.
type Detail struct {
	Invoice
	Notes      string          `json:"notes,omitempty"`
	Items      []Item          `json:"items"`
	TaxSummary []TaxSummaryRow `json:"tax_summary"`
	Sources    []Source        `json:"sources"`
}

// Item is one line.
type Item struct {
	LineNo   int32  `json:"line_no"`
	Name     string `json:"name"`
	Quantity string `json:"quantity"`
	UnitCode string `json:"unit_code,omitempty"`
	// UnitName is the Turkish name of UnitCode ("C62" → "adet", #378).
	UnitName     string `json:"unit_name,omitempty"`
	UnitPrice    string `json:"unit_price"`
	TaxRate      string `json:"tax_rate"`
	TaxExclusive string `json:"tax_exclusive_amount"`
	TaxAmount    string `json:"tax_amount"`
}

// TaxSummaryRow is one KDV rate.
type TaxSummaryRow struct {
	TaxRate     string `json:"tax_rate"`
	Base        string `json:"base"`
	VAT         string `json:"vat"`
	Withholding string `json:"withholding"`
}

// Source is where the invoice came from.
type Source struct {
	SourceSystem  string    `json:"source_system"`
	CaptureMethod string    `json:"capture_method"`
	MatchRule     string    `json:"match_rule"`
	FirstSeenAt   time.Time `json:"first_seen_at"`
}

// FromInvoice maps the stored invoice header.
func FromInvoice(inv db.Invoice) Invoice {
	return Invoice{
		UUID: inv.Uuid, ETTN: UUIDText(inv.Ettn), InvoiceNumber: inv.InvoiceNumber, Direction: inv.Direction, Type: inv.Type,
		Profile: inv.Profile.String, IssueDate: inv.IssueDate.Time.Format("2006-01-02"), Period: inv.Period,
		CounterpartyName: inv.CustomerName.String, CounterpartyID: firstNonEmpty(inv.CustomerTaxNumber.String, inv.CustomerIdentityNumber.String),
		Currency: inv.Currency, TaxExclusive: Money(inv.TaxExclusiveAmount), TaxAmount: Money(inv.TaxAmount),
		Withholding: Money(inv.WithholdingAmount), Payable: Money(inv.PayableAmount),
		Status: inv.StatusNormalized, SourceSystem: inv.SourceSystem, CaptureMethod: inv.CaptureMethod,
	}
}

// LoadDetail reads lines, KDV summary and sources of inv.
func LoadDetail(ctx context.Context, q *db.Queries, inv db.Invoice) (Detail, error) {
	items, err := q.ListInvoiceItems(ctx, db.ListInvoiceItemsParams{TaxpayerID: inv.TaxpayerID, InvoiceID: inv.ID})
	if err != nil {
		return Detail{}, err
	}
	sums, err := q.ListInvoiceTaxSummaries(ctx, db.ListInvoiceTaxSummariesParams{TaxpayerID: inv.TaxpayerID, InvoiceID: inv.ID})
	if err != nil {
		return Detail{}, err
	}
	srcs, err := q.ListInvoiceSources(ctx, db.ListInvoiceSourcesParams{TaxpayerID: inv.TaxpayerID, InvoiceID: inv.ID})
	if err != nil {
		return Detail{}, err
	}
	out := Detail{Invoice: FromInvoice(inv), Notes: inv.Notes.String, Items: []Item{}, TaxSummary: []TaxSummaryRow{}, Sources: []Source{}}
	for _, it := range items {
		out.Items = append(out.Items, Item{
			LineNo: it.LineNo, Name: it.Name, Quantity: repository.Dec(it.Quantity).String(), UnitCode: it.UnitCode.String, UnitName: ublunit.Name(it.UnitCode.String),
			UnitPrice: repository.Dec(it.UnitPrice).String(), TaxRate: repository.Dec(it.TaxRate).String(),
			TaxExclusive: Money(it.TaxExclusiveAmount), TaxAmount: Money(it.TotalTaxAmount),
		})
	}
	for _, t := range sums {
		out.TaxSummary = append(out.TaxSummary, TaxSummaryRow{TaxRate: repository.Dec(t.TaxRate).String(), Base: Money(t.Base), VAT: Money(t.Vat), Withholding: Money(t.Withholding)})
	}
	for _, src := range srcs {
		out.Sources = append(out.Sources, Source{SourceSystem: src.SourceSystem, CaptureMethod: src.CaptureMethod, MatchRule: src.MatchRule, FirstSeenAt: src.FirstSeenAt.Time})
	}
	return out, nil
}

// Money formats an amount with two decimals.
func Money(n pgtype.Numeric) string { return repository.Dec(n).StringFixed(2) }

// UUIDText is the canonical text of a nullable UUID, or "".
func UUIDText(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuid.UUID(u.Bytes).String()
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
