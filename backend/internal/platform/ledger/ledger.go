// Package ledger holds the defter türü of an engagement (#780): bilanço
// (hesap planı, yevmiye), işletme defteri (gelir-gider usulü) or serbest
// meslek kazanç defteri. Only bilanço engagements get account postings
// (600/391/120 ...); the other two keep an income/expense book instead.
package ledger

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// Ledger types (engagements.ledger_type).
const (
	Balance          = "bilanco"
	Operating        = "isletme"
	SelfEmployedBook = "serbest_meslek"
)

// ErrNotBalanceSheet is returned when a bilanço-only feature (journal
// drafts, account postings) is asked for an engagement that keeps an
// işletme defteri or serbest meslek kazanç defteri.
var ErrNotBalanceSheet = errors.New("ledger: the taxpayer does not keep a balance-sheet ledger")

// Valid reports whether t is a known ledger type.
func Valid(t string) bool {
	switch t {
	case Balance, Operating, SelfEmployedBook:
		return true
	}
	return false
}

// SingleEntry reports whether t keeps an income/expense book (no chart of
// accounts).
func SingleEntry(t string) bool { return t == Operating || t == SelfEmployedBook }

// Querier is the query the lookup needs.
type Querier interface {
	GetEngagementLedgerType(ctx context.Context, arg db.GetEngagementLedgerTypeParams) (string, error)
}

// Of returns the ledger type of the engagement. An unknown engagement reads
// as bilanço: callers already resolved it through RequireEngagement.
func Of(ctx context.Context, q Querier, organizationID, engagementID int64) (string, error) {
	t, err := q.GetEngagementLedgerType(ctx, db.GetEngagementLedgerTypeParams{OrganizationID: organizationID, EngagementID: engagementID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Balance, nil
	}
	if err != nil {
		return "", err
	}
	return t, nil
}

// RequireBalance returns ErrNotBalanceSheet unless the engagement keeps a
// bilanço ledger.
func RequireBalance(ctx context.Context, q Querier, organizationID, engagementID int64) error {
	t, err := Of(ctx, q, organizationID, engagementID)
	if err != nil {
		return err
	}
	if SingleEntry(t) {
		return ErrNotBalanceSheet
	}
	return nil
}
