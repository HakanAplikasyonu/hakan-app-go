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
)

// ErrInvalidSgk: an SGK premium is missing, already paid, or the movement
// is not a payment (#565).
var ErrInvalidSgk = errors.New("sgk premiums cannot be matched to this transaction")

// MatchedSgk is an SGK premium (monthly payable) the movement pays.
type MatchedSgk struct {
	SgkPremiumUUID uuid.UUID `json:"sgk_premium_uuid"`
	Period         string    `json:"period"`
	PayableAmount  string    `json:"payable_amount"`
	Amount         string    `json:"amount"`
	Method         string    `json:"method"`
}

// SgkCandidate is an SGK premium of the movement's month or the three
// before it with something still unpaid (#565).
type SgkCandidate struct {
	SgkPremiumUUID uuid.UUID `json:"sgk_premium_uuid"`
	Period         string    `json:"period"`
	PayableAmount  string    `json:"payable_amount"`
	OpenAmount     string    `json:"open_amount"`
	DueDate        string    `json:"due_date"`
}

type sgkPick struct {
	ID     int64
	Amount decimal.Decimal
}

// sgkView fills the SGK side of the panel; an outgoing movement can pay an
// SGK premium like it pays salaries.
func (s *Service) sgkView(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction, v *TransactionView) error {
	v.SgkMatches, v.SgkCandidates = []MatchedSgk{}, []SgkCandidate{}
	rows, err := q.ListBankTransactionSgkMatches(ctx, db.ListBankTransactionSgkMatchesParams{TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID})
	if err != nil {
		return err
	}
	linked := map[uuid.UUID]bool{}
	for _, r := range rows {
		linked[r.SgkPremiumUuid] = true
		v.SgkMatches = append(v.SgkMatches, MatchedSgk{
			SgkPremiumUUID: r.SgkPremiumUuid, Period: r.Period, PayableAmount: dec(r.PayableAmount).StringFixed(2),
			Amount: dec(r.Amount).StringFixed(2), Method: r.Method,
		})
	}
	if !payrollMatchable(t) {
		return nil
	}
	premiums, err := s.sgkPremiums(ctx, q, sub, t)
	if err != nil {
		return err
	}
	for _, p := range premiums {
		open := dec(p.PayableAmount).Sub(dec(p.Paid))
		if linked[p.Uuid] || !open.IsPositive() {
			continue
		}
		v.SgkCandidates = append(v.SgkCandidates, SgkCandidate{
			SgkPremiumUUID: p.Uuid, Period: p.Period, PayableAmount: dec(p.PayableAmount).StringFixed(2),
			OpenAmount: open.StringFixed(2), DueDate: p.DueDate.Time.Format("2006-01-02"),
		})
	}
	return nil
}

// sgkPremiums: a month's premium is paid by the end of the next month, a
// late payment a little after; the movement's month and the three before.
func (s *Service) sgkPremiums(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction) ([]db.ListSgkPremiumsForBankMatchRow, error) {
	d := t.TransactionDate.Time
	first := d.AddDate(0, 0, 1-d.Day())
	return q.ListSgkPremiumsForBankMatch(ctx, db.ListSgkPremiumsForBankMatchParams{
		BankTransactionID: t.ID, TaxpayerID: sub.TaxpayerID,
		PeriodFrom: first.AddDate(0, -3, 0).Format("2006-01"), PeriodTo: first.Format("2006-01"),
	})
}

// pickSgk validates the chosen premiums: each takes what is still unpaid,
// up to what is left of the movement.
func (s *Service) pickSgk(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction, ids []uuid.UUID, remaining decimal.Decimal) ([]sgkPick, decimal.Decimal, error) {
	if len(ids) == 0 {
		return nil, remaining, nil
	}
	if !payrollMatchable(t) {
		return nil, remaining, fmt.Errorf("%w: only an outgoing payment pays an SGK premium", ErrInvalidSgk)
	}
	premiums, err := s.sgkPremiums(ctx, q, sub, t)
	if err != nil {
		return nil, remaining, err
	}
	byUUID := make(map[uuid.UUID]db.ListSgkPremiumsForBankMatchRow, len(premiums))
	for _, p := range premiums {
		byUUID[p.Uuid] = p
	}
	out := make([]sgkPick, 0, len(ids))
	for id := range uniqueUUIDs(ids) {
		p, ok := byUUID[id]
		if !ok {
			return nil, remaining, fmt.Errorf("%w: the premium is missing or not of the movement's month or the three before", ErrInvalidSgk)
		}
		amt := decimal.Min(dec(p.PayableAmount).Sub(dec(p.Paid)), remaining)
		if !amt.IsPositive() {
			return nil, remaining, fmt.Errorf("%w: premium %s is already paid or the amount is used up", ErrInvalidSgk, p.Period)
		}
		remaining = remaining.Sub(amt)
		out = append(out, sgkPick{ID: p.ID, Amount: amt})
	}
	return out, remaining, nil
}

// writeSgk ties the movement to the premiums and marks them paid. Runs after
// DeleteBankTransactionMatches, which gave back what the movement paid before.
func writeSgk(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction, picks []sgkPick, userID int64) error {
	evidence, _ := json.Marshal(map[string]any{"rule": "manual"})
	for _, p := range picks {
		_, err := q.InsertBankTransactionMatch(ctx, db.InsertBankTransactionMatchParams{
			TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID, TargetType: "sgk_premium", TargetID: p.ID,
			Amount: numeric(p.Amount), Score: 100, Method: "manual", Evidence: evidence, MatchedBy: pgtype.Int8{Int64: userID, Valid: true},
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := q.AddSgkPremiumPaid(ctx, db.AddSgkPremiumPaidParams{
			Amount: numeric(p.Amount), PaidAt: t.TransactionDate, BankTransactionID: t.ID,
			TaxpayerID: sub.TaxpayerID, ID: p.ID,
		}); err != nil {
			return err
		}
	}
	return nil
}
