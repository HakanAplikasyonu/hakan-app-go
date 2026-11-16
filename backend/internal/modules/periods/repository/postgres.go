// Package repository persists period locks and change requests in Postgres
// and holds the period guard of the invoice write path (issue #32).
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	invoicerepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/periods/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
)

// DB is what the store needs from Postgres.
type DB interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Postgres implements usecase.Store.
type Postgres struct {
	pool DB
	q    *db.Queries
}

// New builds the store.
func New(pool DB) *Postgres { return &Postgres{pool: pool, q: database.QueriesFor(pool)} }

var _ usecase.Store = (*Postgres)(nil)

func (p *Postgres) inTx(ctx context.Context, fn func(tx pgx.Tx, q *db.Queries) error) error {
	tx, err := database.Begin(ctx, p.pool)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx, db.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Counts implements usecase.Store.
func (p *Postgres) Counts(ctx context.Context, s usecase.Subject, period string) (usecase.Counts, error) {
	out := usecase.Counts{BlockersByCode: map[string]int64{}}
	rows, err := p.q.CountOpenBlockersByCode(ctx, db.CountOpenBlockersByCodeParams{OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Period: period})
	if err != nil {
		return out, fmt.Errorf("count blockers: %w", err)
	}
	for _, r := range rows {
		out.BlockersByCode[r.Code] = r.N
	}
	if out.OpenChangeRequests, err = p.q.CountOpenChangeRequestsForPeriod(ctx, db.CountOpenChangeRequestsForPeriodParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Period: period,
	}); err != nil {
		return out, fmt.Errorf("count change requests: %w", err)
	}
	lock, err := p.q.GetPeriodLock(ctx, db.GetPeriodLockParams{OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Period: period})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return out, fmt.Errorf("period lock: %w", err)
	default:
		l, err := p.lockView(ctx, lock)
		if err != nil {
			return out, err
		}
		out.Lock = &l
	}
	return out, nil
}

func (p *Postgres) lockView(ctx context.Context, r db.PeriodLock) (usecase.Lock, error) {
	l := usecase.Lock{Period: r.Period, Note: r.Note.String, LockedAt: r.LockedAt.Time}
	if r.LockedBy.Valid {
		u, err := p.q.GetUserByID(ctx, r.LockedBy.Int64)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return l, fmt.Errorf("lock user: %w", err)
		}
		if err == nil {
			id := u.Uuid
			l.LockedBy = &id
		}
	}
	return l, nil
}

// Lock implements usecase.Store. The exclusive advisory lock waits for
// invoice writes to the same taxpayer period that are in flight.
func (p *Postgres) Lock(ctx context.Context, s usecase.Subject, period, note string, userID int64) (usecase.Lock, error) {
	var out usecase.Lock
	err := p.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		if err := q.AcquirePeriodLockExclusive(ctx, db.AcquirePeriodLockExclusiveParams{TaxpayerID: s.TaxpayerID, Period: period}); err != nil {
			return fmt.Errorf("lock: advisory: %w", err)
		}
		row, err := q.InsertPeriodLock(ctx, db.InsertPeriodLockParams{
			OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, TaxpayerID: s.TaxpayerID,
			Period: period, Note: text(note), LockedBy: optUser(userID),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return usecase.ErrAlreadyLocked
		}
		if err != nil {
			return fmt.Errorf("lock: %w", err)
		}
		if _, err := q.InsertPeriodLockEvent(ctx, db.InsertPeriodLockEventParams{
			OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Period: period,
			Action: usecase.ActionLock, Reason: text(note), ActorID: optUser(userID),
		}); err != nil {
			return fmt.Errorf("lock event: %w", err)
		}
		out = usecase.Lock{Period: row.Period, Note: row.Note.String, LockedAt: row.LockedAt.Time}
		return nil
	})
	return out, err
}

// Unlock implements usecase.Store.
func (p *Postgres) Unlock(ctx context.Context, s usecase.Subject, period, reason string, userID int64) error {
	return p.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		if err := q.AcquirePeriodLockExclusive(ctx, db.AcquirePeriodLockExclusiveParams{TaxpayerID: s.TaxpayerID, Period: period}); err != nil {
			return fmt.Errorf("unlock: advisory: %w", err)
		}
		n, err := q.DeletePeriodLock(ctx, db.DeletePeriodLockParams{OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Period: period})
		if err != nil {
			return fmt.Errorf("unlock: %w", err)
		}
		if n == 0 {
			return usecase.ErrNotLocked
		}
		if _, err := q.InsertPeriodLockEvent(ctx, db.InsertPeriodLockEventParams{
			OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Period: period,
			Action: usecase.ActionUnlock, Reason: text(reason), ActorID: optUser(userID),
		}); err != nil {
			return fmt.Errorf("unlock event: %w", err)
		}
		return nil
	})
}

// LockEvents implements usecase.Store.
func (p *Postgres) LockEvents(ctx context.Context, s usecase.Subject, period string) ([]usecase.LockEvent, error) {
	rows, err := p.q.ListPeriodLockEvents(ctx, db.ListPeriodLockEventsParams{OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Period: period})
	if err != nil {
		return nil, fmt.Errorf("lock events: %w", err)
	}
	out := make([]usecase.LockEvent, 0, len(rows))
	for _, r := range rows {
		out = append(out, usecase.LockEvent{
			UUID: r.Uuid, Period: r.Period, Action: r.Action, Reason: r.Reason.String,
			ActorUUID: optUUID(r.ActorUuid), CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

// ListChangeRequests implements usecase.Store.
func (p *Postgres) ListChangeRequests(ctx context.Context, s usecase.Subject, f usecase.ChangeRequestFilter) ([]usecase.ChangeRequest, int64, error) {
	rows, err := p.q.ListChangeRequests(ctx, db.ListChangeRequestsParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID,
		Status: text(f.Status), Period: text(f.Period), Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list change requests: %w", err)
	}
	total, err := p.q.CountChangeRequests(ctx, db.CountChangeRequestsParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Status: text(f.Status), Period: text(f.Period),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count change requests: %w", err)
	}
	out := make([]usecase.ChangeRequest, 0, len(rows))
	for _, r := range rows {
		out = append(out, changeRequestView(db.GetChangeRequestRow{
			ID: r.ID, Uuid: r.Uuid, Period: r.Period, EntityType: r.EntityType, EntityID: r.EntityID,
			InvoiceUuid: r.InvoiceUuid, OldHash: r.OldHash, NewHash: r.NewHash, Diff: r.Diff, Status: r.Status,
			DecidedByUuid: r.DecidedByUuid, DecidedAt: r.DecidedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}))
	}
	return out, total, nil
}

// GetChangeRequest implements usecase.Store. NewPayload is the invoice part
// of the stored payload only (internal ids stay inside).
func (p *Postgres) GetChangeRequest(ctx context.Context, s usecase.Subject, id uuid.UUID) (usecase.ChangeRequest, error) {
	return getChangeRequest(ctx, p.q, s, id)
}

func getChangeRequest(ctx context.Context, q *db.Queries, s usecase.Subject, id uuid.UUID) (usecase.ChangeRequest, error) {
	r, err := q.GetChangeRequest(ctx, db.GetChangeRequestParams{OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.ChangeRequest{}, usecase.ErrNotFound
	}
	if err != nil {
		return usecase.ChangeRequest{}, fmt.Errorf("get change request: %w", err)
	}
	cr := changeRequestView(r)
	var pl struct {
		Invoice json.RawMessage `json:"invoice"`
	}
	if json.Unmarshal(r.NewPayload, &pl) == nil && len(pl.Invoice) > 0 {
		cr.NewPayload = pl.Invoice
	}
	return cr, nil
}

func changeRequestView(r db.GetChangeRequestRow) usecase.ChangeRequest {
	cr := usecase.ChangeRequest{
		UUID: r.Uuid, Period: r.Period, EntityType: r.EntityType, InvoiceUUID: optUUID(r.InvoiceUuid),
		Kind: KindUpdate, Diff: json.RawMessage(r.Diff), Status: r.Status, DecidedByUUID: optUUID(r.DecidedByUuid),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if !r.EntityID.Valid {
		cr.Kind = KindCreate
	}
	if r.OldHash.Valid {
		v := r.OldHash.String
		cr.OldHash = &v
	}
	if r.NewHash.Valid {
		v := r.NewHash.String
		cr.NewHash = &v
	}
	if r.DecidedAt.Valid {
		t := r.DecidedAt.Time
		cr.DecidedAt = &t
	}
	if len(cr.Diff) == 0 {
		cr.Diff = json.RawMessage("{}")
	}
	return cr
}

// Decide implements usecase.Store. Applying writes the held content through
// the invoice upsert path without the guard (the lock stays), in the same
// transaction as the decision and the warning refresh.
func (p *Postgres) Decide(ctx context.Context, s usecase.Subject, id uuid.UUID, status string, userID int64) (usecase.ChangeRequest, error) {
	if status != usecase.StatusApplied && status != usecase.StatusRejected {
		return usecase.ChangeRequest{}, usecase.ErrInvalid
	}
	var out usecase.ChangeRequest
	err := p.inTx(ctx, func(tx pgx.Tx, q *db.Queries) error {
		cr, err := q.LockChangeRequest(ctx, db.LockChangeRequestParams{OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Uuid: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return usecase.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock change request: %w", err)
		}
		if cr.Status != usecase.StatusOpen {
			return usecase.ErrDecided
		}
		if status == usecase.StatusApplied {
			if err := applyInvoice(ctx, tx, q, cr); err != nil {
				return err
			}
		}
		n, err := q.DecideChangeRequest(ctx, db.DecideChangeRequestParams{
			Status: status, DecidedBy: optUser(userID), OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Uuid: id,
		})
		if err != nil {
			return fmt.Errorf("decide change request: %w", err)
		}
		if n != 1 {
			return usecase.ErrDecided
		}
		var entityID *int64
		if cr.EntityID.Valid {
			v := cr.EntityID.Int64
			entityID = &v
		}
		subject := engine.Subject{OrganizationID: cr.OrganizationID, EngagementID: cr.EngagementID, TaxpayerID: cr.TaxpayerID}
		if err := refreshWarning(ctx, q, subject, entityID, cr.Period, uuid.Nil); err != nil {
			return err
		}
		out, err = getChangeRequest(ctx, q, s, id)
		return err
	})
	return out, err
}

func applyInvoice(ctx context.Context, tx pgx.Tx, q *db.Queries, cr db.ChangeRequest) error {
	if cr.EntityType != EntityInvoice {
		return fmt.Errorf("%w: entity type %s", usecase.ErrNotApplicable, cr.EntityType)
	}
	var pl Payload
	if err := json.Unmarshal(cr.NewPayload, &pl); err != nil {
		return fmt.Errorf("%w: %v", usecase.ErrInvalidPayload, err)
	}
	res, err := invoicerepo.SaveInvoiceGuarded(ctx, tx, cr.TaxpayerID, pl.Invoice, pl.AttachmentID, nil)
	if err != nil {
		return fmt.Errorf("apply invoice: %w", err)
	}
	if !res.Written {
		// Another source of equal or higher priority won meanwhile.
		return fmt.Errorf("%w: invoice outcome %s", usecase.ErrNotApplicable, res.Outcome)
	}
	if pl.AttachmentID > 0 {
		if _, err := q.SetAttachmentOwner(ctx, db.SetAttachmentOwnerParams{
			TaxpayerID: cr.TaxpayerID, ID: pl.AttachmentID, OwnerType: text("invoice"),
			OwnerID: pgtype.Int8{Int64: res.InvoiceID, Valid: true},
		}); err != nil {
			return fmt.Errorf("apply invoice attachment: %w", err)
		}
	}
	return nil
}

// MatrixEngagements implements usecase.Store.
func (p *Postgres) MatrixEngagements(ctx context.Context, f usecase.MatrixFilter) ([]usecase.MatrixEngagement, int64, error) {
	rows, err := p.q.ListPeriodMatrixEngagements(ctx, db.ListPeriodMatrixEngagementsParams{
		OrganizationID: f.OrganizationID, AllEngagements: f.AllEngagements, UserID: f.UserID, Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("matrix engagements: %w", err)
	}
	total, err := p.q.CountPeriodMatrixEngagements(ctx, db.CountPeriodMatrixEngagementsParams{
		OrganizationID: f.OrganizationID, AllEngagements: f.AllEngagements, UserID: f.UserID,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("matrix engagements count: %w", err)
	}
	out := make([]usecase.MatrixEngagement, 0, len(rows))
	for _, r := range rows {
		out = append(out, usecase.MatrixEngagement{ID: r.ID, UUID: r.Uuid, TaxpayerUUID: r.TaxpayerUuid, TaxpayerTitle: r.TaxpayerTitle, TaxpayerTaxID: r.TaxpayerTaxID})
	}
	return out, total, nil
}

// MatrixCounts implements usecase.Store.
func (p *Postgres) MatrixCounts(ctx context.Context, organizationID int64, ids []int64, periods []string) (usecase.MatrixCounts, error) {
	out := usecase.MatrixCounts{
		Blockers: map[usecase.MatrixKey]int64{}, ChangeRequests: map[usecase.MatrixKey]int64{}, Locks: map[usecase.MatrixKey]time.Time{},
	}
	bl, err := p.q.PeriodMatrixBlockers(ctx, db.PeriodMatrixBlockersParams{OrganizationID: organizationID, EngagementIds: ids, Periods: periods})
	if err != nil {
		return out, fmt.Errorf("matrix blockers: %w", err)
	}
	for _, r := range bl {
		out.Blockers[usecase.MatrixKey{EngagementID: r.EngagementID, Period: r.Period}] = r.N
	}
	crs, err := p.q.PeriodMatrixChangeRequests(ctx, db.PeriodMatrixChangeRequestsParams{OrganizationID: organizationID, EngagementIds: ids, Periods: periods})
	if err != nil {
		return out, fmt.Errorf("matrix change requests: %w", err)
	}
	for _, r := range crs {
		out.ChangeRequests[usecase.MatrixKey{EngagementID: r.EngagementID, Period: r.Period}] = r.N
	}
	locks, err := p.q.PeriodMatrixLocks(ctx, db.PeriodMatrixLocksParams{OrganizationID: organizationID, EngagementIds: ids, Periods: periods})
	if err != nil {
		return out, fmt.Errorf("matrix locks: %w", err)
	}
	for _, r := range locks {
		out.Locks[usecase.MatrixKey{EngagementID: r.EngagementID, Period: r.Period}] = r.LockedAt.Time
	}
	return out, nil
}

func text(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func optUser(id int64) pgtype.Int8 {
	if id <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: id, Valid: true}
}

func optUUID(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	id := uuid.UUID(u.Bytes)
	return &id
}
