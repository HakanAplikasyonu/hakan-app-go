package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
)

// Applier writes an accepted reconciliation suggestion to its item: the
// item gets the suggested status and the explanation as its note. Revert
// puts the previous status and note back.
type Applier struct{}

var _ reviewusecase.Applier = Applier{}

func (Applier) queries(tx reviewusecase.Tx) (*db.Queries, error) {
	q, ok := tx.Handle().(*db.Queries)
	if !ok || q == nil {
		return nil, errors.New("reconciliation: review store is not Postgres")
	}
	return q, nil
}

func parseValue(raw json.RawMessage) (ReviewValue, error) {
	var v ReviewValue
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, fmt.Errorf("%w: %v", reviewusecase.ErrInvalidValue, err)
	}
	switch v.Status {
	case StatusExplained, StatusResolved, StatusIgnored, StatusOpen:
	default:
		return v, fmt.Errorf("%w: status must be explained, resolved, ignored or open", reviewusecase.ErrInvalidValue)
	}
	if len([]rune(v.Note)) > maxNote {
		return v, fmt.Errorf("%w: note is too long", reviewusecase.ErrInvalidValue)
	}
	return v, nil
}

func decider(sug reviewusecase.Suggestion) pgtype.Int8 {
	if sug.DecidedBy == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *sug.DecidedBy, Valid: true}
}

// Apply implements reviewusecase.Applier.
func (a Applier) Apply(ctx context.Context, tx reviewusecase.Tx, sug reviewusecase.Suggestion, value json.RawMessage) (json.RawMessage, error) {
	v, err := parseValue(value)
	if err != nil {
		return nil, err
	}
	q, err := a.queries(tx)
	if err != nil {
		return nil, err
	}
	if sug.Input.TargetID == nil {
		return nil, fmt.Errorf("%w: no target", reviewusecase.ErrInvalidValue)
	}
	note := pgtype.Text{String: v.Note, Valid: v.Note != ""}
	switch sug.Input.TargetType {
	case TargetItem:
		it, err := q.GetReconciliationItemByID(ctx, db.GetReconciliationItemByIDParams{
			OrganizationID: sug.OrganizationID, EngagementID: sug.EngagementID, ID: *sug.Input.TargetID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: the reconciliation item is gone", reviewusecase.ErrInvalidValue)
		}
		if err != nil {
			return nil, err
		}
		prev := previousValue(it.Status, it.Note)
		if _, err := q.UpdateReconciliationItemDecision(ctx, db.UpdateReconciliationItemDecisionParams{
			OrganizationID: sug.OrganizationID, EngagementID: sug.EngagementID, ID: it.ID, Status: v.Status, Note: note, DecidedBy: decider(sug),
		}); err != nil {
			return nil, err
		}
		return prev, nil
	case TargetCounterparty:
		if applyCounterparty == nil {
			return nil, fmt.Errorf("%w: counterparty reconciliation is not available", reviewusecase.ErrInvalidValue)
		}
		return applyCounterparty(ctx, q, sug, v.Status, note)
	}
	return nil, fmt.Errorf("%w: unknown target %q", reviewusecase.ErrInvalidValue, sug.Input.TargetType)
}

// applyCounterparty is set by the counterparty step (cari mutabakat).
var applyCounterparty func(ctx context.Context, q *db.Queries, sug reviewusecase.Suggestion, status string, note pgtype.Text) (json.RawMessage, error)

func previousValue(status string, note pgtype.Text) json.RawMessage {
	if status == StatusInReview {
		status = StatusOpen
	}
	b, _ := json.Marshal(ReviewValue{Status: status, Note: note.String})
	return b
}

// Revert implements reviewusecase.Applier.
func (a Applier) Revert(ctx context.Context, tx reviewusecase.Tx, sug reviewusecase.Suggestion, previous json.RawMessage) error {
	v := ReviewValue{Status: StatusOpen}
	if len(previous) > 0 && string(previous) != "null" {
		var err error
		if v, err = parseValue(previous); err != nil {
			return err
		}
	}
	_, err := a.Apply(ctx, tx, reviewusecase.Suggestion{
		ID: sug.ID, Subject: sug.Subject, Input: sug.Input, DecidedBy: sug.DecidedBy,
	}, mustJSON(v))
	return err
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
