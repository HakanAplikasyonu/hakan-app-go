// Package controls runs the accountant's smart checks (issue #31, 06 T3)
// over one engagement period: FIXED_ASSET_CANDIDATE, UNUSUAL_RECORD_TYPE,
// UNUSUAL_AMOUNT and NEW_PARTY_HIGH_AMOUNT. It reads invoices and their
// history from the common model and hands them to the rules (warnings/rules)
// through the engine, so thresholds come from configuration and re-runs do
// not duplicate warnings.
package controls

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/rules"
)

// Queries is the db subset used here.
type Queries interface {
	ListControlInvoices(ctx context.Context, arg db.ListControlInvoicesParams) ([]db.ListControlInvoicesRow, error)
	ListControlInvoiceItems(ctx context.Context, arg db.ListControlInvoiceItemsParams) ([]db.ListControlInvoiceItemsRow, error)
	PartyAmountsBefore(ctx context.Context, arg db.PartyAmountsBeforeParams) ([]pgtype.Numeric, error)
	TaxpayerAmountsBefore(ctx context.Context, arg db.TaxpayerAmountsBeforeParams) ([]pgtype.Numeric, error)
	PartySeenBefore(ctx context.Context, arg db.PartySeenBeforeParams) (bool, error)
	ItemNamesHistory(ctx context.Context, arg db.ItemNamesHistoryParams) ([]db.ItemNamesHistoryRow, error)
}

// ConnectionChecker refreshes the engagement's connection health warnings
// (#669 K, warnings/connections).
type ConnectionChecker interface {
	Check(ctx context.Context, s engine.Subject) (engine.Report, error)
}

// Runner runs the controls.
type Runner struct {
	q           Queries
	engine      *engine.Engine
	connections ConnectionChecker
}

// New builds a runner.
func New(q Queries, e *engine.Engine) *Runner { return &Runner{q: q, engine: e} }

// WithConnections also refreshes the connection warnings on every run, so
// "kontrolleri çalıştır" does not wait for the periodic sweep.
func (r *Runner) WithConnections(c ConnectionChecker) *Runner { r.connections = c; return r }

// Result summarizes one run.
type Result struct {
	Period   string        `json:"period"`
	Invoices int           `json:"invoices"`
	Items    int           `json:"items"`
	Report   engine.Report `json:"report"`
}

var periodRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// ErrInvalidPeriod is returned for a malformed period.
var ErrInvalidPeriod = fmt.Errorf("controls: period must be YYYY-MM")

// Run checks every invoice and line of the period.
func (r *Runner) Run(ctx context.Context, s engine.Subject, period string) (Result, error) {
	res := Result{Period: period}
	if !periodRe.MatchString(period) {
		return res, ErrInvalidPeriod
	}
	start, _ := time.Parse("2006-01", period)
	yearAgo := start.AddDate(-1, 0, 0)

	invoices, err := r.q.ListControlInvoices(ctx, db.ListControlInvoicesParams{TaxpayerID: s.TaxpayerID, Period: period})
	if err != nil {
		return res, fmt.Errorf("controls: invoices: %w", err)
	}
	res.Invoices = len(invoices)
	for _, inv := range invoices {
		in, err := r.amountInput(ctx, s, period, inv)
		if err != nil {
			return res, err
		}
		rep, err := engine.Run(ctx, r.engine, s, rules.Amounts{}, in)
		if err != nil {
			return res, err
		}
		add(&res.Report, rep)
	}

	items, err := r.q.ListControlInvoiceItems(ctx, db.ListControlInvoiceItemsParams{TaxpayerID: s.TaxpayerID, Period: period})
	if err != nil {
		return res, fmt.Errorf("controls: items: %w", err)
	}
	res.Items = len(items)
	history := map[string][]string{}
	historyLines := map[string]int{}
	for _, dir := range []string{"incoming", "outgoing"} {
		rows, err := r.q.ItemNamesHistory(ctx, db.ItemNamesHistoryParams{
			TaxpayerID: s.TaxpayerID, Direction: dir, FromDate: date(yearAgo), BeforeDate: date(start),
		})
		if err != nil {
			return res, fmt.Errorf("controls: item history: %w", err)
		}
		for _, h := range rows {
			history[dir] = append(history[dir], h.Name)
			historyLines[dir] += int(h.Lines)
		}
	}
	for _, it := range items {
		rep, err := engine.Run(ctx, r.engine, s, rules.Items{}, rules.ItemInput{
			ItemID: it.ID, Period: period, Incoming: it.Direction == "incoming", Name: it.Name,
			Amount: dec(it.TaxExclusiveAmount), History: history[it.Direction], HistoryLines: historyLines[it.Direction],
		})
		if err != nil {
			return res, err
		}
		add(&res.Report, rep)
	}
	if r.connections != nil {
		rep, err := r.connections.Check(ctx, s)
		if err != nil {
			return res, fmt.Errorf("controls: connections: %w", err)
		}
		add(&res.Report, rep)
	}
	return res, nil
}

func (r *Runner) amountInput(ctx context.Context, s engine.Subject, period string, inv db.ListControlInvoicesRow) (rules.AmountInput, error) {
	in := rules.AmountInput{InvoiceID: inv.ID, Period: period, Amount: dec(inv.PayableAmount)}
	from := date(inv.IssueDate.Time.AddDate(-1, 0, 0))
	if inv.PartyKey != "" {
		amounts, err := r.q.PartyAmountsBefore(ctx, db.PartyAmountsBeforeParams{
			TaxpayerID: s.TaxpayerID, Direction: inv.Direction, PartyKey: inv.PartyKey,
			FromDate: from, BeforeDate: inv.IssueDate, BeforeID: inv.ID,
		})
		if err != nil {
			return in, fmt.Errorf("controls: party history: %w", err)
		}
		in.History = decs(amounts)
		seen, err := r.q.PartySeenBefore(ctx, db.PartySeenBeforeParams{
			TaxpayerID: s.TaxpayerID, PartyKey: inv.PartyKey, BeforeDate: inv.IssueDate, BeforeID: inv.ID,
		})
		if err != nil {
			return in, fmt.Errorf("controls: party seen: %w", err)
		}
		in.FirstSeen = !seen
	}
	// 06 T3: no counterparty history -> the taxpayer's own average.
	if len(in.History) == 0 {
		amounts, err := r.q.TaxpayerAmountsBefore(ctx, db.TaxpayerAmountsBeforeParams{
			TaxpayerID: s.TaxpayerID, Direction: inv.Direction, FromDate: from, BeforeDate: inv.IssueDate, BeforeID: inv.ID,
		})
		if err != nil {
			return in, fmt.Errorf("controls: taxpayer history: %w", err)
		}
		in.History = decs(amounts)
	}
	return in, nil
}

func add(t *engine.Report, r engine.Report) {
	t.Created += r.Created
	t.Refreshed += r.Refreshed
	t.Suppressed += r.Suppressed
	t.Disabled += r.Disabled
	t.AutoResolved += r.AutoResolved
}

func date(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }

func dec(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

func decs(ns []pgtype.Numeric) []decimal.Decimal {
	out := make([]decimal.Decimal, 0, len(ns))
	for _, n := range ns {
		out = append(out, dec(n))
	}
	return out
}
