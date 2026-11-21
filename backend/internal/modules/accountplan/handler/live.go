package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/i18n"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ioengine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// istanbul is the default period's zone (the panel's month).
var istanbul = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return l
	}
	return time.FixedZone("TRT", 3*3600)
}()

func (h *Handler) liveReport(w http.ResponseWriter, r *http.Request) (usecase.LiveTrialBalanceReport, bool) {
	q := r.URL.Query()
	level := 0
	if v := q.Get("level"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "level must be a number")
			return usecase.LiveTrialBalanceReport{}, false
		}
		level = n
	}
	drafts := q.Get("include_drafts") == "true"
	period := strings.TrimSpace(q.Get("period"))
	if period == "" {
		period = time.Now().In(istanbul).Format("2006-01")
	}
	rep, err := h.svc.LiveTrialBalance(r.Context(), subject(r), period, drafts, q.Get("compare"), level)
	if err != nil {
		writeError(w, r, err)
		return rep, false
	}
	return rep, true
}

// LiveTrialBalance: GET .../account-plan/live-trial-balance?period=&include_drafts=&compare=&level=
// — the trial balance from the office's own journal entries (#669 E).
func (h *Handler) LiveTrialBalance(w http.ResponseWriter, r *http.Request) {
	rep, ok := h.liveReport(w, r)
	if !ok {
		return
	}
	response.JSON(w, r, http.StatusOK, rep)
}

// LiveTrialBalanceExport: GET .../account-plan/live-trial-balance/export?format=xlsx|pdf&period=…
func (h *Handler) LiveTrialBalanceExport(w http.ResponseWriter, r *http.Request) {
	format := ioengine.ExportFormat(r.URL.Query().Get("format"))
	if format == "" {
		format = ioengine.ExportXLSX
	}
	if format != ioengine.ExportPDF && format != ioengine.ExportXLSX {
		response.BadRequest(w, r, response.CodeValidationError, "format must be xlsx or pdf")
		return
	}
	rep, ok := h.liveReport(w, r)
	if !ok {
		return
	}
	locale := string(i18n.LocaleTR)
	if v := r.URL.Query().Get("locale"); v != "" {
		locale = v
	}
	ds := TrialBalanceDataset(usecase.TrialBalanceReport{ComparedTo: rep.ComparedTo, Lines: rep.Lines})
	title := fmt.Sprintf("%s %s – %s", i18n.Translate(i18n.Normalize(locale), "trial_balance.live_title"), rep.FromPeriod, rep.Period)
	writeExport(w, r, ds, format, locale, title, "canli_mizan_"+rep.Period)
}

// CariBalances: GET .../account-plan/cari-balances?period=&include_drafts=
// — the cari mizanı from the office's own journal entries (#780 C).
func (h *Handler) CariBalances(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	period := strings.TrimSpace(q.Get("period"))
	if period == "" {
		period = time.Now().In(istanbul).Format("2006-01")
	}
	rep, err := h.svc.CariBalances(r.Context(), subject(r), period, q.Get("include_drafts") == "true")
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rep)
}

// writeExport encodes ds and sends it as an attachment.
func writeExport(w http.ResponseWriter, r *http.Request, ds ioengine.Dataset, format ioengine.ExportFormat, locale, title, name string) {
	var (
		body []byte
		err  error
	)
	if format == ioengine.ExportPDF {
		body, err = ioengine.EncodePDF(ds, locale, nil, title)
	} else {
		body, err = ioengine.EncodeXLSX(ds, locale, nil)
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not build the trial balance file")
		return
	}
	w.Header().Set("Content-Type", ioengine.ContentTypeForExport(format))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.%s"`, name, ioengine.FileExtForExport(format)))
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
