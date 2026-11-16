package billing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// DefaultPlan applies to an office without an organization_billing row.
const DefaultPlan = "standart"

var (
	ErrNotFound    = errors.New("billing: not found")
	ErrFinal       = errors.New("billing: statement is final")
	ErrInvalidPlan = errors.New("billing: invalid plan")
	// ErrReadOnly is the AccessPolicy refusal of a write; its text is shown.
	ErrReadOnly error = readOnlyError{}
)

type readOnlyError struct{}

func (readOnlyError) Error() string {
	return "Bu ofis salt okunur kipte: kayıt eklenemez ve değiştirilemez. Platform yöneticisiyle görüşün."
}

// Service owns plans, usage and statements.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewService builds the service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: database.QueriesFor(pool)}
}

func num(d decimal.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(d.String())
	return n
}

func dec(n pgtype.Numeric) decimal.Decimal {
	v, err := n.Value()
	if err != nil || v == nil {
		return decimal.Zero
	}
	d, _ := decimal.NewFromString(v.(string))
	return d
}

func planOf(p db.BillingPlan) Plan {
	return Plan{Code: p.Code, Name: p.Name, Currency: p.Currency, BaseFee: dec(p.BaseFee), PerTaxpayerFee: dec(p.PerTaxpayerFee),
		IncludedTaxpayers: int(p.IncludedTaxpayers), VATRate: dec(p.VatRate), Active: p.Active}
}

// Plans lists the plans.
func (s *Service) Plans(ctx context.Context) ([]Plan, error) {
	rows, err := s.q.ListBillingPlans(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Plan, 0, len(rows))
	for _, r := range rows {
		out = append(out, planOf(r))
	}
	return out, nil
}

// SavePlan creates or updates a plan.
func (s *Service) SavePlan(ctx context.Context, p Plan) (Plan, error) {
	if p.Code == "" || p.Name == "" || p.BaseFee.IsNegative() || p.PerTaxpayerFee.IsNegative() || p.IncludedTaxpayers < 0 ||
		p.VATRate.IsNegative() || p.VATRate.GreaterThan(hundred) {
		return Plan{}, ErrInvalidPlan
	}
	if p.Currency == "" {
		p.Currency = "TRY"
	}
	row, err := s.q.UpsertBillingPlan(ctx, db.UpsertBillingPlanParams{
		Code: p.Code, Name: p.Name, Currency: p.Currency, BaseFee: num(p.BaseFee), PerTaxpayerFee: num(p.PerTaxpayerFee),
		IncludedTaxpayers: int32(p.IncludedTaxpayers), VatRate: num(p.VATRate), Active: p.Active, //nolint:gosec // validated >= 0
	})
	if err != nil {
		return Plan{}, ErrInvalidPlan
	}
	return planOf(row), nil
}

// Account is an office's billing settings.
type Account struct {
	Plan           Plan   `json:"plan"`
	ReadOnly       bool   `json:"read_only"`
	ReadOnlyReason string `json:"read_only_reason,omitempty"`
}

func (s *Service) account(ctx context.Context, orgID int64) (Account, db.BillingPlan, error) {
	ob, err := s.q.GetOrganizationBilling(ctx, orgID)
	var p db.BillingPlan
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		p, err = s.q.GetBillingPlanByCode(ctx, DefaultPlan)
		if err != nil {
			return Account{}, p, err
		}
		return Account{Plan: planOf(p)}, p, nil
	case err != nil:
		return Account{}, p, err
	}
	p, err = s.q.GetBillingPlan(ctx, ob.PlanID)
	if err != nil {
		return Account{}, p, err
	}
	return Account{Plan: planOf(p), ReadOnly: ob.ReadOnly, ReadOnlyReason: ob.ReadOnlyReason}, p, nil
}

// SetAccount puts an office on a plan and sets the read-only mode.
func (s *Service) SetAccount(ctx context.Context, orgID int64, planCode string, readOnly bool, reason string, by int64) (Account, error) {
	if planCode == "" {
		planCode = DefaultPlan
	}
	p, err := s.q.GetBillingPlanByCode(ctx, planCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrInvalidPlan
	} else if err != nil {
		return Account{}, err
	}
	if !readOnly {
		reason = ""
	}
	if _, err := s.q.UpsertOrganizationBilling(ctx, db.UpsertOrganizationBillingParams{
		OrganizationID: orgID, PlanID: p.ID, ReadOnly: readOnly, ReadOnlyReason: reason,
		UpdatedBy: pgtype.Int8{Int64: by, Valid: by > 0},
	}); err != nil {
		return Account{}, err
	}
	a, _, err := s.account(ctx, orgID)
	return a, err
}

// Taxpayer counted in a period.
type Taxpayer struct {
	UUID  uuid.UUID `json:"uuid"`
	Title string    `json:"title"`
	TaxID string    `json:"tax_id"`
}

// Usage is a month's measurement.
type Usage struct {
	Period          string     `json:"period"`
	ActiveTaxpayers int        `json:"active_taxpayers"`
	Taxpayers       []Taxpayer `json:"taxpayers"`
}

// Usage counts the active taxpayers of period (D9): a taxpayer with an
// engagement of this office active on any day of the month, counted once.
func (s *Service) Usage(ctx context.Context, orgID int64, period string) (Usage, error) {
	start, end, err := PeriodBounds(period)
	if err != nil {
		return Usage{}, err
	}
	// The bill counts every active taxpayer of the office, whoever looks:
	// not limited to the engagements the caller may see (issue #166).
	var rows []db.ListActiveTaxpayersInPeriodRow
	err = rls.System(ctx, s.pool, "billing: office-wide active taxpayer count (D9)", func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = db.New(tx).ListActiveTaxpayersInPeriod(ctx, db.ListActiveTaxpayersInPeriodParams{
			OrganizationID: orgID, PeriodEnd: pgtype.Timestamptz{Time: end, Valid: true}, PeriodStart: pgtype.Timestamptz{Time: start, Valid: true},
		})
		return err
	})
	if err != nil {
		return Usage{}, err
	}
	u := Usage{Period: period, ActiveTaxpayers: len(rows), Taxpayers: make([]Taxpayer, 0, len(rows))}
	for _, r := range rows {
		u.Taxpayers = append(u.Taxpayers, Taxpayer{UUID: r.Uuid, Title: r.Title, TaxID: r.TaxID})
	}
	return u, nil
}

// Statement is a monthly invoice draft.
type Statement struct {
	UUID        uuid.UUID  `json:"uuid"`
	Period      string     `json:"period"`
	Status      string     `json:"status"`
	PlanCode    string     `json:"plan_code"`
	Currency    string     `json:"currency"`
	Amounts     Amounts    `json:"amounts"`
	Taxpayers   []Taxpayer `json:"taxpayers"`
	GeneratedAt time.Time  `json:"generated_at"`
}

func statementOf(r db.BillingStatement) Statement {
	st := Statement{UUID: r.Uuid, Period: r.Period, Status: r.Status, PlanCode: r.PlanCode, Currency: r.Currency, GeneratedAt: r.GeneratedAt.Time}
	st.Amounts = Amounts{ActiveTaxpayers: int(r.ActiveTaxpayers), BillableTaxpayers: int(r.BillableTaxpayers), BaseFee: dec(r.BaseFee),
		TaxpayerFeeTotal: dec(r.TaxpayerFeeTotal), Subtotal: dec(r.Subtotal), VATRate: dec(r.VatRate), VATAmount: dec(r.VatAmount), Total: dec(r.Total)}
	_ = json.Unmarshal(r.Lines, &st.Amounts.Lines)
	_ = json.Unmarshal(r.Taxpayers, &st.Taxpayers)
	return st
}

// Generate (re)computes the draft of period with the office's current plan.
// A final statement is not touched (ErrFinal).
func (s *Service) Generate(ctx context.Context, orgID int64, period string) (Statement, error) {
	u, err := s.Usage(ctx, orgID, period)
	if err != nil {
		return Statement{}, err
	}
	acc, _, err := s.account(ctx, orgID)
	if err != nil {
		return Statement{}, err
	}
	a := Compute(acc.Plan, u.ActiveTaxpayers)
	snap, _ := json.Marshal(acc.Plan)
	lines, _ := json.Marshal(a.Lines)
	tps, _ := json.Marshal(u.Taxpayers)
	row, err := s.q.UpsertBillingStatement(ctx, db.UpsertBillingStatementParams{
		OrganizationID: orgID, Period: period, PlanCode: acc.Plan.Code, PlanSnapshot: snap,
		ActiveTaxpayers: int32(a.ActiveTaxpayers), BillableTaxpayers: int32(a.BillableTaxpayers), Currency: acc.Plan.Currency, //nolint:gosec // counts
		BaseFee: num(a.BaseFee), TaxpayerFeeTotal: num(a.TaxpayerFeeTotal), Subtotal: num(a.Subtotal), VatRate: num(a.VATRate),
		VatAmount: num(a.VATAmount), Total: num(a.Total), Lines: lines, Taxpayers: tps,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Statement{}, ErrFinal // ON CONFLICT … WHERE status = 'draft' did not update
	}
	if err != nil {
		return Statement{}, err
	}
	return statementOf(row), nil
}

// Statements lists the office's last 24 statements.
func (s *Service) Statements(ctx context.Context, orgID int64) ([]Statement, error) {
	rows, err := s.q.ListBillingStatements(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]Statement, 0, len(rows))
	for _, r := range rows {
		out = append(out, statementOf(r))
	}
	return out, nil
}

// SetStatus finalizes or voids a draft.
func (s *Service) SetStatus(ctx context.Context, orgID int64, period, status string) (Statement, error) {
	if status != "final" && status != "void" {
		return Statement{}, ErrInvalidPlan
	}
	row, err := s.q.SetBillingStatementStatus(ctx, db.SetBillingStatementStatusParams{Status: status, OrganizationID: orgID, Period: period})
	if errors.Is(err, pgx.ErrNoRows) {
		return Statement{}, ErrNotFound
	}
	if err != nil {
		return Statement{}, err
	}
	return statementOf(row), nil
}

// Overview is what an office sees: plan, mode, this month so far, statements.
type Overview struct {
	Account    Account     `json:"account"`
	Current    Usage       `json:"current"`
	Estimate   Amounts     `json:"estimate"`
	Statements []Statement `json:"statements"`
}

// Overview of orgID at now.
func (s *Service) Overview(ctx context.Context, orgID int64, now time.Time) (Overview, error) {
	acc, _, err := s.account(ctx, orgID)
	if err != nil {
		return Overview{}, err
	}
	u, err := s.Usage(ctx, orgID, now.In(istanbul).Format("2006-01"))
	if err != nil {
		return Overview{}, err
	}
	sts, err := s.Statements(ctx, orgID)
	if err != nil {
		return Overview{}, err
	}
	return Overview{Account: acc, Current: u, Estimate: Compute(acc.Plan, u.ActiveTaxpayers), Statements: sts}, nil
}

// Check implements middleware.OrganizationPolicy: in read-only mode only
// reads pass (GET/HEAD/OPTIONS). Reads never touch the database.
func (s *Service) Check(ctx context.Context, scope orgctx.Scope, r *http.Request) error {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return nil
	}
	ro, err := s.q.OrganizationReadOnly(ctx, scope.ID)
	if err != nil {
		return nil // fail open: billing must not take the product down
	}
	if ro {
		return ErrReadOnly
	}
	return nil
}
