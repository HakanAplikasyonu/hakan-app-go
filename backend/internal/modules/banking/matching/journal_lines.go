package matching

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// Matching a movement to an existing journal line (#669 G): a collection or
// payment with no invoice is tied to the line of a voucher already keyed in
// (a cari 120 / 320 sub-account, an advance, rent …). The match lives in
// bank_transaction_matches with target_type journal_line; the line is
// engagement scoped, so every read filters it by organization + engagement.

// TargetJournalLine is the bank_transaction_matches.target_type of a line.
const TargetJournalLine = "journal_line"

// ErrInvalidLines: a picked line is missing, on the wrong side or currency,
// generated from a bank movement, or already fully matched elsewhere.
var ErrInvalidLines = errors.New("journal lines cannot be matched to this transaction")

// CariMainCodes are the current-account (cari) main accounts of the
// Turkish chart: trade receivables / payables, advances given and received,
// partner and staff accounts. "Cari hareketi" search narrows to them.
var CariMainCodes = []string{
	"120", "121", "126", "127", "131", "132", "133", "135", "136", "159",
	"195", "196", "320", "321", "326", "329", "331", "332", "333", "335", "336", "340",
}

// lineDateWindow: without a search term, lines dated within this many days
// before (or after) the movement whose amount equals it are offered.
const (
	lineDaysBefore = 180
	lineDaysAfter  = 30
)

// LineRef names one journal line: the voucher and the line number.
type LineRef struct {
	JournalDraftUUID uuid.UUID `json:"journal_draft_uuid"`
	LineNo           int32     `json:"line_no"`
}

func (r LineRef) key() string { return fmt.Sprintf("%s:%d", r.JournalDraftUUID, r.LineNo) }

// MatchedJournalLine is a journal line the movement is tied to.
type MatchedJournalLine struct {
	JournalDraftUUID uuid.UUID `json:"journal_draft_uuid"`
	EntryNo          int32     `json:"entry_no"`
	EntryDate        string    `json:"entry_date"`
	Period           string    `json:"period"`
	JournalStatus    string    `json:"journal_status"`
	LineNo           int32     `json:"line_no"`
	AccountCode      string    `json:"account_code"`
	Description      string    `json:"description"`
	Amount           string    `json:"amount"`
	Method           string    `json:"method"`
	Label            string    `json:"label"`
}

// JournalLineCandidate is a line the movement could be tied to.
type JournalLineCandidate struct {
	JournalDraftUUID uuid.UUID `json:"journal_draft_uuid"`
	EntryNo          int32     `json:"entry_no"`
	EntryDate        string    `json:"entry_date"`
	Period           string    `json:"period"`
	JournalStatus    string    `json:"journal_status"`
	LineNo           int32     `json:"line_no"`
	AccountCode      string    `json:"account_code"`
	Description      string    `json:"description"`
	DocumentNo       *string   `json:"document_no"`
	OpenAmount       string    `json:"open_amount"`
	Cari             bool      `json:"cari"`
	Label            string    `json:"label"`
}

// LineLabel names a line in the audit trail and on screen:
// "2026-10 #12/3 120.01.001".
func LineLabel(period string, entryNo, lineNo int32, account string) string {
	return fmt.Sprintf("%s #%d/%d %s", period, entryNo, lineNo, account)
}

// IsCari reports whether the account is under a current-account main code.
func IsCari(account string) bool {
	if len(account) < 3 {
		return false
	}
	main := account[:3]
	for _, c := range CariMainCodes {
		if c == main {
			return true
		}
	}
	return false
}

// lineSide: money in is credited to the counter account (a customer pays
// off 120), money out is debited (a supplier's 320 is paid).
func lineSide(dir banking.Direction) string {
	if dir == banking.DirIn {
		return "credit"
	}
	return "debit"
}

// lineMatchable: any movement that is not cancelled can be tied to a line
// (tax, rent or salary payments too, not only collections / payments).
func lineMatchable(t db.BankTransaction) bool { return !t.IsCancelled }

func (s *Service) journalMatches(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction) ([]MatchedJournalLine, error) {
	rows, err := q.ListBankTransactionMatchJournalLines(ctx, db.ListBankTransactionMatchJournalLinesParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]MatchedJournalLine, 0, len(rows))
	for _, r := range rows {
		out = append(out, MatchedJournalLine{
			JournalDraftUUID: r.JournalDraftUuid, EntryNo: r.EntryNo, EntryDate: r.EntryDate.Time.Format("2006-01-02"),
			Period: r.Period, JournalStatus: r.JournalStatus, LineNo: r.LineNo, AccountCode: r.AccountCode,
			Description: r.Description, Amount: dec(r.Amount).StringFixed(2), Method: r.Method,
			Label: LineLabel(r.Period, r.EntryNo, r.LineNo, r.AccountCode),
		})
	}
	return out, nil
}

// SearchLines lists journal lines the movement could be tied to (#669 G).
// Without a term: lines on the movement's side whose amount equals the
// movement, dated around it. With a term (≥ 2 characters): account code
// prefix, line or voucher description, document number. cariOnly narrows
// to the current-account main codes. Lines already fully matched to other
// movements are left out.
func (s *Service) SearchLines(ctx context.Context, sub Subject, id uuid.UUID, term string, cariOnly bool) ([]JournalLineCandidate, error) {
	q := database.QueriesFor(s.pool)
	t, err := s.transaction(ctx, q, sub, id)
	if err != nil {
		return nil, err
	}
	if !lineMatchable(t) {
		return nil, ErrNotMatchable
	}
	term = strings.TrimSpace(term)
	out := []JournalLineCandidate{}
	if term != "" && len([]rune(term)) < 2 {
		return out, nil
	}
	date := t.TransactionDate.Time
	rows, err := q.SearchJournalLinesForBankMatch(ctx, db.SearchJournalLinesForBankMatchParams{
		Currency: t.Currency, TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID,
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID,
		Side: lineSide(banking.Direction(t.DebitCredit)), CariOnly: cariOnly, CariCodes: CariMainCodes,
		Term:      term,
		DateFrom:  pgtype.Date{Time: date.AddDate(0, 0, -lineDaysBefore), Valid: true},
		DateTo:    pgtype.Date{Time: date.AddDate(0, 0, lineDaysAfter), Valid: true},
		Amount:    t.Amount,
		Tolerance: numeric(s.opt.FeeTolerance.Add(cent)),
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		open := dec(r.LineAmount).Sub(dec(r.MatchedAmount))
		if !open.IsPositive() {
			continue
		}
		out = append(out, JournalLineCandidate{
			JournalDraftUUID: r.JournalDraftUuid, EntryNo: r.EntryNo, EntryDate: r.EntryDate.Time.Format("2006-01-02"),
			Period: r.Period, JournalStatus: r.JournalStatus, LineNo: r.LineNo, AccountCode: r.AccountCode,
			Description: firstNonEmpty(r.Description, r.JournalDescription), DocumentNo: textPtr(r.DocumentNo),
			OpenAmount: open.StringFixed(2), Cari: IsCari(r.AccountCode),
			Label: LineLabel(r.Period, r.EntryNo, r.LineNo, r.AccountCode),
		})
		if len(out) == maxCandidates {
			break
		}
	}
	return out, nil
}

// lineMatch is one journal line to write, with the amount it takes.
type lineMatch struct {
	ID     int64
	Amount decimal.Decimal
	Label  string
}

// pickLines validates the picked lines and splits what is left of the
// movement over them, in voucher order.
func (s *Service) pickLines(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction, refs []LineRef, remaining decimal.Decimal) ([]lineMatch, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(refs))
	seen := map[string]bool{}
	for _, r := range refs {
		if k := r.key(); !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	rows, err := q.ListJournalLinesForManualMatch(ctx, db.ListJournalLinesForManualMatchParams{
		Currency: t.Currency, TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID, Keys: keys,
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Side: lineSide(banking.Direction(t.DebitCredit)),
	})
	if err != nil {
		return nil, err
	}
	if len(rows) != len(keys) {
		return nil, fmt.Errorf("%w: a line is missing, on the wrong side or currency, or generated from a bank movement", ErrInvalidLines)
	}
	out := make([]lineMatch, 0, len(rows))
	for _, r := range rows {
		label := LineLabel(r.Period, r.EntryNo, r.LineNo, r.AccountCode)
		open := dec(r.LineAmount).Sub(dec(r.MatchedAmount))
		if !open.IsPositive() {
			return nil, fmt.Errorf("%w: line %s is already matched in full", ErrInvalidLines, label)
		}
		amt := decimal.Min(open, remaining)
		if !amt.IsPositive() {
			return nil, fmt.Errorf("%w: the transaction amount is used up before line %s", ErrInvalidLines, label)
		}
		remaining = remaining.Sub(amt)
		out = append(out, lineMatch{ID: r.ID, Amount: amt, Label: label})
	}
	return out, nil
}

func writeLines(ctx context.Context, q *db.Queries, sub Subject, txID int64, lines []lineMatch, userID int64) error {
	for _, l := range lines {
		evidence := fmt.Sprintf(`{"rule":"manual","engagement_id":%d,"line":%q}`, sub.EngagementID, l.Label)
		if _, err := q.InsertBankTransactionMatch(ctx, db.InsertBankTransactionMatchParams{
			TaxpayerID: sub.TaxpayerID, BankTransactionID: txID, TargetType: TargetJournalLine, TargetID: l.ID,
			Amount: numeric(l.Amount), Score: 100, Method: "manual", Evidence: []byte(evidence),
			MatchedBy: pgtype.Int8{Int64: userID, Valid: true},
		}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	return nil
}

func lineLabels(ms []MatchedJournalLine) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Label)
	}
	return out
}
