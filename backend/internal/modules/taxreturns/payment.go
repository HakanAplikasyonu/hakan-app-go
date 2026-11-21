package taxreturns

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Tahakkuk ödemesi (#575): a filed KDV1 / MUHSGK return with an amount to
// pay is paid to the tax office by its due date (the filing deadline is
// also the payment deadline for both). The office records the payment day,
// optionally with the bank movement that paid it; the movement is then
// booked to 360 Ödenecek Vergi ve Fonlar. Unpaid after the due date is
// overdue.
var (
	// ErrPaymentNotApplicable: the return is not filed or has nothing to pay.
	ErrPaymentNotApplicable = errors.New("taxreturns: only a filed return with an amount to pay is paid")
	// ErrInvalidPayment: the bank movement is missing, not an outgoing TRY
	// payment, cancelled, or the day is outside the allowed range.
	ErrInvalidPayment = errors.New("taxreturns: invalid payment")
)

// Payment statuses.
const (
	PaymentUnpaid  = "unpaid"
	PaymentPaid    = "paid"
	PaymentOverdue = "overdue"
)

// TaxPaymentAccount is where a tax payment from the bank is booked.
const TaxPaymentAccount = "360"

// TaxPaymentAccountName names TaxPaymentAccount.
const TaxPaymentAccountName = "Ödenecek Vergi ve Fonlar"

// candidate window around filing and due date.
const (
	paymentDaysBeforeFiling = 5
	paymentDaysAfterDue     = 60
)

// AccountBooker books a bank movement to an account (the bank posting
// workflow). Without it the payment is recorded but the movement is not
// booked.
type AccountBooker interface {
	BookBankTransaction(ctx context.Context, sc engagementctx.Scope, userID int64, txn uuid.UUID, code, name string) error
}

// WithAccounts lets a recorded payment book its bank movement to 360.
func (s *Service) WithAccounts(a AccountBooker) *Service { s.accounts = a; return s }

// PaymentMovement is a bank movement that pays (or may pay) the accrual.
type PaymentMovement struct {
	UUID            uuid.UUID `json:"uuid"`
	TransactionDate string    `json:"transaction_date"`
	Amount          Money     `json:"amount"`
	Description     string    `json:"description"`
	SameAmount      bool      `json:"same_amount"`
}

// PaymentView is the payment state of a filed return with an amount to pay.
type PaymentView struct {
	Status          string            `json:"status"`
	Amount          Money             `json:"amount"`
	DueDate         string            `json:"due_date"`
	PaidOn          *string           `json:"paid_on"`
	BankTransaction *PaymentMovement  `json:"bank_transaction"`
	Candidates      []PaymentMovement `json:"candidates"`
}

// PaymentInput is the body of PUT .../payment: paid_on null clears.
type PaymentInput struct {
	PaidOn              *string    `json:"paid_on"`
	BankTransactionUUID *uuid.UUID `json:"bank_transaction_uuid"`
}

func paymentApplies(kind string, r *db.TaxReturn) bool {
	return r != nil && r.Status == StatusFiled && (kind == KindKDV1 || kind == KindMUHSGK) && dec(r.PayableAmount).IsPositive()
}

// paymentStatus: paid with a day, else overdue after the due date.
func paymentStatus(paid bool, due, today time.Time) string {
	switch {
	case paid:
		return PaymentPaid
	case today.After(due):
		return PaymentOverdue
	default:
		return PaymentUnpaid
	}
}

// payment fills the payment view of a filed return; nil when it has
// nothing to pay.
func (s *Service) payment(ctx context.Context, sc engagementctx.Scope, kind string, r *db.TaxReturn, cal Calendar) (*PaymentView, error) {
	if !paymentApplies(kind, r) {
		return nil, nil
	}
	due, err := time.Parse(time.DateOnly, cal.DueDate)
	if err != nil {
		return nil, fmt.Errorf("tax return due date: %w", err)
	}
	today := s.now().In(Istanbul)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	v := &PaymentView{Amount: M(dec(r.PayableAmount)), DueDate: cal.DueDate, PaidOn: datePtr(r.PaidOn), Candidates: []PaymentMovement{}}
	v.Status = paymentStatus(r.PaidOn.Valid, due, today)
	if r.PaymentBankTransactionID.Valid {
		b, err := s.q.GetTaxReturnPaymentTransaction(ctx, db.GetTaxReturnPaymentTransactionParams{TaxpayerID: sc.TaxpayerID, ID: r.PaymentBankTransactionID.Int64})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("tax payment movement: %w", err)
		}
		if err == nil {
			v.BankTransaction = &PaymentMovement{UUID: b.Uuid, TransactionDate: dateStr(b.TransactionDate), Amount: M(dec(b.Amount)),
				Description: b.Description, SameAmount: dec(b.Amount).Equal(dec(r.PayableAmount))}
		}
	}
	if r.PaidOn.Valid {
		return v, nil
	}
	from := due.AddDate(0, 0, -30)
	if r.FiledOn.Valid {
		from = r.FiledOn.Time.AddDate(0, 0, -paymentDaysBeforeFiling)
	}
	rows, err := s.q.ListTaxPaymentCandidates(ctx, db.ListTaxPaymentCandidatesParams{Amount: r.PayableAmount, TaxpayerID: sc.TaxpayerID,
		DateFrom: pgtype.Date{Time: from, Valid: true}, DateTo: pgtype.Date{Time: due.AddDate(0, 0, paymentDaysAfterDue), Valid: true}})
	if err != nil {
		return nil, fmt.Errorf("tax payment candidates: %w", err)
	}
	for _, c := range rows {
		v.Candidates = append(v.Candidates, PaymentMovement{UUID: c.Uuid, TransactionDate: dateStr(c.TransactionDate), Amount: M(dec(c.Amount)),
			Description: c.Description, SameAmount: c.SameAmount})
	}
	return v, nil
}

// SetPayment records (or clears, paid_on null) the payment of a filed
// return. With a bank movement the day defaults to its date and the
// movement is booked to 360.
func (s *Service) SetPayment(ctx context.Context, sc engagementctx.Scope, actorID int64, kind, period string, in PaymentInput) (Preview, error) {
	if err := check(kind, period); err != nil {
		return Preview{}, err
	}
	rec, err := s.record(ctx, sc, kind, period)
	if err != nil {
		return Preview{}, err
	}
	if !paymentApplies(kind, rec) {
		return Preview{}, ErrPaymentNotApplicable
	}
	p := db.SetTaxReturnPaymentParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Kind: kind, Period: period, PaidBy: actor(actorID)}
	var txn *db.GetBankTransactionForTaxPaymentRow
	if in.BankTransactionUUID != nil {
		b, err := s.q.GetBankTransactionForTaxPayment(ctx, db.GetBankTransactionForTaxPaymentParams{TaxpayerID: sc.TaxpayerID, Uuid: *in.BankTransactionUUID})
		if errors.Is(err, pgx.ErrNoRows) {
			return Preview{}, fmt.Errorf("%w: bank movement not found", ErrInvalidPayment)
		} else if err != nil {
			return Preview{}, fmt.Errorf("tax payment movement: %w", err)
		}
		if b.DebitCredit != "B" || b.Currency != "TRY" || b.IsCancelled {
			return Preview{}, fmt.Errorf("%w: the movement must be an outgoing TRY payment", ErrInvalidPayment)
		}
		txn = &b
		p.PaymentBankTransactionID = pgtype.Int8{Int64: b.ID, Valid: true}
		p.PaidOn = b.TransactionDate
	}
	if in.PaidOn != nil && strings.TrimSpace(*in.PaidOn) != "" {
		on, err := time.Parse(time.DateOnly, strings.TrimSpace(*in.PaidOn))
		if err != nil {
			return Preview{}, fmt.Errorf("%w: paid_on must be YYYY-MM-DD", ErrInvalidInput)
		}
		p.PaidOn = pgtype.Date{Time: on, Valid: true}
	}
	if p.PaidOn.Valid {
		start, _ := ParsePeriod(period)
		if p.PaidOn.Time.Before(start) || p.PaidOn.Time.After(s.now().AddDate(0, 0, 1)) {
			return Preview{}, fmt.Errorf("%w: paid_on must be after the period start and not in the future", ErrInvalidPayment)
		}
	} else {
		p.PaidBy, p.PaymentBankTransactionID = pgtype.Int8{}, pgtype.Int8{}
	}
	if _, err := s.q.SetTaxReturnPayment(ctx, p); errors.Is(err, pgx.ErrNoRows) {
		return Preview{}, ErrPaymentNotApplicable
	} else if err != nil {
		return Preview{}, fmt.Errorf("set tax return payment: %w", err)
	}
	if txn != nil && s.accounts != nil {
		if err := s.accounts.BookBankTransaction(ctx, sc, actorID, txn.Uuid, TaxPaymentAccount, TaxPaymentAccountName); err != nil {
			return Preview{}, err
		}
	}
	return s.Preview(ctx, sc, kind, period)
}
