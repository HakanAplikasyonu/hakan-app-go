package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/usecase"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/jackc/pgx/v5/pgtype"
)

// Applier writes accepted "account_code" suggestions to account_assignments
// inside the review decision's transaction (review.Tx.Handle is the
// tx-bound *db.Queries). Register it with review Service.Register(usecase.Kind, ...).
type Applier struct{}

var _ reviewusecase.Applier = Applier{}

type codeValue struct {
	AccountCode string `json:"account_code"`
}

var errNoTx = errors.New("accountrules: review transaction has no Postgres handle")

func queries(tx reviewusecase.Tx) (*db.Queries, error) {
	q, ok := tx.Handle().(*db.Queries)
	if !ok || q == nil {
		return nil, errNoTx
	}
	return q, nil
}

func target(sug reviewusecase.Suggestion) (string, int64, string, error) {
	t, f := sug.Input.TargetType, sug.Input.TargetField
	if sug.Input.TargetID == nil || (t != usecase.TargetItem && t != usecase.TargetCustomer) ||
		(f != usecase.FieldAccount && f != usecase.FieldVATAccount) {
		return "", 0, "", fmt.Errorf("%w: unsupported target %s.%s", usecase.ErrInvalid, t, f)
	}
	return t, *sug.Input.TargetID, f, nil
}

// Apply implements reviewusecase.Applier.
func (Applier) Apply(ctx context.Context, tx reviewusecase.Tx, sug reviewusecase.Suggestion, value json.RawMessage) (json.RawMessage, error) {
	q, err := queries(tx)
	if err != nil {
		return nil, err
	}
	var v codeValue
	if err := json.Unmarshal(value, &v); err != nil || !engine.ValidCode(v.AccountCode) {
		return nil, fmt.Errorf("%w: value must be {\"account_code\": \"770.01\"}", usecase.ErrInvalid)
	}
	tt, tid, field, err := target(sug)
	if err != nil {
		return nil, err
	}
	// Kural sürümü yalnızca önerilen kod aynen uygulandığında bağlanır.
	var rule pgtype.Int8
	var proposed codeValue
	_ = json.Unmarshal(sug.Input.Value, &proposed)
	if id, ok := sug.Input.Provenance.Inputs["rule_id"]; ok && proposed.AccountCode == v.AccountCode {
		if f, ok := id.(float64); ok {
			rule = pgtype.Int8{Int64: int64(f), Valid: true}
		} else if n, ok := id.(int64); ok {
			rule = pgtype.Int8{Int64: n, Valid: true}
		}
	}
	prev, err := q.UpsertAccountAssignment(ctx, db.UpsertAccountAssignmentParams{
		OrganizationID: sug.OrganizationID, EngagementID: sug.EngagementID, TaxpayerID: sug.TaxpayerID,
		TargetType: tt, TargetID: tid, Field: field, AccountCode: v.AccountCode, RuleID: rule,
		SuggestionID: pgtype.Int8{Int64: sug.ID, Valid: sug.ID > 0}, AssignedBy: optInt8(derefID(sug.DecidedBy)),
	})
	if err != nil {
		return nil, fmt.Errorf("assign account: %w", err)
	}
	if prev == "" {
		return json.RawMessage("null"), nil
	}
	return json.Marshal(codeValue{AccountCode: prev})
}

// Revert implements reviewusecase.Applier.
func (Applier) Revert(ctx context.Context, tx reviewusecase.Tx, sug reviewusecase.Suggestion, previous json.RawMessage) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	tt, tid, field, err := target(sug)
	if err != nil {
		return err
	}
	var v codeValue
	if len(previous) == 0 || string(previous) == "null" || json.Unmarshal(previous, &v) != nil || v.AccountCode == "" {
		_, err := q.DeleteAccountAssignment(ctx, db.DeleteAccountAssignmentParams{
			OrganizationID: sug.OrganizationID, EngagementID: sug.EngagementID, TargetType: tt, TargetID: tid, Field: field,
		})
		return err
	}
	_, err = q.UpsertAccountAssignment(ctx, db.UpsertAccountAssignmentParams{
		OrganizationID: sug.OrganizationID, EngagementID: sug.EngagementID, TaxpayerID: sug.TaxpayerID,
		TargetType: tt, TargetID: tid, Field: field, AccountCode: v.AccountCode,
	})
	return err
}

func derefID(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
