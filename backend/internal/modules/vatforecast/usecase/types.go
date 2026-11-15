// Package usecase computes the in-period VAT forecast (KDV erken uyarı,
// issue #33, docs/00 K9 / T10) from the common data model and feeds the
// warning engine.
//
// Rules (decided in #33, see the module README in routes.go):
//
//   - Source: invoices + invoice_tax_summaries of the engagement's taxpayer,
//     period = invoices.period. Only status_normalized = 'active' invoices not
//     deleted at source count (cancelled and draft are reported as excluded).
//   - Direction decides the side: outgoing → hesaplanan KDV, incoming →
//     indirilecek KDV. IADE follows its direction (an incoming return reduces
//     the payable like a deduction, an outgoing one adds to it), as in KDV1.
//   - Tevkifat: on outgoing invoices the buyer pays the withheld part (KDV2),
//     so the seller's hesaplanan KDV is vat − withholding. On incoming
//     invoices the whole vat is indirilecek; the withheld part is the
//     taxpayer's own KDV2 liability (sorumlu sıfatıyla), reported separately
//     and not part of the KDV1 net.
//   - Foreign currency: amounts × invoices.exchange_rate, rounded to kuruş per
//     summary row; invoices without a rate are excluded and counted.
//   - Devreden KDV: the chain runs over earlier periods' invoice data in the
//     system, starting from the nearest manually entered opening carry-forward
//     (vat_carry_forwards) at or before the period, else from zero at the
//     first period with data.
package usecase

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Directions of invoices.direction.
const (
	DirectionOutgoing = "outgoing"
	DirectionIncoming = "incoming"
)

// Warning codes and producer (catalog.go, migration *_vat_forecast).
const (
	CodePayableThreshold = "VAT_PAYABLE_THRESHOLD"
	CodeDeadlineNear     = "VAT_DEADLINE_NEAR"
	Producer             = "control:vat_forecast"
)

// Defaults used when a param is missing or invalid (the seed carries the
// same values; these only guard a broken configuration).
const (
	DefaultDueDay     = 28
	DefaultDaysBefore = 5
)

// Money is a TL amount; JSON is a string with two decimals.
type Money decimal.Decimal

// M wraps a decimal.
func M(d decimal.Decimal) Money { return Money(d) }

// Dec returns the decimal.
func (m Money) Dec() decimal.Decimal { return decimal.Decimal(m) }

// MarshalJSON implements json.Marshaler.
func (m Money) MarshalJSON() ([]byte, error) { return json.Marshal(m.Dec().StringFixed(2)) }

// Subject is the engagement whose taxpayer's VAT is forecast.
type Subject struct {
	OrganizationID int64
	EngagementID   int64
	EngagementUUID uuid.UUID
	TaxpayerID     int64
	TaxpayerUUID   uuid.UUID
}

// Line is one VATForecastLines row (TL).
type Line struct {
	Direction       string
	Type            string
	TaxRate         decimal.Decimal
	WithholdingCode string
	InvoiceCount    int64
	Base            decimal.Decimal
	VAT             decimal.Decimal
	Withholding     decimal.Decimal
}

// PeriodTotal is one earlier period for the carry-forward chain.
type PeriodTotal struct {
	Period string
	Output decimal.Decimal // hesaplanan (vat − tevkifat on sales)
	Input  decimal.Decimal // indirilecek
}

// Opening is a manually entered carry-forward into Period.
type Opening struct {
	Period    string    `json:"period"`
	Amount    Money     `json:"amount"`
	Note      string    `json:"note"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Excluded counts invoices of the period left out of the forecast.
type Excluded struct {
	// Included is not an exclusion: the number of invoices in the forecast
	// (Totals.InvoiceCount); not serialised here.
	Included            int64 `json:"-"`
	Cancelled           int64 `json:"cancelled"`
	Draft               int64 `json:"draft"`
	DeletedAtSource     int64 `json:"deleted_at_source"`
	MissingExchangeRate int64 `json:"missing_exchange_rate"`
}

// Settings is the effective configuration (warning_rule_configs params).
type Settings struct {
	// PayableLimit nil: the threshold check does not run.
	PayableLimit *decimal.Decimal
	DaysBefore   int
	DueDay       int
}

// LineView is a line in the API.
type LineView struct {
	Direction       string `json:"direction"`
	Type            string `json:"type"`
	TaxRate         string `json:"tax_rate"`
	WithholdingCode string `json:"withholding_code"`
	// InvoiceCount: invoices with a summary row in this group; an invoice
	// with several rates appears in several lines.
	InvoiceCount int64 `json:"invoice_count"`
	Base         Money `json:"base"`
	VAT          Money `json:"vat"`
	Withholding  Money `json:"withholding"`
	// Effective is what the line adds to its side of KDV1: vat − withholding
	// on sales, vat on purchases.
	Effective Money `json:"effective_vat"`
}

// RateView is the per-rate breakdown.
type RateView struct {
	TaxRate           string `json:"tax_rate"`
	OutputBase        Money  `json:"output_base"`
	OutputVAT         Money  `json:"output_vat"`
	OutputWithholding Money  `json:"output_withholding"`
	InputBase         Money  `json:"input_base"`
	InputVAT          Money  `json:"input_vat"`
	InputWithholding  Money  `json:"input_withholding"`
}

// Totals is the KDV1 estimate of the period.
type Totals struct {
	// OutputVAT: hesaplanan KDV (satış KDV'si − satışta tevkif edilen).
	OutputVAT Money `json:"output_vat"`
	// OutputWithholding: satışlarda alıcının tevkif ettiği KDV (bilgi).
	OutputWithholding Money `json:"output_withholding"`
	// InputVAT: indirilecek KDV (alış KDV'sinin tamamı).
	InputVAT Money `json:"input_vat"`
	// InputWithholding: alışlarda mükellefin tevkif ettiği KDV; KDV2 ile
	// ayrıca beyan edilir, KDV1 netine girmez.
	InputWithholding Money `json:"input_withholding"`
	// CarriedIn: önceki dönemden devreden KDV.
	CarriedIn Money `json:"carried_in"`
	// Net = OutputVAT − InputVAT − CarriedIn.
	Net Money `json:"net"`
	// Payable = max(Net, 0): ödenecek KDV tahmini.
	Payable Money `json:"payable"`
	// CarryOut = max(−Net, 0): sonraki döneme devredecek KDV.
	CarryOut Money `json:"carry_out"`
	// InvoiceCount: distinct invoices in the forecast.
	InvoiceCount int64 `json:"invoice_count"`
}

// Carry explains where CarriedIn comes from.
type Carry struct {
	// Source: "opening" (elle girilen açılış devri), "computed" (sistemdeki
	// önceki dönemlerden yürütüldü) or "none" (önceki dönem verisi yok).
	Source        string  `json:"source"`
	OpeningPeriod *string `json:"opening_period"`
	OpeningAmount *Money  `json:"opening_amount"`
	// Periods are the earlier periods with data the chain ran over.
	Periods []string `json:"periods"`
}

// Calendar is the KDV1 filing deadline of the period.
type Calendar struct {
	DueDate string `json:"due_date"`
	DueDay  int    `json:"due_day"`
	// DaysLeft: due date − today (Europe/Istanbul); negative once passed.
	DaysLeft int `json:"days_left"`
	// RolledForward: the due day fell on a weekend and moved to Monday.
	RolledForward bool `json:"rolled_forward"`
	// HolidaysChecked is false: public holidays are not applied yet.
	HolidaysChecked bool `json:"holidays_checked"`
}

// Checks is the outcome of the two warning rules on this forecast.
type Checks struct {
	PayableLimit        *Money `json:"payable_limit"`
	PayableLimitReached bool   `json:"payable_limit_reached"`
	DaysBefore          int    `json:"days_before"`
	DeadlineNear        bool   `json:"deadline_near"`
}

// Forecast is the API response.
type Forecast struct {
	Period         string     `json:"period"`
	EngagementUUID uuid.UUID  `json:"engagement_uuid"`
	TaxpayerUUID   uuid.UUID  `json:"taxpayer_uuid"`
	Currency       string     `json:"currency"`
	AsOf           string     `json:"as_of"`
	Totals         Totals     `json:"totals"`
	Rates          []RateView `json:"rates"`
	Lines          []LineView `json:"lines"`
	CarryForward   Carry      `json:"carry_forward"`
	Calendar       Calendar   `json:"calendar"`
	Checks         Checks     `json:"checks"`
	Excluded       Excluded   `json:"excluded"`
}
