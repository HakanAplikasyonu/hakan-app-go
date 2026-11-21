package posting

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// ErrNoPosting: the commission needs the movement's counter account first.
var ErrNoPosting = errors.New("posting: pick the movement's account first")

// commissionCodeRe: the commission is an expense, 653 (komisyon giderleri)
// or 780 (finansman giderleri), or one of their sub accounts.
var commissionCodeRe = regexp.MustCompile(`^(653|780)(\.[0-9A-Za-z]+)*$`)

// CommissionInput is the POS commission the bank kept from a collection
// (#565). A nil Amount removes it.
type CommissionInput struct {
	Amount      *decimal.Decimal `json:"amount"`
	AccountCode string           `json:"account_code"`
}

// SetCommission nets a POS collection: the bank paid in the net, the
// commission it kept is booked to 653 / 780 and the counter account takes
// the gross (net + commission). Only an incoming TRY movement whose account
// is chosen and which is in no journal draft yet.
func (w *Workflow) SetCommission(ctx context.Context, sub Subject, txn uuid.UUID, in CommissionInput) error {
	q := w.q()
	t, err := q.GetBankTransactionForPosting(ctx, db.GetBankTransactionForPostingParams{TaxpayerID: sub.TaxpayerID, Uuid: pgtype.UUID{Bytes: txn, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	p := db.SetBankPostingCommissionParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID,
	}
	if in.Amount != nil {
		if banking.Direction(t.DebitCredit) != banking.DirIn || t.Currency != "TRY" {
			return fmt.Errorf("%w: only an incoming TRY collection has a POS commission", ErrInvalid)
		}
		amt := in.Amount.Round(2)
		if !amt.IsPositive() {
			return fmt.Errorf("%w: amount must be positive", ErrInvalid)
		}
		code := strings.TrimSpace(in.AccountCode)
		if !commissionCodeRe.MatchString(code) {
			return fmt.Errorf("%w: account_code must be 653 or 780 (or a sub account)", ErrInvalid)
		}
		p.CommissionAmount, p.CommissionAccountCode = num(amt), pgtype.Text{String: code, Valid: true}
	}
	view, err := q.GetBankPostingView(ctx, db.GetBankPostingViewParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, BankTransactionID: t.ID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNoPosting
	case err != nil:
		return err
	case view.JournalDraftUuid.Valid:
		return ErrDrafted
	}
	n, err := q.SetBankPostingCommission(ctx, p)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrDrafted
	}
	return nil
}

// commissionLine is the commission's draft line, nil without one.
func commissionLine(r db.ListApprovedBankPostingsWithoutDraftRow) (decimal.Decimal, string, bool) {
	if !r.CommissionAmount.Valid || !r.CommissionAccountCode.Valid || banking.Direction(r.DebitCredit) != banking.DirIn {
		return decimal.Zero, "", false
	}
	c := numDecimal(r.CommissionAmount)
	return c, r.CommissionAccountCode.String, c.IsPositive()
}
