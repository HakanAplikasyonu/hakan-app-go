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

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
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
	Raw          map[string]any
	Hash         string
}

// Excluded counts what was read but not written, by reason.
type Excluded map[string]int

// Batch is one window's mapped records.
type Batch struct {
	Invoices []model.Invoice
	Expenses []Expense
	Excluded Excluded
}

// Report counts one import (02 §11: counts and totals equal the source).
type Report struct {
	InvoicesCreated    int      `json:"invoices_created"`
	InvoicesExisting   int      `json:"invoices_existing"`
	ExpensesCreated    int      `json:"expenses_created"`
	ExpensesUpdated    int      `json:"expenses_updated"`
	ExpensesUnchanged  int      `json:"expenses_unchanged"`
	PossibleDuplicates int      `json:"possible_duplicates"`
	Excluded           Excluded `json:"excluded"`
}

// Write stores a batch in one transaction: invoices through the common
// duplicate rules (01 §6), expenses by their source key.
func Write(ctx context.Context, pool *pgxpool.Pool, taxpayerID int64, sourceSystem string, b Batch) (Report, error) {
	rep := Report{Excluded: b.Excluded}
	if rep.Excluded == nil {
		rep.Excluded = Excluded{}
	}
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
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
			rep.PossibleDuplicates += len(res.PossibleDuplicates)
		}
		q := db.New(tx)
		for _, e := range b.Expenses {
			raw, _ := json.Marshal(e.Raw)
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
				RawPayload: raw, ContentHash: repository.Text(e.Hash),
			})
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				rep.ExpensesUnchanged++
			case err != nil:
				return err
			case row.Inserted:
				rep.ExpensesCreated++
			default:
				rep.ExpensesUpdated++
			}
		}
		return nil
	})
	return rep, err
}
