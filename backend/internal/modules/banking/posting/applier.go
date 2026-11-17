package posting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
)

// Applier writes an accepted "bank_account" suggestion to
// bank_transaction_postings inside the review decision's transaction and,
// when the accountant decided (not the threshold), teaches the alias
// memory. Register it with review Service.Register(SuggestionKind, ...).
type Applier struct{}

var _ reviewusecase.Applier = Applier{}

func applierQueries(tx reviewusecase.Tx) (*db.Queries, error) {
	q, ok := tx.Handle().(*db.Queries)
	if !ok || q == nil {
		return nil, errors.New("bank_account: review transaction has no Postgres handle")
	}
	return q, nil
}

func subjectOfSuggestion(sug reviewusecase.Suggestion) Subject {
	return Subject{OrganizationID: sug.OrganizationID, EngagementID: sug.EngagementID, TaxpayerID: sug.TaxpayerID}
}

// Apply implements reviewusecase.Applier.
func (Applier) Apply(ctx context.Context, tx reviewusecase.Tx, sug reviewusecase.Suggestion, value json.RawMessage) (json.RawMessage, error) {
	q, err := applierQueries(tx)
	if err != nil {
		return nil, err
	}
	var v accountValue
	if err := json.Unmarshal(value, &v); err != nil || !engine.ValidCode(strings.TrimSpace(v.AccountCode)) {
		return nil, fmt.Errorf("%w: value must be {\"account_code\": \"120.01.015\"}", reviewusecase.ErrInvalidValue)
	}
	v.AccountCode = strings.TrimSpace(v.AccountCode)
	if sug.Input.TargetType != TargetType || sug.Input.TargetID == nil {
		return nil, fmt.Errorf("bank_account: unsupported target %s", sug.Input.TargetType)
	}
	sub := subjectOfSuggestion(sug)
	tid := *sug.Input.TargetID
	t, err := q.GetBankTransactionForPosting(ctx, db.GetBankTransactionForPostingParams{TaxpayerID: sub.TaxpayerID, ID: pgtype.Int8{Int64: tid, Valid: true}})
	if err != nil {
		return nil, fmt.Errorf("bank_account: transaction: %w", err)
	}
	var proposed accountValue
	_ = json.Unmarshal(sug.Input.Value, &proposed)
	manual, _ := sug.Input.Provenance.Inputs["manual"].(bool)
	correction := manual || proposed.AccountCode != v.AccountCode

	prev := json.RawMessage("null")
	cur, err := q.GetBankPosting(ctx, db.GetBankPostingParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, BankTransactionID: tid})
	found := err == nil
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if _, err := q.UpsertBankPosting(ctx, db.UpsertBankPostingParams{
			OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID, BankTransactionID: tid,
			AccountCode: v.AccountCode, AccountName: v.AccountName, Confidence: num(sug.Input.Confidence), Source: SourceManual,
			Reason: sug.Input.Rationale,
		}); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	case cur.JournalDraftUuid.Valid:
		return nil, fmt.Errorf("%w: %v", reviewusecase.ErrInvalidValue, ErrDrafted)
	case cur.Status == "approved":
		prev, _ = json.Marshal(accountValue{AccountCode: cur.AccountCode, AccountName: cur.AccountName})
	}
	source := SourceManual
	if !correction && found {
		source = cur.Source
	}
	name := v.AccountName
	if name == "" && !correction {
		name = proposed.AccountName
	}
	if _, err := q.ApproveBankPosting(ctx, db.ApproveBankPostingParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, BankTransactionID: tid,
		AccountCode: v.AccountCode, AccountName: name, Source: source,
		SuggestionID: pgtype.Int8{Int64: sug.ID, Valid: sug.ID > 0},
	}); err != nil {
		return nil, fmt.Errorf("approve posting: %w", err)
	}
	if !isAutomatic(ctx) {
		cp := Counterparty{Name: t.CounterpartyName.String, IBAN: t.CounterpartyIban.String,
			TaxID: t.CounterpartyTaxNumber.String + t.CounterpartyIdentityNumber.String, Direction: banking.Direction(t.DebitCredit)}
		if !KeysOf(cp).empty() {
			if err := Learn(ctx, q, sub, cp, Decision{Correction: correction, AccountCode: v.AccountCode, AccountName: name,
				SuggestionID: sug.ID, At: time.Now()}); err != nil {
				return nil, err
			}
		}
	}
	return prev, nil
}

// Revert implements reviewusecase.Applier: the earlier approval comes back,
// or the movement returns to the review queue; what the decision taught the
// alias memory is taken back.
func (Applier) Revert(ctx context.Context, tx reviewusecase.Tx, sug reviewusecase.Suggestion, previous json.RawMessage) error {
	q, err := applierQueries(tx)
	if err != nil {
		return err
	}
	if sug.Input.TargetID == nil {
		return errors.New("bank_account: suggestion has no target")
	}
	sub := subjectOfSuggestion(sug)
	tid := *sug.Input.TargetID
	cur, err := q.GetBankPosting(ctx, db.GetBankPostingParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, BankTransactionID: tid})
	if err != nil {
		return err
	}
	if cur.JournalDraftUuid.Valid {
		return ErrDrafted
	}
	if err := Unlearn(ctx, q, sub, sug.ID); err != nil {
		return err
	}
	var v accountValue
	if len(previous) > 0 && string(previous) != "null" && json.Unmarshal(previous, &v) == nil && v.AccountCode != "" {
		_, err := q.ApproveBankPosting(ctx, db.ApproveBankPostingParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID,
			BankTransactionID: tid, AccountCode: v.AccountCode, AccountName: v.AccountName, Source: cur.Source, SuggestionID: cur.SuggestionID})
		return err
	}
	var proposed accountValue
	_ = json.Unmarshal(sug.Input.Value, &proposed)
	_, err = q.UnapproveBankPosting(ctx, db.UnapproveBankPostingParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID,
		BankTransactionID: tid, AccountCode: firstNonEmpty(proposed.AccountCode, cur.AccountCode)})
	return err
}
