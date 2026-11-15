package usecase

import (
	"context"
	"fmt"
	"regexp"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/google/uuid"
)

// WarningStore is what the service needs from the repository.
type WarningStore interface {
	List(ctx context.Context, f repository.Filter) ([]repository.Row, int64, error)
	Decide(ctx context.Context, organizationID, engagementID int64, id uuid.UUID, status catalog.Status, userID int64) (bool, error)
}

// Warnings lists data warnings and records accountant decisions.
type Warnings struct{ store WarningStore }

// NewWarnings builds the service.
func NewWarnings(store WarningStore) *Warnings { return &Warnings{store: store} }

// ErrWarningNotFound: no open warning with that uuid in the engagement.
var ErrWarningNotFound = fmt.Errorf("open warning not found")

var periodRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// List validates the filter and returns one page.
func (s *Warnings) List(ctx context.Context, f repository.Filter) ([]repository.Row, int64, error) {
	if f.Period != "" && !periodRe.MatchString(f.Period) {
		return nil, 0, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	if f.Code != "" {
		if _, ok := catalog.Lookup(f.Code); !ok {
			return nil, 0, fmt.Errorf("%w: unknown code", ErrInvalid)
		}
	}
	if f.Severity != "" && !catalog.Severity(f.Severity).Valid() {
		return nil, 0, fmt.Errorf("%w: severity must be info, warning or blocker", ErrInvalid)
	}
	if f.Status != "" && !catalog.Status(f.Status).Valid() {
		return nil, 0, fmt.Errorf("%w: unknown status", ErrInvalid)
	}
	if f.EntityType != "" && !catalog.ValidEntityType(f.EntityType) {
		return nil, 0, fmt.Errorf("%w: unknown entity_type", ErrInvalid)
	}
	return s.store.List(ctx, f)
}

// Decide sets an open warning to acknowledged or resolved.
func (s *Warnings) Decide(ctx context.Context, organizationID, engagementID int64, id uuid.UUID, status catalog.Status, userID int64) error {
	if status != catalog.StatusAcknowledged && status != catalog.StatusResolved {
		return fmt.Errorf("%w: decision must be acknowledged or resolved", ErrInvalid)
	}
	ok, err := s.store.Decide(ctx, organizationID, engagementID, id, status, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrWarningNotFound
	}
	return nil
}
