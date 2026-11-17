package payroll

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

var periodRe = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

// Bank classification (banking/classify.go) of the payments looked at.
const (
	kindSGK      = "SGK_ODEMESI"
	detailSGK    = "sgk_payment"
	detailSalary = "salary_payment"
)

// matchTolerance: a payment may differ from the payroll by bank fees or
// rounding up to this much.
var matchTolerance = decimal.NewFromInt(1)

// BankMatchResult counts a matching run.
type BankMatchResult struct {
	Period       string      `json:"period"`
	Examined     int         `json:"examined"`
	SalaryLinks  int         `json:"salary_matched"`
	SGKLinks     int         `json:"sgk_matched"`
	Transactions []uuid.UUID `json:"bank_transaction_uuids"`
}

// MatchBank matches the period's salary and SGK payments to the payroll
// (21 §6 matching, payroll side): outgoing TRY movements from the period's
// first day to fifteen days after the SGK due date.
//
//   - SGK: an SGK_ODEMESI / sgk_payment movement (or one that says "SGK")
//     whose amount is the open payable of the accrual. The accrual is
//     marked paid.
//   - Salary: one movement for the whole net (toplu maaş ödemesi), or one
//     movement per employee (the employee's IBAN or name, the line's net).
func (s *Service) MatchBank(ctx context.Context, scope engagementctx.Scope, actorID int64, period string) (BankMatchResult, error) {
	res := BankMatchResult{Period: period, Transactions: []uuid.UUID{}}
	due, err := SGKDueDate(period)
	if err != nil {
		return res, ErrInvalid
	}
	err = database.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		q := db.New(tx)
		cands, err := q.ListPayrollBankCandidates(ctx, db.ListPayrollBankCandidatesParams{
			TaxpayerID: scope.TaxpayerID,
			DateFrom:   pgtype.Date{Time: periodStart(period), Valid: true},
			DateTo:     pgtype.Date{Time: due.AddDate(0, 0, 15), Valid: true},
		})
		if err != nil {
			return err
		}
		res.Examined = len(cands)
		used := map[int64]bool{}
		link := func(c db.ListPayrollBankCandidatesRow, target string, targetID int64, amount decimal.Decimal, rule string) error {
			evidence, _ := json.Marshal(map[string]any{"rule": rule, "period": period})
			p := db.InsertBankTransactionMatchParams{
				TaxpayerID: scope.TaxpayerID, BankTransactionID: c.ID, TargetType: target, TargetID: targetID,
				Amount: num(amount), Score: 100, Method: "auto", Evidence: evidence, MatchedBy: actor(actorID),
			}
			if _, err := q.InsertBankTransactionMatch(ctx, p); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			used[c.ID] = true
			res.Transactions = append(res.Transactions, c.Uuid)
			return q.SetBankTransactionMatchStatus(ctx, db.SetBankTransactionMatchStatusParams{
				MatchStatus: "matched", TaxpayerID: scope.TaxpayerID, ID: c.ID,
			})
		}

		// SGK accrual of the period.
		if p, err := q.GetSgkPremiumByPeriod(ctx, db.GetSgkPremiumByPeriodParams{TaxpayerID: scope.TaxpayerID, Period: period}); err == nil {
			open := dec(p.PayableAmount).Sub(dec(p.PaidAmount))
			if open.GreaterThan(matchTolerance) {
				for _, c := range cands {
					if used[c.ID] || !sgkLike(c) || dec(c.Amount).Sub(open).Abs().GreaterThan(matchTolerance) {
						continue
					}
					amt := dec(c.Amount)
					if err := link(c, "sgk_premium", p.ID, amt, "payroll:sgk-amount"); err != nil {
						return err
					}
					if _, err := q.UpdateSgkPremium(ctx, db.UpdateSgkPremiumParams{
						TaxpayerID: scope.TaxpayerID, ID: p.ID, AccrualAmount: p.AccrualAmount, IncentiveAmount: p.IncentiveAmount,
						PayableAmount: p.PayableAmount, DueDate: p.DueDate, DocumentNo: p.DocumentNo,
						PaidAmount: num(dec(p.PaidAmount).Add(amt)), PaidAt: c.TransactionDate,
						BankTransactionID: pgtype.Int8{Int64: c.ID, Valid: true}, Note: p.Note,
					}); err != nil {
						return err
					}
					res.SGKLinks++
					break
				}
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		// Salaries.
		run, err := q.GetPayrollRunByPeriod(ctx, db.GetPayrollRunByPeriodParams{TaxpayerID: scope.TaxpayerID, Period: period})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		paid, err := q.PayrollRunNetPaid(ctx, db.PayrollRunNetPaidParams{TaxpayerID: scope.TaxpayerID, PayrollRunID: run.ID})
		if err != nil {
			return err
		}
		open := dec(run.NetTotal).Sub(dec(paid))
		if !open.GreaterThan(matchTolerance) {
			return nil
		}
		for _, c := range cands {
			if !used[c.ID] && salaryLike(c) && dec(c.Amount).Sub(open).Abs().LessThanOrEqual(matchTolerance) {
				if err := link(c, "payroll_run", run.ID, dec(c.Amount), "payroll:net-total"); err != nil {
					return err
				}
				res.SalaryLinks++
				return nil
			}
		}
		lines, err := q.ListPayrollLines(ctx, db.ListPayrollLinesParams{TaxpayerID: scope.TaxpayerID, PayrollRunID: run.ID})
		if err != nil {
			return err
		}
		for _, l := range lines {
			net := dec(l.Net)
			if !net.IsPositive() {
				continue
			}
			name := parse.Normalize(l.EmployeeName)
			iban := strings.ReplaceAll(l.EmployeeIban.String, " ", "")
			for _, c := range cands {
				if used[c.ID] || dec(c.Amount).Sub(net).Abs().GreaterThan(matchTolerance) {
					continue
				}
				byIBAN := iban != "" && strings.EqualFold(strings.ReplaceAll(c.CounterpartyIban.String, " ", ""), iban)
				byName := name != "" && parse.Normalize(c.CounterpartyName.String) == name
				if !byIBAN && !byName {
					continue
				}
				rule := "payroll:employee-name"
				if byIBAN {
					rule = "payroll:employee-iban"
				}
				if err := link(c, "payroll_run", run.ID, dec(c.Amount), rule); err != nil {
					return err
				}
				res.SalaryLinks++
				break
			}
		}
		return nil
	})
	return res, err
}

func sgkLike(c db.ListPayrollBankCandidatesRow) bool {
	if c.Kind == kindSGK || c.KindDetail == detailSGK {
		return true
	}
	n := " " + parse.Normalize(c.Description+" "+c.CounterpartyName.String) + " "
	return strings.Contains(n, " sgk ") || strings.Contains(n, " ssk ") || strings.Contains(n, "sosyal guvenlik")
}

func salaryLike(c db.ListPayrollBankCandidatesRow) bool {
	if c.KindDetail == detailSalary {
		return true
	}
	n := parse.Normalize(c.Description)
	for _, w := range []string{"maas", "ucret", "personel", "bordro"} {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// unmatch removes a target's bank matches; movements left without any
// match go back to unmatched.
func unmatch(ctx context.Context, q *db.Queries, taxpayerID int64, target string, targetID int64) error {
	ids, err := q.DeleteTargetMatches(ctx, db.DeleteTargetMatchesParams{TaxpayerID: taxpayerID, TargetType: target, TargetID: targetID})
	if err != nil {
		return err
	}
	for _, id := range ids {
		n, err := q.CountBankTransactionMatches(ctx, db.CountBankTransactionMatchesParams{TaxpayerID: taxpayerID, BankTransactionID: id})
		if err != nil {
			return err
		}
		if n == 0 {
			if err := q.SetBankTransactionMatchStatus(ctx, db.SetBankTransactionMatchStatusParams{
				MatchStatus: "unmatched", TaxpayerID: taxpayerID, ID: id,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
