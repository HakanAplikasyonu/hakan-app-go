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

// ErrInvalidPayroll: a payroll run is missing, already paid, or the
// movement is not a payment (#710).
var ErrInvalidPayroll = errors.New("payroll runs cannot be matched to this transaction")

// MatchedPayroll is a payroll run the movement pays (net salaries).
type MatchedPayroll struct {
	PayrollRunUUID uuid.UUID `json:"payroll_run_uuid"`
	Period         string    `json:"period"`
	NetTotal       string    `json:"net_total"`
	Amount         string    `json:"amount"`
	Method         string    `json:"method"`
}

// PayrollCandidate is a payroll run of the movement's month or the two
// before it, with what is still unpaid (#710).
type PayrollCandidate struct {
	PayrollRunUUID uuid.UUID `json:"payroll_run_uuid"`
	Period         string    `json:"period"`
	NetTotal       string    `json:"net_total"`
	OpenAmount     string    `json:"open_amount"`
	EmployeeCount  int       `json:"employee_count"`
}

// payrollMatchable: an outgoing, not cancelled movement can pay salaries.
func payrollMatchable(t db.BankTransaction) bool {
	return !t.IsCancelled && banking.Direction(t.DebitCredit) == banking.DirOut
}

type payrollPick struct {
	ID     int64
	Amount decimal.Decimal
}

func (s *Service) payrollView(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction, v *TransactionView) error {
	v.PayrollMatchable = payrollMatchable(t)
	v.PayrollMatches, v.PayrollCandidates = []MatchedPayroll{}, []PayrollCandidate{}
	rows, err := q.ListBankTransactionPayrollMatches(ctx, db.ListBankTransactionPayrollMatchesParams{TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID})
	if err != nil {
		return err
	}
	linked := map[uuid.UUID]bool{}
	for _, r := range rows {
		linked[r.PayrollRunUuid] = true
		v.PayrollMatches = append(v.PayrollMatches, MatchedPayroll{
			PayrollRunUUID: r.PayrollRunUuid, Period: r.Period, NetTotal: dec(r.NetTotal).StringFixed(2),
			Amount: dec(r.Amount).StringFixed(2), Method: r.Method,
		})
	}
	if !v.PayrollMatchable {
		return nil
	}
	runs, err := s.payrollRuns(ctx, q, sub, t)
	if err != nil {
		return err
	}
	for _, r := range runs {
		open := dec(r.NetTotal).Sub(dec(r.Paid))
		if linked[r.Uuid] || !open.IsPositive() {
			continue
		}
		v.PayrollCandidates = append(v.PayrollCandidates, PayrollCandidate{
			PayrollRunUUID: r.Uuid, Period: r.Period, NetTotal: dec(r.NetTotal).StringFixed(2),
			OpenAmount: open.StringFixed(2), EmployeeCount: int(r.EmployeeCount),
		})
	}
	return nil
}

func (s *Service) payrollRuns(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction) ([]db.ListPayrollRunsForBankMatchRow, error) {
	d := t.TransactionDate.Time
	first := d.AddDate(0, 0, 1-d.Day())
	return q.ListPayrollRunsForBankMatch(ctx, db.ListPayrollRunsForBankMatchParams{
		BankTransactionID: t.ID, TaxpayerID: sub.TaxpayerID,
		PeriodFrom: first.AddDate(0, -2, 0).Format("2006-01"), PeriodTo: first.Format("2006-01"),
	})
}

// pickPayroll validates the chosen payroll runs: each takes what is still
// unpaid, up to what is left of the movement.
func (s *Service) pickPayroll(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction, ids []uuid.UUID, remaining decimal.Decimal) ([]payrollPick, decimal.Decimal, error) {
	if len(ids) == 0 {
		return nil, remaining, nil
	}
	if !payrollMatchable(t) {
		return nil, remaining, fmt.Errorf("%w: only an outgoing payment pays salaries", ErrInvalidPayroll)
	}
	runs, err := s.payrollRuns(ctx, q, sub, t)
	if err != nil {
		return nil, remaining, err
	}
	byUUID := make(map[uuid.UUID]db.ListPayrollRunsForBankMatchRow, len(runs))
	for _, r := range runs {
		byUUID[r.Uuid] = r
	}
	out := make([]payrollPick, 0, len(ids))
	for id := range uniqueUUIDs(ids) {
		r, ok := byUUID[id]
		if !ok {
			return nil, remaining, fmt.Errorf("%w: the payroll is missing or not of the movement's month or the two before", ErrInvalidPayroll)
		}
		amt := decimal.Min(dec(r.NetTotal).Sub(dec(r.Paid)), remaining)
		if !amt.IsPositive() {
			return nil, remaining, fmt.Errorf("%w: payroll %s is already paid or the amount is used up", ErrInvalidPayroll, r.Period)
		}
		remaining = remaining.Sub(amt)
		out = append(out, payrollPick{ID: r.ID, Amount: amt})
	}
	return out, remaining, nil
}

func writePayroll(ctx context.Context, q *db.Queries, sub Subject, txID int64, picks []payrollPick, userID int64) error {
	evidence, _ := json.Marshal(map[string]any{"rule": "manual"})
	for _, p := range picks {
		_, err := q.InsertBankTransactionMatch(ctx, db.InsertBankTransactionMatchParams{
			TaxpayerID: sub.TaxpayerID, BankTransactionID: txID, TargetType: "payroll_run", TargetID: p.ID,
			Amount: numeric(p.Amount), Score: 100, Method: "manual", Evidence: evidence, MatchedBy: pgtype.Int8{Int64: userID, Valid: true},
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	return nil
}
