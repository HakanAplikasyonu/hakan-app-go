// Package usecase runs the account code rules (issue #25): versioned office
// template and engagement rules, suggestions into the review queue (#14)
// with confidence, rationale and provenance, the applier that records the
// accepted code, and rule candidates learnt from the accountant's decisions.
package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Errors.
var (
	ErrInvalid  = errors.New("accountrules: invalid input")
	ErrNotFound = errors.New("accountrules: not found")
)

// Subject is the engagement suggestions belong to.
type Subject struct {
	OrganizationID int64
	EngagementID   int64
	TaxpayerID     int64
}

// RuleInput is a new rule or a new version of one.
type RuleInput struct {
	Role        engine.Role       `json:"role"`
	Priority    int               `json:"priority"`
	Name        string            `json:"name"`
	Conditions  engine.Conditions `json:"conditions"`
	AccountCode string            `json:"account_code"`
	Confidence  *decimal.Decimal  `json:"confidence,omitempty"`
}

// Rule is one stored rule version.
type Rule struct {
	ID          int64             `json:"-"`
	UUID        uuid.UUID         `json:"uuid"`
	Key         uuid.UUID         `json:"rule_key"`
	Version     int               `json:"version"`
	Status      string            `json:"status"`
	Scope       string            `json:"scope"` // office | engagement
	Role        engine.Role       `json:"role"`
	Priority    int               `json:"priority"`
	Name        string            `json:"name"`
	Conditions  engine.Conditions `json:"conditions"`
	AccountCode string            `json:"account_code"`
	Confidence  decimal.Decimal   `json:"confidence"`
	Origin      string            `json:"origin"`
	CreatedAt   time.Time         `json:"created_at"`
	RetiredAt   *time.Time        `json:"retired_at,omitempty"`
}

// Ref is the provenance name of the rule version.
func (r Rule) Ref() string { return refFor(r.ID, r.Version) }

// InvoiceLine is one invoice item as the engine sees it.
type InvoiceLine struct {
	ID       int64
	LineNo   int
	Name     string
	LineType string
	TaxRate  decimal.Decimal
	// Amount: satırın KDV hariç tutarı; incelemede "hangi kalem" sorusu
	// için (#569).
	Amount decimal.Decimal
}

// Invoice is an invoice to code.
type Invoice struct {
	ID             int64
	UUID           uuid.UUID
	Direction      string
	Type           string
	Period         string
	CustomerID     *int64
	TaxNumber      string
	IdentityNumber string
	CustomerName   string
	// Belge bilgisi incelemede gösterilir (#569).
	Number    string
	IssueDate string
	Total     decimal.Decimal
	Lines     []InvoiceLine
}

// HistoryRow is a code the counterparty's earlier lines were booked to.
type HistoryRow struct {
	AccountCode string
	Uses        int64
	LastPeriod  string
}

// Candidate is a repeated decision that could become a rule.
type Candidate struct {
	CounterpartyKey  string    `json:"counterparty_key"`
	CounterpartyName string    `json:"counterparty_name"`
	Direction        string    `json:"direction"`
	AccountCode      string    `json:"account_code"`
	Decisions        int64     `json:"decisions"`
	LastDecidedAt    time.Time `json:"last_decided_at"`
	Question         string    `json:"question"`
}

// Store persists rules and reads the data the engine needs.
type Store interface {
	ListRules(ctx context.Context, org, eng int64, includeRetired bool) ([]Rule, error)
	Versions(ctx context.Context, org, eng int64, key uuid.UUID) ([]Rule, error)
	CreateRule(ctx context.Context, org, eng int64, in RuleInput, origin string, userID int64) (Rule, error)
	// NewVersion retires the active version of key and stores in as the
	// next version, in one transaction.
	NewVersion(ctx context.Context, org, eng int64, key uuid.UUID, in RuleInput, userID int64) (Rule, error)
	Retire(ctx context.Context, org, eng int64, key uuid.UUID) error
	CopyTemplate(ctx context.Context, org, eng int64) (int, error)
	Invoice(ctx context.Context, taxpayerID int64, id uuid.UUID) (Invoice, error)
	History(ctx context.Context, s Subject, field, direction, taxNumber, identityNumber string, excludeInvoiceID int64) ([]HistoryRow, error)
	// PlanUnder lists the accounts under mainCode in the engagement's active
	// plan of its target program (the latest imported plan's target, #319).
	PlanUnder(ctx context.Context, s Subject, mainCode string) ([]engine.PlanAccount, error)
	// CounterpartyAccount is the cari remembered for a counterparty VKN /
	// TCKN under mainCode (#319); ok false when none.
	CounterpartyAccount(ctx context.Context, s Subject, taxID, mainCode string) (code, name string, ok bool, err error)
	Assigned(ctx context.Context, s Subject, targetType string, ids []int64) (map[int64]map[string]string, error)
	Candidates(ctx context.Context, s Subject, minCount int64, includeAccepts bool) ([]Candidate, error)
	// RuleSettings is the office's rule candidate setting (#333); the
	// default when the office has none.
	RuleSettings(ctx context.Context, org int64) (RuleSettings, error)
	SaveRuleSettings(ctx context.Context, org int64, in RuleSettings, userID int64) (RuleSettings, error)
	// ActiveEngagements lists the office's active engagements (id, taxpayer).
	ActiveEngagements(ctx context.Context, org int64) ([]Subject, error)
	// PreviewLines are the invoice lines of the last months for a rule
	// preview: one engagement, or every active one of the office (eng 0).
	PreviewLines(ctx context.Context, org, eng int64, since time.Time, limit int32) ([]PreviewLine, error)
	// HasMemory reports whether the engagement has an imported plan or an
	// active rule (#319: automatic suggestions run only then).
	HasMemory(ctx context.Context, s Subject) (bool, error)
}
