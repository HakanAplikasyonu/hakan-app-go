package matching

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// ErrInvalidFinancial: a financial transaction is missing, of the other
// direction or method, already covered, or the amount is used up (#565).
var ErrInvalidFinancial = errors.New("financial transactions cannot be matched to this transaction")

// financialMethods: the payment methods of a source financial transaction
// (Easytrade, KolayBi, Paraşüt) a bank movement settles: a cheque or a
// promissory note paid or collected through the bank, or a credit card
// spending the card statement payment closes (#565).
var financialMethods = []string{"check", "promissory_note", "credit_card"}

// MatchedFinancial is a source financial transaction the movement settles.
type MatchedFinancial struct {
	FinancialUUID     uuid.UUID `json:"financial_transaction_uuid"`
	TransactionDate   string    `json:"transaction_date"`
	DueDate           *string   `json:"due_date"`
	PaymentMethodType string    `json:"payment_method_type"`
	AmountTRY         string    `json:"amount_try"`
	Description       string    `json:"description"`
	Counterparty      *string   `json:"counterparty_name"`
	DocumentNo        *string   `json:"document_no"`
	SourceSystem      string    `json:"source_system"`
	Amount            string    `json:"amount"`
	Method            string    `json:"method"`
}

// FinancialCandidate is a cheque / note of the same direction around the
// movement's date with something not yet settled.
type FinancialCandidate struct {
	FinancialUUID     uuid.UUID `json:"financial_transaction_uuid"`
	TransactionDate   string    `json:"transaction_date"`
	DueDate           *string   `json:"due_date"`
	PaymentMethodType string    `json:"payment_method_type"`
	AmountTRY         string    `json:"amount_try"`
	OpenAmount        string    `json:"open_amount"`
	Description       string    `json:"description"`
	Counterparty      *string   `json:"counterparty_name"`
	DocumentNo        *string   `json:"document_no"`
	SourceSystem      string    `json:"source_system"`
}

type financialPick struct {
	ID     int64
	Amount decimal.Decimal
}

func financialMatchable(t db.BankTransaction) bool {
	return !t.IsCancelled && t.Currency == "TRY"
}

func dateStr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format("2006-01-02")
	return &s
}

func (s *Service) financialView(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction, v *TransactionView) error {
	v.FinancialMatches, v.FinancialCandidates = []MatchedFinancial{}, []FinancialCandidate{}
	rows, err := q.ListBankTransactionFinancialMatches(ctx, db.ListBankTransactionFinancialMatchesParams{TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID})
	if err != nil {
		return err
	}
	linked := map[uuid.UUID]bool{}
	for _, r := range rows {
		linked[r.FinancialUuid] = true
		v.FinancialMatches = append(v.FinancialMatches, MatchedFinancial{
			FinancialUUID: r.FinancialUuid, TransactionDate: r.TransactionDate.Time.Format("2006-01-02"), DueDate: dateStr(r.DueDate),
			PaymentMethodType: r.PaymentMethodType, AmountTRY: dec(r.AmountTry).StringFixed(2), Description: r.Description,
			Counterparty: textPtr(r.CustomerName), DocumentNo: textPtr(r.ReferenceDocumentNo), SourceSystem: r.SourceSystem,
			Amount: dec(r.Amount).StringFixed(2), Method: r.Method,
		})
	}
	if !financialMatchable(t) {
		return nil
	}
	cands, err := s.financialCandidates(ctx, q, sub, t)
	if err != nil {
		return err
	}
	for _, c := range cands {
		open := dec(c.AmountTry).Sub(dec(c.Matched))
		if linked[c.Uuid] || !open.IsPositive() {
			continue
		}
		v.FinancialCandidates = append(v.FinancialCandidates, FinancialCandidate{
			FinancialUUID: c.Uuid, TransactionDate: c.TransactionDate.Time.Format("2006-01-02"), DueDate: dateStr(c.DueDate),
			PaymentMethodType: c.PaymentMethodType, AmountTRY: dec(c.AmountTry).StringFixed(2), OpenAmount: open.StringFixed(2),
			Description: c.Description, Counterparty: textPtr(c.CustomerName), DocumentNo: textPtr(c.ReferenceDocumentNo), SourceSystem: c.SourceSystem,
		})
	}
	return nil
}

// financialCandidates: same direction, a cheque / note whose date (or due
// date) is up to 120 days before the movement or 10 days after it.
func (s *Service) financialCandidates(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction) ([]db.ListFinancialTransactionsForBankMatchRow, error) {
	dir := "out"
	if banking.Direction(t.DebitCredit) == banking.DirIn {
		dir = "in"
	}
	d := t.TransactionDate.Time
	return q.ListFinancialTransactionsForBankMatch(ctx, db.ListFinancialTransactionsForBankMatchParams{
		BankTransactionID: t.ID, TaxpayerID: sub.TaxpayerID, Direction: dir, Methods: financialMethods,
		DateFrom: pgtype.Date{Time: d.AddDate(0, 0, -120), Valid: true}, DateTo: pgtype.Date{Time: d.AddDate(0, 0, 10), Valid: true},
		Amount: t.Amount,
	})
}

// pickFinancial validates the chosen financial transactions: each takes what
// is not yet settled, up to what is left of the movement.
func (s *Service) pickFinancial(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction, ids []uuid.UUID, remaining decimal.Decimal) ([]financialPick, decimal.Decimal, error) {
	if len(ids) == 0 {
		return nil, remaining, nil
	}
	if !financialMatchable(t) {
		return nil, remaining, fmt.Errorf("%w: only a TRY movement settles a cheque or a note", ErrInvalidFinancial)
	}
	cands, err := s.financialCandidates(ctx, q, sub, t)
	if err != nil {
		return nil, remaining, err
	}
	byUUID := make(map[uuid.UUID]db.ListFinancialTransactionsForBankMatchRow, len(cands))
	for _, c := range cands {
		byUUID[c.Uuid] = c
	}
	out := make([]financialPick, 0, len(ids))
	for id := range uniqueUUIDs(ids) {
		c, ok := byUUID[id]
		if !ok {
			return nil, remaining, fmt.Errorf("%w: the transaction is missing, of the other direction or method, or outside the movement's dates", ErrInvalidFinancial)
		}
		amt := decimal.Min(dec(c.AmountTry).Sub(dec(c.Matched)), remaining)
		if !amt.IsPositive() {
			return nil, remaining, fmt.Errorf("%w: the transaction is already settled or the amount is used up", ErrInvalidFinancial)
		}
		remaining = remaining.Sub(amt)
		out = append(out, financialPick{ID: c.ID, Amount: amt})
	}
	return out, remaining, nil
}

func writeFinancial(ctx context.Context, q *db.Queries, sub Subject, txID int64, picks []financialPick, userID int64) error {
	evidence, _ := json.Marshal(map[string]any{"rule": "manual"})
	for _, p := range picks {
		_, err := q.InsertBankTransactionMatch(ctx, db.InsertBankTransactionMatchParams{
			TaxpayerID: sub.TaxpayerID, BankTransactionID: txID, TargetType: "financial_transaction", TargetID: p.ID,
			Amount: numeric(p.Amount), Score: 100, Method: "manual", Evidence: evidence, MatchedBy: pgtype.Int8{Int64: userID, Valid: true},
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	return nil
}
