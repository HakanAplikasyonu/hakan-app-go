package parasut

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
)

// Report counts one import (02 §11 acceptance: counts and totals equal the
// source).
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
// duplicate rules (01 §6), expenses by their Paraşüt key.
func Write(ctx context.Context, pool *pgxpool.Pool, taxpayerID int64, b Batch) (Report, error) {
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
			p := db.UpsertSourceExpenseParams{
				TaxpayerID: taxpayerID, SourceSystem: SourceSystem, SourceID: e.SourceID,
				ExpenseDate: pgtype.Date{Time: e.Date, Valid: true}, Period: repository.Text(e.Period),
				Amount: repository.Num(e.Amount), Currency: e.Currency, TaxRate: repository.NumPtr(e.TaxRate),
				TaxExclusiveAmount: repository.Num(e.Amount.Sub(e.TaxAmount)), TaxAmount: repository.Num(e.TaxAmount),
				Description: e.Description, ReceiptType: "other", VendorTaxNumber: repository.Text(e.VendorTaxID),
				VendorName: repository.Text(e.VendorName), VehiclePlate: repository.Text(e.VehiclePlate),
				RawPayload: raw, ContentHash: repository.Text(e.Hash),
			}
			row, err := q.UpsertSourceExpense(ctx, p)
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
