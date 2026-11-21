package matching

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// Kur farkı hesapları (#565): kâr 646, zarar 656.
const (
	FxGainAccount = "646"
	FxLossAccount = "656"
)

var (
	// ErrFxNotApplicable: the movement is in TRY or closes no foreign
	// currency invoice; there is no exchange difference to compute.
	ErrFxNotApplicable = errors.New("movement closes no foreign currency invoice")
	// ErrFxInvoiceRate: a matched invoice has no exchange rate; it is
	// entered on the invoice first (#571).
	ErrFxInvoiceRate = errors.New("a matched invoice has no exchange rate")
)

// FxView is the movement's exchange difference: the payment day rate the
// accountant entered and the TRY difference of what the movement closes,
// positive for a gain (646), negative for a loss (656).
type FxView struct {
	RatePayment *string `json:"rate_payment"`
	DiffTRY     *string `json:"diff_try"`
	AccountCode *string `json:"account_code"`
	// MissingInvoiceRate: a matched invoice has no rate; the difference
	// cannot be computed until it is entered on the invoice.
	MissingInvoiceRate bool `json:"missing_invoice_rate"`
}

// FxDiff is the gain (positive) or loss (negative) in TRY when amount of
// foreign currency booked at the invoice rate is settled at the payment
// rate. A collection (incoming) gains when the rate rose; a payment
// (outgoing) loses.
func FxDiff(dir banking.Direction, amount, invoiceRate, paymentRate decimal.Decimal) decimal.Decimal {
	d := amount.Mul(paymentRate.Sub(invoiceRate))
	if dir != banking.DirIn {
		d = d.Neg()
	}
	return d.Round(2)
}

// FxAccount is the account of a difference: 646 for a gain, 656 for a loss.
func FxAccount(diff decimal.Decimal) string {
	if diff.IsNegative() {
		return FxLossAccount
	}
	return FxGainAccount
}

// fxView summarises the movement's foreign currency invoice matches; nil
// for a TRY movement or one with no such match.
func fxView(t db.BankTransaction, rows []db.ListBankTransactionMatchInvoicesRow) *FxView {
	if t.Currency == "TRY" {
		return nil
	}
	v := &FxView{}
	total, any, set := decimal.Zero, false, false
	for _, r := range rows {
		if r.Currency == "TRY" {
			continue
		}
		any = true
		if !r.ExchangeRate.Valid {
			v.MissingInvoiceRate = true
		}
		if r.FxRatePayment.Valid && v.RatePayment == nil {
			s := dec(r.FxRatePayment).String()
			v.RatePayment = &s
		}
		if r.FxDiffTry.Valid {
			total, set = total.Add(dec(r.FxDiffTry)), true
		}
	}
	if !any {
		return nil
	}
	if set {
		s, a := total.StringFixed(2), FxAccount(total)
		v.DiffTRY, v.AccountCode = &s, &a
	}
	return v
}

// SetFxRate records the payment day rate of a foreign currency movement
// and the exchange difference of each foreign currency invoice it closes
// (#565). A nil rate clears it.
func (s *Service) SetFxRate(ctx context.Context, sub Subject, id uuid.UUID, rate *decimal.Decimal) (TransactionView, error) {
	q := database.QueriesFor(s.pool)
	t, err := s.transaction(ctx, q, sub, id)
	if err != nil {
		return TransactionView{}, err
	}
	if t.Currency == "TRY" {
		return TransactionView{}, ErrFxNotApplicable
	}
	rows, err := q.ListBankTransactionFxMatches(ctx, db.ListBankTransactionFxMatchesParams{TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID})
	if err != nil {
		return TransactionView{}, err
	}
	if len(rows) == 0 {
		return TransactionView{}, ErrFxNotApplicable
	}
	dir := banking.Direction(t.DebitCredit)
	err = database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tq := db.New(tx)
		for _, r := range rows {
			p := db.SetBankTransactionMatchFxParams{TaxpayerID: sub.TaxpayerID, ID: r.ID}
			if rate != nil {
				if !r.ExchangeRate.Valid || !dec(r.ExchangeRate).IsPositive() {
					return fmt.Errorf("%w: enter it on the invoice first", ErrFxInvoiceRate)
				}
				inv := dec(r.ExchangeRate)
				p.FxRateInvoice, p.FxRatePayment = numeric(inv), numeric(*rate)
				p.FxDiffTry = numeric(FxDiff(dir, dec(r.Amount), inv, *rate))
			} else {
				p.FxRateInvoice, p.FxRatePayment, p.FxDiffTry = pgtype.Numeric{}, pgtype.Numeric{}, pgtype.Numeric{}
			}
			if err := tq.SetBankTransactionMatchFx(ctx, p); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return TransactionView{}, err
	}
	return s.Transaction(ctx, sub, t.Uuid)
}
