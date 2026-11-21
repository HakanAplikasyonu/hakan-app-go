package matching

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/posting"
)

// Ofis içi virman (#710): money leaving one of the taxpayer's bank accounts
// and arriving at another. The two movements are tied to each other; the
// outgoing one is booked 102.B / 102.A, the incoming one rides on the same
// entry (CreateDrafts books the pair once).
var (
	// ErrInvalidTransfer: the other movement is not a transfer candidate
	// (same account, same direction, other amount or currency, more than
	// two days apart, cancelled or already tied to something).
	ErrInvalidTransfer = errors.New("the movements cannot be matched as a transfer between own accounts")
	// ErrTransferLinked: the movement already has matches (a transfer or
	// invoices); remove them first.
	ErrTransferLinked = errors.New("the movement already has matches; remove them first")
	// ErrTransferSameAccount: both bank accounts book to the same 102
	// account; an entry between them would book the account against itself.
	ErrTransferSameAccount = errors.New("both bank accounts use the same ledger account")
)

// transferWindowDays: a transfer arrives on the same day or up to two days
// apart (weekend, EFT cut-off).
const transferWindowDays = 2

// MatchedTransfer is the other leg of a transfer between own accounts.
type MatchedTransfer struct {
	TransactionUUID   uuid.UUID `json:"transaction_uuid"`
	TransactionDate   string    `json:"transaction_date"`
	DebitCredit       string    `json:"debit_credit"`
	Amount            string    `json:"amount"`
	Description       string    `json:"description"`
	CashBoxUUID       uuid.UUID `json:"cash_box_uuid"`
	CashBoxName       string    `json:"cash_box_name"`
	LedgerAccountCode string    `json:"ledger_account_code"`
	Method            string    `json:"method"`
}

// TransferCandidate is a movement of another own bank account that may be
// the other leg: opposite direction, same amount, within two days.
type TransferCandidate struct {
	TransactionUUID   uuid.UUID `json:"transaction_uuid"`
	TransactionDate   string    `json:"transaction_date"`
	DebitCredit       string    `json:"debit_credit"`
	Amount            string    `json:"amount"`
	Description       string    `json:"description"`
	CashBoxUUID       uuid.UUID `json:"cash_box_uuid"`
	CashBoxName       string    `json:"cash_box_name"`
	LedgerAccountCode string    `json:"ledger_account_code"`
}

func (s *Service) transferMatch(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction) (*db.GetBankTransferMatchRow, error) {
	r, err := q.GetBankTransferMatch(ctx, db.GetBankTransferMatchParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID,
		TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Service) transferCandidates(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction) ([]db.ListBankTransferCandidatesRow, error) {
	d := t.TransactionDate.Time
	return q.ListBankTransferCandidates(ctx, db.ListBankTransferCandidatesParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID,
		BankTransactionID: t.ID, CashBoxID: t.CashBoxID, DebitCredit: t.DebitCredit, Currency: t.Currency, Amount: t.Amount,
		DateFrom: pgtype.Date{Time: d.AddDate(0, 0, -transferWindowDays), Valid: true},
		DateTo:   pgtype.Date{Time: d.AddDate(0, 0, transferWindowDays), Valid: true},
		OnDate:   pgtype.Date{Time: d, Valid: true},
	})
}

// transferView fills the transfer leg, or the candidates while the movement
// is tied to nothing.
func (s *Service) transferView(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction, v *TransactionView) error {
	v.TransferCandidates = []TransferCandidate{}
	m, err := s.transferMatch(ctx, q, sub, t)
	if err != nil {
		return err
	}
	if m != nil {
		v.Transfer = &MatchedTransfer{TransactionUUID: m.Uuid, TransactionDate: m.TransactionDate.Time.Format("2006-01-02"),
			DebitCredit: m.DebitCredit, Amount: dec(m.Amount).StringFixed(2), Description: m.Description,
			CashBoxUUID: m.CashBoxUuid, CashBoxName: m.CashBoxName, LedgerAccountCode: m.LedgerAccountCode, Method: m.Method}
		return nil
	}
	if t.IsCancelled || hasMatches(*v) {
		return nil
	}
	cands, err := s.transferCandidates(ctx, q, sub, t)
	if err != nil {
		return err
	}
	for _, c := range cands {
		v.TransferCandidates = append(v.TransferCandidates, TransferCandidate{TransactionUUID: c.Uuid,
			TransactionDate: c.TransactionDate.Time.Format("2006-01-02"), DebitCredit: c.DebitCredit,
			Amount: dec(c.Amount).StringFixed(2), Description: c.Description,
			CashBoxUUID: c.CashBoxUuid, CashBoxName: c.CashBoxName, LedgerAccountCode: c.LedgerAccountCode})
	}
	return nil
}

// MatchTransfer ties the movement to the other leg of a transfer between
// own accounts: both are matched, the outgoing one is booked to the
// receiving bank's 102 account and the incoming one to the sending bank's.
func (s *Service) MatchTransfer(ctx context.Context, sub Subject, id, other uuid.UUID, userID int64) (TransactionView, error) {
	if s.accounts == nil {
		return TransactionView{}, ErrNoAccounts
	}
	q := database.QueriesFor(s.pool)
	t, err := s.transaction(ctx, q, sub, id)
	if err != nil {
		return TransactionView{}, err
	}
	if t.IsCancelled {
		return TransactionView{}, ErrNotMatchable
	}
	existing, err := q.ListBankTransactionMatches(ctx, db.ListBankTransactionMatchesParams{TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID})
	if err != nil {
		return TransactionView{}, err
	}
	if len(existing) > 0 {
		return TransactionView{}, ErrTransferLinked
	}
	cands, err := s.transferCandidates(ctx, q, sub, t)
	if err != nil {
		return TransactionView{}, err
	}
	var pick *db.ListBankTransferCandidatesRow
	for i := range cands {
		if cands[i].Uuid == other {
			pick = &cands[i]
		}
	}
	if pick == nil {
		return TransactionView{}, fmt.Errorf("%w: the other movement is missing, of the same account or direction, of another amount or more than %d days apart", ErrInvalidTransfer, transferWindowDays)
	}
	own, err := q.GetCashBoxLedger(ctx, db.GetCashBoxLedgerParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID,
		TaxpayerID: sub.TaxpayerID, CashBoxID: t.CashBoxID})
	if err != nil {
		return TransactionView{}, err
	}
	if posting.SameAccount(own.LedgerAccountCode, pick.LedgerAccountCode) {
		return TransactionView{}, ErrTransferSameAccount
	}
	for _, txID := range []int64{t.ID, pick.ID} {
		p, err := q.GetBankPostingView(ctx, db.GetBankPostingViewParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, BankTransactionID: txID})
		if err == nil && p.JournalDraftUuid.Valid {
			return TransactionView{}, posting.ErrDrafted
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return TransactionView{}, err
		}
	}
	evidence, _ := json.Marshal(map[string]any{"rule": "manual_transfer"})
	err = database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tq := db.New(tx)
		for _, pair := range [][2]int64{{t.ID, pick.ID}, {pick.ID, t.ID}} {
			if _, err := tq.InsertBankTransactionMatch(ctx, db.InsertBankTransactionMatchParams{
				TaxpayerID: sub.TaxpayerID, BankTransactionID: pair[0], TargetType: "bank_transaction", TargetID: pair[1],
				Amount: t.Amount, Score: 100, Method: "manual", Evidence: evidence, MatchedBy: pgtype.Int8{Int64: userID, Valid: true},
			}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if _, err := tq.SupersedePendingSuggestions(ctx, db.SupersedePendingSuggestionsParams{
				OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Kind: SuggestionKind,
				TargetType: "bank_transaction", TargetID: pgtype.Int8{Int64: pair[0], Valid: true}, TargetField: "match",
			}); err != nil {
				return err
			}
			if err := tq.SetBankTransactionMatchStatus(ctx, db.SetBankTransactionMatchStatusParams{MatchStatus: "matched", TaxpayerID: sub.TaxpayerID, ID: pair[0]}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return TransactionView{}, err
	}
	// Each leg's counter account is the other bank's 102 account; the entry
	// itself is written once, on the outgoing leg (CreateDrafts).
	psub := posting.Subject{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID}
	if err := s.accounts.SetAccount(ctx, psub, userID, t.Uuid, pick.LedgerAccountCode, pick.CashBoxName); err != nil {
		return TransactionView{}, err
	}
	if err := s.accounts.SetAccount(ctx, psub, userID, pick.Uuid, own.LedgerAccountCode, own.Name); err != nil {
		return TransactionView{}, err
	}
	if pick.Period != t.Period {
		if _, _, err := s.controls(ctx, q, sub, pick.Period); err != nil {
			return TransactionView{}, err
		}
	}
	return s.refreshed(ctx, q, sub, t.Uuid, t.Period)
}

// unmatchTransfer undoes a transfer: both legs are open again and their
// counter accounts (not yet in a journal draft) are dropped. A leg already
// in a draft blocks the undo.
func (s *Service) unmatchTransfer(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction, m *db.GetBankTransferMatchRow) error {
	ids := []int64{t.ID, m.ID}
	for _, txID := range ids {
		p, err := q.GetBankPostingView(ctx, db.GetBankPostingViewParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, BankTransactionID: txID})
		if err == nil && p.JournalDraftUuid.Valid {
			return posting.ErrDrafted
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	err := database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tq := db.New(tx)
		for _, txID := range ids {
			if err := tq.DeleteBankTransactionMatches(ctx, db.DeleteBankTransactionMatchesParams{TaxpayerID: sub.TaxpayerID, BankTransactionID: txID}); err != nil {
				return err
			}
			if err := tq.SetBankTransactionMatchStatus(ctx, db.SetBankTransactionMatchStatusParams{MatchStatus: "unmatched", TaxpayerID: sub.TaxpayerID, ID: txID}); err != nil {
				return err
			}
		}
		return tq.DeleteUndraftedBankPostings(ctx, db.DeleteUndraftedBankPostingsParams{OrganizationID: sub.OrganizationID,
			EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID, BankTransactionIds: ids})
	})
	if err != nil {
		return err
	}
	if m.Period != t.Period {
		if _, _, err := s.controls(ctx, q, sub, m.Period); err != nil {
			return err
		}
	}
	return nil
}
