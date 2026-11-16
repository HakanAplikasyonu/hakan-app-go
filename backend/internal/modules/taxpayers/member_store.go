package taxpayers

import (
	"context"
	"errors"
	"fmt"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errDuplicateMember = errors.New("taxpayers: duplicate member")

// MemberPostgres implements MemberStore.
type MemberPostgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewMemberPostgres builds the store.
func NewMemberPostgres(pool *pgxpool.Pool) *MemberPostgres {
	return &MemberPostgres{pool: pool, q: database.QueriesFor(pool)}
}

var _ MemberStore = (*MemberPostgres)(nil)

func (p *MemberPostgres) ResolveMembership(ctx context.Context, taxpayerUUID uuid.UUID, userID int64) (db.ResolveTaxpayerMembershipRow, error) {
	row, err := p.q.ResolveTaxpayerMembership(ctx, db.ResolveTaxpayerMembershipParams{TaxpayerUuid: taxpayerUUID, UserID: userID})
	return row, mapErr(err)
}

func (p *MemberPostgres) ListMembershipsByUser(ctx context.Context, userID int64) ([]db.ListTaxpayerMembershipsByUserRow, error) {
	return p.q.ListTaxpayerMembershipsByUser(ctx, userID)
}

func (p *MemberPostgres) GetMember(ctx context.Context, taxpayerID, userID int64) (db.TaxpayerMember, error) {
	row, err := p.q.GetTaxpayerMember(ctx, db.GetTaxpayerMemberParams{TaxpayerID: taxpayerID, UserID: userID})
	return row, mapErr(err)
}

func (p *MemberPostgres) CreateInvitation(ctx context.Context, params db.CreateTaxpayerInvitationParams) (db.TaxpayerInvitation, error) {
	tx, err := database.Begin(ctx, p.pool)
	if err != nil {
		return db.TaxpayerInvitation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := p.q.WithTx(tx)
	if err := q.RevokePendingTaxpayerInvitationsForEmail(ctx, db.RevokePendingTaxpayerInvitationsForEmailParams{
		OrganizationID: params.OrganizationID, EngagementID: params.EngagementID, Email: params.Email,
	}); err != nil {
		return db.TaxpayerInvitation{}, err
	}
	row, err := q.CreateTaxpayerInvitation(ctx, params)
	if err != nil {
		return row, err
	}
	return row, tx.Commit(ctx)
}

func (p *MemberPostgres) ListInvitations(ctx context.Context, orgID, engagementID int64) ([]db.ListTaxpayerInvitationsByEngagementRow, error) {
	return p.q.ListTaxpayerInvitationsByEngagement(ctx, db.ListTaxpayerInvitationsByEngagementParams{
		OrganizationID: orgID, EngagementID: engagementID,
	})
}

func (p *MemberPostgres) RevokeInvitation(ctx context.Context, orgID, engagementID int64, id uuid.UUID) (int64, error) {
	return p.q.RevokeTaxpayerInvitation(ctx, db.RevokeTaxpayerInvitationParams{
		OrganizationID: orgID, EngagementID: engagementID, Uuid: id,
	})
}

func (p *MemberPostgres) GetInvitationByTokenHash(ctx context.Context, hash string) (db.GetTaxpayerInvitationByTokenHashRow, error) {
	row, err := p.q.GetTaxpayerInvitationByTokenHash(ctx, hash)
	return row, mapErr(err)
}

func (p *MemberPostgres) AcceptInvitation(ctx context.Context, invitationID, taxpayerID, userID int64, role string) error {
	tx, err := database.Begin(ctx, p.pool)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := p.q.WithTx(tx)
	if _, err := q.AcceptTaxpayerInvitation(ctx, invitationID); err != nil {
		return mapErr(err)
	}
	if err := q.AddTaxpayerMember(ctx, db.AddTaxpayerMemberParams{TaxpayerID: taxpayerID, UserID: userID, Role: role}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return errDuplicateMember
		}
		return fmt.Errorf("add taxpayer member: %w", err)
	}
	return tx.Commit(ctx)
}

func (p *MemberPostgres) GetUser(ctx context.Context, id int64) (db.User, error) {
	row, err := p.q.GetUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, errNoRows
	}
	return row, err
}

func (p *MemberPostgres) GetUserByEmail(ctx context.Context, email string) (db.User, error) {
	row, err := p.q.GetUserByEmail(ctx, email)
	return row, mapErr(err)
}
