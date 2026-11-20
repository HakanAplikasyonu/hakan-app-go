// Package engagements serves the ofis <-> mükellef hizmet ilişkisi
// (engagement) of tenant-core faz 6a (issue #2): opening and ending
// engagements, and assigning office members to them. RequireEngagement
// (package middleware) uses Service as its engagementctx.Resolver.
package engagements

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Service errors, mapped to HTTP by the handler.
var (
	ErrTaxpayerNotFound   = errors.New("engagements: taxpayer not found")
	ErrNotFound           = errors.New("engagements: engagement not found")
	ErrAlreadyActive      = errors.New("engagements: taxpayer already has an active engagement")
	ErrEnded              = errors.New("engagements: engagement has ended")
	ErrMemberNotFound     = errors.New("engagements: member not found")
	ErrAssignmentNotFound = errors.New("engagements: assignment not found")
	ErrInvalidRequest     = errors.New("engagements: invalid request")
)

// Engagement is the API shape of an engagement.
type Engagement struct {
	UUID         uuid.UUID `json:"uuid"`
	TaxpayerUUID uuid.UUID `json:"taxpayer_uuid"`
	// TaxpayerTitle is the mükellef's unvan / ad soyad, so that office
	// clients (masaüstü ajanı #206) can list engagements by name.
	TaxpayerTitle string     `json:"taxpayer_title"`
	Status        string     `json:"status"`
	StartedAt     time.Time  `json:"started_at"`
	EndedAt       *time.Time `json:"ended_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// Assignment is a member assigned to an engagement.
type Assignment struct {
	UserUUID   uuid.UUID `json:"user_uuid"`
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	Surname    string    `json:"surname"`
	AssignedAt time.Time `json:"assigned_at"`
}

// Service implements the engagement use cases and engagementctx.Resolver.
type Service struct {
	store       Store
	invalidator orgctx.Invalidator
}

var _ engagementctx.Resolver = (*Service)(nil)

// New builds the service. invalidator (the organization cache) may be nil.
func New(store Store, invalidator orgctx.Invalidator) *Service {
	return &Service{store: store, invalidator: invalidator}
}

// Create opens an engagement between the organization and a taxpayer. A
// taxpayer has at most one active engagement across all offices (D1).
//
// NOTE: answering ErrAlreadyActive tells the office that the taxpayer is
// active elsewhere. Issue #7 moves engagement creation behind the
// mükellef's own consent so that no office-facing route can learn it (D1b).
func (s *Service) Create(ctx context.Context, orgID int64, taxpayerUUID uuid.UUID) (Engagement, error) {
	if taxpayerUUID == uuid.Nil {
		return Engagement{}, fmt.Errorf("%w: taxpayer_uuid is required", ErrInvalidRequest)
	}
	tp, err := s.store.GetTaxpayerByUUID(ctx, taxpayerUUID)
	if err != nil {
		if errors.Is(err, errNoRows) {
			return Engagement{}, ErrTaxpayerNotFound
		}
		return Engagement{}, err
	}
	row, err := s.store.CreateEngagement(ctx, orgID, tp.ID)
	if err != nil {
		if errors.Is(err, errActiveExists) {
			return Engagement{}, ErrAlreadyActive
		}
		return Engagement{}, err
	}
	return Engagement{
		UUID: row.Uuid, TaxpayerUUID: tp.Uuid, TaxpayerTitle: tp.Title, Status: row.Status,
		StartedAt: row.StartedAt.Time, EndedAt: tsPtr(row.EndedAt), CreatedAt: row.CreatedAt.Time,
	}, nil
}

// List returns the engagements the caller may see: all of the office with
// tenant.engagements.read_all, otherwise only those assigned to userID.
func (s *Service) List(ctx context.Context, scope orgctx.Scope, userID int64, status string, limit, offset int32) ([]Engagement, int64, error) {
	st := pgtype.Text{}
	switch status {
	case "":
	case engagementctx.StatusActive, engagementctx.StatusEnded:
		st = pgtype.Text{String: status, Valid: true}
	default:
		return nil, 0, fmt.Errorf("%w: status must be active or ended", ErrInvalidRequest)
	}
	if scope.Has(rbac.PermTenantEngagementsReadAll) {
		rows, total, err := s.store.ListEngagements(ctx, scope.ID, st, limit, offset)
		if err != nil {
			return nil, 0, err
		}
		out := make([]Engagement, 0, len(rows))
		for _, r := range rows {
			out = append(out, Engagement{
				UUID: r.Uuid, TaxpayerUUID: r.TaxpayerUuid, TaxpayerTitle: r.TaxpayerTitle, Status: r.Status,
				StartedAt: r.StartedAt.Time, EndedAt: tsPtr(r.EndedAt), CreatedAt: r.CreatedAt.Time,
			})
		}
		return out, total, nil
	}
	rows, total, err := s.store.ListAssignedEngagements(ctx, scope.ID, userID, st, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Engagement, 0, len(rows))
	for _, r := range rows {
		out = append(out, Engagement{
			UUID: r.Uuid, TaxpayerUUID: r.TaxpayerUuid, TaxpayerTitle: r.TaxpayerTitle, Status: r.Status,
			StartedAt: r.StartedAt.Time, EndedAt: tsPtr(r.EndedAt), CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, total, nil
}

// Get returns one engagement of the organization.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (Engagement, error) {
	r, err := s.store.GetEngagement(ctx, orgID, id)
	if err != nil {
		if errors.Is(err, errNoRows) {
			return Engagement{}, ErrNotFound
		}
		return Engagement{}, err
	}
	return Engagement{
		UUID: r.Uuid, TaxpayerUUID: r.TaxpayerUuid, TaxpayerTitle: r.TaxpayerTitle, Status: r.Status,
		StartedAt: r.StartedAt.Time, EndedAt: tsPtr(r.EndedAt), CreatedAt: r.CreatedAt.Time,
	}, nil
}

// End closes an active engagement. Its members lose access at once.
func (s *Service) End(ctx context.Context, orgID int64, id uuid.UUID) (Engagement, error) {
	if _, err := s.store.EndEngagement(ctx, orgID, id); err != nil {
		if errors.Is(err, errNoRows) {
			// Unknown or already ended: tell them apart for the caller,
			// who already passed RequireEngagementAllowEnded.
			if _, gerr := s.Get(ctx, orgID, id); gerr == nil {
				return Engagement{}, ErrEnded
			}
			return Engagement{}, ErrNotFound
		}
		return Engagement{}, err
	}
	if err := orgctx.Invalidate(ctx, s.invalidator, orgID); err != nil {
		return Engagement{}, err
	}
	return s.Get(ctx, orgID, id)
}

// Assign gives an office member access to the engagement.
func (s *Service) Assign(ctx context.Context, orgID, engagementID int64, userUUID uuid.UUID) error {
	userID, err := s.memberID(ctx, orgID, userUUID)
	if err != nil {
		return err
	}
	if err := s.store.AddAssignment(ctx, orgID, engagementID, userID); err != nil {
		return err
	}
	return orgctx.Invalidate(ctx, s.invalidator, orgID)
}

// Unassign removes a member's assignment. Members with
// tenant.engagements.read_all keep access through the permission.
func (s *Service) Unassign(ctx context.Context, orgID, engagementID int64, userUUID uuid.UUID) error {
	userID, err := s.memberID(ctx, orgID, userUUID)
	if err != nil {
		return err
	}
	n, err := s.store.DeleteAssignment(ctx, orgID, engagementID, userID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrAssignmentNotFound
	}
	return orgctx.Invalidate(ctx, s.invalidator, orgID)
}

// EngagementAssignments is one engagement's assignees in the office-wide list.
type EngagementAssignments struct {
	EngagementUUID uuid.UUID    `json:"engagement_uuid"`
	Items          []Assignment `json:"items"`
}

// ListOrgAssignments lists the assignees of every engagement the caller can
// see (all with read_all, otherwise the ones the caller is assigned to), so
// office lists do not ask once per row (#639). Engagements without an
// assignee are left out.
func (s *Service) ListOrgAssignments(ctx context.Context, scope orgctx.Scope, userID int64, status string) ([]EngagementAssignments, error) {
	st := pgtype.Text{}
	switch status {
	case "":
	case engagementctx.StatusActive, engagementctx.StatusEnded:
		st = pgtype.Text{String: status, Valid: true}
	default:
		return nil, fmt.Errorf("%w: status must be active or ended", ErrInvalidRequest)
	}
	rows, err := s.store.ListOrgAssignments(ctx, db.ListOrgEngagementAssignmentsParams{
		OrganizationID: scope.ID, Status: st, ReadAll: scope.Has(rbac.PermTenantEngagementsReadAll), UserID: userID,
	})
	if err != nil {
		return nil, err
	}
	out := []EngagementAssignments{}
	for _, r := range rows {
		if n := len(out); n == 0 || out[n-1].EngagementUUID != r.EngagementUuid {
			out = append(out, EngagementAssignments{EngagementUUID: r.EngagementUuid, Items: []Assignment{}})
		}
		last := &out[len(out)-1]
		last.Items = append(last.Items, Assignment{
			UserUUID: r.UserUuid, Email: r.Email, Name: r.Name, Surname: r.Surname, AssignedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

// ListAssignments lists the members assigned to an engagement.
func (s *Service) ListAssignments(ctx context.Context, orgID, engagementID int64) ([]Assignment, error) {
	rows, err := s.store.ListAssignments(ctx, orgID, engagementID)
	if err != nil {
		return nil, err
	}
	out := make([]Assignment, 0, len(rows))
	for _, r := range rows {
		out = append(out, Assignment{
			UserUUID: r.UserUuid, Email: r.Email, Name: r.Name, Surname: r.Surname, AssignedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

// ResolveEngagement implements engagementctx.Resolver.
func (s *Service) ResolveEngagement(ctx context.Context, orgID int64, id uuid.UUID, userID int64) (engagementctx.Access, error) {
	r, err := s.store.ResolveEngagementAccess(ctx, orgID, id, userID)
	if err != nil {
		if errors.Is(err, errNoRows) {
			return engagementctx.Access{}, engagementctx.ErrNotFound
		}
		return engagementctx.Access{}, err
	}
	return engagementctx.Access{
		ID: r.ID, UUID: r.Uuid, OrganizationID: r.OrganizationID,
		TaxpayerID: r.TaxpayerID, TaxpayerUUID: r.TaxpayerUuid,
		Status: r.Status, StartedAt: r.StartedAt.Time, EndedAt: tsPtr(r.EndedAt),
		ArchiveUntil: tsPtr(r.ArchiveUntil), Assigned: r.Assigned,
	}, nil
}

func (s *Service) memberID(ctx context.Context, orgID int64, userUUID uuid.UUID) (int64, error) {
	id, err := s.store.GetMemberUserID(ctx, orgID, userUUID)
	if err != nil {
		if errors.Is(err, errNoRows) {
			return 0, ErrMemberNotFound
		}
		return 0, err
	}
	return id, nil
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
