// Package usecase is the period management service (issue #32): readiness
// ("beyana hazır"), period locks with an audit trail, and the change
// requests opened when data arrives for a locked period
// (02-adaptor-mimarisi.md §3.4).
package usecase

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/doctasks/missingdocs"
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
	ErrTargetLocked     = errors.New("periods: target period is locked")
)

// Change request statuses (000037 chk_change_requests_status).
const (
	StatusOpen     = "open"
	StatusApplied  = "applied"
	StatusRejected = "rejected"
)

// Change request decisions (000198 chk_change_requests_decision). apply is
// the old behaviour and the default: the held content is written to its own
// (locked) period. correction writes the same way but records that the
// closed period is corrected (a correction return may be needed);
// target_period writes the record to the chosen month instead.
const (
	DecisionApply        = "apply"
	DecisionCorrection   = "correction"
	DecisionTargetPeriod = "target_period"
	DecisionReject       = "reject"
)

// ApplyOptions is the office's choice when applying a change request.
type ApplyOptions struct {
	Mode         string `json:"mode"`
	TargetPeriod string `json:"target_period"`
}

// Decision is what Store.Decide records.
type Decision struct {
	Status       string
	Mode         string
	TargetPeriod string
}

// Normalize validates the options; an empty mode is apply.
func (o ApplyOptions) Normalize() (ApplyOptions, error) {
	if o.Mode == "" {
		o.Mode = DecisionApply
	}
	switch o.Mode {
	case DecisionApply, DecisionCorrection:
		if o.TargetPeriod != "" {
			return o, fmt.Errorf("%w: target_period is only for mode target_period", ErrInvalid)
		}
	case DecisionTargetPeriod:
		if !ValidPeriod(o.TargetPeriod) {
			return o, fmt.Errorf("%w: target_period must be YYYY-MM", ErrInvalid)
		}
	default:
		return o, fmt.Errorf("%w: mode must be apply, correction or target_period", ErrInvalid)
	}
	return o, nil
}

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
	Decision      *string         `json:"decision"`
	TargetPeriod  *string         `json:"target_period"`
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
	LedgerType    string
}

// MatrixCounts are the per (engagement, period) counts of the matrix.
type MatrixCounts struct {
	Blockers map[MatrixKey]int64
	// Warnings: every open data warning, whatever its severity (#529).
	Warnings       map[MatrixKey]int64
	ChangeRequests map[MatrixKey]int64
	Locks          map[MatrixKey]time.Time
	Returns        map[MatrixKey][]CellReturn
	DocTasks       map[MatrixKey][2]int64 // open, submitted
	// MissingStatements: bank accounts without a statement for the period.
	MissingStatements map[MatrixKey][]string
	// MissingDocuments: other expected kinds with nothing this month (#780).
	MissingDocuments map[MatrixKey]MissingKinds
	// OpenRequestTitles: titles of open / submitted document requests.
	OpenRequestTitles map[MatrixKey][]string
	// Pending: records still to process before the return (#575): bank
	// movements, VAT expense receipts, Z reports.
	Pending map[MatrixKey]PendingWork
}

// MissingKinds are the expected documents (besides statements) with no
// record in a closed month although one of the three months before had one
// (#780, ay başı belge toplama).
type MissingKinds struct {
	ZReports         bool
	Expenses         bool
	PurchaseInvoices bool
}

// PendingWork counts the period's records not processed yet (#575): bank
// movements neither matched, ignored nor booked; VAT expense receipts and
// Z reports waiting for approval.
type PendingWork struct {
	BankTransactions int64 `json:"bank_transactions"`
	Expenses         int64 `json:"expenses"`
	RetailSales      int64 `json:"retail_sales"`
}

// Total is every pending record.
func (p PendingWork) Total() int64 { return p.BankTransactions + p.Expenses + p.RetailSales }

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
	OpenWarnings       int64      `json:"open_warnings"`
	OpenChangeRequests int64      `json:"open_change_requests"`
	// Returns: the period's tax return records (#575); a kind without a
	// record has not been started.
	Returns []CellReturn `json:"returns"`
	// OpenDocumentTasks: documents still expected from the taxpayer;
	// SubmittedDocuments: sent by the taxpayer, not processed yet.
	OpenDocumentTasks  int64 `json:"open_document_tasks"`
	SubmittedDocuments int64 `json:"submitted_documents"`
	// MissingBankStatements: bank accounts (names) with movements in the
	// three months before but none in this closed month: the statement has
	// not been loaded (#568).
	MissingBankStatements []string `json:"missing_bank_statements"`
	// MissingZReports, MissingExpenses, MissingPurchaseInvoices: the same
	// rule for Z reports, expense receipts and incoming invoices (#780).
	MissingZReports         bool `json:"missing_z_reports"`
	MissingExpenses         bool `json:"missing_expenses"`
	MissingPurchaseInvoices bool `json:"missing_purchase_invoices"`
	// MissingDocuments: how many items MissingItems has.
	MissingDocuments int `json:"missing_documents"`
	// MissingItems: every missing document with its request title and
	// whether it was already requested from the taxpayer.
	MissingItems []missingdocs.Item `json:"missing_items"`
	// Pending: records still to process; a period with any is not ready
	// for the return (#575).
	Pending PendingWork `json:"pending"`
}

// CellReturn is one tax return record of a cell.
type CellReturn struct {
	Kind          string  `json:"kind"`
	Status        string  `json:"status"`
	PayableAmount *string `json:"payable_amount"`
	FiledOn       *string `json:"filed_on"`
	// PaidOn: the accrual was paid that day (#575).
	PaidOn *string `json:"paid_on"`
}

// MatrixRow is one engagement of the matrix.
type MatrixRow struct {
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	TaxpayerUUID   uuid.UUID `json:"taxpayer_uuid"`
	TaxpayerTitle  string    `json:"taxpayer_title"`
	TaxpayerTaxID  string    `json:"taxpayer_tax_id"`
	// LedgerType: defter türü (#780), bilanco / isletme / serbest_meslek.
	LedgerType string `json:"ledger_type"`
	Cells      []Cell `json:"cells"`
}

// Matrix is the office "command center" view (docs/01 §3).
type Matrix struct {
	Periods []string `json:"periods"`
	// Deadlines: filing deadline of each tracked return for every period
	// (#575), the same rule the return screen uses (weekend → Monday).
	Deadlines []Deadline  `json:"deadlines"`
	Items     []MatrixRow `json:"items"`
	Total     int64       `json:"total"`
	Limit     int32       `json:"limit"`
	Offset    int32       `json:"offset"`
}

// Deadline is the filing deadline of one return kind for one period.
type Deadline struct {
	Period        string `json:"period"`
	Kind          string `json:"kind"`
	DueDate       string `json:"due_date"`
	RolledForward bool   `json:"rolled_forward"`
}
