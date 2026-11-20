package matching

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/posting"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// ActionExcluded / ActionIncluded: the office took a (duplicate) movement
// out of the account or put it back (#710).
const (
	ActionExcluded = "bank.transaction.excluded"
	ActionIncluded = "bank.transaction.included"
)

// ErrCancelledByBank: the bank cancelled the movement; it is not the
// office's to put back.
var ErrCancelledByBank = errors.New("the bank cancelled this movement")

// Exclusion is the result of taking a movement out or putting it back.
type Exclusion struct {
	UUID     uuid.UUID `json:"uuid"`
	Excluded bool      `json:"excluded"`
}

// SetExcluded takes a movement out of the account (a duplicate the bank
// balance does not carry) or puts an excluded one back (#710). Taking out
// drops its matches; a movement already in a journal draft is refused. A
// movement the bank cancelled cannot be put back. The period's bank
// warnings (balance mismatch among them) are refreshed at once.
func (s *Service) SetExcluded(ctx context.Context, sub Subject, id uuid.UUID, excluded bool, userID int64) (Exclusion, error) {
	q := database.QueriesFor(s.pool)
	t, err := s.transaction(ctx, q, sub, id)
	if err != nil {
		return Exclusion{}, err
	}
	if excluded {
		p, err := q.GetBankPostingView(ctx, db.GetBankPostingViewParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, BankTransactionID: t.ID})
		switch {
		case err == nil && p.JournalDraftUuid.Valid:
			return Exclusion{}, posting.ErrDrafted
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return Exclusion{}, err
		}
	}
	var period string
	err = database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tq := db.New(tx)
		row, err := tq.SetBankTransactionExcluded(ctx, db.SetBankTransactionExcludedParams{
			Excluded: excluded, UserID: pgtype.Int8{Int64: userID, Valid: true}, TaxpayerID: sub.TaxpayerID, Uuid: id,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCancelledByBank
		}
		if err != nil {
			return err
		}
		period = row.Period
		if !excluded {
			return nil
		}
		if err := tq.DeleteBankTransactionMatches(ctx, db.DeleteBankTransactionMatchesParams{TaxpayerID: sub.TaxpayerID, BankTransactionID: row.ID}); err != nil {
			return err
		}
		if _, err := tq.SupersedePendingSuggestions(ctx, db.SupersedePendingSuggestionsParams{
			OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Kind: SuggestionKind,
			TargetType: "bank_transaction", TargetID: pgtype.Int8{Int64: row.ID, Valid: true}, TargetField: "match",
		}); err != nil {
			return err
		}
		return tq.SetBankTransactionMatchStatus(ctx, db.SetBankTransactionMatchStatusParams{MatchStatus: "unmatched", TaxpayerID: sub.TaxpayerID, ID: row.ID})
	})
	if err != nil {
		return Exclusion{}, err
	}
	if _, _, err := s.controls(ctx, q, sub, period); err != nil {
		return Exclusion{}, err
	}
	return Exclusion{UUID: id, Excluded: excluded}, nil
}

type exclusionBody struct {
	Excluded *bool `json:"excluded"`
}

// exclusion: PUT .../bank-transactions/{id}/exclusion {"excluded": true|false} (#710).
func (h *Handler) exclusion(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionUuid"))
	if err != nil {
		response.NotFound(w, r, "bank transaction not found")
		return
	}
	var in exclusionBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || in.Excluded == nil {
		response.BadRequest(w, r, "VALIDATION_ERROR", "excluded (true or false) is required")
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	before, _ := h.svc.Transaction(r.Context(), subjectOf(r), id)
	res, err := h.svc.SetExcluded(r.Context(), subjectOf(r), id, *in.Excluded, p.UserInternal)
	if err == nil {
		action := ActionIncluded
		if res.Excluded {
			action = ActionExcluded
		}
		h.audit(r, action, before, matchedPayload(before))
	}
	h.reply(w, r, res, err)
}
