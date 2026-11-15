// Package handler serves the AI module's tenant routes: the office cost
// report and the provider privacy statement.
package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Service is what the handler needs from the use case.
type Service interface {
	Report(ctx context.Context, organizationID int64, from, to time.Time) ([]usecase.ReportRow, error)
	Privacy() usecase.Privacy
}

// Handler serves /v1/tenant/ai/*.
type Handler struct {
	svc Service
	loc *time.Location
}

// New builds the handler. Report dates are office-local (Europe/Istanbul).
func New(svc Service) *Handler {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		loc = time.FixedZone("TRT", 3*60*60)
	}
	return &Handler{svc: svc, loc: loc}
}

// maxReportDays bounds one report request.
const maxReportDays = 366

type usageReport struct {
	From   string              `json:"from"`
	To     string              `json:"to"`
	Rows   []usecase.ReportRow `json:"rows"`
	Totals usageTotals         `json:"totals"`
}

type usageTotals struct {
	Calls         int64  `json:"calls"`
	FallbackCalls int64  `json:"fallback_calls"`
	UnpricedCalls int64  `json:"unpriced_calls"`
	InputTokens   int64  `json:"input_tokens"`
	OutputTokens  int64  `json:"output_tokens"`
	CostUSD       string `json:"cost_usd"`
}

// Usage returns the office's AI cost report. Query: from, to (YYYY-MM-DD,
// to inclusive). Default: the current month.
func (h *Handler) Usage(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	now := time.Now().In(h.loc)
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, h.loc)
	toIncl := from.AddDate(0, 1, -1)
	var ok bool
	if v := r.URL.Query().Get("from"); v != "" {
		if from, ok = h.parseDate(v); !ok {
			response.BadRequest(w, r, response.CodeValidationError, "from must be YYYY-MM-DD")
			return
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if toIncl, ok = h.parseDate(v); !ok {
			response.BadRequest(w, r, response.CodeValidationError, "to must be YYYY-MM-DD")
			return
		}
	}
	to := toIncl.AddDate(0, 0, 1)
	if !to.After(from) || to.Sub(from) > maxReportDays*24*time.Hour {
		response.BadRequest(w, r, response.CodeValidationError, "range must be 1-366 days with from <= to")
		return
	}
	rows, err := h.svc.Report(r.Context(), s.ID, from, to)
	if err != nil {
		response.Internal(w, r, "ai usage report failed")
		return
	}
	out := usageReport{From: from.Format(time.DateOnly), To: toIncl.Format(time.DateOnly), Rows: rows}
	var micro int64
	for _, row := range rows {
		out.Totals.Calls += row.Calls
		out.Totals.FallbackCalls += row.FallbackCalls
		out.Totals.UnpricedCalls += row.UnpricedCalls
		out.Totals.InputTokens += row.InputTokens
		out.Totals.OutputTokens += row.OutputTokens
		micro += parseMicroUSD(row.CostUSD)
	}
	out.Totals.CostUSD = usecase.FormatUSD(micro * 1000)
	response.JSON(w, r, http.StatusOK, out)
}

// Privacy returns the provider data-handling statement.
func (h *Handler) Privacy(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, h.svc.Privacy())
}

func (h *Handler) parseDate(v string) (time.Time, bool) {
	t, err := time.ParseInLocation(time.DateOnly, v, h.loc)
	return t, err == nil
}

// parseMicroUSD parses a non-negative decimal string ("1.234567") into
// micro-USD; extra fraction digits are truncated.
func parseMicroUSD(s string) int64 {
	whole, frac, _ := strings.Cut(strings.TrimSpace(s), ".")
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0
	}
	frac = (frac + "000000")[:6]
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0
	}
	return w*1_000_000 + f
}
