// Package taxreturns prepares the period tax returns of a taxpayer in the
// shape of the GİB forms (issue #390): KDV1 (1 No.lu KDV beyannamesi),
// MUHSGK (muhtasar ve prim hizmet beyannamesi) and Form Ba / Bs.
//
// The preview is computed on every request from the common data: invoices
// (invoice_tax_summaries), approved expenses and Z reports (#384), the
// payroll and SGK accrual (#382). The office's decision for a period lives
// in tax_returns: manual adjustments (devreden KDV, ilave edilecek KDV,
// diğer indirimler), acknowledged checklist items, the frozen snapshot of
// "hazırlandı" and the filing record (date, tahakkuk fiş no).
//
// Rules are documented on the compute functions (kdv1.go, muhsgk.go,
// babs.go). Amounts are TL with two decimals.
package taxreturns

import (
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/shopspring/decimal"
)

// Kinds of tax_returns.kind.
const (
	KindKDV1   = "kdv1"
	KindMUHSGK = "muhsgk"
	KindBABS   = "babs"
)

// Statuses of tax_returns.status.
const (
	StatusDraft    = "draft"
	StatusPrepared = "prepared"
	StatusFiled    = "filed"
)

// Kinds lists the kinds in screen order.
var Kinds = []string{KindKDV1, KindMUHSGK, KindBABS}

// ValidKind reports whether k is a known kind.
func ValidKind(k string) bool { return k == KindKDV1 || k == KindMUHSGK || k == KindBABS }

// Due days of the following month (VUK 18 hafta sonu kuralı DueDate'te).
// Ba/Bs is due on the last day of the following month (31 is clamped).
var dueDays = map[string]int{KindKDV1: 28, KindMUHSGK: 26, KindBABS: 31}

// BABSThreshold: Form Ba / Bs reports a counterparty when the period's
// documents reach 5.000 TL excluding VAT (VUK 396 Sıra No.lu Tebliğ).
var BABSThreshold = decimal.NewFromInt(5000)

// Errors.
var (
	ErrInvalidPeriod  = errors.New("taxreturns: period must be YYYY-MM")
	ErrInvalidKind    = errors.New("taxreturns: unknown kind")
	ErrNotDraft       = errors.New("taxreturns: return is not a draft")
	ErrNotPrepared    = errors.New("taxreturns: return is not prepared")
	ErrNotFound       = errors.New("taxreturns: return not found")
	ErrUnacknowledged = errors.New("taxreturns: checklist warnings are not acknowledged")
	ErrInvalidInput   = errors.New("taxreturns: invalid input")
)

var periodRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// Istanbul is Türkiye time (UTC+3 all year).
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

// PreviousPeriod is the month before p (p must be valid).
func PreviousPeriod(p string) string {
	t, _ := ParsePeriod(p)
	return t.AddDate(0, -1, 0).Format("2006-01")
}

// DueDate is the filing deadline of kind for period: the kind's day of the
// following month (clamped to the month's last day), moved to Monday when it
// falls on a weekend. Public holidays are not applied.
func DueDate(kind, period string) (time.Time, bool, error) {
	start, err := ParsePeriod(period)
	if err != nil {
		return time.Time{}, false, err
	}
	day, ok := dueDays[kind]
	if !ok {
		return time.Time{}, false, ErrInvalidKind
	}
	next := start.AddDate(0, 1, 0)
	if last := next.AddDate(0, 1, -1).Day(); day > last {
		day = last
	}
	due := time.Date(next.Year(), next.Month(), day, 0, 0, 0, 0, Istanbul)
	switch due.Weekday() {
	case time.Saturday:
		return due.AddDate(0, 0, 2), true, nil
	case time.Sunday:
		return due.AddDate(0, 0, 1), true, nil
	}
	return due, false, nil
}

// Money is a TL amount; JSON is a string with two decimals.
type Money decimal.Decimal

// M wraps a decimal (rounded to kuruş).
func M(d decimal.Decimal) Money { return Money(d.Round(2)) }

// Dec returns the decimal.
func (m Money) Dec() decimal.Decimal { return decimal.Decimal(m) }

// MarshalJSON implements json.Marshaler.
func (m Money) MarshalJSON() ([]byte, error) { return json.Marshal(m.Dec().StringFixed(2)) }

// UnmarshalJSON implements json.Unmarshaler (snapshots are read back).
func (m *Money) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		var f json.Number
		if err2 := json.Unmarshal(b, &f); err2 != nil {
			return err
		}
		s = f.String()
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return err
	}
	*m = Money(d)
	return nil
}

// Severities of checklist items. A warning must be acknowledged before the
// return is prepared; info is shown only.
const (
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// CheckItem is one row of the checklist (kontrol listesi).
type CheckItem struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	// Count: how many records the item is about (0 when not countable).
	Count int64 `json:"count"`
	// Amount: the TL amount concerned, when there is one.
	Amount *Money `json:"amount,omitempty"`
	// Acknowledged: the accountant confirmed the item for this period.
	Acknowledged bool `json:"acknowledged"`
}

func item(code, sev, msg string, count int64) CheckItem {
	return CheckItem{Code: code, Severity: sev, Message: msg, Count: count}
}

func itemAmount(code, sev, msg string, count int64, amount decimal.Decimal) CheckItem {
	a := M(amount)
	return CheckItem{Code: code, Severity: sev, Message: msg, Count: count, Amount: &a}
}

// applyAcks marks acknowledged items and reports the warnings still open.
func applyAcks(items []CheckItem, acks []string) (open []string) {
	set := map[string]bool{}
	for _, a := range acks {
		set[a] = true
	}
	for i := range items {
		items[i].Acknowledged = set[items[i].Code]
		if items[i].Severity == SeverityWarning && !items[i].Acknowledged {
			open = append(open, items[i].Code)
		}
	}
	return open
}

// rateString prints a VAT rate without trailing zeros ("20", "1", "0.5").
func rateString(d decimal.Decimal) string { return d.String() }
