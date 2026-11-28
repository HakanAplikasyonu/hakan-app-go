package posting

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
)

// Z continuity warning codes.
const (
	CodeZNumberGap       = "Z_NUMBER_GAP"
	CodeZNumberDuplicate = "Z_NUMBER_DUPLICATE"
	CodeZDayGap          = "Z_DAY_GAP"
)

// ZCodes are the codes the continuity check owns.
var ZCodes = []string{CodeZNumberGap, CodeZNumberDuplicate, CodeZDayGap}

// ZDay is one Z report of a device.
type ZDay struct {
	ID     int64
	Date   time.Time
	Number string
}

// ZFinding is one continuity problem on a Z report.
type ZFinding struct {
	Code   string
	Params map[string]any
}

// MaxGapDays is the default longest gap between two Z reports of a device
// that is not reported (warning_codes.default_params of Z_DAY_GAP).
const MaxGapDays = 1

// Continuity checks the Z reports of one ÖKC (sorted here by date, then
// number): a number skipped between two consecutive reports
// (Z_NUMBER_GAP), the same number on another day (Z_NUMBER_DUPLICATE) and
// more than maxGapDays days without a report (Z_DAY_GAP, the device may be
// closed on those days). The finding is on the later report. Reports
// without a date are skipped; without a numeric Z number only the day gap
// is checked.
func Continuity(days []ZDay, maxGapDays int) map[int64][]ZFinding {
	if maxGapDays < 1 {
		maxGapDays = MaxGapDays
	}
	rows := make([]ZDay, 0, len(days))
	for _, d := range days {
		if !d.Date.IsZero() {
			rows = append(rows, d)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].Date.Equal(rows[j].Date) {
			return rows[i].Date.Before(rows[j].Date)
		}
		ni, _ := zNumber(rows[i].Number)
		nj, _ := zNumber(rows[j].Number)
		return ni < nj
	})
	out := map[int64][]ZFinding{}
	seen := map[int64]ZDay{}
	for i, cur := range rows {
		n, okN := zNumber(cur.Number)
		if okN {
			if first, dup := seen[n]; dup && !first.Date.Equal(cur.Date) {
				out[cur.ID] = append(out[cur.ID], ZFinding{Code: CodeZNumberDuplicate, Params: map[string]any{
					"z_number": cur.Number, "first_date": first.Date.Format(time.DateOnly), "date": cur.Date.Format(time.DateOnly)}})
			} else if !dup {
				seen[n] = cur
			}
		}
		if i == 0 {
			continue
		}
		prev := rows[i-1]
		if p, okP := zNumber(prev.Number); okN && okP && n > p+1 {
			out[cur.ID] = append(out[cur.ID], ZFinding{Code: CodeZNumberGap, Params: map[string]any{
				"from": strconv.FormatInt(p+1, 10), "to": strconv.FormatInt(n-1, 10), "missing": n - p - 1,
				"previous_z": prev.Number, "z_number": cur.Number}})
		}
		if gap := int(cur.Date.Sub(prev.Date).Hours()/24 + 0.5); gap > maxGapDays {
			out[cur.ID] = append(out[cur.ID], ZFinding{Code: CodeZDayGap, Params: map[string]any{
				"previous_date": prev.Date.Format(time.DateOnly), "date": cur.Date.Format(time.DateOnly), "days": gap - 1}})
		}
	}
	return out
}

// zNumber parses a Z number ("0123", "Z 123", "Z-0123").
func zNumber(s string) (int64, bool) {
	s = strings.TrimLeft(strings.TrimSpace(strings.ToUpper(s)), "Z -:#NO.")
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil && n >= 0
}

// ZVatLine is one rate of a Z report: base (excl. VAT) and VAT.
type ZVatLine struct {
	Rate decimal.Decimal
	Base decimal.Decimal
	Tax  decimal.Decimal
}

// ZInput is an approved Z report.
type ZInput struct {
	SourceID string // "retail_sale:<uuid>"
	Date     time.Time
	ZNumber  string
	OKC      string
	Total    decimal.Decimal
	Tax      decimal.Decimal // total VAT, used when there are no rate lines
	Cash     *decimal.Decimal
	Card     *decimal.Decimal
	Lines    []ZVatLine
	Plan     []engine.PlanAccount
}

// ErrZBreakdown: the rate lines do not add up to the Z total.
var ErrZBreakdown = errors.New("posting: VAT breakdown does not match the Z total")

// zTolerance absorbs OCR / rounding differences into the largest base.
var zTolerance = decimal.RequireFromString("0.10")

// ZEntry is the sales entry of a Z report: 100 Kasa (cash) and 108 Diğer
// hazır değerler (card / POS) debit; per VAT rate 600 Yurt içi satışlar
// (base) and 391 Hesaplanan KDV credit. Without a payment breakdown the
// whole total is cash; without rate lines the total VAT is one line.
//
// Amounts are rounded to kuruş first (total, payments, every base and VAT),
// so the difference absorbed into the largest base is in kuruş too and the
// rounded lines still balance.
func ZEntry(in ZInput) (Entry, error) {
	in.Total = in.Total.Round(2)
	if !in.Total.IsPositive() {
		return Entry{}, errNoAmount
	}
	in.Tax = in.Tax.Round(2)
	in.Cash, in.Card = round2(in.Cash), round2(in.Card)
	lines := make([]ZVatLine, len(in.Lines))
	for i, l := range in.Lines {
		l.Base, l.Tax = l.Base.Round(2), l.Tax.Round(2)
		lines[i] = l
	}
	if len(lines) == 0 {
		tax := in.Tax
		if tax.IsNegative() || tax.GreaterThan(in.Total) {
			tax = decimal.Zero
		}
		lines = []ZVatLine{{Rate: decimal.NewFromInt(-1), Base: in.Total.Sub(tax), Tax: tax}}
	}
	var sum decimal.Decimal
	big := 0
	for i, l := range lines {
		sum = sum.Add(l.Base).Add(l.Tax)
		if l.Base.GreaterThan(lines[big].Base) {
			big = i
		}
	}
	diff := in.Total.Sub(sum)
	if diff.Abs().GreaterThan(zTolerance) {
		return Entry{}, ErrZBreakdown
	}
	lines[big].Base = lines[big].Base.Add(diff)

	desc := "Z raporu"
	if in.ZNumber != "" {
		desc += " " + in.ZNumber
	}
	if in.OKC != "" {
		desc += " (" + in.OKC + ")"
	}
	cash, card := in.Total, decimal.Zero
	if in.Card != nil && in.Card.IsPositive() {
		card = decimal.Min(*in.Card, in.Total)
		cash = in.Total.Sub(card)
	} else if in.Cash != nil && in.Cash.LessThan(in.Total) && !in.Cash.IsNegative() {
		// Only cash given: the rest was collected by card.
		cash, card = *in.Cash, in.Total.Sub(*in.Cash)
	}
	out := []Line{
		{Account: ResolveLeaf("100", "kasa", in.Plan).Code, Description: desc + " nakit", Debit: cash, SourceID: in.SourceID},
		{Account: ResolveLeaf("108", "pos kredi kartı", in.Plan).Code, Description: desc + " kredi kartı", Debit: card, SourceID: in.SourceID},
	}
	for _, l := range lines {
		label := ""
		if !l.Rate.IsNegative() {
			label = " %" + l.Rate.String()
		}
		sales := rateLeaf("600", l.Rate, "perakende satış", in.Plan)
		vat := rateLeaf("391", l.Rate, "hesaplanan kdv", in.Plan)
		out = append(out,
			Line{Account: sales, Description: desc + label + " satış", Credit: l.Base, SourceID: in.SourceID},
			Line{Account: vat, Description: desc + label + " hesaplanan KDV", Credit: l.Tax, SourceID: in.SourceID})
	}
	return Entry{Date: in.Date, Description: desc, DocumentType: "z_report", DocumentNo: in.ZNumber, Lines: trimLines(out)}, nil
}

// rateLeaf is the leaf under main for a VAT rate: the only leaf naming the
// rate; when the plan splits main by rate but has no leaf for this one, main
// itself (the export's plan check then asks for the account to be opened);
// otherwise ResolveLeaf. A negative rate means unknown.
func rateLeaf(main string, rate decimal.Decimal, hint string, plan []engine.PlanAccount) string {
	if rate.IsNegative() {
		return ResolveLeaf(main, hint, plan).Code
	}
	if a, ok := engine.VATSubAccount(main, rate, plan); ok {
		return a.Code
	}
	for _, a := range plan {
		if a.IsLeaf && strings.HasPrefix(a.Code, main+".") {
			if _, ok := engine.RateInName(a.Name); ok {
				return main
			}
		}
	}
	return ResolveLeaf(main, hint, plan).Code
}

// round2 rounds an optional amount to kuruş.
func round2(v *decimal.Decimal) *decimal.Decimal {
	if v == nil {
		return nil
	}
	r := v.Round(2)
	return &r
}
