// Package related serves the "İlişkili kayıtlar" view (#469): for one record
// of an engagement (invoice, bank transaction, journal draft, warning,
// expense, Z report, payroll run, Easytrade transaction) the records it is
// linked to: matched payments, the journal it produced, warnings about it,
// its review suggestion, the source document of a journal or a warning.
//
// No table of its own: links are read from existing columns
// (queries/related_records.sql). Each related kind is shown only to users
// who may read that kind (payroll is sensitive, #382).
package related

import (
	"context"
	"errors"
	"fmt"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// SourceKinds are the record kinds whose related records can be listed.
var SourceKinds = []string{
	"invoice", "bank_transaction", "journal_draft", "warning",
	"expense", "retail_sale", "payroll_run", "financial_transaction",
}

// kindPermission is the read permission a kind needs, as a source and as a
// related item ("rule" follows warnings, "account" follows the bank).
var kindPermission = map[string]string{
	"invoice":               rbac.PermTenantInvoicesRead,
	"bank_transaction":      rbac.PermTenantBankRead,
	"journal_draft":         rbac.PermTenantJournalsRead,
	"warning":               rbac.PermTenantWarningsRead,
	"rule":                  rbac.PermTenantWarningsRead,
	"suggestion":            rbac.PermTenantReviewRead,
	"expense":               rbac.PermTenantReceiptsRead,
	"retail_sale":           rbac.PermTenantReceiptsRead,
	"payroll_run":           rbac.PermTenantPayrollRead,
	"financial_transaction": rbac.PermTenantInvoicesRead,
	"account":               rbac.PermTenantBankRead,
}

// IsSourceKind reports whether kind can be asked for.
func IsSourceKind(kind string) bool {
	for _, k := range SourceKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Permission is the read permission kind needs ("" for an unknown kind).
func Permission(kind string) string { return kindPermission[kind] }

// Item is one related record. Empty strings become JSON null.
type Item struct {
	Kind     string  `json:"kind"`
	UUID     *string `json:"uuid"`
	Relation string  `json:"relation"`
	Date     *string `json:"date"`
	Amount   *string `json:"amount"`
	Currency *string `json:"currency"`
	Status   string  `json:"status"`
	Label    string  `json:"label"`
	Detail   *string `json:"detail"`
}

// Result is the response body.
type Result struct {
	Kind  string `json:"kind"`
	UUID  string `json:"uuid"`
	Items []Item `json:"items"`
}

// Subject is the engagement the request is scoped to.
type Subject struct {
	OrganizationID, EngagementID, TaxpayerID int64
}

// Queries is the subset of db.Querier used here.
type Queries interface {
	ResolveRelatedSource(ctx context.Context, arg db.ResolveRelatedSourceParams) (int64, error)
	ListRelatedRecords(ctx context.Context, arg db.ListRelatedRecordsParams) ([]db.ListRelatedRecordsRow, error)
}

// ErrNotFound: the source record is not in this engagement (or not visible).
var ErrNotFound = errors.New("related: source not found")

// Service lists related records.
type Service struct{ q Queries }

// New builds the service.
func New(q Queries) *Service { return &Service{q: q} }

// List returns the records related to (kind, id). can reports whether the
// caller holds a permission; items of kinds the caller may not read are left
// out.
func (s *Service) List(ctx context.Context, sub Subject, kind string, id uuid.UUID, can func(string) bool) (Result, error) {
	srcID, err := s.q.ResolveRelatedSource(ctx, db.ResolveRelatedSourceParams{
		Kind: kind, TaxpayerID: sub.TaxpayerID, Uuid: id, EngagementID: sub.EngagementID,
		OrganizationID: sub.OrganizationID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrNotFound
	}
	if err != nil {
		return Result{}, fmt.Errorf("resolve related source: %w", err)
	}
	rows, err := s.q.ListRelatedRecords(ctx, db.ListRelatedRecordsParams{
		Kind: kind, TaxpayerID: sub.TaxpayerID, SourceID: srcID, EngagementID: sub.EngagementID,
		OrganizationID: sub.OrganizationID,
	})
	if err != nil {
		return Result{}, fmt.Errorf("list related records: %w", err)
	}
	out := Result{Kind: kind, UUID: id.String(), Items: make([]Item, 0, len(rows))}
	for _, r := range rows {
		if p := kindPermission[r.Kind]; p == "" || !can(p) {
			continue
		}
		out.Items = append(out.Items, Item{
			Kind: r.Kind, UUID: opt(r.Uuid), Relation: r.Relation, Date: opt(r.RecDate),
			Amount: opt(money(r.Amount)), Currency: opt(r.Currency), Status: r.Status, Label: r.Label,
			Detail: opt(r.Detail),
		})
	}
	return out, nil
}

// money writes a NUMERIC(18,4) amount with two decimals ("12000.0000" →
// "12000.00"); a non-number is left as it is.
func money(s string) string {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return s
	}
	return d.StringFixed(2)
}

func opt(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
