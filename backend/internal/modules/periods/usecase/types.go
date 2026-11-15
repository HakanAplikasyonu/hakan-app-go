// Package usecase is the period management service (issue #32): readiness
// ("beyana hazır"), period locks with an audit trail, and the change
// requests opened when data arrives for a locked period
// (02-adaptor-mimarisi.md §3.4).
package usecase

import (
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"
)

// Errors.
var (
	ErrInvalid          = errors.New("periods: invalid input")
	ErrAlreadyLocked    = errors.New("periods: period already locked")
	ErrNotLocked        = errors.New("periods: period not locked")
	ErrNotFound         = errors.New("periods: change request not found")
	ErrDecided          = errors.New("periods: change request already decided")
	ErrNotApplicable    = errors.New("periods: change request no longer applies")
	ErrReasonRequired   = errors.New("periods: unlock needs a reason")
	ErrInvalidPayload   = errors.New("periods: change request payload unreadable")
	ErrEngagementScoped = errors.New("periods: subject needs organization, engagement and taxpayer")
)

// Change request statuses (000037 chk_change_requests_status).
const (
	StatusOpen     = "open"
	StatusApplied  = "applied"
	StatusRejected = "rejected"
)

// Lock event actions.
const (
	ActionLock   = "lock"
	ActionUnlock = "unlock"
)

// Readiness reason codes.
const (
	ReasonOpenBlocker        = "OPEN_BLOCKER"
	ReasonOpenChangeRequests = "OPEN_CHANGE_REQUESTS"
)

// Subject is one engagement (office × taxpayer).
type Subject struct {
	OrganizationID int64
	EngagementID   int64
	TaxpayerID     int64
}

func (s Subject) valid() bool {
	return s.OrganizationID > 0 && s.EngagementID > 0 && s.TaxpayerID > 0
}

var periodRe = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

// ValidPeriod reports whether p is YYYY-MM.
func ValidPeriod(p string) bool { return periodRe.MatchString(p) }

// Lock is a period lock.
type Lock struct {
	Period   string     `json:"period"`
	Note     string     `json:"note,omitempty"`
	LockedBy *uuid.UUID `json:"locked_by_uuid,omitempty"`
	LockedAt time.Time  `json:"locked_at"`
}

// LockEvent is one entry of the lock audit trail.
type LockEvent struct {
	UUID      uuid.UUID  `json:"uuid"`
	Period    string     `json:"period"`
	Action    string     `json:"action"`
	Reason    string     `json:"reason,omitempty"`
	ActorUUID *uuid.UUID `json:"actor_uuid"`
	CreatedAt time.Time  `json:"created_at"`
}

// Counts is what the store reports for one engagement period.
type Counts struct {
	// BlockersByCode: open data_warnings with severity blocker, by code.
	BlockersByCode     map[string]int64
	OpenChangeRequests int64
	Lock               *Lock
}

// Reason is one thing that keeps a period from being ready.
type Reason struct {
	Code        string `json:"code"`
	WarningCode string `json:"warning_code,omitempty"`
	Count       int64  `json:"count"`
}

// Readiness is the state of one engagement period.
type Readiness struct {
	Period             string   `json:"period"`
	Ready              bool     `json:"ready"`
	Locked             bool     `json:"locked"`
	Lock               *Lock    `json:"lock"`
	OpenBlockers       int64    `json:"open_blockers"`
	OpenChangeRequests int64    `json:"open_change_requests"`
	Reasons            []Reason `json:"reasons"`
}

// ChangeRequest is a change held back by a period lock.
type ChangeRequest struct {
	UUID          uuid.UUID       `json:"uuid"`
	Period        string          `json:"period"`
	EntityType    string          `json:"entity_type"`
	InvoiceUUID   *uuid.UUID      `json:"invoice_uuid"`
	Kind          string          `json:"kind"` // create | update
	OldHash       *string         `json:"old_hash"`
	NewHash       *string         `json:"new_hash"`
	Diff          json.RawMessage `json:"diff"`
	NewPayload    json.RawMessage `json:"new_payload,omitempty"`
	Status        string          `json:"status"`
	DecidedByUUID *uuid.UUID      `json:"decided_by_uuid"`
	DecidedAt     *time.Time      `json:"decided_at"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// ChangeRequestFilter narrows a change request list.
type ChangeRequestFilter struct {
	Status, Period string
	Limit, Offset  int32
}

// MatrixFilter selects the engagements of the office matrix.
type MatrixFilter struct {
	OrganizationID int64
	AllEngagements bool
	UserID         int64
	Limit, Offset  int32
}

// MatrixEngagement is one row of the office matrix before cells are filled.
type MatrixEngagement struct {
	ID            int64
	UUID          uuid.UUID
	TaxpayerUUID  uuid.UUID
	TaxpayerTitle string
	TaxpayerTaxID string
}

// MatrixCounts are the per (engagement, period) counts of the matrix.
type MatrixCounts struct {
	Blockers       map[MatrixKey]int64
	ChangeRequests map[MatrixKey]int64
	Locks          map[MatrixKey]time.Time
}

// MatrixKey addresses one cell.
type MatrixKey struct {
	EngagementID int64
	Period       string
}

// Cell is one engagement × period.
type Cell struct {
	Period             string     `json:"period"`
	Ready              bool       `json:"ready"`
	Locked             bool       `json:"locked"`
	LockedAt           *time.Time `json:"locked_at"`
	OpenBlockers       int64      `json:"open_blockers"`
	OpenChangeRequests int64      `json:"open_change_requests"`
}

// MatrixRow is one engagement of the matrix.
type MatrixRow struct {
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	TaxpayerUUID   uuid.UUID `json:"taxpayer_uuid"`
	TaxpayerTitle  string    `json:"taxpayer_title"`
	TaxpayerTaxID  string    `json:"taxpayer_tax_id"`
	Cells          []Cell    `json:"cells"`
}

// Matrix is the office "command center" view (docs/01 §3).
type Matrix struct {
	Periods []string    `json:"periods"`
	Items   []MatrixRow `json:"items"`
	Total   int64       `json:"total"`
	Limit   int32       `json:"limit"`
	Offset  int32       `json:"offset"`
}
