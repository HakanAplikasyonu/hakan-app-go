package easytrade

import (
	"context"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// Totals is one side of the VAT reconciliation.
type Totals struct {
	Count int    `json:"invoice_count"`
	Base  string `json:"base"`
	VAT   string `json:"vat"`
}

// Side compares Easytrade's VAT report with what the portal holds.
type Side struct {
	Source Totals `json:"source"`
	Portal Totals `json:"portal"`
	Match  bool   `json:"match"`
}

// Reconciliation is 09 §8: the automatic form of acceptance test #4 (02 §11).
type Reconciliation struct {
	Period    string `json:"period"`
	Sales     Side   `json:"sales"`
	Purchases Side   `json:"purchases"`
}

var tolerance = decimal.RequireFromString("0.05")

// Reconcile reads GET /company/reports/vat for period (YYYY-MM) and compares
// count, base and VAT per direction with the portal's invoices from
// Easytrade. ⚠️ Only the documented fields (invoice_count, totals.base,
// totals.vat) are read; rates and summary are 09 §9 soru 13.
func (c *Client) Reconcile(ctx context.Context, pool *pgxpool.Pool, taxpayerID int64, period string) (Reconciliation, error) {
	p, err := time.Parse("2006-01", period)
	if err != nil {
		return Reconciliation{}, err
	}
	var out struct {
		Data struct {
			Sales     report `json:"sales"`
			Purchases report `json:"purchases"`
		} `json:"data"`
	}
	if err := c.get(ctx, "/company/reports/vat", url.Values{"period": {p.Format("2006-01-02")}}, &out); err != nil {
		return Reconciliation{}, err
	}
	rec := Reconciliation{Period: period}
	for _, s := range []struct {
		dir  string
		rep  report
		side *Side
	}{{"outgoing", out.Data.Sales, &rec.Sales}, {"incoming", out.Data.Purchases, &rec.Purchases}} {
		var n int
		var base, vat decimal.Decimal
		if err := pool.QueryRow(ctx, `
			SELECT count(DISTINCT i.id), COALESCE(sum(t.base), 0), COALESCE(sum(t.vat), 0)
			FROM invoices i LEFT JOIN invoice_tax_summaries t ON t.invoice_id = i.id
			WHERE i.taxpayer_id = $1 AND i.source_system = $2 AND i.direction = $3 AND i.period = $4`,
			taxpayerID, SourceSystem, s.dir, period).Scan(&n, &base, &vat); err != nil {
			return rec, err
		}
		sb, sv := decimalOf(s.rep.Totals.Base), decimalOf(s.rep.Totals.VAT)
		*s.side = Side{
			Source: Totals{Count: s.rep.InvoiceCount, Base: sb.StringFixed(2), VAT: sv.StringFixed(2)},
			Portal: Totals{Count: n, Base: base.StringFixed(2), VAT: vat.StringFixed(2)},
		}
		s.side.Match = n == s.rep.InvoiceCount && sb.Sub(base).Abs().LessThanOrEqual(tolerance) && sv.Sub(vat).Abs().LessThanOrEqual(tolerance)
	}
	return rec, nil
}

type report struct {
	InvoiceCount int `json:"invoice_count"`
	Totals       struct {
		Base any `json:"base"`
		VAT  any `json:"vat"`
	} `json:"totals"`
}

func decimalOf(v any) decimal.Decimal {
	switch x := v.(type) {
	case float64:
		return decimal.NewFromFloat(x)
	case string:
		d, _ := decimal.NewFromString(x)
		return d
	}
	return decimal.Zero
}
