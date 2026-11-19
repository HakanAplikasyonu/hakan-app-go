package matching

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
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
)

// maxCandidates on the screen; the matcher scores up to 200.
const maxCandidates = 20

// TransactionView is one bank movement with its invoice matches and the
// scored open invoices it could close (#528).
type TransactionView struct {
	UUID         uuid.UUID        `json:"uuid"`
	Date         string           `json:"transaction_date"`
	DebitCredit  string           `json:"debit_credit"`
	Amount       string           `json:"amount"`
	Currency     string           `json:"currency"`
	Description  string           `json:"description"`
	Counterparty *string          `json:"counterparty_name"`
	IBAN         *string          `json:"counterparty_iban"`
	KindDetail   string           `json:"kind_detail"`
	Period       string           `json:"period"`
	MatchStatus  string           `json:"match_status"`
	Matchable    bool             `json:"matchable"`
	Matches      []MatchedInvoice `json:"matches"`
	Candidates   []CandidateView  `json:"candidates"`
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
	}
	matches, err := q.ListBankTransactionMatchInvoices(ctx, db.ListBankTransactionMatchInvoicesParams{TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID})
	if err != nil {
		return v, err
	}
	matched := map[uuid.UUID]bool{}
	for _, r := range matches {
		matched[r.InvoiceUuid] = true
		v.Matches = append(v.Matches, MatchedInvoice{
			InvoiceUUID: r.InvoiceUuid, InvoiceNumber: r.InvoiceNumber, IssueDate: r.IssueDate.Time.Format("2006-01-02"),
			Counterparty: textPtr(r.CustomerName), Amount: dec(r.Amount).StringFixed(2), Method: r.Method, Score: int(r.Score),
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
			ranked = append(ranked, ScoreCandidate(m, c, s.opt))
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

// Search finds open invoices for a manual match by number, name or tax
// id, outside the candidates' date and amount window (#565). The movement
// must be matchable; results carry no score.
func (s *Service) Search(ctx context.Context, sub Subject, id uuid.UUID, term string) ([]CandidateView, error) {
	q := database.QueriesFor(s.pool)
	t, err := s.transaction(ctx, q, sub, id)
	if err != nil {
		return nil, err
	}
	if !matchable(t) {
		return nil, ErrNotMatchable
	}
	term = strings.TrimSpace(term)
	out := []CandidateView{}
	if len([]rune(term)) < 2 {
		return out, nil
	}
	rows, err := q.SearchInvoicesForBankMatch(ctx, db.SearchInvoicesForBankMatchParams{
		Currency: t.Currency, TaxpayerID: sub.TaxpayerID, Term: term,
		Direction: invoiceDirection(banking.Direction(t.DebitCredit)),
	})
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

// Match replaces the movement's matches with the chosen invoices (the
// accountant's decision, method manual), supersedes a pending bank_match
// suggestion for it, then refreshes the period's bank
// warnings so the ones it settles are resolved at once.
func (s *Service) Match(ctx context.Context, sub Subject, id uuid.UUID, invoices []uuid.UUID, userID int64) (TransactionView, error) {
	q := database.QueriesFor(s.pool)
	t, err := s.transaction(ctx, q, sub, id)
	if err != nil {
		return TransactionView{}, err
	}
	if !matchable(t) {
		return TransactionView{}, ErrNotMatchable
	}
	rows, err := q.ListInvoicesForManualMatch(ctx, db.ListInvoicesForManualMatchParams{
		Currency: t.Currency, BankTransactionID: t.ID, TaxpayerID: sub.TaxpayerID, Uuids: invoices,
		Direction: invoiceDirection(banking.Direction(t.DebitCredit)),
	})
	if err != nil {
		return TransactionView{}, err
	}
	if len(rows) != len(uniqueUUIDs(invoices)) {
		return TransactionView{}, fmt.Errorf("%w: an invoice is missing, inactive or of the wrong direction or currency", ErrInvalidInvoices)
	}
	v := matchValue{BankTransactionID: t.ID, Score: 100}
	remaining := dec(t.Amount)
	for _, r := range rows {
		open := dec(r.ComparableAmount).Sub(dec(r.MatchedAmount))
		if !open.IsPositive() {
			return TransactionView{}, fmt.Errorf("%w: invoice %s is already paid", ErrInvalidInvoices, r.InvoiceNumber)
		}
		amt := decimal.Min(open, remaining)
		if !amt.IsPositive() {
			return TransactionView{}, fmt.Errorf("%w: the transaction amount is used up before invoice %s", ErrInvalidInvoices, r.InvoiceNumber)
		}
		remaining = remaining.Sub(amt)
		v.Invoices = append(v.Invoices, matchInvoice{ID: r.ID, Number: r.InvoiceNumber, Amount: amt.StringFixed(4), Score: 100, Parts: map[string]int{"manual": 100}})
	}
	v.Combination = len(v.Invoices) > 1
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
		return writeValue(ctx, tq, sub.TaxpayerID, v, "manual", &userID)
	})
	if err != nil {
		return TransactionView{}, err
	}
	return s.refreshed(ctx, q, sub, t.Uuid, t.Period)
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

func (s *Service) refreshed(ctx context.Context, q *db.Queries, sub Subject, id uuid.UUID, period string) (TransactionView, error) {
	if _, _, err := s.controls(ctx, q, sub, period); err != nil {
		return TransactionView{}, err
	}
	return s.Transaction(ctx, sub, id)
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
