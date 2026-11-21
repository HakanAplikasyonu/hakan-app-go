package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/doctasks/missingdocs"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/taxreturns"
)

// Store persists locks and change requests; repository.Postgres is the real one.
type Store interface {
	Counts(ctx context.Context, s Subject, period string) (Counts, error)
	// Lock locks the period; ErrAlreadyLocked when it is locked.
	Lock(ctx context.Context, s Subject, period, note string, userID int64) (Lock, error)
	// Unlock removes the lock; ErrNotLocked when there is none.
	Unlock(ctx context.Context, s Subject, period, reason string, userID int64) error
	LockEvents(ctx context.Context, s Subject, period string) ([]LockEvent, error)
	ListChangeRequests(ctx context.Context, s Subject, f ChangeRequestFilter) ([]ChangeRequest, int64, error)
	GetChangeRequest(ctx context.Context, s Subject, id uuid.UUID) (ChangeRequest, error)
	// Decide applies (writes the held content, bypassing the lock) or
	// rejects an open change request, in one transaction.
	Decide(ctx context.Context, s Subject, id uuid.UUID, status string, userID int64) (ChangeRequest, error)
	MatrixEngagements(ctx context.Context, f MatrixFilter) ([]MatrixEngagement, int64, error)
	MatrixCounts(ctx context.Context, organizationID int64, engagementIDs []int64, periods []string) (MatrixCounts, error)
}

// Service is the period management use case.
type Service struct {
	store Store
	now   func() time.Time
}

// New builds the service.
func New(store Store) *Service { return &Service{store: store, now: time.Now} }

// WithClock overrides the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }

// Matrix limits.
const (
	DefaultMatrixPeriods = 6
	MaxMatrixPeriods     = 24
)

func check(s Subject, period string) error {
	if !s.valid() {
		return ErrEngagementScoped
	}
	if !ValidPeriod(period) {
		return fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	return nil
}

// Readiness reports whether the period is ready for the declaration: no
// open blocker and no open change request. The lock is reported but does not
// decide readiness.
func (s *Service) Readiness(ctx context.Context, sub Subject, period string) (Readiness, error) {
	if err := check(sub, period); err != nil {
		return Readiness{}, err
	}
	c, err := s.store.Counts(ctx, sub, period)
	if err != nil {
		return Readiness{}, err
	}
	return readiness(period, c), nil
}

func readiness(period string, c Counts) Readiness {
	r := Readiness{Period: period, Lock: c.Lock, Locked: c.Lock != nil, OpenChangeRequests: c.OpenChangeRequests, Reasons: []Reason{}}
	codes := make([]string, 0, len(c.BlockersByCode))
	for code := range c.BlockersByCode {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		n := c.BlockersByCode[code]
		if n <= 0 {
			continue
		}
		r.OpenBlockers += n
		r.Reasons = append(r.Reasons, Reason{Code: ReasonOpenBlocker, WarningCode: code, Count: n})
	}
	if c.OpenChangeRequests > 0 {
		r.Reasons = append(r.Reasons, Reason{Code: ReasonOpenChangeRequests, Count: c.OpenChangeRequests})
	}
	r.Ready = len(r.Reasons) == 0
	return r
}

// Lock locks the period and returns its readiness.
func (s *Service) Lock(ctx context.Context, sub Subject, period, note string, userID int64) (Readiness, error) {
	if err := check(sub, period); err != nil {
		return Readiness{}, err
	}
	if _, err := s.store.Lock(ctx, sub, period, strings.TrimSpace(note), userID); err != nil {
		return Readiness{}, err
	}
	return s.Readiness(ctx, sub, period)
}

// Unlock removes the lock; reason is required and kept in the audit trail.
func (s *Service) Unlock(ctx context.Context, sub Subject, period, reason string, userID int64) (Readiness, error) {
	if err := check(sub, period); err != nil {
		return Readiness{}, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Readiness{}, ErrReasonRequired
	}
	if len(reason) > 2000 {
		return Readiness{}, fmt.Errorf("%w: reason too long", ErrInvalid)
	}
	if err := s.store.Unlock(ctx, sub, period, reason, userID); err != nil {
		return Readiness{}, err
	}
	return s.Readiness(ctx, sub, period)
}

// LockEvents returns the lock audit trail of the period, newest first.
func (s *Service) LockEvents(ctx context.Context, sub Subject, period string) ([]LockEvent, error) {
	if err := check(sub, period); err != nil {
		return nil, err
	}
	return s.store.LockEvents(ctx, sub, period)
}

// ListChangeRequests lists the engagement's change requests.
func (s *Service) ListChangeRequests(ctx context.Context, sub Subject, f ChangeRequestFilter) ([]ChangeRequest, int64, error) {
	if !sub.valid() {
		return nil, 0, ErrEngagementScoped
	}
	if f.Period != "" && !ValidPeriod(f.Period) {
		return nil, 0, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	switch f.Status {
	case "", StatusOpen, StatusApplied, StatusRejected:
	default:
		return nil, 0, fmt.Errorf("%w: status", ErrInvalid)
	}
	return s.store.ListChangeRequests(ctx, sub, f)
}

// GetChangeRequest returns one change request with its new payload.
func (s *Service) GetChangeRequest(ctx context.Context, sub Subject, id uuid.UUID) (ChangeRequest, error) {
	if !sub.valid() {
		return ChangeRequest{}, ErrEngagementScoped
	}
	return s.store.GetChangeRequest(ctx, sub, id)
}

// Apply writes the held content to the record (the lock stays) and closes
// the request; its LOCKED_PERIOD_CHANGE warning auto-resolves.
func (s *Service) Apply(ctx context.Context, sub Subject, id uuid.UUID, userID int64) (ChangeRequest, error) {
	if !sub.valid() {
		return ChangeRequest{}, ErrEngagementScoped
	}
	return s.store.Decide(ctx, sub, id, StatusApplied, userID)
}

// Reject closes the request without touching the record.
func (s *Service) Reject(ctx context.Context, sub Subject, id uuid.UUID, userID int64) (ChangeRequest, error) {
	if !sub.valid() {
		return ChangeRequest{}, ErrEngagementScoped
	}
	return s.store.Decide(ctx, sub, id, StatusRejected, userID)
}

// Periods returns the n months ending at until (YYYY-MM), oldest first.
func Periods(until string, n int) ([]string, error) {
	if !ValidPeriod(until) {
		return nil, fmt.Errorf("%w: until must be YYYY-MM", ErrInvalid)
	}
	if n < 1 || n > MaxMatrixPeriods {
		return nil, fmt.Errorf("%w: periods must be 1..%d", ErrInvalid, MaxMatrixPeriods)
	}
	t, err := time.Parse("2006-01", until)
	if err != nil {
		return nil, fmt.Errorf("%w: until", ErrInvalid)
	}
	out := make([]string, n)
	for i := range n {
		out[n-1-i] = t.AddDate(0, -i, 0).Format("2006-01")
	}
	return out, nil
}

// istanbul is the office's calendar for "the current month".
var istanbul = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("TRT", 3*60*60)
}()

// CurrentPeriod is this month in Türkiye time.
func (s *Service) CurrentPeriod() string { return s.now().In(istanbul).Format("2006-01") }

// Matrix is the office view: engagements × the last n periods ending at
// until (default: the current month).
func (s *Service) Matrix(ctx context.Context, f MatrixFilter, until string, n int) (Matrix, error) {
	if f.OrganizationID <= 0 {
		return Matrix{}, ErrEngagementScoped
	}
	if until == "" {
		until = s.CurrentPeriod()
	}
	if n == 0 {
		n = DefaultMatrixPeriods
	}
	periods, err := Periods(until, n)
	if err != nil {
		return Matrix{}, err
	}
	engs, total, err := s.store.MatrixEngagements(ctx, f)
	if err != nil {
		return Matrix{}, err
	}
	out := Matrix{Periods: periods, Deadlines: deadlines(periods), Items: make([]MatrixRow, 0, len(engs)), Total: total, Limit: f.Limit, Offset: f.Offset}
	if len(engs) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(engs))
	for _, e := range engs {
		ids = append(ids, e.ID)
	}
	c, err := s.store.MatrixCounts(ctx, f.OrganizationID, ids, periods)
	if err != nil {
		return Matrix{}, err
	}
	for _, e := range engs {
		row := MatrixRow{EngagementUUID: e.UUID, TaxpayerUUID: e.TaxpayerUUID, TaxpayerTitle: e.TaxpayerTitle, TaxpayerTaxID: e.TaxpayerTaxID, LedgerType: e.LedgerType, Cells: make([]Cell, 0, len(periods))}
		for _, p := range periods {
			k := MatrixKey{EngagementID: e.ID, Period: p}
			cell := Cell{Period: p, OpenBlockers: c.Blockers[k], OpenWarnings: c.Warnings[k], OpenChangeRequests: c.ChangeRequests[k],
				Returns: c.Returns[k], OpenDocumentTasks: c.DocTasks[k][0], SubmittedDocuments: c.DocTasks[k][1],
				MissingBankStatements: c.MissingStatements[k], Pending: c.Pending[k]}
			if cell.Returns == nil {
				cell.Returns = []CellReturn{}
			}
			md := c.MissingDocuments[k]
			cell.MissingZReports, cell.MissingExpenses, cell.MissingPurchaseInvoices = md.ZReports, md.Expenses, md.PurchaseInvoices
			cell.MissingItems = missingdocs.Build(p, cell.MissingBankStatements, md.ZReports, md.Expenses, md.PurchaseInvoices, c.OpenRequestTitles[k])
			cell.MissingDocuments = len(cell.MissingItems)
			if cell.MissingBankStatements == nil {
				cell.MissingBankStatements = []string{}
			}
			if at, ok := c.Locks[k]; ok {
				cell.Locked, cell.LockedAt = true, &at
			}
			// Unprocessed records keep the period from being ready (#575).
			cell.Ready = cell.OpenBlockers == 0 && cell.OpenChangeRequests == 0 && cell.Pending.Total() == 0
			row.Cells = append(row.Cells, cell)
		}
		out.Items = append(out.Items, row)
	}
	return out, nil
}

// deadlines lists the filing deadlines of the tracked returns per period.
func deadlines(periods []string) []Deadline {
	out := make([]Deadline, 0, len(periods)*len(taxreturns.Kinds))
	for _, p := range periods {
		for _, k := range taxreturns.Kinds {
			due, rolled, err := taxreturns.DueDate(k, p)
			if err != nil {
				continue
			}
			out = append(out, Deadline{Period: p, Kind: k, DueDate: due.Format(time.DateOnly), RolledForward: rolled})
		}
	}
	return out
}
