// Package sources holds what every pre-accounting source adapter shares
// (02 §2–3): the mapped batch, the writer into the common model and the
// import report. Adapters live in subpackages (parasut, easytrade …).
package sources

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
)

// Expense is a source program's expense record (01 §4.10).
type Expense struct {
	SourceID     string
	Date         time.Time
	Period       string
	Amount       decimal.Decimal // KDV dahil
	TaxAmount    decimal.Decimal
	TaxRate      *decimal.Decimal
	Currency     string
	Description  string
	ReceiptType  string // expenses.receipt_type; empty → other
	VendorName   string
	VendorTaxID  string
	VehiclePlate string
	// PaymentMethod is expenses.payment_method (cash, bank_transfer,
	// credit_card, check, promissory_note, credit); empty → NULL.
	PaymentMethod string
	ReceiptNumber string
	// VatLines is the per-rate breakdown the program sent (#780): matrah
	// and KDV per rate. Empty → "kırılım yok"; a breakdown is never derived
	// from the total VAT.
	VatLines []ExpenseVatLine
	Raw      map[string]any
	Hash     string
}

// ExpenseVatLine is one VAT rate of an expense (expense_vat_lines).
type ExpenseVatLine struct {
	Rate decimal.Decimal
	Base decimal.Decimal // KDV hariç
	Tax  decimal.Decimal
}

// FinancialTransaction is a collection, payment or other money movement
// (01 §4.9). Names and values are Easytrade's.
type FinancialTransaction struct {
	SourceID            string
	Date                time.Time
	DueDate             *time.Time
	Period              string
	TransactionType     string
	TransactionFlow     string
	Direction           string // in, out, unknown
	PaymentMethodType   string // cash, bank_transfer, credit_card, check, promissory_note, credit, unknown
	Amount              decimal.Decimal
	Currency            string
	ExchangeRate        *decimal.Decimal
	AmountTRY           decimal.Decimal
	Description         string
	ReferenceDocumentNo string
	Status              string
	CustomerSourceID    string
	CustomerName        string
	CustomerTaxNumber   string
	CashBoxSourceID     string
	CashBoxName         string
	CashBoxType         string
	// ReferenceableType Invoice links the movement to the portal invoice
	// with source_id ReferenceableID (matching input).
	ReferenceableType string
	ReferenceableID   string
	Raw               map[string]any
	Hash              string
}

// ZReport is one fiscal day-end report (01 §4.7), written to retail_sales.
type ZReport struct {
	SourceID        string
	Date            time.Time
	Period          string
	ZReportNumber   string
	OKCSerialNumber string
	Subtotal        decimal.Decimal
	Tax             decimal.Decimal
	Total           decimal.Decimal
	Currency        string
	Raw             map[string]any // payments and VAT breakdown stay here
	Hash            string
}

// Excluded counts what was read but not written, by reason.
type Excluded map[string]int

// Batch is one window's mapped records.
type Batch struct {
	Invoices              []model.Invoice
	Expenses              []Expense
	FinancialTransactions []FinancialTransaction
	ZReports              []ZReport
	Excluded              Excluded
	// InvoiceWarnings are the "kontrol et" flags of an invoice by its
	// SourceID: a field the source did not give is left empty, never
	// guessed, and flagged (data_warnings, like the e-invoice channel).
	InvoiceWarnings map[string][]model.Warning
}

// Warn adds the flags of the invoice with sourceID.
func (b *Batch) Warn(sourceID string, ws ...model.Warning) {
	if len(ws) == 0 {
		return
	}
	if b.InvoiceWarnings == nil {
		b.InvoiceWarnings = map[string][]model.Warning{}
	}
	b.InvoiceWarnings[sourceID] = append(b.InvoiceWarnings[sourceID], ws...)
}

// raiseWarnings records an invoice's flags for the office that serves the
// taxpayer, in a savepoint: a flag that cannot be written (no active
// office, a disabled rule) never fails the import.
func raiseWarnings(ctx context.Context, tx pgx.Tx, taxpayerID int64, sourceSystem string, inv model.Invoice, saved repository.SaveResult, ws []model.Warning) {
	if len(ws) == 0 || !saved.Written {
		return
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		return
	}
	q := db.New(sp)
	eng, err := q.GetActiveEngagementForTaxpayer(ctx, taxpayerID)
	if err != nil {
		_ = sp.Rollback(ctx)
		return
	}
	for _, w := range ws {
		setting, err := q.ResolveWarningSetting(ctx, db.ResolveWarningSettingParams{
			Fallback: model.Severity(w.Code), Code: w.Code, OrganizationID: eng.OrganizationID,
			EngagementID: pgtype.Int8{Int64: eng.ID, Valid: true},
		})
		if err != nil {
			_ = sp.Rollback(ctx)
			return
		}
		if !setting.Enabled {
			continue
		}
		entityID := saved.InvoiceID
		if w.EntityType == model.EntityInvoiceItem {
			if id, ok := saved.ItemIDs[w.LineNo]; ok {
				entityID = id
			}
		}
		params := map[string]any{}
		for k, v := range w.Params {
			params[k] = v
		}
		if w.LineNo > 0 {
			params["line_no"] = w.LineNo
		}
		raw, _ := json.Marshal(params)
		if _, err := q.UpsertOpenDataWarning(ctx, db.UpsertOpenDataWarningParams{
			OrganizationID: eng.OrganizationID, EngagementID: eng.ID, TaxpayerID: taxpayerID,
			Period: repository.Text(inv.Period), EntityType: w.EntityType, EntityID: pgtype.Int8{Int64: entityID, Valid: true},
			Code: w.Code, Severity: setting.Severity, Params: raw, Producer: "source:" + sourceSystem,
		}); err != nil {
			_ = sp.Rollback(ctx)
			return
		}
	}
	_ = sp.Commit(ctx)
}

// Report counts one import (02 §11: counts and totals equal the source).
type Report struct {
	InvoicesCreated    int `json:"invoices_created"`
	InvoicesExisting   int `json:"invoices_existing"`
	ExpensesCreated    int `json:"expenses_created"`
	ExpensesUpdated    int `json:"expenses_updated"`
	ExpensesUnchanged  int `json:"expenses_unchanged"`
	PossibleDuplicates int `json:"possible_duplicates"`
	// Financial transactions and Z reports (issue #385); sources without
	// them leave the counts out.
	FinancialCreated   int      `json:"financial_created,omitempty"`
	FinancialUpdated   int      `json:"financial_updated,omitempty"`
	FinancialUnchanged int      `json:"financial_unchanged,omitempty"`
	ZReportsCreated    int      `json:"z_reports_created,omitempty"`
	ZReportsUpdated    int      `json:"z_reports_updated,omitempty"`
	ZReportsUnchanged  int      `json:"z_reports_unchanged,omitempty"`
	Excluded           Excluded `json:"excluded"`
}

// Add sums r into rep.
func (rep *Report) Add(r Report) {
	rep.InvoicesCreated += r.InvoicesCreated
	rep.InvoicesExisting += r.InvoicesExisting
	rep.ExpensesCreated += r.ExpensesCreated
	rep.ExpensesUpdated += r.ExpensesUpdated
	rep.ExpensesUnchanged += r.ExpensesUnchanged
	rep.PossibleDuplicates += r.PossibleDuplicates
	rep.FinancialCreated += r.FinancialCreated
	rep.FinancialUpdated += r.FinancialUpdated
	rep.FinancialUnchanged += r.FinancialUnchanged
	rep.ZReportsCreated += r.ZReportsCreated
	rep.ZReportsUpdated += r.ZReportsUpdated
	rep.ZReportsUnchanged += r.ZReportsUnchanged
	if rep.Excluded == nil {
		rep.Excluded = Excluded{}
	}
	for k, v := range r.Excluded {
		rep.Excluded[k] += v
	}
}

func count(err error, inserted bool, created, updated, unchanged *int) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		*unchanged++
	case err != nil:
		return err
	case inserted:
		*created++
	default:
		*updated++
	}
	return nil
}

func date(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

// Write stores a batch in one transaction: invoices through the common
// duplicate rules (01 §6), expenses by their source key.
func Write(ctx context.Context, pool *pgxpool.Pool, taxpayerID int64, sourceSystem string, b Batch) (Report, error) {
	rep := Report{Excluded: b.Excluded}
	if rep.Excluded == nil {
		rep.Excluded = Excluded{}
	}
	err := database.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		for _, inv := range b.Invoices {
			res, err := repository.SaveInvoice(ctx, tx, taxpayerID, inv, 0)
			if err != nil {
				return err
			}
			if res.Duplicate() {
				rep.InvoicesExisting++
			} else {
				rep.InvoicesCreated++
			}
			raiseWarnings(ctx, tx, taxpayerID, sourceSystem, inv, res, b.InvoiceWarnings[inv.SourceID])
			rep.PossibleDuplicates += len(res.PossibleDuplicates)
		}
		q := db.New(tx)
		for _, e := range b.Expenses {
			raw, err := json.Marshal(e.Raw)
			if err != nil {
				return err
			}
			if raw, err = secrets.SealField(secrets.PurposeExpenseRawPayload, raw); err != nil {
				return err
			}
			rt := e.ReceiptType
			if rt == "" {
				rt = "other"
			}
			row, err := q.UpsertSourceExpense(ctx, db.UpsertSourceExpenseParams{
				TaxpayerID: taxpayerID, SourceSystem: sourceSystem, SourceID: e.SourceID,
				ExpenseDate: pgtype.Date{Time: e.Date, Valid: true}, Period: repository.Text(e.Period),
				Amount: repository.Num(e.Amount), Currency: e.Currency, TaxRate: repository.NumPtr(e.TaxRate),
				TaxExclusiveAmount: repository.Num(e.Amount.Sub(e.TaxAmount)), TaxAmount: repository.Num(e.TaxAmount),
				Description: e.Description, ReceiptType: rt, VendorTaxNumber: repository.Text(e.VendorTaxID),
				VendorName: repository.Text(e.VendorName), VehiclePlate: repository.Text(e.VehiclePlate),
				PaymentMethod: repository.Text(e.PaymentMethod), ReceiptNumber: repository.Text(e.ReceiptNumber),
				RawPayload: raw, ContentHash: repository.Text(e.Hash),
			})
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				rep.ExpensesUnchanged++
				continue
			case err != nil:
				return err
			case row.Inserted:
				rep.ExpensesCreated++
			default:
				rep.ExpensesUpdated++
			}
			if err := replaceExpenseVatLines(ctx, q, taxpayerID, row.ID, e.VatLines); err != nil {
				return err
			}
		}
		for _, f := range b.FinancialTransactions {
			raw, err := json.Marshal(f.Raw)
			if err != nil {
				return err
			}
			if raw, err = secrets.SealField(secrets.PurposeFinancialTxRawPayload, raw); err != nil {
				return err
			}
			cur := f.Currency
			if cur == "" {
				cur = "TRY"
			}
			row, err := q.UpsertSourceFinancialTransaction(ctx, db.UpsertSourceFinancialTransactionParams{
				TaxpayerID: taxpayerID, SourceSystem: sourceSystem, SourceID: f.SourceID,
				TransactionDate: pgtype.Date{Time: f.Date, Valid: true}, DueDate: date(f.DueDate), Period: f.Period,
				TransactionType: f.TransactionType, TransactionFlow: f.TransactionFlow, Direction: f.Direction,
				PaymentMethodType: f.PaymentMethodType, Amount: repository.Num(f.Amount), Currency: cur,
				ExchangeRate: repository.NumPtr(f.ExchangeRate), AmountTry: repository.Num(f.AmountTRY),
				Description: f.Description, ReferenceDocumentNo: repository.Text(f.ReferenceDocumentNo), Status: f.Status,
				CustomerSourceID: repository.Text(f.CustomerSourceID), CustomerName: repository.Text(f.CustomerName),
				CustomerTaxNumber: repository.Text(f.CustomerTaxNumber), CashBoxSourceID: repository.Text(f.CashBoxSourceID),
				CashBoxName: repository.Text(f.CashBoxName), CashBoxType: repository.Text(f.CashBoxType),
				ReferenceableType: repository.Text(f.ReferenceableType), ReferenceableID: repository.Text(f.ReferenceableID),
				RawPayload: raw, ContentHash: repository.Text(f.Hash),
			})
			if err := count(err, row.Inserted, &rep.FinancialCreated, &rep.FinancialUpdated, &rep.FinancialUnchanged); err != nil {
				return err
			}
		}
		for _, z := range b.ZReports {
			raw, err := json.Marshal(z.Raw)
			if err != nil {
				return err
			}
			if raw, err = secrets.SealField(secrets.PurposeRetailSaleRawPayload, raw); err != nil {
				return err
			}
			cur := z.Currency
			if cur == "" {
				cur = "TRY"
			}
			row, err := q.UpsertSourceZReport(ctx, db.UpsertSourceZReportParams{
				TaxpayerID: taxpayerID, SourceSystem: sourceSystem, SourceID: z.SourceID,
				SaleDate: pgtype.Date{Time: z.Date, Valid: true}, Period: repository.Text(z.Period),
				ZReportNumber: repository.Text(z.ZReportNumber), OkcSerialNumber: repository.Text(z.OKCSerialNumber),
				SubtotalAmount: repository.Num(z.Subtotal), TaxAmount: repository.Num(z.Tax), TotalAmount: repository.Num(z.Total),
				Currency: cur, RawPayload: raw, ContentHash: repository.Text(z.Hash),
			})
			if err := count(err, row.Inserted, &rep.ZReportsCreated, &rep.ZReportsUpdated, &rep.ZReportsUnchanged); err != nil {
				return err
			}
		}
		return nil
	})
	return rep, err
}

// replaceExpenseVatLines writes the expense's per-rate breakdown anew (a
// changed record may have changed rates). No lines → none stored.
func replaceExpenseVatLines(ctx context.Context, q *db.Queries, taxpayerID, expenseID int64, lines []ExpenseVatLine) error {
	if err := q.DeleteExpenseVatLines(ctx, db.DeleteExpenseVatLinesParams{TaxpayerID: taxpayerID, ExpenseID: expenseID}); err != nil {
		return err
	}
	rows := ExpenseVatRows(lines)
	if len(rows) == 0 {
		return nil
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	_, err = q.InsertExpenseVatLines(ctx, db.InsertExpenseVatLinesParams{TaxpayerID: taxpayerID, ExpenseID: expenseID, Rows: raw})
	return err
}

// ExpenseVatRows is the jsonb row set InsertExpenseVatLines takes. Lines of
// the same rate are summed; a negative amount is left out.
func ExpenseVatRows(lines []ExpenseVatLine) []map[string]string {
	by := map[string]*ExpenseVatLine{}
	var keys []string
	for _, l := range lines {
		if l.Rate.IsNegative() || l.Base.IsNegative() || l.Tax.IsNegative() {
			continue
		}
		k := l.Rate.String()
		if cur, ok := by[k]; ok {
			cur.Base, cur.Tax = cur.Base.Add(l.Base), cur.Tax.Add(l.Tax)
			continue
		}
		c := l
		by[k] = &c
		keys = append(keys, k)
	}
	out := make([]map[string]string, 0, len(keys))
	for _, k := range keys {
		l := by[k]
		out = append(out, map[string]string{"tax_rate": l.Rate.String(), "tax_base": l.Base.String(), "tax_amount": l.Tax.String()})
	}
	return out
}
