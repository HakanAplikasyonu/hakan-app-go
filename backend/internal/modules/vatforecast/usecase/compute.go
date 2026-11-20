package usecase

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/workday"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/shopspring/decimal"
)

// ErrInvalidPeriod: period is not YYYY-MM.
var ErrInvalidPeriod = errors.New("vatforecast: period must be YYYY-MM")

var periodRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// Istanbul is Türkiye time (UTC+3 all year since 2016); a fixed zone keeps
// the result independent of the host's tzdata.
var Istanbul = time.FixedZone("TRT", 3*60*60)

// ParsePeriod parses YYYY-MM into the first day of the month (Istanbul).
func ParsePeriod(p string) (time.Time, error) {
	if !periodRe.MatchString(p) {
		return time.Time{}, ErrInvalidPeriod
	}
	t, err := time.ParseInLocation("2006-01", p, Istanbul)
	if err != nil {
		return time.Time{}, ErrInvalidPeriod
	}
	return t, nil
}

// PeriodOf is the YYYY-MM of t in Istanbul time.
func PeriodOf(t time.Time) string { return t.In(Istanbul).Format("2006-01") }

// PreviousPeriod is the month before p (p must be valid).
func PreviousPeriod(p string) string {
	t, _ := ParsePeriod(p)
	return t.AddDate(0, -1, 0).Format("2006-01")
}

// DueDate is the KDV1 deadline of period: dueDay of the following month
// (clamped to the month's last day), moved to the next working day when it
// falls on a weekend or a public holiday (VUK 18; see workday).
func DueDate(period string, dueDay int) (due time.Time, rolled bool, err error) {
	start, err := ParsePeriod(period)
	if err != nil {
		return time.Time{}, false, err
	}
	if dueDay < 1 || dueDay > 31 {
		dueDay = DefaultDueDay
	}
	next := start.AddDate(0, 1, 0)
	last := next.AddDate(0, 1, -1).Day()
	if dueDay > last {
		dueDay = last
	}
	due = time.Date(next.Year(), next.Month(), dueDay, 0, 0, 0, 0, Istanbul)
	// VUK 18: hafta sonu ya da resmi tatil ilk iş gününe kayar (#575).
	due, moved := workday.Next(due)
	return due, moved, nil
}

// daysBetween counts calendar days from a's date to b's date (Istanbul).
func daysBetween(a, b time.Time) int {
	a, b = a.In(Istanbul), b.In(Istanbul)
	da := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	db := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(db.Sub(da).Hours() / 24)
}

// SettingsFrom reads the effective params of the two codes. The calendar
// (due_day) applies even when VAT_DEADLINE_NEAR is disabled.
func SettingsFrom(cfg map[string]engine.Effective) (Settings, error) {
	s := Settings{DaysBefore: DefaultDaysBefore, DueDay: DefaultDueDay}
	if eff, ok := cfg[CodePayableThreshold]; ok {
		d, set, err := paramDecimal(eff.Params, "limit")
		if err != nil {
			return s, fmt.Errorf("%s.limit: %w", CodePayableThreshold, err)
		}
		if set {
			s.PayableLimit = &d
		}
	}
	if eff, ok := cfg[CodeDeadlineNear]; ok {
		if n, ok := paramInt(eff.Params, "days_before"); ok && n >= 0 {
			s.DaysBefore = n
		}
		if n, ok := paramInt(eff.Params, "due_day"); ok && n >= 1 && n <= 31 {
			s.DueDay = n
		}
	}
	return s, nil
}

func paramDecimal(p map[string]any, key string) (decimal.Decimal, bool, error) {
	v, ok := p[key]
	if !ok || v == nil {
		return decimal.Zero, false, nil
	}
	switch t := v.(type) {
	case string:
		d, err := decimal.NewFromString(t)
		return d, err == nil, err
	case float64:
		return decimal.NewFromFloat(t), true, nil
	case json.Number:
		d, err := decimal.NewFromString(t.String())
		return d, err == nil, err
	}
	return decimal.Zero, false, fmt.Errorf("unsupported type %T", v)
}

// paramInt accepts a whole number or a decimal string with no fraction.
func paramInt(p map[string]any, key string) (int, bool) {
	switch t := p[key].(type) {
	case float64:
		if t == float64(int(t)) {
			return int(t), true
		}
	case string:
		if n, err := strconv.Atoi(t); err == nil {
			return n, true
		}
	}
	return 0, false
}

// Input is everything Compute needs; the service loads it.
type Input struct {
	Subject  Subject
	Period   string
	Lines    []Line
	History  []PeriodTotal // earlier periods, ascending, from the chain start
	Opening  *Opening      // nearest opening at or before Period
	Excluded Excluded
	Settings Settings
	Now      time.Time
}

// Compute builds the forecast. It has no I/O.
func Compute(in Input) (Forecast, error) {
	if _, err := ParsePeriod(in.Period); err != nil {
		return Forecast{}, err
	}
	f := Forecast{
		Period: in.Period, EngagementUUID: in.Subject.EngagementUUID, TaxpayerUUID: in.Subject.TaxpayerUUID,
		Currency: "TRY", AsOf: in.Now.In(Istanbul).Format("2006-01-02"),
		Rates: []RateView{}, Lines: []LineView{}, Excluded: in.Excluded,
		CarryForward: Carry{Periods: []string{}},
	}

	var out, outW, inp, inpW decimal.Decimal
	type rateAcc struct{ ob, ov, ow, ib, iv, iw decimal.Decimal }
	rates := map[string]*rateAcc{}
	var rateKeys []decimal.Decimal
	for _, l := range in.Lines {
		key := l.TaxRate.String()
		r := rates[key]
		if r == nil {
			r = &rateAcc{}
			rates[key] = r
			rateKeys = append(rateKeys, l.TaxRate)
		}
		eff := l.VAT
		switch l.Direction {
		case DirectionOutgoing:
			eff = l.VAT.Sub(l.Withholding)
			out, outW = out.Add(eff), outW.Add(l.Withholding)
			r.ob, r.ov, r.ow = r.ob.Add(l.Base), r.ov.Add(eff), r.ow.Add(l.Withholding)
		case DirectionIncoming:
			inp, inpW = inp.Add(l.VAT), inpW.Add(l.Withholding)
			r.ib, r.iv, r.iw = r.ib.Add(l.Base), r.iv.Add(l.VAT), r.iw.Add(l.Withholding)
		default:
			return Forecast{}, fmt.Errorf("vatforecast: unknown direction %q", l.Direction)
		}
		f.Lines = append(f.Lines, LineView{
			Direction: l.Direction, Type: l.Type, TaxRate: l.TaxRate.StringFixed(2), WithholdingCode: l.WithholdingCode,
			InvoiceCount: l.InvoiceCount, Base: M(l.Base), VAT: M(l.VAT), Withholding: M(l.Withholding), Effective: M(eff),
		})
	}
	sort.Slice(rateKeys, func(i, j int) bool { return rateKeys[i].LessThan(rateKeys[j]) })
	for _, k := range rateKeys {
		r := rates[k.String()]
		f.Rates = append(f.Rates, RateView{
			TaxRate: k.StringFixed(2), OutputBase: M(r.ob), OutputVAT: M(r.ov), OutputWithholding: M(r.ow),
			InputBase: M(r.ib), InputVAT: M(r.iv), InputWithholding: M(r.iw),
		})
	}

	f.Totals.InvoiceCount = in.Excluded.Included
	carried := carryIn(in, &f.CarryForward)
	net := out.Sub(inp).Sub(carried)
	f.Totals.OutputVAT, f.Totals.OutputWithholding = M(out), M(outW)
	f.Totals.InputVAT, f.Totals.InputWithholding = M(inp), M(inpW)
	f.Totals.CarriedIn, f.Totals.Net = M(carried), M(net)
	f.Totals.Payable, f.Totals.CarryOut = M(decimal.Max(net, decimal.Zero)), M(decimal.Max(net.Neg(), decimal.Zero))

	due, rolled, err := DueDate(in.Period, in.Settings.DueDay)
	if err != nil {
		return Forecast{}, err
	}
	f.Calendar = Calendar{
		DueDate: due.Format("2006-01-02"), DueDay: in.Settings.DueDay, RolledForward: rolled,
		DaysLeft: daysBetween(in.Now, due), HolidaysChecked: workday.Covered(due.Year()),
	}

	f.Checks.DaysBefore = in.Settings.DaysBefore
	f.Checks.DeadlineNear = f.Calendar.DaysLeft >= 0 && f.Calendar.DaysLeft <= in.Settings.DaysBefore
	if in.Settings.PayableLimit != nil {
		l := M(*in.Settings.PayableLimit)
		f.Checks.PayableLimit = &l
		f.Checks.PayableLimitReached = f.Totals.Payable.Dec().GreaterThanOrEqual(*in.Settings.PayableLimit)
	}
	return f, nil
}

// carryIn runs the devreden KDV chain: start from the opening (or zero), and
// for every earlier period net = output − input − carry; the next carry is
// max(−net, 0). A period without data passes the carry on unchanged, so only
// periods with data need to be visited.
func carryIn(in Input, c *Carry) decimal.Decimal {
	carry := decimal.Zero
	from := ""
	if in.Opening != nil {
		carry = in.Opening.Amount.Dec()
		from = in.Opening.Period
		p, a := in.Opening.Period, in.Opening.Amount
		c.Source, c.OpeningPeriod, c.OpeningAmount = "opening", &p, &a
	}
	for _, h := range in.History {
		if h.Period < from || h.Period >= in.Period {
			continue
		}
		net := h.Output.Sub(h.Input).Sub(carry)
		carry = decimal.Max(net.Neg(), decimal.Zero)
		c.Periods = append(c.Periods, h.Period)
	}
	if c.Source == "" {
		c.Source = "none"
		if len(c.Periods) > 0 {
			c.Source = "computed"
		}
	}
	return carry
}
