// Package repository persists account rules and assignments in Postgres.
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// Postgres implements usecase.Store.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// New builds the store.
func New(pool *pgxpool.Pool, q *db.Queries) *Postgres { return &Postgres{pool: pool, q: q} }

var _ usecase.Store = (*Postgres)(nil)

func engArg(eng int64) pgtype.Int8 {
	if eng <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: eng, Valid: true}
}

func optInt8(v int64) pgtype.Int8 {
	if v <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}

func optText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func toNumeric(d decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

func fromNumeric(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(new(big.Int).Set(n.Int), n.Exp)
}

func fromRule(r db.AccountRule) usecase.Rule {
	var c engine.Conditions
	_ = json.Unmarshal(r.Conditions, &c)
	out := usecase.Rule{
		ID: r.ID, UUID: r.Uuid, Key: r.RuleKey, Version: int(r.Version), Status: r.Status, Scope: "office",
		Role: engine.Role(r.Role), Priority: int(r.Priority), Name: r.Name, Conditions: c,
		AccountCode: r.AccountCode, Confidence: fromNumeric(r.Confidence), Origin: r.Origin, CreatedAt: r.CreatedAt.Time,
	}
	if r.EngagementID.Valid {
		out.Scope = "engagement"
	}
	if r.RetiredAt.Valid {
		t := r.RetiredAt.Time
		out.RetiredAt = &t
	}
	return out
}

func insertParams(org, eng int64, key *uuid.UUID, version int32, in usecase.RuleInput, origin string, userID int64) (db.InsertAccountRuleParams, error) {
	cond, err := json.Marshal(in.Conditions)
	if err != nil {
		return db.InsertAccountRuleParams{}, err
	}
	p := db.InsertAccountRuleParams{
		OrganizationID: org, EngagementID: engArg(eng), Version: version, Role: string(in.Role),
		Priority: int32(in.Priority), Name: in.Name, Conditions: cond, AccountCode: in.AccountCode, //nolint:gosec // validated 1..999
		Confidence: toNumeric(*in.Confidence), Origin: origin, CreatedBy: optInt8(userID),
	}
	if key != nil {
		p.RuleKey = pgtype.UUID{Bytes: *key, Valid: true}
	}
	return p, nil
}

// ListRules implements usecase.Store.
func (p *Postgres) ListRules(ctx context.Context, org, eng int64, includeRetired bool) ([]usecase.Rule, error) {
	rows, err := p.q.ListAccountRules(ctx, db.ListAccountRulesParams{OrganizationID: org, EngagementID: engArg(eng), IncludeRetired: includeRetired})
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	out := make([]usecase.Rule, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRule(r))
	}
	return out, nil
}

// Versions implements usecase.Store.
func (p *Postgres) Versions(ctx context.Context, org, eng int64, key uuid.UUID) ([]usecase.Rule, error) {
	rows, err := p.q.ListAccountRuleVersions(ctx, db.ListAccountRuleVersionsParams{OrganizationID: org, EngagementID: engArg(eng), RuleKey: key})
	if err != nil {
		return nil, fmt.Errorf("rule versions: %w", err)
	}
	out := make([]usecase.Rule, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRule(r))
	}
	return out, nil
}

// CreateRule implements usecase.Store.
func (p *Postgres) CreateRule(ctx context.Context, org, eng int64, in usecase.RuleInput, origin string, userID int64) (usecase.Rule, error) {
	params, err := insertParams(org, eng, nil, 1, in, origin, userID)
	if err != nil {
		return usecase.Rule{}, err
	}
	row, err := p.q.InsertAccountRule(ctx, params)
	if err != nil {
		return usecase.Rule{}, fmt.Errorf("insert rule: %w", err)
	}
	return fromRule(row), nil
}

// NewVersion implements usecase.Store.
func (p *Postgres) NewVersion(ctx context.Context, org, eng int64, key uuid.UUID, in usecase.RuleInput, userID int64) (usecase.Rule, error) {
	tx, err := database.Begin(ctx, p.pool)
	if err != nil {
		return usecase.Rule{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := p.q.WithTx(tx)
	cur, err := q.GetActiveAccountRuleForUpdate(ctx, db.GetActiveAccountRuleForUpdateParams{OrganizationID: org, EngagementID: engArg(eng), RuleKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.Rule{}, usecase.ErrNotFound
	}
	if err != nil {
		return usecase.Rule{}, fmt.Errorf("get rule: %w", err)
	}
	if _, err := q.RetireAccountRule(ctx, db.RetireAccountRuleParams{OrganizationID: org, ID: cur.ID}); err != nil {
		return usecase.Rule{}, fmt.Errorf("retire rule: %w", err)
	}
	params, err := insertParams(org, eng, &key, cur.Version+1, in, cur.Origin, userID)
	if err != nil {
		return usecase.Rule{}, err
	}
	params.CopiedFrom = cur.CopiedFrom
	row, err := q.InsertAccountRule(ctx, params)
	if err != nil {
		return usecase.Rule{}, fmt.Errorf("insert rule version: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return usecase.Rule{}, fmt.Errorf("commit: %w", err)
	}
	return fromRule(row), nil
}

// Retire implements usecase.Store.
func (p *Postgres) Retire(ctx context.Context, org, eng int64, key uuid.UUID) error {
	tx, err := database.Begin(ctx, p.pool)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := p.q.WithTx(tx)
	cur, err := q.GetActiveAccountRuleForUpdate(ctx, db.GetActiveAccountRuleForUpdateParams{OrganizationID: org, EngagementID: engArg(eng), RuleKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("get rule: %w", err)
	}
	if _, err := q.RetireAccountRule(ctx, db.RetireAccountRuleParams{OrganizationID: org, ID: cur.ID}); err != nil {
		return fmt.Errorf("retire rule: %w", err)
	}
	return tx.Commit(ctx)
}

// CopyTemplate implements usecase.Store.
func (p *Postgres) CopyTemplate(ctx context.Context, org, eng int64) (int, error) {
	n, err := p.q.CopyOfficeAccountRules(ctx, db.CopyOfficeAccountRulesParams{OrganizationID: org, EngagementID: eng})
	if err != nil {
		return 0, fmt.Errorf("copy template: %w", err)
	}
	return int(n), nil
}

// Invoice implements usecase.Store.
func (p *Postgres) Invoice(ctx context.Context, taxpayerID int64, id uuid.UUID) (usecase.Invoice, error) {
	row, err := p.q.GetInvoiceForAccounting(ctx, db.GetInvoiceForAccountingParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.Invoice{}, usecase.ErrNotFound
	}
	if err != nil {
		return usecase.Invoice{}, fmt.Errorf("get invoice: %w", err)
	}
	inv := usecase.Invoice{
		ID: row.ID, UUID: row.Uuid, Direction: row.Direction, Type: row.Type, Period: row.Period,
		TaxNumber: row.CustomerTaxNumber.String, IdentityNumber: row.CustomerIdentityNumber.String, CustomerName: row.CustomerName.String,
	}
	if row.CustomerID.Valid {
		c := row.CustomerID.Int64
		inv.CustomerID = &c
	}
	items, err := p.q.ListInvoiceItems(ctx, db.ListInvoiceItemsParams{TaxpayerID: taxpayerID, InvoiceID: row.ID})
	if err != nil {
		return usecase.Invoice{}, fmt.Errorf("list items: %w", err)
	}
	for _, it := range items {
		inv.Lines = append(inv.Lines, usecase.InvoiceLine{ID: it.ID, LineNo: int(it.LineNo), Name: it.Name, LineType: it.LineType, TaxRate: fromNumeric(it.TaxRate)})
	}
	return inv, nil
}

// History implements usecase.Store.
func (p *Postgres) History(ctx context.Context, s usecase.Subject, field, direction, taxNumber, identityNumber string, excludeInvoiceID int64) ([]usecase.HistoryRow, error) {
	rows, err := p.q.CounterpartyAccountHistory(ctx, db.CounterpartyAccountHistoryParams{
		TaxpayerID: s.TaxpayerID, OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Field: field,
		Direction: direction, TaxNumber: optText(taxNumber), IdentityNumber: optText(identityNumber), ExcludeInvoiceID: excludeInvoiceID,
	})
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	out := make([]usecase.HistoryRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, usecase.HistoryRow{AccountCode: r.AccountCode, Uses: r.Uses, LastPeriod: r.LastPeriod})
	}
	return out, nil
}

// PlanUnder implements usecase.Store.
func (p *Postgres) PlanUnder(ctx context.Context, s usecase.Subject, mainCode string) ([]engine.PlanAccount, error) {
	target, err := p.q.ActiveAccountPlanTarget(ctx, db.ActiveAccountPlanTargetParams{OrganizationID: s.OrganizationID, EngagementID: s.EngagementID})
	if err != nil {
		return nil, fmt.Errorf("plan target: %w", err)
	}
	rows, err := p.q.ListAccountPlanCodesUnder(ctx, db.ListAccountPlanCodesUnderParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Target: target, MainCode: mainCode,
	})
	if err != nil {
		return nil, fmt.Errorf("plan accounts: %w", err)
	}
	out := make([]engine.PlanAccount, 0, len(rows))
	for _, r := range rows {
		out = append(out, engine.PlanAccount{Code: r.Code, Name: r.Name, IsLeaf: r.IsLeaf})
	}
	return out, nil
}

// CounterpartyAccount implements usecase.Store.
func (p *Postgres) CounterpartyAccount(ctx context.Context, s usecase.Subject, taxID, mainCode string) (string, string, bool, error) {
	row, err := p.q.GetCounterpartyAccount(ctx, db.GetCounterpartyAccountParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, CounterpartyTaxID: taxID, MainCode: mainCode,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("counterparty account: %w", err)
	}
	return row.AccountCode, row.AccountName, true, nil
}

// Assigned implements usecase.Store: target id → field → code.
func (p *Postgres) Assigned(ctx context.Context, s usecase.Subject, targetType string, ids []int64) (map[int64]map[string]string, error) {
	out := map[int64]map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := p.q.ListAccountAssignmentsForTargets(ctx, db.ListAccountAssignmentsForTargetsParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, TargetType: targetType, TargetIds: ids,
	})
	if err != nil {
		return nil, fmt.Errorf("assignments: %w", err)
	}
	for _, r := range rows {
		if out[r.TargetID] == nil {
			out[r.TargetID] = map[string]string{}
		}
		out[r.TargetID][r.Field] = r.AccountCode
	}
	return out, nil
}

// Candidates implements usecase.Store.
func (p *Postgres) Candidates(ctx context.Context, s usecase.Subject, minCount int64, includeAccepts bool) ([]usecase.Candidate, error) {
	rows, err := p.q.AccountRuleCandidates(ctx, db.AccountRuleCandidatesParams{OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, MinCount: minCount, IncludeAccepts: includeAccepts})
	if err != nil {
		return nil, fmt.Errorf("candidates: %w", err)
	}
	out := make([]usecase.Candidate, 0, len(rows))
	for _, r := range rows {
		out = append(out, usecase.Candidate{
			CounterpartyKey: r.CounterpartyKey, CounterpartyName: r.CounterpartyName, Direction: r.Direction,
			AccountCode: r.AccountCode, Decisions: r.Decisions, LastDecidedAt: r.LastDecidedAt.Time,
		})
	}
	return out, nil
}

// HasMemory implements usecase.Store.
func (p *Postgres) HasMemory(ctx context.Context, s usecase.Subject) (bool, error) {
	ok, err := p.q.EngagementHasAccountMemory(ctx, db.EngagementHasAccountMemoryParams{OrganizationID: s.OrganizationID, EngagementID: s.EngagementID})
	if err != nil {
		return false, fmt.Errorf("account memory: %w", err)
	}
	return ok, nil
}

// RuleSettings implements usecase.Store.
func (p *Postgres) RuleSettings(ctx context.Context, org int64) (usecase.RuleSettings, error) {
	row, err := p.q.GetAccountRuleSettings(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.DefaultRuleSettings(), nil
	}
	if err != nil {
		return usecase.RuleSettings{}, fmt.Errorf("rule settings: %w", err)
	}
	t := row.UpdatedAt.Time
	return usecase.RuleSettings{MinCount: int(row.RuleCandidateMinCount), Decisions: row.RuleCandidateDecisions, UpdatedAt: &t}, nil
}

// SaveRuleSettings implements usecase.Store.
func (p *Postgres) SaveRuleSettings(ctx context.Context, org int64, in usecase.RuleSettings, userID int64) (usecase.RuleSettings, error) {
	row, err := p.q.UpsertAccountRuleSettings(ctx, db.UpsertAccountRuleSettingsParams{
		OrganizationID: org, RuleCandidateMinCount: int32(in.MinCount), RuleCandidateDecisions: in.Decisions, //nolint:gosec // 2..10
		UpdatedBy: pgtype.Int8{Int64: userID, Valid: userID > 0},
	})
	if err != nil {
		return usecase.RuleSettings{}, fmt.Errorf("save rule settings: %w", err)
	}
	t := row.UpdatedAt.Time
	return usecase.RuleSettings{MinCount: int(row.RuleCandidateMinCount), Decisions: row.RuleCandidateDecisions, UpdatedAt: &t}, nil
}

// ActiveEngagements implements usecase.Store.
func (p *Postgres) ActiveEngagements(ctx context.Context, org int64) ([]usecase.Subject, error) {
	rows, err := p.q.ListActiveEngagementsForRules(ctx, org)
	if err != nil {
		return nil, fmt.Errorf("active engagements: %w", err)
	}
	out := make([]usecase.Subject, 0, len(rows))
	for _, r := range rows {
		out = append(out, usecase.Subject{OrganizationID: org, EngagementID: r.ID, TaxpayerID: r.TaxpayerID})
	}
	return out, nil
}

// PreviewLines implements usecase.Store.
func (p *Postgres) PreviewLines(ctx context.Context, org, eng int64, since time.Time, limit int32) ([]usecase.PreviewLine, error) {
	rows, err := p.q.RulePreviewLines(ctx, db.RulePreviewLinesParams{
		OrganizationID: org, EngagementID: pgtype.Int8{Int64: eng, Valid: eng > 0},
		Since: pgtype.Date{Time: since, Valid: true}, Lim: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("rule preview lines: %w", err)
	}
	out := make([]usecase.PreviewLine, 0, len(rows))
	for _, r := range rows {
		out = append(out, usecase.PreviewLine{
			EngagementID: r.EngagementID, InvoiceID: r.InvoiceID, InvoiceUUID: r.InvoiceUuid, InvoiceNumber: r.InvoiceNumber,
			IssueDate: r.IssueDate.Time, Direction: r.Direction, InvoiceType: r.Type, CustomerID: r.CustomerID.Int64,
			TaxNumber: r.CustomerTaxNumber, IdentityNumber: r.CustomerIdentityNumber, CustomerName: r.CustomerName,
			ItemID: r.ItemID, ItemName: r.ItemName, LineType: r.LineType, TaxRate: fromNumeric(r.TaxRate),
			LineCode: r.LineCode.String, VATCode: r.VatCode.String, CustomerCode: r.CustomerCode.String,
		})
	}
	return out, nil
}
