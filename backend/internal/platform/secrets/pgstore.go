package secrets

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// Queries is the sqlc subset PGStore needs (db.Querier satisfies it).
type Queries interface {
	InsertSecretRecord(ctx context.Context, arg db.InsertSecretRecordParams) error
	GetSecretRecord(ctx context.Context, argUuid uuid.UUID) (db.GetSecretRecordRow, error)
	ReplaceSecretRecord(ctx context.Context, arg db.ReplaceSecretRecordParams) (int64, error)
	RewrapSecretRecord(ctx context.Context, arg db.RewrapSecretRecordParams) (int64, error)
	ListSecretRecordsNotOnKEK(ctx context.Context, arg db.ListSecretRecordsNotOnKEKParams) ([]db.ListSecretRecordsNotOnKEKRow, error)
	CountSecretRecordsByKEK(ctx context.Context) ([]db.CountSecretRecordsByKEKRow, error)
	DeleteSecretRecord(ctx context.Context, argUuid uuid.UUID) (int64, error)
}

// PGStore keeps envelopes in secret_records.
type PGStore struct{ q Queries }

var _ Store = (*PGStore)(nil)

func NewPGStore(q Queries) *PGStore { return &PGStore{q: q} }

func (s *PGStore) Insert(ctx context.Context, e envelope) error {
	return s.q.InsertSecretRecord(ctx, db.InsertSecretRecordParams{
		Uuid: e.ID, Purpose: e.Purpose, Alg: e.Alg, KekID: e.KEKID, WrappedDek: e.WrappedDEK, Ciphertext: e.Ciphertext,
	})
}

func (s *PGStore) Get(ctx context.Context, id uuid.UUID) (envelope, error) {
	r, err := s.q.GetSecretRecord(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return envelope{}, ErrNotFound
	}
	if err != nil {
		return envelope{}, err
	}
	return envelope{ID: r.Uuid, Purpose: r.Purpose, Alg: r.Alg, KEKID: r.KekID, WrappedDEK: r.WrappedDek, Ciphertext: r.Ciphertext}, nil
}

func (s *PGStore) Replace(ctx context.Context, e envelope) (bool, error) {
	n, err := s.q.ReplaceSecretRecord(ctx, db.ReplaceSecretRecordParams{
		Uuid: e.ID, KekID: e.KEKID, WrappedDek: e.WrappedDEK, Ciphertext: e.Ciphertext, Purpose: e.Purpose,
	})
	return n == 1, err
}

func (s *PGStore) Rewrap(ctx context.Context, e envelope, oldKEK string) (bool, error) {
	n, err := s.q.RewrapSecretRecord(ctx, db.RewrapSecretRecordParams{
		NewKekID: e.KEKID, WrappedDek: e.WrappedDEK, Uuid: e.ID, OldKekID: oldKEK,
	})
	return n == 1, err
}

func (s *PGStore) ListNotOnKEK(ctx context.Context, active string, limit int32) ([]envelope, error) {
	rows, err := s.q.ListSecretRecordsNotOnKEK(ctx, db.ListSecretRecordsNotOnKEKParams{ActiveKekID: active, Lim: limit})
	if err != nil {
		return nil, err
	}
	out := make([]envelope, 0, len(rows))
	for _, r := range rows {
		out = append(out, envelope{ID: r.Uuid, Purpose: r.Purpose, Alg: r.Alg, KEKID: r.KekID, WrappedDEK: r.WrappedDek, Ciphertext: r.Ciphertext})
	}
	return out, nil
}

func (s *PGStore) CountByKEK(ctx context.Context) (map[string]int64, error) {
	rows, err := s.q.CountSecretRecordsByKEK(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.KekID] = r.N
	}
	return out, nil
}

func (s *PGStore) Delete(ctx context.Context, id uuid.UUID) (bool, error) {
	n, err := s.q.DeleteSecretRecord(ctx, id)
	return n == 1, err
}
