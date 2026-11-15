// Package usecase is the review queue (issue #14, docs/01 §3): every
// suggestion (account code, match, OCR field ...) carries a confidence score,
// a rationale and its provenance (document / rule / model / input). Below
// the office / engagement threshold it waits for the accountant; above it is
// applied automatically and can be reverted. Every decision is logged with
// who, when and why; corrections are flagged as rule candidates.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Errors.
var (
	ErrInvalid     = errors.New("review: invalid input")
	ErrNotFound    = errors.New("review: suggestion not found")
	ErrState       = errors.New("review: action not allowed in the suggestion's status")
	ErrNoApplier   = errors.New("review: no applier for this suggestion kind")
	ErrBadSubject  = errors.New("review: subject needs organization, engagement and taxpayer")
	ErrApplyFailed = errors.New("review: applying the suggestion failed")
)

// Origin is the primary provenance of a suggestion (G12).
type Origin string

const (
	OriginDocument Origin = "document"
	OriginRule     Origin = "rule"
	OriginModel    Origin = "model"
	OriginInput    Origin = "input"
)

// Status of a suggestion.
type Status string

const (
	StatusPending      Status = "pending"
	StatusAutoAccepted Status = "auto_accepted"
	StatusAccepted     Status = "accepted"
	StatusCorrected    Status = "corrected"
	StatusRejected     Status = "rejected"
	StatusReverted     Status = "reverted"
	StatusSuperseded   Status = "superseded"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusAutoAccepted, StatusAccepted, StatusCorrected, StatusRejected, StatusReverted, StatusSuperseded:
		return true
	}
	return false
}

// Applied reports whether the suggestion's value is in effect on the target.
func (s Status) Applied() bool {
	return s == StatusAutoAccepted || s == StatusAccepted || s == StatusCorrected
}

// Action is a logged decision.
type Action string

const (
	ActionAutoAccept Action = "auto_accept"
	ActionAccept     Action = "accept"
	ActionReject     Action = "reject"
	ActionCorrect    Action = "correct"
	ActionRevert     Action = "revert"
	ActionSupersede  Action = "supersede"
)

// Subject is the engagement a suggestion belongs to.
type Subject struct {
	OrganizationID int64
	EngagementID   int64
	TaxpayerID     int64
}

// Provenance says where a suggestion comes from.
type Provenance struct {
	Origin Origin `json:"origin"`
	// SourceEntityType/ID is the source document (required for document).
	SourceEntityType string `json:"source_entity_type,omitempty"`
	SourceEntityID   *int64 `json:"source_entity_id,omitempty"`
	// RuleRef names the rule (required for rule), e.g. "office_rule:42".
	RuleRef string `json:"rule_ref,omitempty"`
	// Model is the model id (required for model); AIUsageEventID links the
	// ai_usage_events row of the call.
	Model          string `json:"model,omitempty"`
	AIUsageEventID *int64 `json:"-"`
	// Inputs are the values the suggestion was derived from.
	Inputs map[string]any `json:"inputs"`
}

// Input is a new suggestion.
type Input struct {
	Kind        string
	TargetType  string
	TargetID    *int64
	TargetField string
	Period      string
	Value       json.RawMessage
	Confidence  decimal.Decimal
	Rationale   string
	Provenance  Provenance
}

// Suggestion is a stored suggestion.
type Suggestion struct {
	ID int64
	Subject
	UUID          uuid.UUID
	Input         Input
	Status        Status
	AppliedValue  json.RawMessage
	PreviousValue json.RawMessage
	RuleCandidate bool
	DecidedBy     *int64
	DecidedAt     *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Decision is one audit-log entry.
type Decision struct {
	Action      Action
	ActorUserID *int64
	ActorUUID   *uuid.UUID
	Reason      string
	Value       json.RawMessage
	Confidence  decimal.Decimal
	Threshold   *decimal.Decimal
	CreatedAt   time.Time
}

// Superseded is a pending suggestion replaced by a newer one.
type Superseded struct {
	ID         int64
	Confidence decimal.Decimal
}

// Tx is a store transaction.
type Tx interface {
	SupersedePending(ctx context.Context, s Subject, in Input) ([]Superseded, error)
	Insert(ctx context.Context, s Subject, in Input) (Suggestion, error)
	GetForUpdate(ctx context.Context, s Subject, id uuid.UUID) (Suggestion, error)
	UpdateDecision(ctx context.Context, sug Suggestion) (Suggestion, error)
	InsertDecision(ctx context.Context, s Subject, suggestionID int64, d Decision) error
	// Handle is the store-specific handle appliers write through (the pgx
	// transaction's *db.Queries for Postgres; nil in memory).
	Handle() any
}

// Applier writes a suggestion's value to its target, inside the decision's
// transaction. Modules that own targets (account codes, matches ...)
// register one per suggestion kind.
type Applier interface {
	// Apply writes value and returns the value it replaced (JSON null if none).
	Apply(ctx context.Context, tx Tx, sug Suggestion, value json.RawMessage) (json.RawMessage, error)
	// Revert writes previous back.
	Revert(ctx context.Context, tx Tx, sug Suggestion, previous json.RawMessage) error
}

// SettingRow is one review_settings row.
type SettingRow struct {
	EngagementID int64 // 0 = office row
	Kind         string
	AutoAccept   *bool
	Threshold    *decimal.Decimal
}

// Filter narrows a suggestion list.
type Filter struct {
	OrganizationID int64
	EngagementID   int64
	AllEngagements bool
	UserID         int64
	Status         string
	Kind           string
	RuleCandidate  *bool
	Limit, Offset  int32
}

// ListRow is a listed suggestion with its engagement's public id.
type ListRow struct {
	Suggestion
	EngagementUUID uuid.UUID
}

// Store persists suggestions.
type Store interface {
	InTx(ctx context.Context, fn func(Tx) error) error
	Get(ctx context.Context, s Subject, id uuid.UUID) (Suggestion, []Decision, error)
	List(ctx context.Context, f Filter) ([]ListRow, int64, error)
	Settings(ctx context.Context, organizationID, engagementID int64) ([]SettingRow, error)
	UpsertSetting(ctx context.Context, organizationID, engagementID int64, row SettingRow, userID int64) error
	DeleteSetting(ctx context.Context, organizationID, engagementID int64, kind string) (bool, error)
}
