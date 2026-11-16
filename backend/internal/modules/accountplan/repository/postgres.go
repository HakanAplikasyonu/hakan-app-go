// Package repository persists target account plans in Postgres.
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/usecase"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres implements usecase.Store.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// New builds the store.
func New(pool *pgxpool.Pool, q *db.Queries) *Postgres { return &Postgres{pool: pool, q: q} }

var _ usecase.Store = (*Postgres)(nil)

type accountRow struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	MainCode string `json:"main_code"`
	Level    int    `json:"level"`
	IsLeaf   bool   `json:"is_leaf"`
}

// Replace implements usecase.Store.
func (p *Postgres) Replace(ctx context.Context, s usecase.Subject, imp usecase.NewImport) (usecase.Import, error) {
	rows := make([]accountRow, 0, len(imp.Accounts))
	for _, a := range imp.Accounts {
		rows = append(rows, accountRow(a))
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		return usecase.Import{}, err
	}
	tx, err := database.Begin(ctx, p.pool)
	if err != nil {
		return usecase.Import{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := p.q.WithTx(tx)
	var by pgtype.Int8
	if imp.UserID > 0 {
		by = pgtype.Int8{Int64: imp.UserID, Valid: true}
	}
	row, err := q.InsertAccountPlanImport(ctx, db.InsertAccountPlanImportParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Target: imp.Target,
		SourceFormat: imp.Format, SourceKind: imp.Kind, FileName: imp.FileName, Sha256: imp.SHA256,
		AccountCount: int32(len(imp.Accounts)), SkippedRows: int32(imp.Skipped), //nolint:gosec // bounded by MaxAccounts
		ImportedBy: by,
	})
	if err != nil {
		return usecase.Import{}, fmt.Errorf("insert import: %w", err)
	}
	if _, err := q.DeleteAccountPlanAccounts(ctx, db.DeleteAccountPlanAccountsParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Target: imp.Target,
	}); err != nil {
		return usecase.Import{}, fmt.Errorf("delete accounts: %w", err)
	}
	if _, err := q.InsertAccountPlanAccounts(ctx, db.InsertAccountPlanAccountsParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, ImportID: row.ID,
		Target: imp.Target, Rows: payload,
	}); err != nil {
		return usecase.Import{}, fmt.Errorf("insert accounts: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return usecase.Import{}, fmt.Errorf("commit: %w", err)
	}
	return fromImport(row), nil
}

// Active implements usecase.Store.
func (p *Postgres) Active(ctx context.Context, s usecase.Subject, target string) (usecase.Import, error) {
	row, err := p.q.GetActiveAccountPlanImport(ctx, db.GetActiveAccountPlanImportParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Target: target,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.Import{}, usecase.ErrNotFound
	}
	if err != nil {
		return usecase.Import{}, fmt.Errorf("active import: %w", err)
	}
	return fromImport(row), nil
}

// Imports implements usecase.Store.
func (p *Postgres) Imports(ctx context.Context, s usecase.Subject, limit int32) ([]usecase.Import, error) {
	rows, err := p.q.ListAccountPlanImports(ctx, db.ListAccountPlanImportsParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Lim: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list imports: %w", err)
	}
	out := make([]usecase.Import, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromImport(r))
	}
	return out, nil
}

// Accounts implements usecase.Store.
func (p *Postgres) Accounts(ctx context.Context, s usecase.Subject, target, q string, limit, offset int32) (usecase.Page, error) {
	var qt pgtype.Text
	if q != "" {
		qt = pgtype.Text{String: q, Valid: true}
	}
	rows, err := p.q.ListAccountPlanAccounts(ctx, db.ListAccountPlanAccountsParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Target: target, Q: qt, Lim: limit, Off: offset,
	})
	if err != nil {
		return usecase.Page{}, fmt.Errorf("list accounts: %w", err)
	}
	total, err := p.q.CountAccountPlanAccounts(ctx, db.CountAccountPlanAccountsParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Target: target, Q: qt,
	})
	if err != nil {
		return usecase.Page{}, fmt.Errorf("count accounts: %w", err)
	}
	page := usecase.Page{Items: make([]usecase.Account, 0, len(rows)), Total: total}
	for _, r := range rows {
		page.Items = append(page.Items, usecase.Account{
			Code: r.Code, Name: r.Name, MainCode: r.MainCode, Level: int(r.Level), IsLeaf: r.IsLeaf,
		})
	}
	return page, nil
}

// Find implements usecase.Store.
func (p *Postgres) Find(ctx context.Context, s usecase.Subject, target string, codes []string) ([]usecase.Found, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	rows, err := p.q.FindAccountPlanCodes(ctx, db.FindAccountPlanCodesParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Target: target, Codes: codes,
	})
	if err != nil {
		return nil, fmt.Errorf("find codes: %w", err)
	}
	out := make([]usecase.Found, 0, len(rows))
	for _, r := range rows {
		out = append(out, usecase.Found{Code: r.Code, IsLeaf: r.IsLeaf})
	}
	return out, nil
}

// Codes implements usecase.Store.
func (p *Postgres) Codes(ctx context.Context, s usecase.Subject, target string) ([]string, error) {
	codes, err := p.q.ListAccountPlanCodeSet(ctx, db.ListAccountPlanCodeSetParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Target: target,
	})
	if err != nil {
		return nil, fmt.Errorf("plan codes: %w", err)
	}
	return codes, nil
}

func fromImport(r db.AccountPlanImport) usecase.Import {
	return usecase.Import{
		ID: r.ID, UUID: r.Uuid, Target: r.Target, SourceFormat: r.SourceFormat, SourceKind: r.SourceKind,
		FileName: r.FileName, SHA256: r.Sha256, AccountCount: int(r.AccountCount), SkippedRows: int(r.SkippedRows),
		CreatedAt: r.CreatedAt.Time,
	}
}
