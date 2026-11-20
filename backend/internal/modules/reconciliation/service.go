package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Service errors.
var (
	ErrNotFound       = errors.New("reconciliation: not found")
	ErrInvalid        = errors.New("reconciliation: invalid input")
	ErrNoTrialBalance = errors.New("reconciliation: no trial balance with amounts for the period")
	ErrNoReview       = errors.New("reconciliation: review queue is not configured")
	ErrMatched        = errors.New("reconciliation: the account has no difference")
	ErrInReview       = errors.New("reconciliation: the item is waiting in the review queue")
	// ErrNoRun: "latest" asked but the engagement (or the period) has no
	// run yet. An empty state, not a missing resource (#639): getRun answers
	// 200 with a null run; other callers still see ErrNotFound.
	ErrNoRun = fmt.Errorf("%w: no run yet", ErrNotFound)
)

// SuggestionKind is the review queue kind of a reconciliation item.
const SuggestionKind = "reconciliation"

// Review targets.
const (
	TargetItem         = "reconciliation_item"
	TargetCounterparty = "counterparty_reconciliation"
	TargetField        = "status"
)

// DefaultTolerance is the difference treated as none (TL).
var DefaultTolerance = decimal.NewFromInt(1)

// Reviewer is the review queue (#14) subset.
type Reviewer interface {
	Submit(ctx context.Context, sub reviewusecase.Subject, in reviewusecase.Input) (reviewusecase.Suggestion, error)
}

// Service is the reconciliation use case layer.
type Service struct {
	pool    database.Pool
	q       *db.Queries
	review  Reviewer
	now     func() time.Time
	letters *LetterConfig
	// afterRun runs inside the run's transaction after the account items
	// are stored (the counterparty step).
	afterRun func(ctx context.Context, q *db.Queries, sc engagementctx.Scope, run db.ReconciliationRun, c collected) error
}

// New builds the service; review may be nil (sending to the queue fails).
func New(pool database.Pool, review Reviewer) *Service {
	s := &Service{pool: pool, q: database.QueriesFor(pool), review: review, now: time.Now}
	s.afterRun = s.counterpartyStep
	return s
}

// ---- views ----

// TrialBalanceRef names the compared trial balance.
type TrialBalanceRef struct {
	UUID     uuid.UUID `json:"uuid"`
	FileName string    `json:"file_name"`
}

// KindCount is the number and amount of unposted records of a kind.
type KindCount struct {
	Count  int    `json:"count"`
	Amount string `json:"amount"`
}

// BankBalance is the 102 balance of the trial balance against the bank
// statements' closing balances.
type BankBalance struct {
	LedgerBalance    string `json:"ledger_balance"`
	StatementBalance string `json:"statement_balance"`
	Difference       string `json:"difference"`
	Accounts         int    `json:"accounts"`
	Matched          bool   `json:"matched"`
}

// Summary of a run.
type Summary struct {
	Accounts         int                  `json:"accounts"`
	Matched          int                  `json:"matched"`
	Differences      int                  `json:"differences"`
	MissingInLedger  int                  `json:"missing_in_ledger"`
	MissingInSources int                  `json:"missing_in_sources"`
	DifferenceTotal  string               `json:"difference_total"`
	UnpostedCount    int                  `json:"unposted_count"`
	Unposted         map[string]KindCount `json:"unposted"`
	Cumulative       bool                 `json:"cumulative"`
	InvoicesDirect   bool                 `json:"invoices_direct"`
	Bank             *BankBalance         `json:"bank"`
	// Counterparty (cari) and BA/BS counts, filled by the counterparty step.
	Counterparties          int `json:"counterparties"`
	CounterpartyDifferences int `json:"counterparty_differences"`
	BABSIssues              int `json:"babs_issues"`
}

// Run is the API view of a reconciliation run.
type Run struct {
	ID           int64            `json:"-"`
	UUID         uuid.UUID        `json:"uuid"`
	Period       string           `json:"period"`
	WindowFrom   string           `json:"window_from"`
	WindowTo     string           `json:"window_to"`
	Tolerance    string           `json:"tolerance"`
	TrialBalance *TrialBalanceRef `json:"trial_balance"`
	Summary      Summary          `json:"summary"`
	CreatedAt    time.Time        `json:"created_at"`
}

// ReviewRef is the item's suggestion in the review queue.
type ReviewRef struct {
	UUID   uuid.UUID `json:"uuid"`
	Status string    `json:"status"`
}

// ItemView is the API view of an account comparison.
type ItemView struct {
	UUID                 uuid.UUID     `json:"uuid"`
	MainCode             string        `json:"main_code"`
	AccountName          string        `json:"account_name"`
	LedgerDebit          string        `json:"ledger_debit"`
	LedgerCredit         string        `json:"ledger_credit"`
	LedgerNet            string        `json:"ledger_net"`
	SourceDebit          string        `json:"source_debit"`
	SourceCredit         string        `json:"source_credit"`
	SourceNet            string        `json:"source_net"`
	Difference           string        `json:"difference"`
	Result               string        `json:"result"`
	Status               string        `json:"status"`
	Sources              []KindTotal   `json:"sources"`
	Unposted             []UnpostedRef `json:"unposted"`
	SubAccounts          []SubAccount  `json:"sub_accounts"`
	Suggestion           *string       `json:"suggestion"`
	SuggestionCode       *string       `json:"suggestion_code"`
	SuggestionConfidence *string       `json:"suggestion_confidence"`
	Note                 *string       `json:"note"`
	Review               *ReviewRef    `json:"review"`
	DecidedAt            *time.Time    `json:"decided_at"`
}

// RunDetail is a run with its items.
type RunDetail struct {
	Run   Run        `json:"run"`
	Items []ItemView `json:"items"`
}

// ---- running ----

// RunInput starts a comparison.
type RunInput struct {
	Period string `json:"period"`
	// TrialBalanceUUID picks the trial balance; nil = the newest upload of
	// the period.
	TrialBalanceUUID *uuid.UUID `json:"trial_balance_uuid"`
	// Tolerance in TL (default 1).
	Tolerance *string `json:"tolerance"`
}

type tbRow struct {
	id       int64
	uuid     uuid.UUID
	period   string
	fileName string
}

// ValidPeriod reports whether p is "YYYY-MM".
func ValidPeriod(p string) bool {
	if len(p) != 7 || p[4] != '-' {
		return false
	}
	y, err1 := strconv.Atoi(p[:4])
	m, err2 := strconv.Atoi(p[5:])
	return err1 == nil && err2 == nil && y >= 2000 && y <= 2100 && m >= 1 && m <= 12
}

// nextPeriod returns the month after p.
func nextPeriod(p string) string {
	t, _ := time.Parse("2006-01", p)
	return t.AddDate(0, 1, 0).Format("2006-01")
}

func periodBounds(from, to string) (time.Time, time.Time) {
	a, _ := time.Parse("2006-01", from)
	b, _ := time.Parse("2006-01", to)
	return a, b.AddDate(0, 1, -1)
}

// pickTrialBalances chooses the period's trial balance and the one whose
// totals are subtracted (an earlier period of the same year).
func pickTrialBalances(all []tbRow, period string, want *uuid.UUID) (tbRow, *tbRow, error) {
	var cur *tbRow
	for i := range all {
		t := all[i]
		if want != nil {
			if t.uuid == *want {
				cur = &all[i]
				break
			}
			continue
		}
		if t.period == period && (cur == nil || t.id > cur.id) {
			cur = &all[i]
		}
	}
	if cur == nil {
		if want != nil {
			return tbRow{}, nil, ErrNotFound
		}
		return tbRow{}, nil, ErrNoTrialBalance
	}
	if cur.period == "" {
		cur.period = period
	}
	var prev *tbRow
	for i := range all {
		t := all[i]
		if t.period == "" || t.period[:4] != period[:4] || t.period >= period {
			continue
		}
		if prev == nil || t.period > prev.period || (t.period == prev.period && t.id > prev.id) {
			prev = &all[i]
		}
	}
	return *cur, prev, nil
}

// Run compares the period's trial balance with the sources and stores the
// result. Decisions and notes of the period's previous run carry over to
// an account whose difference did not change.
func (s *Service) Run(ctx context.Context, sc engagementctx.Scope, userID int64, in RunInput) (RunDetail, error) {
	in.Period = strings.TrimSpace(in.Period)
	if !ValidPeriod(in.Period) {
		return RunDetail{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	tol := DefaultTolerance
	if in.Tolerance != nil {
		t, err := decimal.NewFromString(strings.TrimSpace(strings.ReplaceAll(*in.Tolerance, ",", ".")))
		if err != nil || t.IsNegative() || t.GreaterThan(decimal.NewFromInt(100000)) {
			return RunDetail{}, fmt.Errorf("%w: tolerance", ErrInvalid)
		}
		tol = t.Round(2)
	}
	imports, err := s.q.ListTrialBalanceImports(ctx, db.ListTrialBalanceImportsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Lim: 240})
	if err != nil {
		return RunDetail{}, err
	}
	all := make([]tbRow, 0, len(imports))
	for _, r := range imports {
		all = append(all, tbRow{id: r.ID, uuid: r.Uuid, period: r.Period.String, fileName: r.FileName})
	}
	cur, prev, err := pickTrialBalances(all, in.Period, in.TrialBalanceUUID)
	if err != nil {
		return RunDetail{}, err
	}
	windowFrom := in.Period[:4] + "-01"
	if prev != nil {
		windowFrom = nextPeriod(prev.period)
	}
	src, err := s.collect(ctx, sc, cur.id, prevID(prev), windowFrom, in.Period, tol)
	if err != nil {
		return RunDetail{}, err
	}

	var out RunDetail
	err = database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		carried := map[string]db.ListReconciliationItemsRow{}
		if last, err := q.LatestReconciliationRun(ctx, db.LatestReconciliationRunParams{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Period: in.Period,
		}); err == nil {
			rows, err := q.ListReconciliationItems(ctx, db.ListReconciliationItemsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, RunID: last.ID})
			if err != nil {
				return err
			}
			for _, r := range rows {
				carried[r.MainCode] = r
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		summary, err := json.Marshal(src.summary)
		if err != nil {
			return err
		}
		run, err := q.InsertReconciliationRun(ctx, db.InsertReconciliationRunParams{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, Period: in.Period,
			TrialBalanceImportID: pgtype.Int8{Int64: cur.id, Valid: true}, PreviousImportID: optID(prevID(prev)),
			WindowFrom: windowFrom, WindowTo: in.Period, Tolerance: num(tol), Summary: summary, CreatedBy: optID(userID),
		})
		if err != nil {
			return err
		}
		rows := make([]itemRow, 0, len(src.items))
		for _, it := range src.items {
			row := newItemRow(it)
			if old, ok := carried[it.MainCode]; ok && dec(old.Difference).Equal(it.Difference) {
				row.Note = textPtr(old.Note)
				switch old.Status {
				case StatusExplained, StatusResolved, StatusIgnored:
					row.Status = old.Status
				}
			}
			rows = append(rows, row)
		}
		body, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		if _, err := q.InsertReconciliationItems(ctx, db.InsertReconciliationItemsParams{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, RunID: run.ID, Rows: body,
		}); err != nil {
			return err
		}
		if s.afterRun != nil {
			if err := s.afterRun(ctx, q, sc, run, src); err != nil {
				return err
			}
		}
		out.Run = runView(run.ID, run.Uuid, run.Period, run.WindowFrom, run.WindowTo, run.Tolerance, run.Summary, run.CreatedAt,
			pgtype.UUID{Bytes: cur.uuid, Valid: true}, pgtype.Text{String: cur.fileName, Valid: true})
		return nil
	})
	if err != nil {
		return RunDetail{}, err
	}
	return s.Detail(ctx, sc, out.Run.UUID.String(), "")
}

func prevID(p *tbRow) int64 {
	if p == nil {
		return 0
	}
	return p.id
}

// collected is what one run compares.
type collected struct {
	items   []Item
	summary Summary
	// For the counterparty step.
	current    []LedgerLine
	windowFrom string
	windowTo   string
}

type itemRow struct {
	MainCode             string          `json:"main_code"`
	AccountName          string          `json:"account_name"`
	LedgerDebit          decimal.Decimal `json:"ledger_debit"`
	LedgerCredit         decimal.Decimal `json:"ledger_credit"`
	SourceDebit          decimal.Decimal `json:"source_debit"`
	SourceCredit         decimal.Decimal `json:"source_credit"`
	Difference           decimal.Decimal `json:"difference"`
	Result               string          `json:"result"`
	Status               string          `json:"status"`
	Sources              []KindTotal     `json:"sources"`
	Unposted             []UnpostedRef   `json:"unposted"`
	SubAccounts          []SubAccount    `json:"sub_accounts"`
	Suggestion           *string         `json:"suggestion"`
	SuggestionCode       *string         `json:"suggestion_code"`
	SuggestionConfidence *string         `json:"suggestion_confidence"`
	Note                 *string         `json:"note"`
}

func newItemRow(it Item) itemRow {
	r := itemRow{
		MainCode: it.MainCode, AccountName: it.AccountName, LedgerDebit: it.LedgerDebit, LedgerCredit: it.LedgerCredit,
		SourceDebit: it.SourceDebit, SourceCredit: it.SourceCredit, Difference: it.Difference, Result: it.Result, Status: StatusOpen,
		Sources: nonNil(it.Sources), Unposted: nonNil(it.Unposted), SubAccounts: nonNil(it.SubAccounts),
	}
	if it.Result == ResultMatched {
		r.Status = StatusResolved
	}
	if it.Suggestion != "" {
		sug, code, conf := it.Suggestion, it.SuggestCode, it.Confidence.StringFixed(4)
		r.Suggestion, r.SuggestionCode, r.SuggestionConfidence = &sug, &code, &conf
	}
	return r
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// collect reads the trial balances and the sources and compares them.
func (s *Service) collect(ctx context.Context, sc engagementctx.Scope, curID, prevImportID int64, from, to string, tol decimal.Decimal) (collected, error) {
	c := collected{windowFrom: from, windowTo: to}
	var err error
	if c.current, err = s.ledger(ctx, sc, curID); err != nil {
		return c, err
	}
	var previous []LedgerLine
	if prevImportID > 0 {
		if previous, err = s.ledger(ctx, sc, prevImportID); err != nil {
			return c, err
		}
	}
	totals, err := s.q.ReconciliationJournalTotals(ctx, db.ReconciliationJournalTotalsParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, WindowFrom: from, WindowTo: to,
	})
	if err != nil {
		return c, err
	}
	sources := make([]SourceLine, 0, len(totals))
	for _, t := range totals {
		sources = append(sources, SourceLine{
			Account: t.AccountCode, Kind: SourceKindOf(t.SourceKind, t.IsRetail), Draft: t.IsDraft,
			Debit: dec(t.Debit), Credit: dec(t.Credit), Lines: t.LineCount,
		})
	}
	direct := false
	if es, err := s.q.GetExportSettings(ctx, db.GetExportSettingsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID}); err == nil {
		direct = es.InvoicesReachLucaDirectly
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return c, err
	}
	unposted, invoiceSources, err := s.unposted(ctx, sc, from, to, direct)
	if err != nil {
		return c, err
	}
	sources = append(sources, invoiceSources...)
	c.items = Compare(Input{Current: c.current, Previous: previous, Sources: sources, Unposted: unposted, Tolerance: tol})

	sm := Summary{Unposted: map[string]KindCount{}, Cumulative: previous == nil, InvoicesDirect: direct}
	total := decimal.Zero
	for _, it := range c.items {
		sm.Accounts++
		switch it.Result {
		case ResultMatched:
			sm.Matched++
		case ResultDifference:
			sm.Differences++
		case ResultMissingInLedger:
			sm.MissingInLedger++
		case ResultMissingInSources:
			sm.MissingInSources++
		}
		if it.Result != ResultMatched {
			total = total.Add(it.Difference.Abs())
		}
	}
	sm.DifferenceTotal = total.StringFixed(2)
	amounts := map[string]decimal.Decimal{}
	for _, u := range unposted {
		k := sm.Unposted[u.Kind]
		k.Count++
		sm.Unposted[u.Kind] = k
		amounts[u.Kind] = amounts[u.Kind].Add(u.Amount)
		sm.UnpostedCount++
	}
	for k, v := range sm.Unposted {
		v.Amount = amounts[k].StringFixed(2)
		sm.Unposted[k] = v
	}
	_, end := periodBounds(from, to)
	if sm.Bank, err = s.bankBalance(ctx, sc, c.current, end, tol); err != nil {
		return c, err
	}
	c.summary = sm
	return c, nil
}

func (s *Service) ledger(ctx context.Context, sc engagementctx.Scope, importID int64) ([]LedgerLine, error) {
	rows, err := s.q.ListTrialBalanceLines(ctx, db.ListTrialBalanceLinesParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, ImportID: importID})
	if err != nil {
		return nil, err
	}
	out := make([]LedgerLine, 0, len(rows))
	for _, r := range rows {
		out = append(out, LedgerLine{Code: r.Code, Name: r.Name, MainCode: r.MainCode, Level: int(r.Level), IsLeaf: r.IsLeaf,
			Debit: dec(r.Debit), Credit: dec(r.Credit), Balance: dec(r.Balance)})
	}
	return out, nil
}

func (s *Service) bankBalance(ctx context.Context, sc engagementctx.Scope, cur []LedgerLine, end time.Time, tol decimal.Decimal) (*BankBalance, error) {
	rows, err := s.q.ReconciliationBankClosingBalances(ctx, db.ReconciliationBankClosingBalancesParams{
		TaxpayerID: sc.TaxpayerID, DateTo: pgtype.Date{Time: end, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	statement, n := decimal.Zero, 0
	for _, r := range rows {
		if r.Currency != "TRY" {
			continue
		}
		statement = statement.Add(dec(r.BalanceAfter))
		n++
	}
	if n == 0 {
		return nil, nil
	}
	ledger := decimal.Zero
	for _, l := range cur {
		if l.IsLeaf && MainCodeOf(l.Code) == "102" {
			ledger = ledger.Add(l.Debit.Sub(l.Credit))
		}
	}
	diff := ledger.Sub(statement).Round(2)
	return &BankBalance{
		LedgerBalance: ledger.StringFixed(2), StatementBalance: statement.StringFixed(2), Difference: diff.StringFixed(2),
		Accounts: n, Matched: diff.Abs().LessThanOrEqual(tol),
	}, nil
}

// tryAmount converts an invoice amount to TL (false when the rate is
// unknown).
func tryAmount(v decimal.Decimal, currency string, rate pgtype.Numeric) (decimal.Decimal, bool) {
	if currency == "" || currency == "TRY" {
		return v, true
	}
	r := dec(rate)
	if r.IsZero() {
		return decimal.Zero, false
	}
	return v.Mul(r).Round(2), true
}

// InvoiceEffects is what an invoice posts per main account (debit −
// credit). The expense side of a purchase is not known here (it is the
// account code decision) and is left out.
func InvoiceEffects(direction, typ string, net, tax, payable decimal.Decimal) map[string]decimal.Decimal {
	ret := typ == "IADE"
	switch {
	case direction == "outgoing" && !ret:
		return map[string]decimal.Decimal{"120": payable, "600": net.Neg(), "391": tax.Neg()}
	case direction == "outgoing" && ret: // iade to the supplier
		return map[string]decimal.Decimal{"320": payable, "191": tax.Neg()}
	case direction == "incoming" && ret: // a customer's return
		return map[string]decimal.Decimal{"120": payable.Neg(), "610": net, "391": tax}
	default:
		return map[string]decimal.Decimal{"320": payable.Neg(), "191": tax}
	}
}

// unposted lists the sources' records that are in no journal draft. When
// invoices reach the accounting program directly (export settings), the
// unposted invoices are a source of their own instead of missing entries.
func (s *Service) unposted(ctx context.Context, sc engagementctx.Scope, from, to string, invoicesDirect bool) ([]Unposted, []SourceLine, error) {
	start, end := periodBounds(from, to)
	var out []Unposted
	var invoiceSources []SourceLine

	bank, err := s.q.ReconciliationUnpostedBank(ctx, db.ReconciliationUnpostedBankParams{
		TaxpayerID: sc.TaxpayerID, OrganizationID: sc.OrganizationID, EngagementID: sc.ID,
		DateFrom: pgtype.Date{Time: start, Valid: true}, DateTo: pgtype.Date{Time: end, Valid: true},
	})
	if err != nil {
		return nil, nil, err
	}
	for _, b := range bank {
		amt := dec(b.Amount).Round(2)
		u := Unposted{Kind: KindBank, ID: b.Uuid.String(), Date: day(b.TransactionDate), Label: b.Label, Amount: amt}
		if b.Currency == "TRY" {
			eff := amt
			if b.DebitCredit == "B" {
				eff = amt.Neg()
			}
			u.Effects = map[string]decimal.Decimal{"102": eff}
		}
		out = append(out, u)
	}

	invoices, err := s.q.ReconciliationUnpostedInvoices(ctx, db.ReconciliationUnpostedInvoicesParams{
		TaxpayerID: sc.TaxpayerID, OrganizationID: sc.OrganizationID, EngagementID: sc.ID, WindowFrom: from, WindowTo: to,
	})
	if err != nil {
		return nil, nil, err
	}
	for _, i := range invoices {
		net, ok1 := tryAmount(dec(i.TaxExclusiveAmount), i.Currency, i.ExchangeRate)
		tax, ok2 := tryAmount(dec(i.TaxAmount), i.Currency, i.ExchangeRate)
		payable, ok3 := tryAmount(dec(i.PayableAmount), i.Currency, i.ExchangeRate)
		if i.PayableAmountTry.Valid {
			payable, ok3 = dec(i.PayableAmountTry), true
		}
		label := strings.TrimSpace(i.InvoiceNumber + " " + i.CounterpartyName)
		u := Unposted{Kind: KindInvoice, ID: i.Uuid.String(), Date: day(i.IssueDate), Label: label, Amount: payable.Round(2)}
		if ok1 && ok2 && ok3 {
			u.Effects = InvoiceEffects(i.Direction, i.Type, net.Round(2), tax.Round(2), payable.Round(2))
		}
		if invoicesDirect {
			for main, eff := range u.Effects {
				line := SourceLine{Account: main, Kind: KindInvoice, Lines: 1}
				if eff.IsNegative() {
					line.Credit = eff.Neg()
				} else {
					line.Debit = eff
				}
				invoiceSources = append(invoiceSources, line)
			}
			continue
		}
		out = append(out, u)
	}

	expenses, err := s.q.ReconciliationUnpostedExpenses(ctx, db.ReconciliationUnpostedExpensesParams{
		TaxpayerID: sc.TaxpayerID, OrganizationID: sc.OrganizationID, EngagementID: sc.ID, WindowFrom: from, WindowTo: to,
	})
	if err != nil {
		return nil, nil, err
	}
	for _, e := range expenses {
		total, tax := dec(e.Amount).Round(2), dec(e.TaxAmount).Round(2)
		main := MainCodeOf(e.AccountCode)
		if main == "" {
			main = "770"
		}
		u := Unposted{Kind: KindExpense, ID: e.Uuid.String(), Date: day(e.ExpenseDate), Label: e.Label, Amount: total}
		if e.Currency == "" || e.Currency == "TRY" {
			u.Effects = map[string]decimal.Decimal{main: total.Sub(tax)}
			if !tax.IsZero() {
				u.Effects["191"] = u.Effects["191"].Add(tax)
			}
		}
		out = append(out, u)
	}

	retail, err := s.q.ReconciliationUnpostedRetailSales(ctx, db.ReconciliationUnpostedRetailSalesParams{
		TaxpayerID: sc.TaxpayerID, OrganizationID: sc.OrganizationID, EngagementID: sc.ID, WindowFrom: from, WindowTo: to,
	})
	if err != nil {
		return nil, nil, err
	}
	for _, r := range retail {
		label := "Z raporu"
		if r.ZReportNumber != "" {
			label += " " + r.ZReportNumber
		}
		out = append(out, Unposted{Kind: KindZReport, ID: r.Uuid.String(), Date: day(r.SaleDate), Label: label,
			Amount:  dec(r.TotalAmount).Round(2),
			Effects: map[string]decimal.Decimal{"600": dec(r.SubtotalAmount).Round(2).Neg(), "391": dec(r.TaxAmount).Round(2).Neg()}})
	}

	runs, err := s.q.ReconciliationUnpostedPayrollRuns(ctx, db.ReconciliationUnpostedPayrollRunsParams{
		TaxpayerID: sc.TaxpayerID, WindowFrom: from, WindowTo: to,
	})
	if err != nil {
		return nil, nil, err
	}
	for _, r := range runs {
		expense := dec(r.GrossTotal).Add(dec(r.SgkEmployerTotal)).Add(dec(r.UnemploymentEmployerTotal)).Sub(dec(r.SgkIncentiveTotal)).Round(2)
		out = append(out, Unposted{Kind: KindPayroll, ID: r.Uuid.String(), Date: r.Period, Label: r.Period + " bordrosu",
			Amount: expense, Effects: map[string]decimal.Decimal{"770": expense, "335": dec(r.NetTotal).Round(2).Neg()}})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, invoiceSources, nil
}

// ---- reading ----

// Runs lists the engagement's runs, newest period first.
func (s *Service) Runs(ctx context.Context, sc engagementctx.Scope, period string) ([]Run, error) {
	if period != "" && !ValidPeriod(period) {
		return nil, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	rows, err := s.q.ListReconciliationRuns(ctx, db.ListReconciliationRunsParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Period: optText(period), Lim: 100,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Run, 0, len(rows))
	for _, r := range rows {
		out = append(out, runView(r.ID, r.Uuid, r.Period, r.WindowFrom, r.WindowTo, r.Tolerance, r.Summary, r.CreatedAt, r.TrialBalanceUuid, r.TrialBalanceFileName))
	}
	return out, nil
}

// Detail returns a run ("latest" or a uuid; latest may be narrowed to a
// period) with its items.
func (s *Service) Detail(ctx context.Context, sc engagementctx.Scope, which, period string) (RunDetail, error) {
	run, err := s.pickRun(ctx, sc, which, period)
	if err != nil {
		return RunDetail{}, err
	}
	rows, err := s.q.ListReconciliationItems(ctx, db.ListReconciliationItemsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, RunID: run.ID})
	if err != nil {
		return RunDetail{}, err
	}
	out := RunDetail{Run: run, Items: make([]ItemView, 0, len(rows))}
	for _, r := range rows {
		out.Items = append(out.Items, itemView(db.ReconciliationItem{
			ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, EngagementID: r.EngagementID, RunID: r.RunID,
			MainCode: r.MainCode, AccountName: r.AccountName, LedgerDebit: r.LedgerDebit, LedgerCredit: r.LedgerCredit,
			SourceDebit: r.SourceDebit, SourceCredit: r.SourceCredit, Difference: r.Difference, Result: r.Result, Status: r.Status,
			Sources: r.Sources, Unposted: r.Unposted, SubAccounts: r.SubAccounts, Suggestion: r.Suggestion, SuggestionCode: r.SuggestionCode,
			SuggestionConfidence: r.SuggestionConfidence, Note: r.Note, ReviewSuggestionID: r.ReviewSuggestionID, DecidedAt: r.DecidedAt,
		}, r.ReviewUuid, r.ReviewStatus))
	}
	return out, nil
}

func (s *Service) pickRun(ctx context.Context, sc engagementctx.Scope, which, period string) (Run, error) {
	if which == "" || which == "latest" {
		if period != "" && !ValidPeriod(period) {
			return Run{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
		}
		rows, err := s.q.ListReconciliationRuns(ctx, db.ListReconciliationRunsParams{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Period: optText(period), Lim: 1,
		})
		if err != nil {
			return Run{}, err
		}
		if len(rows) == 0 {
			return Run{}, ErrNoRun
		}
		r := rows[0]
		return runView(r.ID, r.Uuid, r.Period, r.WindowFrom, r.WindowTo, r.Tolerance, r.Summary, r.CreatedAt, r.TrialBalanceUuid, r.TrialBalanceFileName), nil
	}
	id, err := uuid.Parse(which)
	if err != nil {
		return Run{}, ErrNotFound
	}
	r, err := s.q.GetReconciliationRun(ctx, db.GetReconciliationRunParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, err
	}
	return runView(r.ID, r.Uuid, r.Period, r.WindowFrom, r.WindowTo, r.Tolerance, r.Summary, r.CreatedAt, r.TrialBalanceUuid, r.TrialBalanceFileName), nil
}

func runView(id int64, u uuid.UUID, period, from, to string, tol pgtype.Numeric, summary []byte, created pgtype.Timestamptz, tb pgtype.UUID, file pgtype.Text) Run {
	r := Run{ID: id, UUID: u, Period: period, WindowFrom: from, WindowTo: to, Tolerance: dec(tol).StringFixed(2), CreatedAt: created.Time}
	_ = json.Unmarshal(summary, &r.Summary)
	if r.Summary.Unposted == nil {
		r.Summary.Unposted = map[string]KindCount{}
	}
	if tb.Valid {
		r.TrialBalance = &TrialBalanceRef{UUID: tb.Bytes, FileName: file.String}
	}
	return r
}

// EffectiveStatus folds the review queue's decision into the item's status:
// a rejected or reverted suggestion reopens it.
func EffectiveStatus(status string, reviewStatus pgtype.Text) string {
	if status != StatusInReview || !reviewStatus.Valid {
		return status
	}
	switch reviewusecase.Status(reviewStatus.String) {
	case reviewusecase.StatusRejected, reviewusecase.StatusReverted, reviewusecase.StatusSuperseded:
		return StatusOpen
	}
	return status
}

func itemView(r db.ReconciliationItem, reviewUUID pgtype.UUID, reviewStatus pgtype.Text) ItemView {
	ledger := dec(r.LedgerDebit).Sub(dec(r.LedgerCredit))
	source := dec(r.SourceDebit).Sub(dec(r.SourceCredit))
	v := ItemView{
		UUID: r.Uuid, MainCode: r.MainCode, AccountName: r.AccountName,
		LedgerDebit: dec(r.LedgerDebit).StringFixed(2), LedgerCredit: dec(r.LedgerCredit).StringFixed(2), LedgerNet: ledger.StringFixed(2),
		SourceDebit: dec(r.SourceDebit).StringFixed(2), SourceCredit: dec(r.SourceCredit).StringFixed(2), SourceNet: source.StringFixed(2),
		Difference: dec(r.Difference).StringFixed(2), Result: r.Result, Status: EffectiveStatus(r.Status, reviewStatus),
		Suggestion: textPtr(r.Suggestion), SuggestionCode: textPtr(r.SuggestionCode), Note: textPtr(r.Note),
		Sources: []KindTotal{}, Unposted: []UnpostedRef{}, SubAccounts: []SubAccount{},
	}
	if r.SuggestionConfidence.Valid {
		c := dec(r.SuggestionConfidence).StringFixed(2)
		v.SuggestionConfidence = &c
	}
	_ = json.Unmarshal(r.Sources, &v.Sources)
	_ = json.Unmarshal(r.Unposted, &v.Unposted)
	_ = json.Unmarshal(r.SubAccounts, &v.SubAccounts)
	if reviewUUID.Valid {
		v.Review = &ReviewRef{UUID: reviewUUID.Bytes, Status: reviewStatus.String}
	}
	if r.DecidedAt.Valid {
		t := r.DecidedAt.Time
		v.DecidedAt = &t
	}
	return v
}

// ---- decisions ----

// DecisionInput is the accountant's decision on an item.
type DecisionInput struct {
	Status string  `json:"status"`
	Note   *string `json:"note"`
}

const maxNote = 2000

func cleanNote(n *string) (*string, error) {
	if n == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*n)
	if v == "" {
		return nil, nil
	}
	if len([]rune(v)) > maxNote {
		return nil, fmt.Errorf("%w: note is longer than %d characters", ErrInvalid, maxNote)
	}
	return &v, nil
}

// Decide sets an item's status and note.
func (s *Service) Decide(ctx context.Context, sc engagementctx.Scope, itemUUID uuid.UUID, userID int64, in DecisionInput) (ItemView, error) {
	if !ValidDecision(in.Status) {
		return ItemView{}, fmt.Errorf("%w: status must be open, explained, resolved or ignored", ErrInvalid)
	}
	note, err := cleanNote(in.Note)
	if err != nil {
		return ItemView{}, err
	}
	if in.Status == StatusExplained && note == nil {
		return ItemView{}, fmt.Errorf("%w: an explanation needs a note", ErrInvalid)
	}
	it, err := s.q.GetReconciliationItem(ctx, db.GetReconciliationItemParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: itemUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ItemView{}, ErrNotFound
	}
	if err != nil {
		return ItemView{}, err
	}
	if EffectiveStatus(it.Status, it.ReviewStatus) == StatusInReview && it.ReviewStatus.String == string(reviewusecase.StatusPending) {
		return ItemView{}, ErrInReview
	}
	row, err := s.q.UpdateReconciliationItemDecision(ctx, db.UpdateReconciliationItemDecisionParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, ID: it.ID, Status: in.Status, Note: ptrText(note), DecidedBy: optID(userID),
	})
	if err != nil {
		return ItemView{}, err
	}
	return itemView(row, it.ReviewUuid, it.ReviewStatus), nil
}

// ReviewValue is the value of a reconciliation suggestion: the status the
// item gets when the suggestion is accepted, with the explanation.
type ReviewValue struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

// SendToReview puts an item with a difference into the review queue (#14)
// with the explanation (the given note, else the engine's suggestion).
func (s *Service) SendToReview(ctx context.Context, sc engagementctx.Scope, itemUUID uuid.UUID, userID int64, note *string) (ItemView, error) {
	if s.review == nil {
		return ItemView{}, ErrNoReview
	}
	n, err := cleanNote(note)
	if err != nil {
		return ItemView{}, err
	}
	it, err := s.q.GetReconciliationItem(ctx, db.GetReconciliationItemParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: itemUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ItemView{}, ErrNotFound
	}
	if err != nil {
		return ItemView{}, err
	}
	if it.Result == ResultMatched {
		return ItemView{}, ErrMatched
	}
	if it.ReviewStatus.Valid && it.ReviewStatus.String == string(reviewusecase.StatusPending) {
		return ItemView{}, ErrInReview
	}
	explanation := it.Suggestion.String
	if n != nil {
		explanation = *n
	}
	if explanation == "" {
		explanation = "Mutabakat farkı incelensin."
	}
	value, _ := json.Marshal(ReviewValue{Status: StatusResolved, Note: explanation})
	conf := dec(it.SuggestionConfidence)
	if limit := decimal.NewFromFloat(0.5); conf.GreaterThan(limit) {
		// Sent by a person to be decided by a person: never above the usual
		// auto-accept thresholds.
		conf = limit
	}
	diff := dec(it.Difference)
	rationale := fmt.Sprintf("%s %s hesabında mizan ile kaynaklar arasında %s fark. %s",
		it.MainCode, it.AccountName, Money(diff.Abs()), explanation)
	id := it.ID
	sug, err := s.review.Submit(ctx, reviewusecase.Subject{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID},
		reviewusecase.Input{
			Kind: SuggestionKind, TargetType: TargetItem, TargetID: &id, TargetField: TargetField, Period: it.RunPeriod,
			Value: value, Confidence: conf, Rationale: strings.TrimSpace(rationale),
			Provenance: reviewusecase.Provenance{
				Origin: reviewusecase.OriginRule, RuleRef: "reconciliation:" + orDefault(it.SuggestionCode.String, "manual"),
				Inputs: map[string]any{
					"run": it.RunUuid.String(), "main_code": it.MainCode, "result": it.Result,
					"ledger_net": dec(it.LedgerDebit).Sub(dec(it.LedgerCredit)).StringFixed(2),
					"source_net": dec(it.SourceDebit).Sub(dec(it.SourceCredit)).StringFixed(2),
					"difference": diff.StringFixed(2),
				},
			},
		})
	if err != nil {
		return ItemView{}, err
	}
	if sug.Status.Applied() {
		// Auto-accepted by the office's policy: the applier resolved it.
		return s.itemByUUID(ctx, sc, itemUUID)
	}
	if err := s.q.SetReconciliationItemReview(ctx, db.SetReconciliationItemReviewParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, ID: it.ID, ReviewSuggestionID: pgtype.Int8{Int64: sug.ID, Valid: true}, DecidedBy: optID(userID),
	}); err != nil {
		return ItemView{}, err
	}
	return s.itemByUUID(ctx, sc, itemUUID)
}

func (s *Service) itemByUUID(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (ItemView, error) {
	it, err := s.q.GetReconciliationItem(ctx, db.GetReconciliationItemParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return ItemView{}, ErrNotFound
	}
	if err != nil {
		return ItemView{}, err
	}
	return itemView(db.ReconciliationItem{
		ID: it.ID, Uuid: it.Uuid, OrganizationID: it.OrganizationID, EngagementID: it.EngagementID, RunID: it.RunID,
		MainCode: it.MainCode, AccountName: it.AccountName, LedgerDebit: it.LedgerDebit, LedgerCredit: it.LedgerCredit,
		SourceDebit: it.SourceDebit, SourceCredit: it.SourceCredit, Difference: it.Difference, Result: it.Result, Status: it.Status,
		Sources: it.Sources, Unposted: it.Unposted, SubAccounts: it.SubAccounts, Suggestion: it.Suggestion, SuggestionCode: it.SuggestionCode,
		SuggestionConfidence: it.SuggestionConfidence, Note: it.Note, ReviewSuggestionID: it.ReviewSuggestionID, DecidedAt: it.DecidedAt,
	}, it.ReviewUuid, it.ReviewStatus), nil
}

// ---- helpers ----

func dec(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

func num(d decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

func optID(id int64) pgtype.Int8 { return pgtype.Int8{Int64: id, Valid: id > 0} }

func optText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func ptrText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func day(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
