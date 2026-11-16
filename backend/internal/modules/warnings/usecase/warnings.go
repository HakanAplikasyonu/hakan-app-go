package usecase

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/google/uuid"
)

// WarningStore is what the service needs from the repository.
type WarningStore interface {
	List(ctx context.Context, f repository.Filter) ([]repository.Row, int64, error)
	Decide(ctx context.Context, organizationID, engagementID int64, id uuid.UUID, status catalog.Status, userID int64) (bool, error)
	Groups(ctx context.Context, f repository.Filter) ([]repository.Group, error)
	AcknowledgeByCode(ctx context.Context, f repository.Filter) (int64, error)
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
	if err := validateFilter(f); err != nil {
		return nil, 0, err
	}
	return s.store.List(ctx, f)
}

func validateFilter(f repository.Filter) error {
	if f.Period != "" && !periodRe.MatchString(f.Period) {
		return fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	if f.Code != "" {
		if _, ok := catalog.Lookup(f.Code); !ok {
			return fmt.Errorf("%w: unknown code", ErrInvalid)
		}
	}
	if f.Severity != "" && !catalog.Severity(f.Severity).Valid() {
		return fmt.Errorf("%w: severity must be info, warning or blocker", ErrInvalid)
	}
	if f.Status != "" && !catalog.Status(f.Status).Valid() {
		return fmt.Errorf("%w: unknown status", ErrInvalid)
	}
	if f.EntityType != "" && !catalog.ValidEntityType(f.EntityType) {
		return fmt.Errorf("%w: unknown entity_type", ErrInvalid)
	}
	return nil
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

// Groups counts warnings by code (issue #333). Status defaults to open;
// "all" means any status.
func (s *Warnings) Groups(ctx context.Context, f repository.Filter) ([]repository.Group, error) {
	switch f.Status {
	case "":
		f.Status = string(catalog.StatusOpen)
	case "all":
		f.Status = ""
	}
	f.Code, f.EntityType = "", ""
	if err := validateFilter(f); err != nil {
		return nil, err
	}
	return s.store.Groups(ctx, f)
}

// MaxNoteLen bounds the note of a bulk acknowledgement.
const MaxNoteLen = 500

// BulkInput is the body of a bulk acknowledgement.
type BulkInput struct {
	Code   string `json:"code"`
	Period string `json:"period"`
	Note   string `json:"note"`
}

// AcknowledgeBulk acknowledges every open warning of in.Code (in in.Period
// when given) in the scope of f: the bulk form of Decide(acknowledged).
func (s *Warnings) AcknowledgeBulk(ctx context.Context, f repository.Filter, in *BulkInput) (int64, error) {
	in.Code, in.Period, in.Note = strings.TrimSpace(in.Code), strings.TrimSpace(in.Period), strings.TrimSpace(in.Note)
	if in.Code == "" {
		return 0, fmt.Errorf("%w: code is required", ErrInvalid)
	}
	if _, ok := catalog.Lookup(in.Code); !ok {
		return 0, fmt.Errorf("%w: unknown code", ErrInvalid)
	}
	if in.Period != "" && !periodRe.MatchString(in.Period) {
		return 0, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	if utf8.RuneCountInString(in.Note) > MaxNoteLen {
		return 0, fmt.Errorf("%w: note must be at most %d characters", ErrInvalid, MaxNoteLen)
	}
	f.Code, f.Period = in.Code, in.Period
	return s.store.AcknowledgeByCode(ctx, f)
}
