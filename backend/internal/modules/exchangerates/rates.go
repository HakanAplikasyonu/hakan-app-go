// Package exchangerates keeps the TCMB daily exchange rates (#780) and
// fills the rate of foreign-currency invoices from them.
//
// Rule ("o tarihte geçerli kur"): the rate of an invoice is the last TCMB
// bulletin published BEFORE the invoice date (bulletin date < invoice
// date), looking back at most LookbackDays (long bank holidays). TCMB
// publishes at 15:30 on business days; that bulletin is the rate of the
// following day, so a Monday invoice takes Friday's bulletin and an
// invoice after a holiday takes the last bulletin before it. The kind
// (döviz alış by default) is an office setting. A rate the office entered
// by hand always wins: an automatic rate never overwrites it.
package exchangerates

import (
	"time"

	"github.com/shopspring/decimal"
)

// Rate kinds (office setting).
const (
	KindForexBuying     = "forex_buying"     // döviz alış (default)
	KindForexSelling    = "forex_selling"    // döviz satış
	KindBanknoteBuying  = "banknote_buying"  // efektif alış
	KindBanknoteSelling = "banknote_selling" // efektif satış
)

// Rate sources stored next to an invoice's rate.
const (
	SourceTCMB   = "tcmb"   // filled from the TCMB table
	SourceManual = "manual" // entered in the office
)

// LookbackDays bounds how far before the invoice date a bulletin is taken.
const LookbackDays = 14

// ValidKind reports whether k is a known rate kind.
func ValidKind(k string) bool {
	switch k {
	case KindForexBuying, KindForexSelling, KindBanknoteBuying, KindBanknoteSelling:
		return true
	}
	return false
}

// Rate is one currency of one TCMB bulletin. Values are per Unit.
type Rate struct {
	Date            time.Time
	Currency        string
	Unit            int
	ForexBuying     *decimal.Decimal
	ForexSelling    *decimal.Decimal
	BanknoteBuying  *decimal.Decimal
	BanknoteSelling *decimal.Decimal
}

// Value is the TL price of one unit of the currency for kind (value / unit,
// 6 decimals); false when TCMB gave no value of that kind.
func (r Rate) Value(kind string) (decimal.Decimal, bool) {
	var v *decimal.Decimal
	switch kind {
	case KindForexSelling:
		v = r.ForexSelling
	case KindBanknoteBuying:
		v = r.BanknoteBuying
	case KindBanknoteSelling:
		v = r.BanknoteSelling
	default:
		v = r.ForexBuying
	}
	if v == nil || !v.IsPositive() {
		return decimal.Zero, false
	}
	unit := r.Unit
	if unit <= 0 {
		unit = 1
	}
	return v.Div(decimal.NewFromInt(int64(unit))).Round(6), true
}

// day truncates t to its calendar day (UTC midnight of the same Y-M-D).
func day(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// PickRate returns the rate valid for an invoice dated invoiceDate: the
// latest bulletin strictly before that day and at most LookbackDays
// earlier, of the given kind. rates may be in any order.
func PickRate(rates []Rate, invoiceDate time.Time, kind string) (Rate, decimal.Decimal, bool) {
	inv := day(invoiceDate)
	from := inv.AddDate(0, 0, -LookbackDays)
	var best Rate
	var bestVal decimal.Decimal
	found := false
	for _, r := range rates {
		d := day(r.Date)
		if !d.Before(inv) || d.Before(from) {
			continue
		}
		v, ok := r.Value(kind)
		if !ok {
			continue
		}
		if !found || d.After(day(best.Date)) {
			best, bestVal, found = r, v, true
		}
	}
	return best, bestVal, found
}

// MayAutoFill reports whether an automatic rate may be written over the
// stored one: only when there is none or it was itself automatic. A rate
// entered by hand or given by the source document is kept.
func MayAutoFill(hasRate bool, source string) bool {
	return !hasRate || source == SourceTCMB
}

// MissingDays lists the business days (Monday to Friday) from from to to
// (inclusive) that have no bulletin in have. Bank holidays are not known
// here: their archive file answers 404 and is skipped.
func MissingDays(from, to time.Time, have map[string]bool) []time.Time {
	var out []time.Time
	for d := day(from); !d.After(day(to)); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		if have[d.Format("2006-01-02")] {
			continue
		}
		out = append(out, d)
	}
	return out
}
