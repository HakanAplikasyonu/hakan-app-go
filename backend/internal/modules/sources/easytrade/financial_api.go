package easytrade

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
)

// FinancialListLimit caps one period's listing.
const FinancialListLimit = 1000

// FinancialTransaction is one row in the API.
type FinancialTransaction struct {
	UUID                uuid.UUID  `json:"uuid"`
	SourceID            string     `json:"source_id"`
	TransactionDate     string     `json:"transaction_date"`
	DueDate             *string    `json:"due_date,omitempty"`
	TransactionType     string     `json:"transaction_type"`
	TransactionFlow     string     `json:"transaction_flow,omitempty"`
	Direction           string     `json:"direction"`
	PaymentMethodType   string     `json:"payment_method_type"`
	Amount              string     `json:"amount"`
	Currency            string     `json:"currency"`
	AmountTRY           string     `json:"amount_try"`
	Description         string     `json:"description"`
	ReferenceDocumentNo string     `json:"reference_document_no,omitempty"`
	Status              string     `json:"status,omitempty"`
	CustomerName        string     `json:"customer_name,omitempty"`
	CustomerTaxNumber   string     `json:"customer_tax_number,omitempty"`
	CashBoxName         string     `json:"cash_box_name,omitempty"`
	CashBoxType         string     `json:"cash_box_type,omitempty"`
	ReferenceableType   string     `json:"referenceable_type,omitempty"`
	ReferenceableID     string     `json:"referenceable_id,omitempty"`
	InvoiceUUID         *uuid.UUID `json:"invoice_uuid,omitempty"`
}

// PaymentTotal is one payment_method_type × direction total.
type PaymentTotal struct {
	PaymentMethodType string `json:"payment_method_type"`
	Direction         string `json:"direction"`
	Count             int32  `json:"count"`
	Total             string `json:"total"`
}

// CustomerTotal is one cari (kişi/cari) total of the period.
type CustomerTotal struct {
	CustomerName      string `json:"customer_name"`
	CustomerTaxNumber string `json:"customer_tax_number,omitempty"`
	Count             int32  `json:"count"`
	TotalIn           string `json:"total_in"`
	TotalOut          string `json:"total_out"`
}

// FinancialSummary is the period's view: rows, payment-type and cari
// totals and how many invoice references found their portal invoice.
type FinancialSummary struct {
	Period         string                 `json:"period"`
	Items          []FinancialTransaction `json:"items"`
	ByPaymentType  []PaymentTotal         `json:"by_payment_type"`
	ByCustomer     []CustomerTotal        `json:"by_customer"`
	InvoiceRefs    int32                  `json:"invoice_refs"`
	InvoiceMatched int32                  `json:"invoice_matched"`
}

func dateStr(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

func money(n pgtype.Numeric) string { return repository.Dec(n).StringFixed(2) }

// FinancialTransactions lists the period's Easytrade financial transactions.
func (s *Service) FinancialTransactions(ctx context.Context, taxpayerID int64, id uuid.UUID, period string) (FinancialSummary, error) {
	if _, err := s.row(ctx, taxpayerID, id); err != nil {
		return FinancialSummary{}, err
	}
	out := FinancialSummary{Period: period, Items: []FinancialTransaction{}, ByPaymentType: []PaymentTotal{}, ByCustomer: []CustomerTotal{}}
	rows, err := s.q.ListSourceFinancialTransactions(ctx, db.ListSourceFinancialTransactionsParams{
		TaxpayerID: taxpayerID, SourceSystem: SourceSystem, Period: period, Lim: FinancialListLimit,
	})
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		f := FinancialTransaction{
			UUID: r.Uuid, SourceID: r.SourceID, TransactionDate: dateStr(r.TransactionDate), TransactionType: r.TransactionType,
			TransactionFlow: r.TransactionFlow, Direction: r.Direction, PaymentMethodType: r.PaymentMethodType,
			Amount: money(r.Amount), Currency: r.Currency, AmountTRY: money(r.AmountTry), Description: r.Description,
			ReferenceDocumentNo: r.ReferenceDocumentNo.String, Status: r.Status, CustomerName: r.CustomerName.String,
			CustomerTaxNumber: r.CustomerTaxNumber.String, CashBoxName: r.CashBoxName.String, CashBoxType: r.CashBoxType.String,
			ReferenceableType: r.ReferenceableType.String, ReferenceableID: r.ReferenceableID.String,
		}
		if r.DueDate.Valid {
			d := dateStr(r.DueDate)
			f.DueDate = &d
		}
		if u, err := uuid.Parse(r.InvoiceUuid); err == nil {
			f.InvoiceUUID = &u
		}
		out.Items = append(out.Items, f)
	}
	p := db.SummarizeSourceFinancialByPaymentParams{TaxpayerID: taxpayerID, SourceSystem: SourceSystem, Period: period}
	byPay, err := s.q.SummarizeSourceFinancialByPayment(ctx, p)
	if err != nil {
		return out, err
	}
	for _, r := range byPay {
		out.ByPaymentType = append(out.ByPaymentType, PaymentTotal{PaymentMethodType: r.PaymentMethodType, Direction: r.Direction, Count: r.Count, Total: money(r.Total)})
	}
	byCust, err := s.q.SummarizeSourceFinancialByCustomer(ctx, db.SummarizeSourceFinancialByCustomerParams(p))
	if err != nil {
		return out, err
	}
	for _, r := range byCust {
		out.ByCustomer = append(out.ByCustomer, CustomerTotal{CustomerName: r.CustomerName, CustomerTaxNumber: r.CustomerTaxNumber, Count: r.Count,
			TotalIn: money(r.TotalIn), TotalOut: money(r.TotalOut)})
	}
	m, err := s.q.CountSourceFinancialMatches(ctx, db.CountSourceFinancialMatchesParams(p))
	if err != nil {
		return out, err
	}
	out.InvoiceRefs, out.InvoiceMatched = m.InvoiceRefs, m.InvoiceMatched
	return out, nil
}

// ZReport is one Z report in the API.
type ZReport struct {
	UUID            uuid.UUID         `json:"uuid"`
	SourceID        string            `json:"source_id"`
	ReportDate      string            `json:"report_date"`
	ZReportNumber   string            `json:"z_report_number,omitempty"`
	OKCSerialNumber string            `json:"okc_serial_number,omitempty"`
	Subtotal        string            `json:"subtotal_amount"`
	Tax             string            `json:"tax_amount"`
	Total           string            `json:"total_amount"`
	Currency        string            `json:"currency"`
	ApprovalStatus  string            `json:"approval_status"`
	Payments        map[string]string `json:"payments"`
}

// ZReportList is the period's Z reports with totals.
type ZReportList struct {
	Period    string            `json:"period"`
	Items     []ZReport         `json:"items"`
	Total     string            `json:"total_amount"`
	Tax       string            `json:"tax_amount"`
	Payments  map[string]string `json:"payments"`
	Supported bool              `json:"supported"`
}

// ZReports lists the period's Easytrade Z reports (retail_sales "z:*").
// Supported is false while the last run found no Z endpoint at Easytrade.
func (s *Service) ZReports(ctx context.Context, taxpayerID int64, id uuid.UUID, period string) (ZReportList, error) {
	row, err := s.row(ctx, taxpayerID, id)
	if err != nil {
		return ZReportList{}, err
	}
	out := ZReportList{Period: period, Items: []ZReport{}, Payments: map[string]string{}, Supported: true}
	if rs, err := s.q.ListSyncRuns(ctx, db.ListSyncRunsParams{TaxpayerID: taxpayerID, ConnectionID: row.ID, Lim: 1}); err == nil && len(rs) > 0 {
		var c Counters
		if json.Unmarshal(rs[0].Counters, &c) == nil {
			for _, u := range c.Unsupported {
				if u == ItemZReports {
					out.Supported = false
				}
			}
		}
	}
	rows, err := s.q.ListSourceZReports(ctx, db.ListSourceZReportsParams{TaxpayerID: taxpayerID, SourceSystem: SourceSystem, Period: pgtype.Text{String: period, Valid: true}})
	if err != nil {
		return out, err
	}
	total, tax := decimal.Zero, decimal.Zero
	pay := map[string]decimal.Decimal{}
	for _, r := range rows {
		z := ZReport{
			UUID: r.Uuid, SourceID: strings.TrimPrefix(r.SourceID, "z:"), ReportDate: dateStr(r.SaleDate),
			ZReportNumber: r.ZReportNumber.String, OKCSerialNumber: r.OkcSerialNumber.String,
			Subtotal: money(r.SubtotalAmount), Tax: money(r.TaxAmount), Total: money(r.TotalAmount), Currency: r.Currency,
			ApprovalStatus: r.ApprovalStatus, Payments: map[string]string{},
		}
		total, tax = total.Add(repository.Dec(r.TotalAmount)), tax.Add(repository.Dec(r.TaxAmount))
		if plain, err := secrets.OpenField(secrets.PurposeRetailSaleRawPayload, r.RawPayload); err == nil {
			var raw map[string]any
			if json.Unmarshal(plain, &raw) == nil {
				for k, v := range ZPayments(raw) {
					z.Payments[k] = v.StringFixed(2)
					pay[k] = pay[k].Add(v)
				}
			}
		}
		out.Items = append(out.Items, z)
	}
	out.Total, out.Tax = total.StringFixed(2), tax.StringFixed(2)
	for k, v := range pay {
		out.Payments[k] = v.StringFixed(2)
	}
	return out, nil
}

// periodOK: YYYY-MM.
func periodOK(p string) bool {
	_, err := time.Parse("2006-01", p)
	return err == nil && len(p) == 7
}
