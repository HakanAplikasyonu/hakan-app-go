package taxpayers

import (
	"context"
	"errors"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	errNoRows    = errors.New("taxpayers: no rows")
	errDuplicate = errors.New("taxpayers: duplicate tax id")
)

// Store is the persistence the service needs.
type Store interface {
	Create(ctx context.Context, p db.CreateTaxpayerParams) (db.Taxpayer, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (db.Taxpayer, error)
	GetByTaxID(ctx context.Context, taxID string) (db.Taxpayer, error)
	List(ctx context.Context, q pgtype.Text, limit, offset int32) ([]db.Taxpayer, int64, error)
	UpdateProfile(ctx context.Context, p db.UpdateTaxpayerProfileParams) (db.Taxpayer, error)
	ForOfficeMember(ctx context.Context, p db.GetTaxpayerForOfficeMemberParams) (db.Taxpayer, error)
}

// Postgres implements Store with sqlc queries.
type Postgres struct{ q db.Querier }

// NewPostgres builds the store.
func NewPostgres(q db.Querier) *Postgres { return &Postgres{q: q} }

var _ Store = (*Postgres)(nil)

func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errNoRows
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_taxpayers_tax_id" {
		return errDuplicate
	}
	return err
}

func (p *Postgres) Create(ctx context.Context, params db.CreateTaxpayerParams) (db.Taxpayer, error) {
	row, err := p.q.CreateTaxpayer(ctx, params)
	return row, mapErr(err)
}

func (p *Postgres) GetByUUID(ctx context.Context, id uuid.UUID) (db.Taxpayer, error) {
	row, err := p.q.GetTaxpayerByUUID(ctx, id)
	return row, mapErr(err)
}

func (p *Postgres) GetByTaxID(ctx context.Context, taxID string) (db.Taxpayer, error) {
	row, err := p.q.GetTaxpayerByTaxID(ctx, taxID)
	return row, mapErr(err)
}

func (p *Postgres) List(ctx context.Context, q pgtype.Text, limit, offset int32) ([]db.Taxpayer, int64, error) {
	rows, err := p.q.ListTaxpayers(ctx, db.ListTaxpayersParams{Q: q, LimitCount: limit, OffsetCount: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := p.q.CountTaxpayers(ctx, q)
	return rows, total, err
}

func (p *Postgres) UpdateProfile(ctx context.Context, params db.UpdateTaxpayerProfileParams) (db.Taxpayer, error) {
	row, err := p.q.UpdateTaxpayerProfile(ctx, params)
	return row, mapErr(err)
}

func (p *Postgres) ForOfficeMember(ctx context.Context, params db.GetTaxpayerForOfficeMemberParams) (db.Taxpayer, error) {
	row, err := p.q.GetTaxpayerForOfficeMember(ctx, params)
	return row, mapErr(err)
}
