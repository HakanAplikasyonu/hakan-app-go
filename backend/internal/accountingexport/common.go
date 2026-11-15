package accountingexport

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// Totals returns Σ debit and Σ credit of an entry.
func (e Entry) Totals() (debit, credit decimal.Decimal) {
	for _, l := range e.Lines {
		debit = debit.Add(l.Debit)
		credit = credit.Add(l.Credit)
	}
	return debit, credit
}

// Balanced reports Σ debit = Σ credit (exact; amounts are decimal).
func (e Entry) Balanced() bool {
	d, c := e.Totals()
	return d.Equal(c)
}

// IsInvoiceEntry reports whether any line comes from an invoice.
func (e Entry) IsInvoiceEntry() bool {
	for _, l := range e.Lines {
		if l.Source.Kind == "invoice" {
			return true
		}
	}
	return false
}

// SelectEntries applies the double-booking rule (docs/02 §4): when invoices
// already reach the program directly, invoice-sourced entries stay out unless
// the accountant chose to include them. Skipped entries come back as info
// issues.
func SelectEntries(b Batch) ([]Entry, []Issue) {
	if !b.Settings.InvoicesReachLucaDirectly || b.Settings.IncludeInvoiceEntries {
		return b.Entries, nil
	}
	var out []Entry
	var issues []Issue
	for _, e := range b.Entries {
		if e.IsInvoiceEntry() {
			issues = append(issues, Issue{Code: IssueInvoiceEntryExcluded, Severity: SeverityInfo, EntryNo: e.No})
			continue
		}
		out = append(out, e)
	}
	return out, issues
}

// CommonIssues runs the checks every driver needs before it sees a batch
// (docs/02 §2): each entry has lines, is balanced (blocker), and every account
// is in the target plan when one is loaded (warning, or blocker when
// Settings.BlockOnUnknownAccount).
func CommonIssues(b Batch) []Issue {
	entries, issues := SelectEntries(b)
	planSeverity := SeverityWarning
	if b.Settings.BlockOnUnknownAccount {
		planSeverity = SeverityBlocker
	}
	for _, e := range entries {
		if len(e.Lines) == 0 {
			issues = append(issues, Issue{Code: IssueEmptyEntry, Severity: SeverityBlocker, EntryNo: e.No})
			continue
		}
		if d, c := e.Totals(); !d.Equal(c) {
			issues = append(issues, Issue{Code: IssueUnbalanced, Severity: SeverityBlocker, EntryNo: e.No, Params: map[string]any{
				"debit": d.StringFixed(2), "credit": c.StringFixed(2), "difference": d.Sub(c).StringFixed(2),
			}})
		}
		if b.TargetPlan == nil {
			continue
		}
		for i, l := range e.Lines {
			if !b.TargetPlan.Has(l.Account) {
				issues = append(issues, Issue{Code: IssueAccountNotInPlan, Severity: planSeverity, EntryNo: e.No, Line: i + 1,
					Params: map[string]any{"account": l.Account}})
			}
		}
	}
	return issues
}

// SplitIntoFiles cuts entries into groups of at most max (0 = one group).
func SplitIntoFiles(entries []Entry, maxPerFile int) [][]Entry {
	if maxPerFile <= 0 || len(entries) <= maxPerFile {
		if len(entries) == 0 {
			return nil
		}
		return [][]Entry{entries}
	}
	var out [][]Entry
	for start := 0; start < len(entries); start += maxPerFile {
		end := min(start+maxPerFile, len(entries))
		out = append(out, entries[start:end])
	}
	return out
}

// SplitEntry cuts an entry longer than maxLines into pieces of at most
// maxLines lines, each balanced on its own (docs/02 §3.1). Lines keep their
// order; a cut is only made where the running Σ debit − Σ credit is zero, and
// the latest such point that fits is taken. Pieces are numbered
// "<No>-1", "<No>-2" … so a program that groups by number and date keeps them
// apart. ok is false when no balanced split exists.
func SplitEntry(e Entry, maxLines int) (pieces []Entry, ok bool) {
	if maxLines <= 0 || len(e.Lines) <= maxLines {
		return []Entry{e}, true
	}
	// zero[i]: the prefix lines[:i] is balanced.
	zero := make([]bool, len(e.Lines)+1)
	run := decimal.Zero
	zero[0] = true
	for i, l := range e.Lines {
		run = run.Add(l.Debit).Sub(l.Credit)
		zero[i+1] = run.IsZero()
	}
	if !zero[len(e.Lines)] {
		return nil, false
	}
	var cuts []int
	start := 0
	for start < len(e.Lines) {
		end := -1
		for i := min(start+maxLines, len(e.Lines)); i > start; i-- {
			if zero[i] {
				end = i
				break
			}
		}
		if end < 0 {
			return nil, false
		}
		cuts = append(cuts, end)
		start = end
	}
	prev := 0
	for n, c := range cuts {
		p := e
		p.No = fmt.Sprintf("%s-%d", e.No, n+1)
		p.Lines = append([]Line(nil), e.Lines[prev:c]...)
		pieces = append(pieces, p)
		prev = c
	}
	return pieces, true
}
