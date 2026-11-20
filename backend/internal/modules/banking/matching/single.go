package matching

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/posting"
)

// Errors of the single-transaction screen (#528).
var (
	ErrTransactionNotFound = errors.New("bank transaction not found")
	// ErrNotMatchable: a cancelled movement or one that neither collects
	// nor pays (e.g. a transfer between own accounts) closes no invoice.
	ErrNotMatchable = errors.New("bank transaction cannot be matched to invoices")
	// ErrInvalidInvoices: an invoice is missing, of the wrong direction or
	// currency, inactive, or already fully paid by other movements.
	ErrInvalidInvoices = errors.New("invoices cannot be matched to this transaction")
	// ErrNoAccounts: the posting workflow is not wired (tenancy off).
	ErrNoAccounts = errors.New("bank posting is not configured")
)

// maxCandidates on the screen; the matcher scores up to 200.
const maxCandidates = 20

// TransactionView is one bank movement with its invoice matches and the
// scored open invoices it could close (#528).
type TransactionView struct {
	UUID         uuid.UUID `json:"uuid"`
	Date         string    `json:"transaction_date"`
	DebitCredit  string    `json:"debit_credit"`
	Amount       string    `json:"amount"`
	Currency     string    `json:"currency"`
	Description  string    `json:"description"`
	Counterparty *string   `json:"counterparty_name"`
	// CounterpartyGuessed: the bank sent no name; it is read from the
	// description (#565) and shown as such.
	CounterpartyGuessed bool             `json:"counterparty_from_description"`
	IBAN                *string          `json:"counterparty_iban"`
	KindDetail          string           `json:"kind_detail"`
	Period              string           `json:"period"`
	MatchStatus         string           `json:"match_status"`
	Matchable           bool             `json:"matchable"`
	Matches             []MatchedInvoice `json:"matches"`
	Candidates          []CandidateView  `json:"candidates"`
	// LineMatchable: the movement can be tied to an existing journal line
	// (any movement that is not cancelled, #669 G).
	LineMatchable  bool                 `json:"line_matchable"`
	JournalMatches []MatchedJournalLine `json:"journal_matches"`
	// Posting is the trial-balance account the movement is booked to (or
	// proposed for), nil when there is none yet (#565).
	Posting *PostingView `json:"posting"`
	// PayrollMatchable: an outgoing payment can be tied to a payroll run
	// (net salaries); candidates are the runs of its month and the two
	// before with something unpaid (#710).
	PayrollMatchable  bool               `json:"payroll_matchable"`
	PayrollMatches    []MatchedPayroll   `json:"payroll_matches"`
	PayrollCandidates []PayrollCandidate `json:"payroll_candidates"`
	// FinancialMatches / FinancialCandidates: source financial transactions
	// (cheque, promissory note from Easytrade, KolayBi, Paraşüt) the movement
	// settles, and those of the same direction around its date (#565).
	FinancialMatches    []MatchedFinancial   `json:"financial_matches"`
	FinancialCandidates []FinancialCandidate `json:"financial_candidates"`
}

// PostingView is the movement's counter account: suggested (waits for the
// accountant), approved, or drafted (already in a journal draft).
type PostingView struct {
	Status           string     `json:"status"`
	AccountCode      string     `json:"account_code"`
	AccountName      string     `json:"account_name"`
	Source           string     `json:"source"`
	JournalDraftUUID *uuid.UUID `json:"journal_draft_uuid"`
	// CommissionAmount / CommissionAccountCode: the POS commission the bank
	// kept from a collection (#565); GrossAmount = movement + commission is
	// what the counter account takes. Nil without a commission.
	CommissionAmount      *string `json:"commission_amount"`
	CommissionAccountCode *string `json:"commission_account_code"`
	GrossAmount           *string `json:"gross_amount"`
}

// MatchedInvoice is an invoice the movement closes (fully or in part).
type MatchedInvoice struct {
	InvoiceUUID   uuid.UUID `json:"invoice_uuid"`
	InvoiceNumber string    `json:"invoice_number"`
	IssueDate     string    `json:"issue_date"`
	Counterparty  *string   `json:"counterparty_name"`
	Amount        string    `json:"amount"`
	Method        string    `json:"method"`
	Score         int       `json:"score"`
	// Parts: why the matcher proposed it (21 §6.2 score parts); empty for
	// a match made by hand (#710).
	Parts map[string]int `json:"parts"`
}

// CandidateView is an open invoice with its 21 §6.2 score.
type CandidateView struct {
	InvoiceUUID   uuid.UUID      `json:"invoice_uuid"`
	InvoiceNumber string         `json:"invoice_number"`
	IssueDate     string         `json:"issue_date"`
	Counterparty  *string        `json:"counterparty_name"`
	OpenAmount    string         `json:"open_amount"`
	Score         int            `json:"score"`
	Parts         map[string]int `json:"parts"`
}

func matchable(t db.BankTransaction) bool {
	if t.IsCancelled {
		return false
	}
	switch t.KindDetail {
	case "customer_collection", "supplier_payment", "rent_payment", "utility_payment":
		return true
	}
	return false
}

// candidates loads the movement's open invoice candidates (shared by the
// period run and the single-transaction screen).
func (s *Service) candidates(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction) (Movement, []Candidate, map[int64]db.ListInvoiceMatchCandidatesRow, error) {
	m := Movement{
		ID: t.ID, Date: t.TransactionDate.Time, Direction: banking.Direction(t.DebitCredit),
		Amount: dec(t.Amount), IBAN: t.CounterpartyIban.String, Name: t.CounterpartyName.String,
		TaxID:       t.CounterpartyTaxNumber.String + t.CounterpartyIdentityNumber.String,
		Description: t.Description + " " + t.DescriptionExtra.String,
	}
	numbers := InvoiceNumbers(m.Description)
	if numbers == nil {
		numbers = []string{}
	}
	rows, err := q.ListInvoiceMatchCandidates(ctx, db.ListInvoiceMatchCandidatesParams{
		Currency: t.Currency, TaxpayerID: sub.TaxpayerID, Direction: invoiceDirection(m.Direction),
		DateFrom: pgtype.Date{Time: m.Date.Add(-windowBefore), Valid: true},
		DateTo:   pgtype.Date{Time: m.Date.Add(windowAfter), Valid: true},
		Amount:   t.Amount, Tolerance: numeric(s.opt.FeeTolerance.Add(cent)),
		TaxNumber: t.CounterpartyTaxNumber, IdentityNumber: t.CounterpartyIdentityNumber,
		InvoiceNumbers: numbers, Iban: t.CounterpartyIban,
	})
	if err != nil {
		return m, nil, nil, err
	}
	cands := make([]Candidate, 0, len(rows))
	byID := make(map[int64]db.ListInvoiceMatchCandidatesRow, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
		c := Candidate{
			ID: r.ID, Number: r.InvoiceNumber, IssueDate: r.IssueDate.Time,
			Open:  dec(r.ComparableAmount).Sub(dec(r.MatchedAmount)),
			TaxID: r.CustomerTaxNumber.String + r.CustomerIdentityNumber.String, Name: r.CustomerName.String,
		}
		if r.PaymentDueDate.Valid {
			d := r.PaymentDueDate.Time
			c.DueDate = &d
		}
		var accounts []map[string]any
		if json.Unmarshal(r.BankAccounts, &accounts) == nil {
			for _, a := range accounts {
				if iban, ok := a["iban"].(string); ok {
					c.IBANs = append(c.IBANs, iban)
				}
			}
		}
		cands = append(cands, c)
	}
	return m, cands, byID, nil
}

// invoiceDirection: a collection closes a sales invoice, a payment a
// purchase invoice.
func invoiceDirection(d banking.Direction) string {
	if d == banking.DirIn {
		return "outgoing"
	}
	return "incoming"
}

func (s *Service) transaction(ctx context.Context, q *db.Queries, sub Subject, id uuid.UUID) (db.BankTransaction, error) {
	t, err := q.GetBankTransactionByUUID(ctx, db.GetBankTransactionByUUIDParams{TaxpayerID: sub.TaxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrTransactionNotFound
	}
	return t, err
}

// Transaction returns the movement, its matches and its scored candidates.
func (s *Service) Transaction(ctx context.Context, sub Subject, id uuid.UUID) (TransactionView, error) {
	q := database.QueriesFor(s.pool)
	t, err := s.transaction(ctx, q, sub, id)
	if err != nil {
		return TransactionView{}, err
	}
	return s.view(ctx, q, sub, t)
}

func (s *Service) view(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction) (TransactionView, error) {
	v := TransactionView{
		UUID: t.Uuid, Date: t.TransactionDate.Time.Format("2006-01-02"), DebitCredit: t.DebitCredit,
		Amount: dec(t.Amount).StringFixed(2), Currency: t.Currency, Description: t.Description,
		Counterparty: textPtr(t.CounterpartyName), IBAN: textPtr(t.CounterpartyIban),
		KindDetail: t.KindDetail, Period: t.Period, MatchStatus: t.MatchStatus, Matchable: matchable(t),
		Matches: []MatchedInvoice{}, Candidates: []CandidateView{},
		LineMatchable: lineMatchable(t), JournalMatches: []MatchedJournalLine{},
	}
	if v.Counterparty == nil || strings.TrimSpace(*v.Counterparty) == "" {
		if name := CounterpartyFromDescription(t.Description); name != "" {
			v.Counterparty, v.CounterpartyGuessed = &name, true
		}
	}
	p, err := q.GetBankPostingView(ctx, db.GetBankPostingViewParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, BankTransactionID: t.ID})
	switch {
	case err == nil:
		v.Posting = &PostingView{Status: p.Status, AccountCode: p.AccountCode, AccountName: p.AccountName, Source: p.Source}
		if p.CommissionAmount.Valid && p.CommissionAccountCode.Valid {
			c := dec(p.CommissionAmount)
			amt, code, gross := c.StringFixed(2), p.CommissionAccountCode.String, dec(t.Amount).Add(c).StringFixed(2)
			v.Posting.CommissionAmount, v.Posting.CommissionAccountCode, v.Posting.GrossAmount = &amt, &code, &gross
		}
		if p.JournalDraftUuid.Valid {
			id := uuid.UUID(p.JournalDraftUuid.Bytes)
			v.Posting.JournalDraftUUID, v.Posting.Status = &id, "drafted"
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return v, err
	}
	matches, err := q.ListBankTransactionMatchInvoices(ctx, db.ListBankTransactionMatchInvoicesParams{TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID})
	if err != nil {
		return v, err
	}
	if v.JournalMatches, err = s.journalMatches(ctx, q, sub, t); err != nil {
		return v, err
	}
	if err := s.payrollView(ctx, q, sub, t, &v); err != nil {
		return v, err
	}
	if err := s.financialView(ctx, q, sub, t, &v); err != nil {
		return v, err
	}
	matched := map[uuid.UUID]bool{}
	for _, r := range matches {
		matched[r.InvoiceUuid] = true
		v.Matches = append(v.Matches, MatchedInvoice{
			InvoiceUUID: r.InvoiceUuid, InvoiceNumber: r.InvoiceNumber, IssueDate: r.IssueDate.Time.Format("2006-01-02"),
			Counterparty: textPtr(r.CustomerName), Amount: dec(r.Amount).StringFixed(2), Method: r.Method, Score: int(r.Score),
			Parts: evidenceParts(r.Evidence, r.Method),
		})
	}
	if !v.Matchable {
		return v, nil
	}
	m, cands, byID, err := s.candidates(ctx, q, sub, t)
	if err != nil {
		return v, err
	}
	ranked := make([]Score, 0, len(cands))
	for _, c := range cands {
		if c.Open.IsPositive() && !matched[byID[c.ID].Uuid] {
			if sc := ScoreCandidate(m, c, s.opt); Relevant(sc) {
				ranked = append(ranked, sc)
			}
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Total != ranked[j].Total {
			return ranked[i].Total > ranked[j].Total
		}
		return ranked[i].Candidate.IssueDate.Before(ranked[j].Candidate.IssueDate)
	})
	for _, r := range ranked[:min(len(ranked), maxCandidates)] {
		row := byID[r.Candidate.ID]
		v.Candidates = append(v.Candidates, CandidateView{
			InvoiceUUID: row.Uuid, InvoiceNumber: row.InvoiceNumber, IssueDate: row.IssueDate.Time.Format("2006-01-02"),
			Counterparty: textPtr(row.CustomerName), OpenAmount: r.Candidate.Open.StringFixed(2), Score: r.Total, Parts: r.Parts,
		})
	}
	return v, nil
}

// SearchFilter narrows the manual invoice search (#565). Every field is
// optional; an empty filter finds nothing.
type SearchFilter struct {
	// Term: invoice number or counterparty name (contains), VKN/TCKN (equals);
	// at least two characters.
	Term string
	// From / To: invoice issue date range, inclusive.
	From, To *time.Time
	// Min / Max: invoice payable amount in the movement's currency, inclusive.
	Min, Max *decimal.Decimal
}

func (f SearchFilter) empty() bool {
	return f.Term == "" && f.From == nil && f.To == nil && f.Min == nil && f.Max == nil
}

// Search finds open invoices for a manual match by number, name or tax
// id, outside the candidates' date and amount window (#565). The movement
// must be matchable; results carry no score.
func (s *Service) Search(ctx context.Context, sub Subject, id uuid.UUID, term string) ([]CandidateView, error) {
	return s.SearchFiltered(ctx, sub, id, SearchFilter{Term: term})
}

// SearchFiltered is Search with a date and amount range (#565): the
// accountant narrows by "around this date" or "about this much" when the
// number or name is not known.
func (s *Service) SearchFiltered(ctx context.Context, sub Subject, id uuid.UUID, f SearchFilter) ([]CandidateView, error) {
	q := database.QueriesFor(s.pool)
	t, err := s.transaction(ctx, q, sub, id)
	if err != nil {
		return nil, err
	}
	if !matchable(t) {
		return nil, ErrNotMatchable
	}
	f.Term = strings.TrimSpace(f.Term)
	out := []CandidateView{}
	if f.Term != "" && len([]rune(f.Term)) < 2 {
		f.Term = ""
	}
	if f.empty() {
		return out, nil
	}
	p := db.SearchInvoicesForBankMatchParams{
		Currency: t.Currency, TaxpayerID: sub.TaxpayerID,
		Direction: invoiceDirection(banking.Direction(t.DebitCredit)),
	}
	if f.Term != "" {
		p.Term = pgtype.Text{String: f.Term, Valid: true}
	}
	if f.From != nil {
		p.DateFrom = pgtype.Date{Time: *f.From, Valid: true}
	}
	if f.To != nil {
		p.DateTo = pgtype.Date{Time: *f.To, Valid: true}
	}
	if f.Min != nil {
		p.AmountMin = numeric(*f.Min)
	}
	if f.Max != nil {
		p.AmountMax = numeric(*f.Max)
	}
	rows, err := q.SearchInvoicesForBankMatch(ctx, p)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		open := dec(r.ComparableAmount).Sub(dec(r.MatchedAmount))
		if !open.IsPositive() {
			continue
		}
		out = append(out, CandidateView{
			InvoiceUUID: r.Uuid, InvoiceNumber: r.InvoiceNumber, IssueDate: r.IssueDate.Time.Format("2006-01-02"),
			Counterparty: textPtr(r.CustomerName), OpenAmount: open.StringFixed(2), Parts: map[string]int{},
		})
		if len(out) == maxCandidates {
			break
		}
	}
	return out, nil
}

// Match replaces the movement's matches with the chosen invoices and / or
// journal lines (the accountant's decision, method manual) in one step:
// re-matching needs no separate undo (#669 G). It supersedes a pending
// bank_match suggestion for the movement, then refreshes the period's bank
// warnings so the ones it settles are resolved at once. Invoices take the
// amount first, the lines what is left.
func (s *Service) Match(ctx context.Context, sub Subject, id uuid.UUID, invoices []uuid.UUID, lines []LineRef, userID int64) (TransactionView, error) {
	return s.MatchAll(ctx, sub, id, MatchInput{Invoices: invoices, Lines: lines}, userID)
}

// MatchInput is what the accountant ties the movement to: invoices,
// payroll runs (net salaries, #710) and / or journal lines.
type MatchInput struct {
	Invoices    []uuid.UUID
	PayrollRuns []uuid.UUID
	Financial   []uuid.UUID
	Lines       []LineRef
}

// MatchAll is Match with payroll runs. Invoices take the amount first, then
// the payroll runs, the lines what is left.
func (s *Service) MatchAll(ctx context.Context, sub Subject, id uuid.UUID, in MatchInput, userID int64) (TransactionView, error) {
	invoices, lines := in.Invoices, in.Lines
	q := database.QueriesFor(s.pool)
	t, err := s.transaction(ctx, q, sub, id)
	if err != nil {
		return TransactionView{}, err
	}
	if len(invoices)+len(lines)+len(in.PayrollRuns)+len(in.Financial) == 0 {
		return TransactionView{}, fmt.Errorf("%w: pick an invoice or a journal line", ErrInvalidInvoices)
	}
	if (len(invoices) > 0 && !matchable(t)) || !lineMatchable(t) {
		return TransactionView{}, ErrNotMatchable
	}
	v := matchValue{BankTransactionID: t.ID, Score: 100}
	remaining := dec(t.Amount)
	if len(invoices) > 0 {
		if remaining, err = s.pickInvoices(ctx, q, sub, t, invoices, &v, remaining); err != nil {
			return TransactionView{}, err
		}
	}
	payroll, remaining, err := s.pickPayroll(ctx, q, sub, t, in.PayrollRuns, remaining)
	if err != nil {
		return TransactionView{}, err
	}
	financial, remaining, err := s.pickFinancial(ctx, q, sub, t, in.Financial, remaining)
	if err != nil {
		return TransactionView{}, err
	}
	picked, err := s.pickLines(ctx, q, sub, t, lines, remaining)
	if err != nil {
		return TransactionView{}, err
	}
	err = database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tq := db.New(tx)
		if err := tq.DeleteBankTransactionMatches(ctx, db.DeleteBankTransactionMatchesParams{TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID}); err != nil {
			return err
		}
		// The accountant decided: a pending suggestion for this movement is
		// moot and leaves the review queue (#529).
		if _, err := tq.SupersedePendingSuggestions(ctx, db.SupersedePendingSuggestionsParams{
			OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Kind: SuggestionKind,
			TargetType: "bank_transaction", TargetID: pgtype.Int8{Int64: t.ID, Valid: true}, TargetField: "match",
		}); err != nil {
			return err
		}
		if err := writeLines(ctx, tq, sub, t.ID, picked, userID); err != nil {
			return err
		}
		if err := writePayroll(ctx, tq, sub, t.ID, payroll, userID); err != nil {
			return err
		}
		if err := writeFinancial(ctx, tq, sub, t.ID, financial, userID); err != nil {
			return err
		}
		// writeValue marks the movement matched (with or without invoices).
		return writeValue(ctx, tq, sub.TaxpayerID, v, "manual", &userID)
	})
	if err != nil {
		return TransactionView{}, err
	}
	return s.refreshed(ctx, q, sub, t.Uuid, t.Period)
}

// pickInvoices validates the picked invoices and fills v with the amount
// each takes; it returns what is left of the movement.
func (s *Service) pickInvoices(ctx context.Context, q *db.Queries, sub Subject, t db.BankTransaction, invoices []uuid.UUID, v *matchValue, remaining decimal.Decimal) (decimal.Decimal, error) {
	rows, err := q.ListInvoicesForManualMatch(ctx, db.ListInvoicesForManualMatchParams{
		Currency: t.Currency, BankTransactionID: t.ID, TaxpayerID: sub.TaxpayerID, Uuids: invoices,
		Direction: invoiceDirection(banking.Direction(t.DebitCredit)),
	})
	if err != nil {
		return remaining, err
	}
	if len(rows) != len(uniqueUUIDs(invoices)) {
		return remaining, fmt.Errorf("%w: an invoice is missing, inactive or of the wrong direction or currency", ErrInvalidInvoices)
	}
	for _, r := range rows {
		open := dec(r.ComparableAmount).Sub(dec(r.MatchedAmount))
		if !open.IsPositive() {
			return remaining, fmt.Errorf("%w: invoice %s is already paid", ErrInvalidInvoices, r.InvoiceNumber)
		}
		amt := decimal.Min(open, remaining)
		if !amt.IsPositive() {
			return remaining, fmt.Errorf("%w: the transaction amount is used up before invoice %s", ErrInvalidInvoices, r.InvoiceNumber)
		}
		remaining = remaining.Sub(amt)
		v.Invoices = append(v.Invoices, matchInvoice{ID: r.ID, Number: r.InvoiceNumber, Amount: amt.StringFixed(4), Score: 100, Parts: map[string]int{"manual": 100}})
	}
	v.Combination = len(v.Invoices) > 1
	return remaining, nil
}

// Unmatch removes the movement's matches; it is open again and the period
// warnings are refreshed.
func (s *Service) Unmatch(ctx context.Context, sub Subject, id uuid.UUID) (TransactionView, error) {
	q := database.QueriesFor(s.pool)
	t, err := s.transaction(ctx, q, sub, id)
	if err != nil {
		return TransactionView{}, err
	}
	err = database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tq := db.New(tx)
		if err := tq.DeleteBankTransactionMatches(ctx, db.DeleteBankTransactionMatchesParams{TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID}); err != nil {
			return err
		}
		return tq.SetBankTransactionMatchStatus(ctx, db.SetBankTransactionMatchStatusParams{MatchStatus: "unmatched", TaxpayerID: sub.TaxpayerID, ID: t.ID})
	})
	if err != nil {
		return TransactionView{}, err
	}
	return s.refreshed(ctx, q, sub, t.Uuid, t.Period)
}

// SetAccount books the movement to the account the accountant chose
// (cari 120/320, advance, salary, tax, rent, bank cost ...), with or
// without an invoice match, and refreshes the period warnings: a movement
// booked by hand no longer counts as an uninvoiced collection or an
// undocumented payment (#565). The decision is kept in the review queue
// (who, when, which account) and can be reverted there.
func (s *Service) SetAccount(ctx context.Context, sub Subject, id uuid.UUID, userID int64, code, name string) (TransactionView, error) {
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
	psub := posting.Subject{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID}
	if err := s.accounts.SetAccount(ctx, psub, userID, t.Uuid, code, name); err != nil {
		return TransactionView{}, err
	}
	return s.refreshed(ctx, q, sub, t.Uuid, t.Period)
}

func (s *Service) refreshed(ctx context.Context, q *db.Queries, sub Subject, id uuid.UUID, period string) (TransactionView, error) {
	if _, _, err := s.controls(ctx, q, sub, period); err != nil {
		return TransactionView{}, err
	}
	return s.Transaction(ctx, sub, id)
}

// evidenceParts reads the score parts kept with an automatic or suggested
// match; a manual match has no reason to show.
func evidenceParts(raw []byte, method string) map[string]int {
	out := map[string]int{}
	if method == "manual" {
		return out
	}
	var e struct {
		Parts map[string]int `json:"parts"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Parts != nil {
		out = e.Parts
	}
	return out
}

func uniqueUUIDs(ids []uuid.UUID) map[uuid.UUID]bool {
	m := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}
