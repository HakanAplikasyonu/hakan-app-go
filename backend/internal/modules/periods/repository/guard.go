package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	invoicerepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warningsrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
)

// Producer raises LOCKED_PERIOD_CHANGE (05 §1 naming).
const Producer = "control:period_lock"

// CodeLockedPeriodChange is the warning code of a held change (05 §3.2).
const CodeLockedPeriodChange = "LOCKED_PERIOD_CHANGE"

// EntityInvoice is the change_requests.entity_type of invoices.
const EntityInvoice = "invoice"

// ActiveLock is the lock a taxpayer period is under: the period lock of the
// taxpayer's active engagement (D1: at most one).
type ActiveLock struct {
	OrganizationID int64
	EngagementID   int64
	TaxpayerID     int64
	Period         string
	LockedAt       time.Time
}

// Guard keeps locked periods unchanged on the invoice write path (02 §3.4).
// It implements invoices/repository.PeriodGuard.
type Guard struct{}

// NewGuard builds the guard.
func NewGuard() *Guard { return &Guard{} }

var _ invoicerepo.PeriodGuard = (*Guard)(nil)

// Check returns the lock on the taxpayer's period, nil when the period is
// open. It takes a shared transaction-level advisory lock on (taxpayer,
// period) first, so a concurrent Lock waits until tx ends and a write that
// passed Check cannot land in a period locked meanwhile.
func (Guard) Check(ctx context.Context, tx pgx.Tx, taxpayerID int64, period string) (*ActiveLock, error) {
	q := db.New(tx)
	if err := q.AcquirePeriodWriteLock(ctx, db.AcquirePeriodWriteLockParams{TaxpayerID: taxpayerID, Period: period}); err != nil {
		return nil, fmt.Errorf("periods: write lock: %w", err)
	}
	row, err := q.GetActivePeriodLockForTaxpayer(ctx, db.GetActivePeriodLockForTaxpayerParams{TaxpayerID: taxpayerID, Period: period})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("periods: lock lookup: %w", err)
	}
	return &ActiveLock{OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, TaxpayerID: row.TaxpayerID, Period: row.Period, LockedAt: row.LockedAt.Time}, nil
}

// Hold implements invoices/repository.PeriodGuard: when the invoice's
// period (or, for new content of an existing invoice, its current period)
// is locked, it opens a change request with old / new hash, a diff and the
// new content, and raises LOCKED_PERIOD_CHANGE. The same content arriving
// again reuses the open request.
func (g Guard) Hold(ctx context.Context, tx pgx.Tx, c invoicerepo.PendingChange) (invoicerepo.Hold, error) {
	var periods []string
	if c.Existing != nil {
		periods = append(periods, c.Existing.Period)
	}
	if c.Invoice.Period != "" && (len(periods) == 0 || periods[0] != c.Invoice.Period) {
		periods = append(periods, c.Invoice.Period)
	}
	for _, p := range periods {
		lock, err := g.Check(ctx, tx, c.TaxpayerID, p)
		if err != nil {
			return invoicerepo.Hold{}, err
		}
		if lock == nil {
			continue
		}
		id, err := openChangeRequest(ctx, tx, *lock, c)
		if err != nil {
			return invoicerepo.Hold{}, err
		}
		return invoicerepo.Hold{Held: true, ChangeRequestUUID: id, Period: lock.Period}, nil
	}
	return invoicerepo.Hold{}, nil
}

// Payload is change_requests.new_payload for invoices: the content to write
// when the request is applied.
type Payload struct {
	Invoice      model.Invoice `json:"invoice"`
	AttachmentID int64         `json:"attachment_id,omitempty"`
	MatchRule    string        `json:"match_rule,omitempty"`
}

func openChangeRequest(ctx context.Context, tx pgx.Tx, lock ActiveLock, c invoicerepo.PendingChange) (uuid.UUID, error) {
	q := db.New(tx)
	payload, err := json.Marshal(Payload{Invoice: c.Invoice, AttachmentID: c.AttachmentID, MatchRule: c.MatchRule})
	if err != nil {
		return uuid.Nil, fmt.Errorf("periods: payload: %w", err)
	}
	newHash := c.Invoice.ContentHash
	if newHash == "" { // no source hash: hash the content itself
		sum := sha256.Sum256(payload)
		newHash = hex.EncodeToString(sum[:])
	}
	var entityID *int64
	oldHash := pgtype.Text{}
	if c.Existing != nil {
		id := c.Existing.ID
		entityID = &id
		oldHash = c.Existing.ContentHash
	}
	if found, err := q.FindOpenChangeRequestByHash(ctx, db.FindOpenChangeRequestByHashParams{
		OrganizationID: lock.OrganizationID, EngagementID: lock.EngagementID, EntityType: EntityInvoice,
		EntityID: optInt8(entityID), Period: lock.Period, NewHash: pgtype.Text{String: newHash, Valid: true},
	}); err == nil {
		return found.Uuid, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("periods: find change request: %w", err)
	}
	diff, err := json.Marshal(invoiceDiff(c.Existing, c.Invoice))
	if err != nil {
		return uuid.Nil, fmt.Errorf("periods: diff: %w", err)
	}
	cr, err := q.InsertChangeRequest(ctx, db.InsertChangeRequestParams{
		OrganizationID: lock.OrganizationID, EngagementID: lock.EngagementID, TaxpayerID: lock.TaxpayerID,
		Period: lock.Period, EntityType: EntityInvoice, EntityID: optInt8(entityID),
		OldHash: oldHash, NewHash: pgtype.Text{String: newHash, Valid: true}, Diff: diff, NewPayload: payload,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("periods: insert change request: %w", err)
	}
	subject := engine.Subject{OrganizationID: lock.OrganizationID, EngagementID: lock.EngagementID, TaxpayerID: lock.TaxpayerID}
	if err := refreshWarning(ctx, q, subject, entityID, lock.Period, cr.Uuid); err != nil {
		return uuid.Nil, err
	}
	return cr.Uuid, nil
}

// refreshWarning keeps LOCKED_PERIOD_CHANGE in step with the open change
// requests of one entity: raised while any is open, auto-resolved when none
// is. A held change to an existing invoice is reported on the invoice; a
// held new invoice on the period. latest goes into params so a new request
// reopens a warning the accountant acknowledged for an earlier one.
func refreshWarning(ctx context.Context, q *db.Queries, s engine.Subject, entityID *int64, period string, latest uuid.UUID) error {
	n, err := q.CountOpenChangeRequestsForEntity(ctx, db.CountOpenChangeRequestsForEntityParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, EntityType: EntityInvoice,
		EntityID: optInt8(entityID), Period: period,
	})
	if err != nil {
		return fmt.Errorf("periods: count change requests: %w", err)
	}
	ent := engine.Entity{Type: catalog.EntityInvoice, ID: entityID, Period: period}
	if entityID == nil {
		ent = engine.Entity{Type: catalog.EntityPeriod, Period: period}
	}
	var findings []engine.Finding
	if n > 0 {
		params := map[string]any{"period": period, "open_change_requests": n}
		if latest != uuid.Nil {
			params["change_request_uuid"] = latest.String()
		}
		findings = append(findings, engine.Finding{Code: CodeLockedPeriodChange, Params: params})
	}
	eng := engine.New(warningsrepo.NewWarnings(q), warningsrepo.NewConfig(q))
	if _, err := eng.Apply(ctx, s, Producer, ent, []string{CodeLockedPeriodChange}, findings); err != nil {
		return fmt.Errorf("periods: warning: %w", err)
	}
	return nil
}

func optInt8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
