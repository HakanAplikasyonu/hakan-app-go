package rules

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
)

// CodePOSVsSales: the card total of the month's Z reports differs from the
// POS collections the bank received (21 §6.3, müşavir örneği: Z 64.000 TL,
// bankaya 62.000 TL).
const CodePOSVsSales = "BNK_POS_VS_SALES"

// POSVsSalesProducer is the producer of BNK_POS_VS_SALES (05 §1).
const POSVsSalesProducer = "control:pos_vs_sales"

// Defaults, used only when the configuration has no such param (the seed
// sets the same values; an office or engagement overrides them in
// warning_rule_configs).
const (
	// DefaultPOSLagDays: the bank pays a day's card sales after the value
	// date (valör), so the bank side of the month is shifted this many days.
	DefaultPOSLagDays = 1
	// MaxPOSLagDays caps tolerance_days: the bank days are read up to this
	// many days after the month.
	MaxPOSLagDays = 15
)

var (
	defaultPOSMinAmount = decimal.NewFromInt(100)
	defaultPOSPercent   = decimal.NewFromInt(1)
)

// POSDay is one day's POS collections in the bank: the amount the bank paid
// (net) and the commission the accountant entered on those movements (#565).
type POSDay struct {
	Date       time.Time
	Net        decimal.Decimal
	Commission decimal.Decimal
	Movements  int
}

// POSVsSalesInput is one taxpayer month.
type POSVsSalesInput struct {
	Period string // YYYY-MM
	// ZCard is the card total of the month's Z reports, ZReports how many
	// carry a card amount.
	ZCard    decimal.Decimal
	ZReports int
	// Bank is the POS collections from the first day of the month to
	// MaxPOSLagDays after it.
	Bank []POSDay
	// LastBankDate is the newest bank movement of the taxpayer (any kind):
	// how far the statements reach. Zero when there is none.
	LastBankDate time.Time
}

// POSVsSales compares the month's Z card total with the gross POS
// collections (net + entered commission) of the same month shifted by the
// value-date lag (tolerance_days). A difference above the larger of
// min_amount and tolerance_percent of the Z total raises the warning; a
// commission entered on the movements that explains the gap does not.
//
// The month is the unit on purpose: Z reports, bank controls and the VAT
// return are monthly (period), and a bank pays a day's sales one or more
// days later, often several days at once, so a day-by-day comparison would
// warn on every weekend.
type POSVsSales struct{}

var _ engine.Rule[POSVsSalesInput] = POSVsSales{}

// Producer implements engine.Rule.
func (POSVsSales) Producer() string { return POSVsSalesProducer }

// Codes implements engine.Rule.
func (POSVsSales) Codes() []string { return []string{CodePOSVsSales} }

// Check implements engine.Rule.
func (POSVsSales) Check(in POSVsSalesInput, cfg map[string]engine.Effective) ([]engine.Outcome, error) {
	start, err := time.Parse("2006-01", in.Period)
	if err != nil {
		return nil, fmt.Errorf("%s: period %q: %w", CodePOSVsSales, in.Period, err)
	}
	out := []engine.Outcome{{
		Entity: engine.Entity{Type: catalog.EntityPeriod, Period: in.Period},
		Owned:  []string{CodePOSVsSales},
	}}
	lag, err := posParam(cfg, "tolerance_days", decimal.NewFromInt(DefaultPOSLagDays))
	if err != nil {
		return nil, err
	}
	minAmount, err := posParam(cfg, "min_amount", defaultPOSMinAmount)
	if err != nil {
		return nil, err
	}
	percent, err := posParam(cfg, "tolerance_percent", defaultPOSPercent)
	if err != nil {
		return nil, err
	}
	f, ok := ComparePOS(in, start, int(lag.IntPart()), minAmount, percent)
	if ok {
		out[0].Findings = []engine.Finding{f}
	}
	return out, nil
}

// ComparePOS is the verdict on one month. No Z card sales: nothing to
// compare. Statements that do not yet reach the end of the bank window: not
// comparable yet (the month's last sales are not paid in), no warning.
func ComparePOS(in POSVsSalesInput, start time.Time, lag int, minAmount, percent decimal.Decimal) (engine.Finding, bool) {
	if !in.ZCard.IsPositive() {
		return engine.Finding{}, false
	}
	if lag < 0 {
		lag = 0
	}
	if lag > MaxPOSLagDays {
		lag = MaxPOSLagDays
	}
	end := start.AddDate(0, 1, -1)
	from, to := start.AddDate(0, 0, lag), end.AddDate(0, 0, lag)
	if in.LastBankDate.IsZero() || in.LastBankDate.Before(to) {
		return engine.Finding{}, false
	}
	net, commission := decimal.Zero, decimal.Zero
	movements := 0
	for _, d := range in.Bank {
		if d.Date.Before(from) || d.Date.After(to) {
			continue
		}
		net = net.Add(d.Net)
		commission = commission.Add(d.Commission)
		movements += d.Movements
	}
	gross := net.Add(commission)
	diff := in.ZCard.Sub(gross).Round(2)
	threshold := in.ZCard.Mul(percent).Div(decimal.NewFromInt(100)).Round(2)
	if minAmount.GreaterThan(threshold) {
		threshold = minAmount
	}
	if diff.Abs().LessThanOrEqual(threshold) {
		return engine.Finding{}, false
	}
	return engine.Finding{Code: CodePOSVsSales, Params: map[string]any{
		"z_card_total":    money(in.ZCard),
		"z_reports":       in.ZReports,
		"bank_net":        money(net),
		"bank_commission": money(commission),
		"bank_gross":      money(gross),
		"bank_movements":  movements,
		"difference":      money(diff),
		"threshold":       money(threshold),
		"bank_from":       from.Format("2006-01-02"),
		"bank_to":         to.Format("2006-01-02"),
	}}, true
}

// posParam reads a non-negative number; absent or null: def.
func posParam(cfg map[string]engine.Effective, key string, def decimal.Decimal) (decimal.Decimal, error) {
	v, ok, err := paramDecimal(cfg, CodePOSVsSales, key)
	if err != nil {
		return decimal.Zero, err
	}
	if !ok {
		return def, nil
	}
	if v.IsNegative() {
		return decimal.Zero, fmt.Errorf("%s.%s: must not be negative", CodePOSVsSales, key)
	}
	return v, nil
}
