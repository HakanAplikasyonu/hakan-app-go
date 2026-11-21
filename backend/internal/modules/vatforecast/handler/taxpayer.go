package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// SettingTaxpayerPanel is the office setting (organizations.settings) that
// shows the VAT card and sales report in the mükellef panel (#780).
// Missing or true: shown. False: hidden, and the route answers 403.
const SettingTaxpayerPanel = "taxpayer_vat_sales_panel"

// CodeTaxpayerPanelDisabled is the 403 code when the office turned the
// mükellef VAT and sales report off.
const CodeTaxpayerPanelDisabled = "TAXPAYER_VAT_PANEL_DISABLED"

// Sales report sizes.
const (
	salesTrendMonths = 6
	salesTopBuyers   = 5
)

// TaxpayerPanelEnabled reads SettingTaxpayerPanel from an organization's
// settings object.
func TaxpayerPanelEnabled(settings []byte) bool {
	var s map[string]any
	if len(settings) == 0 || json.Unmarshal(settings, &s) != nil {
		return true
	}
	on, ok := s[SettingTaxpayerPanel].(bool)
	return !ok || on
}

// PortalQueries is what the mükellef route reads besides the forecast.
// *db.Queries implements it.
type PortalQueries interface {
	GetActiveEngagementForTaxpayer(ctx context.Context, taxpayerID int64) (db.Engagement, error)
	GetOrganizationByID(ctx context.Context, id int64) (db.Organization, error)
	TaxpayerSalesMonthly(ctx context.Context, arg db.TaxpayerSalesMonthlyParams) ([]db.TaxpayerSalesMonthlyRow, error)
	TaxpayerSalesTopBuyers(ctx context.Context, arg db.TaxpayerSalesTopBuyersParams) ([]db.TaxpayerSalesTopBuyersRow, error)
}

// WithPortal enables GET /v1/taxpayer/vat-sales. Without it the route
// answers 404.
func (h *Handler) WithPortal(q PortalQueries) *Handler {
	h.portal = q
	return h
}

// TaxpayerVATSales is the mükellef panel's VAT card and sales report.
type TaxpayerVATSales struct {
	Period        string           `json:"period"`
	CurrentPeriod string           `json:"current_period"`
	Totals        usecase.Totals   `json:"totals"`
	Calendar      usecase.Calendar `json:"calendar"`
	Sales         TaxpayerSales    `json:"sales"`
}

// TaxpayerSales is the sales report of the period.
type TaxpayerSales struct {
	// Base / VAT: net sales of the period (returns deducted), TRY.
	Base      usecase.Money `json:"base"`
	VAT       usecase.Money `json:"vat"`
	Invoices  int64         `json:"invoices"`
	TopBuyers []SalesBuyer  `json:"top_buyers"`
	// Trend: the last months up to the period, oldest first, every month
	// present (zero when there was no sale).
	Trend []SalesMonth `json:"trend"`
}

// SalesBuyer is one buyer of the period.
type SalesBuyer struct {
	Name     string        `json:"name"`
	TaxID    string        `json:"tax_id"`
	Invoices int64         `json:"invoices"`
	Base     usecase.Money `json:"base"`
}

// SalesMonth is one month of the sales trend.
type SalesMonth struct {
	Period   string        `json:"period"`
	Base     usecase.Money `json:"base"`
	VAT      usecase.Money `json:"vat"`
	Invoices int64         `json:"invoices"`
}

// Taxpayer: GET /v1/taxpayer/vat-sales (behind X-Taxpayer). The forecast
// runs for the taxpayer's active engagement in that office's scope.
func (h *Handler) Taxpayer(w http.ResponseWriter, r *http.Request) {
	if h.portal == nil || h.svc == nil {
		response.NotFound(w, r, "")
		return
	}
	s := taxpayerctx.Must(r.Context())
	period := h.period(r)
	start, err := usecase.ParsePeriod(period)
	if err != nil {
		writeError(w, r, err)
		return
	}
	eng, err := h.portal.GetActiveEngagementForTaxpayer(r.Context(), s.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "The taxpayer has no active office")
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	var out TaxpayerVATSales
	disabled := false
	err = rls.ForOffice(r.Context(), eng.OrganizationID, s.ID, func(ctx context.Context) error {
		org, err := h.portal.GetOrganizationByID(ctx, eng.OrganizationID)
		if err != nil {
			return err
		}
		if !TaxpayerPanelEnabled(org.Settings) {
			disabled = true
			return nil
		}
		f, err := h.svc.Forecast(ctx, usecase.Subject{
			OrganizationID: eng.OrganizationID, EngagementID: eng.ID, EngagementUUID: eng.Uuid,
			TaxpayerID: s.ID, TaxpayerUUID: s.UUID,
		}, period)
		if err != nil {
			return err
		}
		out = TaxpayerVATSales{Period: period, CurrentPeriod: h.svc.CurrentPeriod(), Totals: f.Totals, Calendar: f.Calendar}
		out.Sales, err = h.sales(ctx, s.ID, start)
		return err
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	if disabled {
		response.Error(w, r, http.StatusForbidden, CodeTaxpayerPanelDisabled, "The office turned the VAT and sales report off")
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// sales builds the sales report of the month starting at start.
func (h *Handler) sales(ctx context.Context, taxpayerID int64, start time.Time) (TaxpayerSales, error) {
	months := TrendPeriods(start, salesTrendMonths)
	period := months[len(months)-1]
	rows, err := h.portal.TaxpayerSalesMonthly(ctx, db.TaxpayerSalesMonthlyParams{
		TaxpayerID: taxpayerID, FromPeriod: months[0], ToPeriod: period,
	})
	if err != nil {
		return TaxpayerSales{}, err
	}
	buyers, err := h.portal.TaxpayerSalesTopBuyers(ctx, db.TaxpayerSalesTopBuyersParams{
		TaxpayerID: taxpayerID, Period: period, Lim: salesTopBuyers,
	})
	if err != nil {
		return TaxpayerSales{}, err
	}
	byPeriod := make(map[string]db.TaxpayerSalesMonthlyRow, len(rows))
	for _, row := range rows {
		byPeriod[row.Period] = row
	}
	out := TaxpayerSales{
		Base: usecase.M(decimal.Zero), VAT: usecase.M(decimal.Zero),
		TopBuyers: make([]SalesBuyer, 0, len(buyers)), Trend: make([]SalesMonth, 0, len(months)),
	}
	for _, p := range months {
		row, ok := byPeriod[p]
		m := SalesMonth{Period: p, Base: usecase.M(decimal.Zero), VAT: usecase.M(decimal.Zero)}
		if ok {
			m.Base = usecase.M(repository.Dec(row.Base).Round(2))
			m.VAT = usecase.M(repository.Dec(row.Vat).Round(2))
			m.Invoices = row.Invoices
		}
		out.Trend = append(out.Trend, m)
		if p == period {
			out.Base, out.VAT, out.Invoices = m.Base, m.VAT, m.Invoices
		}
	}
	for _, b := range buyers {
		out.TopBuyers = append(out.TopBuyers, SalesBuyer{
			Name: b.Name, TaxID: b.TaxID, Invoices: b.Invoices, Base: usecase.M(repository.Dec(b.Base).Round(2)),
		})
	}
	return out, nil
}

// TrendPeriods returns n months (YYYY-MM) ending with the month of start,
// oldest first.
func TrendPeriods(start time.Time, n int) []string {
	if n < 1 {
		n = 1
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = start.AddDate(0, i-n+1, 0).Format("2006-01")
	}
	return out
}
