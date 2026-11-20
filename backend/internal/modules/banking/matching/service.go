package matching

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/posting"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
)

// SuggestionKind is the review queue kind of a bank–invoice match.
const SuggestionKind = "bank_match"

// Producer of the 21 §6.3 control warnings (05 §1: control:<rule>).
const Producer = "control:bank"

// Window: invoices issued up to this long before the payment (or a little
// after it, for prepayments) are candidates.
const (
	windowBefore = 180 * 24 * time.Hour
	windowAfter  = 15 * 24 * time.Hour
)

// Submitter is the review queue (#14).
type Submitter interface {
	Submit(ctx context.Context, sub reviewusecase.Subject, in reviewusecase.Input) (reviewusecase.Suggestion, error)
}

// WarningEngine is the #13 engine.
type WarningEngine interface {
	Apply(ctx context.Context, s engine.Subject, producer string, ent engine.Entity, owned []string, findings []engine.Finding) (engine.Report, error)
}

// Service runs matching and the period controls for one engagement.
type Service struct {
	pool     *pgxpool.Pool
	review   Submitter
	warnings WarningEngine
	accounts AccountBooker
	opt      Options
}

// AccountBooker books one movement to a trial-balance account (the bank
// posting workflow, #383).
type AccountBooker interface {
	SetAccount(ctx context.Context, sub posting.Subject, userID int64, txn uuid.UUID, code, name string) error
}

// WithAccounts lets the single-movement screen book the movement to an
// account (#565). Without it SetAccount answers ErrNoAccounts.
func (s *Service) WithAccounts(a AccountBooker) *Service {
	s.accounts = a
	return s
}

// NewService builds the service. review and warnings may be nil (tests,
// tenancy off): suggestions or warnings are then not written.
func NewService(pool *pgxpool.Pool, review Submitter, warnings WarningEngine, opt Options) *Service {
	return &Service{pool: pool, review: review, warnings: warnings, opt: opt}
}

// Subject is the engagement the run is for.
type Subject struct {
	OrganizationID int64
	EngagementID   int64
	TaxpayerID     int64
}

// Result counts a run.
type Result struct {
	Period    string        `json:"period"`
	Examined  int           `json:"examined"`
	Auto      int           `json:"auto_matched"`
	Suggested int           `json:"suggested"`
	Unmatched int           `json:"unmatched"`
	Warnings  engine.Report `json:"warnings"`
	Findings  []string      `json:"findings"`
}

// matchValue is the suggestion value and the evidence of a match.
type matchValue struct {
	BankTransactionID int64          `json:"bank_transaction_id"`
	Invoices          []matchInvoice `json:"invoices"`
	Score             int            `json:"score"`
	Combination       bool           `json:"combination,omitempty"`
}

type matchInvoice struct {
	ID     int64          `json:"id"`
	Number string         `json:"number"`
	Amount string         `json:"amount"`
	Score  int            `json:"score"`
	Parts  map[string]int `json:"parts"`
}

// Run matches the period's open collections / payments to invoices and then
// runs the controls. Matching first: a payment it matches is no longer
// "undocumented".
func (s *Service) Run(ctx context.Context, sub Subject, period string) (Result, error) {
	res := Result{Period: period}
	q := database.QueriesFor(s.pool)
	txns, err := q.ListBankTransactionsForMatching(ctx, db.ListBankTransactionsForMatchingParams{TaxpayerID: sub.TaxpayerID, Period: period})
	if err != nil {
		return res, err
	}
	for _, t := range txns {
		res.Examined++
		out, err := s.matchOne(ctx, q, sub, t)
		if err != nil {
			return res, fmt.Errorf("bank transaction %s: %w", t.Uuid, err)
		}
		switch out {
		case OutcomeAuto:
			res.Auto++
		case OutcomeSuggest:
			res.Suggested++
		default:
			res.Unmatched++
		}
	}
	findings, rep, err := s.controls(ctx, q, sub, period)
	if err != nil {
		return res, err
	}
	res.Warnings = rep
	for _, f := range findings {
		res.Findings = append(res.Findings, f.Code)
	}
	return res, nil
}

func (s *Service) matchOne(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction) (Outcome, error) {
	m, cands, _, err := s.candidates(ctx, q, sub, t)
	if err != nil {
		return OutcomeNone, err
	}
	d := Decide(m, cands, s.opt)
	switch d.Outcome {
	case OutcomeAuto:
		err := database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			return writeMatches(ctx, db.New(tx), sub.TaxpayerID, m, d.Picks, "auto", nil)
		})
		return d.Outcome, err
	case OutcomeSuggest:
		if s.review == nil {
			return OutcomeNone, nil
		}
		if err := s.suggest(ctx, sub, t, m, d); err != nil {
			return OutcomeNone, err
		}
		return d.Outcome, q.SetBankTransactionMatchStatus(ctx, db.SetBankTransactionMatchStatusParams{
			MatchStatus: "suggested", TaxpayerID: sub.TaxpayerID, ID: t.ID,
		})
	}
	return OutcomeNone, nil
}

func valueOf(m Movement, picks []Score, combination bool) matchValue {
	v := matchValue{BankTransactionID: m.ID, Combination: combination}
	remaining := m.Amount
	for _, p := range picks {
		amt := p.Candidate.Open
		if amt.GreaterThan(remaining) {
			amt = remaining
		}
		remaining = remaining.Sub(amt)
		v.Invoices = append(v.Invoices, matchInvoice{
			ID: p.Candidate.ID, Number: p.Candidate.Number, Amount: amt.StringFixed(2), Score: p.Total, Parts: p.Parts,
		})
		if v.Score == 0 || p.Total < v.Score {
			v.Score = p.Total // a combination is as strong as its weakest invoice
		}
	}
	return v
}

// writeMatches records the matches and marks the movement matched.
func writeMatches(ctx context.Context, q *db.Queries, taxpayerID int64, m Movement, picks []Score, method string, userID *int64) error {
	v := valueOf(m, picks, len(picks) > 1)
	return writeValue(ctx, q, taxpayerID, v, method, userID)
}

func writeValue(ctx context.Context, q *db.Queries, taxpayerID int64, v matchValue, method string, userID *int64) error {
	for _, inv := range v.Invoices {
		evidence, _ := json.Marshal(map[string]any{"parts": inv.Parts, "combination": v.Combination, "rule": "21 §6.2"})
		amt := decimal.RequireFromString(inv.Amount)
		if !amt.IsPositive() {
			continue
		}
		p := db.InsertBankTransactionMatchParams{
			TaxpayerID: taxpayerID, BankTransactionID: v.BankTransactionID, TargetType: "invoice", TargetID: inv.ID,
			Amount: numeric(amt), Score: int16(min(inv.Score, 200)), Method: method, Evidence: evidence, //nolint:gosec // bounded
		}
		if userID != nil {
			p.MatchedBy = pgtype.Int8{Int64: *userID, Valid: true}
		}
		if _, err := q.InsertBankTransactionMatch(ctx, p); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	return q.SetBankTransactionMatchStatus(ctx, db.SetBankTransactionMatchStatusParams{
		MatchStatus: "matched", TaxpayerID: taxpayerID, ID: v.BankTransactionID,
	})
}

func (s *Service) suggest(ctx context.Context, sub Subject, t db.BankTransaction, m Movement, d Decision) error {
	v := valueOf(m, d.Picks, d.Combination)
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	numbers := make([]string, len(v.Invoices))
	for i, inv := range v.Invoices {
		numbers[i] = inv.Number
	}
	rationale := fmt.Sprintf("Banka hareketi %s (%s %s) ↔ fatura %s, puan %d (21 §6.2)",
		t.TransactionDate.Time.Format("2006-01-02"), m.Amount.StringFixed(2), t.Currency, strings.Join(numbers, ", "), v.Score)
	if d.Combination {
		rationale = fmt.Sprintf("Toplu ödeme: %d fatura toplamı hareketle birebir aynı (%s %s); %s",
			len(numbers), m.Amount.StringFixed(2), t.Currency, strings.Join(numbers, ", "))
	}
	alternatives := []map[string]any{}
	for _, r := range d.Ranked {
		if len(alternatives) == 3 {
			break
		}
		alternatives = append(alternatives, map[string]any{"invoice": r.Candidate.Number, "score": r.Total})
	}
	id := t.ID
	// Below the auto threshold by construction (≤ 0.79), so the queue keeps
	// it for the accountant whatever its own threshold.
	conf := decimal.NewFromInt(int64(min(v.Score, AutoScore-1))).Div(decimal.NewFromInt(100))
	_, err = s.review.Submit(ctx, reviewusecase.Subject{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID,
	}, reviewusecase.Input{
		Kind: SuggestionKind, TargetType: "bank_transaction", TargetID: &id, TargetField: "match",
		Period: t.Period, Value: raw, Confidence: conf, Rationale: rationale,
		Provenance: reviewusecase.Provenance{
			Origin: reviewusecase.OriginRule, RuleRef: "bank_match:21-6.2",
			Inputs: map[string]any{"amount": m.Amount.StringFixed(2), "currency": t.Currency, "date": t.TransactionDate.Time.Format("2006-01-02"),
				"counterparty": t.CounterpartyName.String, "alternatives": alternatives},
		},
	})
	return err
}

// Applier writes an accepted bank_match suggestion (review queue, #14).
type Applier struct{}

// Apply records the matches of the (possibly corrected) value.
func (Applier) Apply(ctx context.Context, tx reviewusecase.Tx, sug reviewusecase.Suggestion, value json.RawMessage) (json.RawMessage, error) {
	q, ok := tx.Handle().(*db.Queries)
	if !ok {
		return nil, fmt.Errorf("bank_match: no database handle")
	}
	var v matchValue
	if err := json.Unmarshal(value, &v); err != nil || v.BankTransactionID == 0 || len(v.Invoices) == 0 {
		return nil, fmt.Errorf("%w: bank_match: value must name the bank transaction and its invoices", reviewusecase.ErrInvalidValue)
	}
	if sug.Input.TargetID == nil || *sug.Input.TargetID != v.BankTransactionID {
		return nil, fmt.Errorf("%w: bank_match: value is for another bank transaction", reviewusecase.ErrInvalidValue)
	}
	method := "suggested_accepted"
	if sug.Status == reviewusecase.StatusAutoAccepted {
		method = "auto"
	}
	if err := writeValue(ctx, q, sug.TaxpayerID, v, method, nil); err != nil {
		return nil, err
	}
	return json.RawMessage("null"), nil
}

// Revert removes the matches and puts the movement back to suggested.
func (Applier) Revert(ctx context.Context, tx reviewusecase.Tx, sug reviewusecase.Suggestion, _ json.RawMessage) error {
	q, ok := tx.Handle().(*db.Queries)
	if !ok || sug.Input.TargetID == nil {
		return fmt.Errorf("bank_match: no database handle")
	}
	if err := q.DeleteBankTransactionMatches(ctx, db.DeleteBankTransactionMatchesParams{TaxpayerID: sug.TaxpayerID, BankTransactionID: *sug.Input.TargetID}); err != nil {
		return err
	}
	return q.SetBankTransactionMatchStatus(ctx, db.SetBankTransactionMatchStatusParams{
		MatchStatus: "suggested", TaxpayerID: sug.TaxpayerID, ID: *sug.Input.TargetID,
	})
}

func (s *Service) controls(ctx context.Context, q *db.Queries, sub Subject, period string) ([]Finding, engine.Report, error) {
	rows, err := q.ListBankTransactionsForControls(ctx, db.ListBankTransactionsForControlsParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID, Period: period})
	if err != nil {
		return nil, engine.Report{}, err
	}
	in := make([]Row, len(rows))
	for i, r := range rows {
		in[i] = Row{
			ID: r.ID, UUID: r.Uuid.String(), Date: r.TransactionDate.Time.Format("2006-01-02"),
			Direction: banking.Direction(r.DebitCredit), Amount: dec(r.Amount), Currency: r.Currency,
			// A movement the accountant booked to an account by hand (cari,
			// advance, expense) is settled like a matched one (#565).
			Detail: banking.Detail(r.KindDetail), Matched: r.MatchStatus == "matched" || r.BookedByHand,
			Counterparty: r.CounterpartyName.String, TaxNumber: r.CounterpartyTaxNumber.String,
			IdentityNumber: r.CounterpartyIdentityNumber.String, PaymentPurpose: r.PaymentPurpose.String,
		}
	}
	findings := Controls(in)
	if s.warnings == nil {
		return findings, engine.Report{}, nil
	}
	ef := make([]engine.Finding, len(findings))
	for i, f := range findings {
		ef[i] = engine.Finding{Code: f.Code, Params: f.Params}
	}
	rep, err := s.warnings.Apply(ctx, engine.Subject{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID,
	}, Producer, engine.Entity{Type: "period", Period: period}, OwnedCodes, ef)
	return findings, rep, err
}

func dec(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid {
		return decimal.Zero
	}
	v, err := n.Value()
	if err != nil || v == nil {
		return decimal.Zero
	}
	d, err := decimal.NewFromString(fmt.Sprint(v))
	if err != nil {
		return decimal.Zero
	}
	return d
}

func numeric(d decimal.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(d.String())
	return n
}
