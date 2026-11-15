package engagements

import (
	"context"
	"errors"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// errNoRows is returned by Store implementations for a missing row.
var errNoRows = errors.New("engagements: no rows")

// errActiveExists is returned by CreateEngagement when the taxpayer already
// has an active engagement (uq_engagements_active_taxpayer).
var errActiveExists = errors.New("engagements: taxpayer already has an active engagement")

// Store is the persistence the service needs.
type Store interface {
	GetTaxpayerByUUID(ctx context.Context, id uuid.UUID) (db.Taxpayer, error)
	CreateEngagement(ctx context.Context, orgID, taxpayerID int64) (db.Engagement, error)
	GetEngagement(ctx context.Context, orgID int64, id uuid.UUID) (db.GetEngagementByUUIDRow, error)
	ListEngagements(ctx context.Context, orgID int64, status pgtype.Text, limit, offset int32) ([]db.ListEngagementsByOrgRow, int64, error)
	ListAssignedEngagements(ctx context.Context, orgID, userID int64, status pgtype.Text, limit, offset int32) ([]db.ListAssignedEngagementsRow, int64, error)
	EndEngagement(ctx context.Context, orgID int64, id uuid.UUID) (db.Engagement, error)
	ResolveEngagementAccess(ctx context.Context, orgID int64, id uuid.UUID, userID int64) (db.ResolveEngagementAccessRow, error)
	GetMemberUserID(ctx context.Context, orgID int64, userUUID uuid.UUID) (int64, error)
	AddAssignment(ctx context.Context, orgID, engagementID, userID int64) error
	DeleteAssignment(ctx context.Context, orgID, engagementID, userID int64) (int64, error)
	ListAssignments(ctx context.Context, orgID, engagementID int64) ([]db.ListEngagementAssignmentsRow, error)
}

// Postgres implements Store with sqlc queries.
type Postgres struct{ q *db.Queries }

// NewPostgres builds the store.
func NewPostgres(q *db.Queries) *Postgres { return &Postgres{q: q} }

var _ Store = (*Postgres)(nil)

func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errNoRows
	}
	return err
}

func (p *Postgres) GetTaxpayerByUUID(ctx context.Context, id uuid.UUID) (db.Taxpayer, error) {
	row, err := p.q.GetTaxpayerByUUID(ctx, id)
	return row, mapErr(err)
}

func (p *Postgres) CreateEngagement(ctx context.Context, orgID, taxpayerID int64) (db.Engagement, error) {
	row, err := p.q.CreateEngagement(ctx, db.CreateEngagementParams{OrganizationID: orgID, TaxpayerID: taxpayerID})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_engagements_active_taxpayer" {
		return row, errActiveExists
	}
	return row, err
}

func (p *Postgres) GetEngagement(ctx context.Context, orgID int64, id uuid.UUID) (db.GetEngagementByUUIDRow, error) {
	row, err := p.q.GetEngagementByUUID(ctx, db.GetEngagementByUUIDParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

func (p *Postgres) ListEngagements(ctx context.Context, orgID int64, status pgtype.Text, limit, offset int32) ([]db.ListEngagementsByOrgRow, int64, error) {
	rows, err := p.q.ListEngagementsByOrg(ctx, db.ListEngagementsByOrgParams{
		OrganizationID: orgID, Status: status, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := p.q.CountEngagementsByOrg(ctx, db.CountEngagementsByOrgParams{OrganizationID: orgID, Status: status})
	return rows, total, err
}

func (p *Postgres) ListAssignedEngagements(ctx context.Context, orgID, userID int64, status pgtype.Text, limit, offset int32) ([]db.ListAssignedEngagementsRow, int64, error) {
	rows, err := p.q.ListAssignedEngagements(ctx, db.ListAssignedEngagementsParams{
		OrganizationID: orgID, UserID: userID, Status: status, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := p.q.CountAssignedEngagements(ctx, db.CountAssignedEngagementsParams{OrganizationID: orgID, UserID: userID, Status: status})
	return rows, total, err
}

func (p *Postgres) EndEngagement(ctx context.Context, orgID int64, id uuid.UUID) (db.Engagement, error) {
	row, err := p.q.EndEngagement(ctx, db.EndEngagementParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

func (p *Postgres) ResolveEngagementAccess(ctx context.Context, orgID int64, id uuid.UUID, userID int64) (db.ResolveEngagementAccessRow, error) {
	row, err := p.q.ResolveEngagementAccess(ctx, db.ResolveEngagementAccessParams{OrganizationID: orgID, Uuid: id, UserID: userID})
	return row, mapErr(err)
}

func (p *Postgres) GetMemberUserID(ctx context.Context, orgID int64, userUUID uuid.UUID) (int64, error) {
	row, err := p.q.GetOrganizationMemberByUserUUID(ctx, db.GetOrganizationMemberByUserUUIDParams{OrganizationID: orgID, Uuid: userUUID})
	return row.UserID, mapErr(err)
}

func (p *Postgres) AddAssignment(ctx context.Context, orgID, engagementID, userID int64) error {
	return p.q.AddEngagementAssignment(ctx, db.AddEngagementAssignmentParams{
		OrganizationID: orgID, EngagementID: engagementID, UserID: userID,
	})
}

func (p *Postgres) DeleteAssignment(ctx context.Context, orgID, engagementID, userID int64) (int64, error) {
	return p.q.DeleteEngagementAssignment(ctx, db.DeleteEngagementAssignmentParams{
		OrganizationID: orgID, EngagementID: engagementID, UserID: userID,
	})
}

func (p *Postgres) ListAssignments(ctx context.Context, orgID, engagementID int64) ([]db.ListEngagementAssignmentsRow, error) {
	return p.q.ListEngagementAssignments(ctx, db.ListEngagementAssignmentsParams{OrganizationID: orgID, EngagementID: engagementID})
}
