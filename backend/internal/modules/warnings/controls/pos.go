package controls

import (
	"context"
	"fmt"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/rules"
)

// POSQueries is the db subset of the Z report ↔ bank POS check.
type POSQueries interface {
	POSVsSalesZCard(ctx context.Context, arg db.POSVsSalesZCardParams) (db.POSVsSalesZCardRow, error)
	POSVsSalesBankDays(ctx context.Context, arg db.POSVsSalesBankDaysParams) ([]db.POSVsSalesBankDaysRow, error)
}

// POSCheck raises BNK_POS_VS_SALES (rules.POSVsSales) for one engagement
// month: the Z reports' card total against the POS collections the bank
// received. It runs with the accountant controls and after the bank
// controls (banking/matching), so a new statement or Z report re-checks it.
type POSCheck struct {
	q      POSQueries
	engine *engine.Engine
}

// NewPOSCheck builds the check.
func NewPOSCheck(q POSQueries, e *engine.Engine) *POSCheck { return &POSCheck{q: q, engine: e} }

// Check runs the rule for period (YYYY-MM).
func (c *POSCheck) Check(ctx context.Context, s engine.Subject, period string) (engine.Report, error) {
	if !periodRe.MatchString(period) {
		return engine.Report{}, ErrInvalidPeriod
	}
	start, _ := time.Parse("2006-01", period)
	end := start.AddDate(0, 1, -1)
	z, err := c.q.POSVsSalesZCard(ctx, db.POSVsSalesZCardParams{TaxpayerID: s.TaxpayerID, FromDate: date(start), ToDate: date(end)})
	if err != nil {
		return engine.Report{}, fmt.Errorf("controls: pos z reports: %w", err)
	}
	days, err := c.q.POSVsSalesBankDays(ctx, db.POSVsSalesBankDaysParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, TaxpayerID: s.TaxpayerID,
		FromDate: date(start), ToDate: date(end.AddDate(0, 0, rules.MaxPOSLagDays)),
	})
	if err != nil {
		return engine.Report{}, fmt.Errorf("controls: pos bank days: %w", err)
	}
	in := rules.POSVsSalesInput{Period: period, ZCard: dec(z.CardTotal), ZReports: int(z.Reports)}
	if z.LastBankDate.Valid {
		in.LastBankDate = z.LastBankDate.Time
	}
	for _, d := range days {
		in.Bank = append(in.Bank, rules.POSDay{Date: d.Day.Time, Net: dec(d.Net), Commission: dec(d.Commission), Movements: int(d.Movements)})
	}
	return engine.Run(ctx, c.engine, s, rules.POSVsSales{}, in)
}
