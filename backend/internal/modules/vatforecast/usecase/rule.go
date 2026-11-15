package usecase

import (
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
)

// Rule is the warning rule over a forecast (engine.Rule[Forecast]). It owns
// both codes on the period entity, so a condition that no longer holds (the
// payable fell under the limit, the deadline passed) auto-resolves.
//
// Params are kept stable on purpose: an acknowledged warning stays
// acknowledged until they change (engine.Store.Raise). VAT_DEADLINE_NEAR
// therefore carries the due date, not the days left (which change daily);
// VAT_PAYABLE_THRESHOLD carries the payable, so a new invoice that moves the
// estimate reopens it.
type Rule struct{}

var _ engine.Rule[Forecast] = Rule{}

// Producer implements engine.Rule.
func (Rule) Producer() string { return Producer }

// Codes implements engine.Rule.
func (Rule) Codes() []string { return []string{CodePayableThreshold, CodeDeadlineNear} }

// Check implements engine.Rule. Thresholds come from cfg.
func (r Rule) Check(f Forecast, cfg map[string]engine.Effective) ([]engine.Outcome, error) {
	s, err := SettingsFrom(cfg)
	if err != nil {
		return nil, err
	}
	var findings []engine.Finding
	if s.PayableLimit != nil && f.Totals.Payable.Dec().GreaterThanOrEqual(*s.PayableLimit) {
		findings = append(findings, engine.Finding{Code: CodePayableThreshold, Params: map[string]any{
			"payable": f.Totals.Payable.Dec().StringFixed(2),
			"limit":   s.PayableLimit.StringFixed(2),
		}})
	}
	if f.Calendar.DaysLeft >= 0 && f.Calendar.DaysLeft <= s.DaysBefore {
		findings = append(findings, engine.Finding{Code: CodeDeadlineNear, Params: map[string]any{
			"due_date":    f.Calendar.DueDate,
			"days_before": s.DaysBefore,
		}})
	}
	return []engine.Outcome{{
		Entity:   engine.Entity{Type: catalog.EntityPeriod, Period: f.Period},
		Owned:    r.Codes(),
		Findings: findings,
	}}, nil
}
