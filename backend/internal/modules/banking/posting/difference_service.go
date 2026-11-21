package posting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// DifferenceReport is the period's statement vs ledger comparison of every
// bank account (#780 C).
type DifferenceReport struct {
	Period     string              `json:"period"`
	FromPeriod string              `json:"from_period"`
	Items      []DifferenceAccount `json:"items"`
}

func decPtr(n pgtype.Numeric) *decimal.Decimal {
	if !n.Valid {
		return nil
	}
	d := numDecimal(n)
	return &d
}

// Difference compares each bank account's statement balance at the end of
// the period with its 102 balance from January to the period (drafts
// included) and breaks the difference down by reason.
func (w *Workflow) Difference(ctx context.Context, sub Subject, period string) (DifferenceReport, error) {
	if !periodRe.MatchString(period) {
		return DifferenceReport{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	from := period[:4] + "-01"
	out := DifferenceReport{Period: period, FromPeriod: from, Items: []DifferenceAccount{}}
	q := w.q()
	boxRows, err := q.BankDifferenceBoxes(ctx, db.BankDifferenceBoxesParams{OrganizationID: sub.OrganizationID,
		EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID})
	if err != nil {
		return out, err
	}
	var boxes []diffBox
	for _, b := range boxRows {
		if b.Currency != "TRY" {
			continue // the ledger is in TRY; a foreign-currency account is not compared
		}
		boxes = append(boxes, diffBox{ID: b.ID, UUID: b.Uuid, Name: b.Name, IBAN: b.Iban.String, Code: normalizeLedgerCode(b.AccountCode)})
	}
	if len(boxes) == 0 {
		return out, nil
	}
	txRows, err := q.BankDifferenceTransactions(ctx, db.BankDifferenceTransactionsParams{TaxpayerID: sub.TaxpayerID,
		FromPeriod: from, ToPeriod: period})
	if err != nil {
		return out, err
	}
	txs := make([]diffTx, len(txRows))
	for i, r := range txRows {
		s := numDecimal(r.Amount)
		if r.DebitCredit == "B" {
			s = s.Neg()
		}
		txs[i] = diffTx{ID: r.ID, UUID: r.Uuid, Box: r.CashBoxID, Date: r.TransactionDate.Time, Period: r.Period, Signed: s,
			Before: decPtr(r.BalanceBefore), After: decPtr(r.BalanceAfter), Cancelled: r.IsCancelled, Flagged: r.Flagged,
			Partner: r.TransferPartnerID, Description: r.Description}
	}
	lineRows, err := q.BankDifferenceLedgerLines(ctx, db.BankDifferenceLedgerLinesParams{OrganizationID: sub.OrganizationID,
		EngagementID: sub.EngagementID, FromPeriod: from, ToPeriod: period})
	if err != nil {
		return out, err
	}
	lines := make([]diffLine, 0, len(lineRows))
	for _, r := range lineRows {
		code := normalizeLedgerCode(r.AccountCode)
		if !codeUnder(code, DefaultBankAccount) {
			continue // "1020" is not a 102 code
		}
		lines = append(lines, diffLine{DraftUUID: r.DraftUuid, EntryNo: r.EntryNo, Period: r.Period, EntryType: r.EntryType,
			Date: r.EntryDate.Time, LineNo: r.LineNo, Code: code, Description: r.Description,
			Signed: numDecimal(r.Debit).Sub(numDecimal(r.Credit)), SourceKind: r.SourceKind.String, SourceID: r.SourceID.String})
	}
	resRows, err := q.ListBankDifferenceResolutions(ctx, db.ListBankDifferenceResolutionsParams{OrganizationID: sub.OrganizationID,
		EngagementID: sub.EngagementID, Period: period})
	if err != nil {
		return out, err
	}
	byBox := map[uuid.UUID]map[string][]DifferenceResolution{}
	for _, r := range resRows {
		m := byBox[r.CashBoxUuid]
		if m == nil {
			m = map[string][]DifferenceResolution{}
			byBox[r.CashBoxUuid] = m
		}
		res := DifferenceResolution{UUID: r.Uuid, Action: r.Action, Explanation: r.Explanation, CreatedBy: r.CreatedByName,
			CreatedAt: r.CreatedAt.Time}
		if r.Amount.Valid {
			s := numDecimal(r.Amount).StringFixed(2)
			res.Amount = &s
		}
		if r.CounterAccount.Valid {
			s := r.CounterAccount.String
			res.CounterAccount = &s
		}
		if r.JournalDraftUuid.Valid {
			id := uuid.UUID(r.JournalDraftUuid.Bytes)
			res.DraftUUID = &id
		}
		m[r.Reason] = append(m[r.Reason], res)
	}
	for _, b := range boxes {
		out.Items = append(out.Items, computeDifference(b, boxes, txs, lines, byBox[b.UUID]))
	}
	return out, nil
}

// ResolutionInput is the office's decision on one reason of one account's
// difference: explained (no entry needed) or corrected (a correction draft
// against the chosen counter account).
type ResolutionInput struct {
	CashBoxUUID    uuid.UUID `json:"cash_box_uuid"`
	Period         string    `json:"period"`
	Reason         string    `json:"reason"`
	Action         string    `json:"action"`
	Explanation    string    `json:"explanation"`
	CounterAccount string    `json:"counter_account,omitempty"`
	// Amount is signed like the reason: positive raises the 102 balance
	// (102 borç), negative lowers it (102 alacak).
	Amount string `json:"amount,omitempty"`
}

// ResolutionResult is the stored decision and, for a correction, its draft.
type ResolutionResult struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	CreatedAt time.Time       `json:"created_at"`
	Draft     *journals.Draft `json:"draft"`
}

// ErrAlreadyExplained: the reason is already marked explained.
var ErrAlreadyExplained = errors.New("posting: reason already explained")

func lastDayOf(period string) (time.Time, error) {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return time.Time{}, err
	}
	return t.AddDate(0, 1, -1), nil
}

// Resolve records the office's decision; a correction books a draft (bank
// line source "bankfark:<reason>" so the comparison counts it).
func (w *Workflow) Resolve(ctx context.Context, scope engagementctx.Scope, userID int64, in ResolutionInput) (ResolutionResult, error) {
	in.Explanation = strings.TrimSpace(in.Explanation)
	in.CounterAccount = normalizeLedgerCode(in.CounterAccount)
	switch {
	case !periodRe.MatchString(in.Period):
		return ResolutionResult{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	case !ValidReason(in.Reason):
		return ResolutionResult{}, fmt.Errorf("%w: reason", ErrInvalid)
	case in.Action != ActionExplained && in.Action != ActionCorrected:
		return ResolutionResult{}, fmt.Errorf("%w: action must be explained or corrected", ErrInvalid)
	case in.Explanation == "":
		return ResolutionResult{}, fmt.Errorf("%w: explanation is required", ErrInvalid)
	case len(in.Explanation) > 1000:
		return ResolutionResult{}, fmt.Errorf("%w: explanation is too long", ErrInvalid)
	}
	sub := Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID}
	q := w.q()
	boxes, err := q.BankDifferenceBoxes(ctx, db.BankDifferenceBoxesParams{OrganizationID: sub.OrganizationID,
		EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID})
	if err != nil {
		return ResolutionResult{}, err
	}
	var box *db.BankDifferenceBoxesRow
	for i := range boxes {
		if boxes[i].Uuid == in.CashBoxUUID {
			box = &boxes[i]
		}
	}
	if box == nil {
		return ResolutionResult{}, ErrNotFound
	}
	params := db.InsertBankDifferenceResolutionParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID,
		TaxpayerID: sub.TaxpayerID, CashBoxID: box.ID, Period: in.Period, Reason: in.Reason, Action: in.Action,
		Explanation: in.Explanation, CreatedBy: pgtype.Int8{Int64: userID, Valid: userID > 0}}
	res := ResolutionResult{Action: in.Action}
	if in.Action == ActionCorrected {
		lines, amount, err := correctionLines(normalizeLedgerCode(box.AccountCode), in)
		if err != nil {
			return ResolutionResult{}, err
		}
		if w.drafts == nil {
			return ResolutionResult{}, errors.New("posting: journal drafts are not configured")
		}
		day, _ := lastDayOf(in.Period)
		d, err := w.drafts.CreateDraft(ctx, scope, userID, journals.DraftInput{Type: accountingexport.EntryMahsup, Date: day,
			Description: "Banka bakiye farkı düzeltmesi: " + in.Explanation,
			Origin:      map[string]any{"source": "bank_ledger_difference", "reason": in.Reason, "cash_box_uuid": box.Uuid.String()},
			Lines:       lines})
		if err != nil {
			return ResolutionResult{}, err
		}
		res.Draft = &d
		params.Amount = num(amount)
		params.CounterAccount = pgtype.Text{String: in.CounterAccount, Valid: true}
		params.JournalDraftUuid = pgtype.UUID{Bytes: d.UUID, Valid: true}
	}
	row, err := q.InsertBankDifferenceResolution(ctx, params)
	if err != nil {
		if strings.Contains(err.Error(), "uq_bank_difference_resolutions_explained") {
			return ResolutionResult{}, ErrAlreadyExplained
		}
		return ResolutionResult{}, err
	}
	res.UUID, res.CreatedAt = row.Uuid, row.CreatedAt.Time
	return res, nil
}

// correctionLines: the bank line (102 code, borç when the amount is
// positive) against the counter account.
func correctionLines(bankCode string, in ResolutionInput) ([]journals.LineInput, decimal.Decimal, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(in.Amount))
	if err != nil || amount.Round(2).IsZero() {
		return nil, decimal.Zero, fmt.Errorf("%w: amount must be a non-zero number", ErrInvalid)
	}
	amount = amount.Round(2)
	if !engine.ValidCode(in.CounterAccount) {
		return nil, decimal.Zero, fmt.Errorf("%w: counter_account %q", ErrInvalid, in.CounterAccount)
	}
	if codeUnder(in.CounterAccount, bankCode) || codeUnder(bankCode, in.CounterAccount) {
		return nil, decimal.Zero, fmt.Errorf("%w: counter_account must differ from the bank account", ErrInvalid)
	}
	desc := "Banka bakiye farkı: " + in.Explanation
	bank := journals.LineInput{Account: bankCode, Description: desc, SourceKind: "manual", SourceID: CorrectionSourcePrefix + in.Reason}
	counter := journals.LineInput{Account: in.CounterAccount, Description: desc}
	if amount.IsPositive() {
		bank.Debit, counter.Credit = amount, amount
	} else {
		bank.Credit, counter.Debit = amount.Neg(), amount.Neg()
	}
	return []journals.LineInput{bank, counter}, amount, nil
}

// Unexplain removes an "explained" mark. A correction stays a draft that
// the office edits or reverses in the journals screen; the comparison
// follows its lines.
func (w *Workflow) Unexplain(ctx context.Context, sub Subject, id uuid.UUID) error {
	n, err := w.q().DeleteBankDifferenceExplanation(ctx, db.DeleteBankDifferenceExplanationParams{OrganizationID: sub.OrganizationID,
		EngagementID: sub.EngagementID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
