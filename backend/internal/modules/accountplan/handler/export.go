package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/i18n"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ioengine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// trialBalanceColumns are the mizan export columns (#669 E); labels in the
// i18n catalog under trial_balance.*.
func trialBalanceColumns(compare bool) []ioengine.Column {
	cols := []ioengine.Column{
		{Key: "code", LabelKey: "trial_balance.code", Type: ioengine.ColumnTypeString},
		{Key: "name", LabelKey: "trial_balance.name", Type: ioengine.ColumnTypeString},
		{Key: "debit", LabelKey: "trial_balance.debit", Type: ioengine.ColumnTypeString},
		{Key: "credit", LabelKey: "trial_balance.credit", Type: ioengine.ColumnTypeString},
		{Key: "debit_balance", LabelKey: "trial_balance.debit_balance", Type: ioengine.ColumnTypeString},
		{Key: "credit_balance", LabelKey: "trial_balance.credit_balance", Type: ioengine.ColumnTypeString},
	}
	if compare {
		cols = append(cols,
			ioengine.Column{Key: "previous_balance", LabelKey: "trial_balance.previous_balance", Type: ioengine.ColumnTypeString},
			ioengine.Column{Key: "change", LabelKey: "trial_balance.change", Type: ioengine.ColumnTypeString})
	}
	return cols
}

// TrialBalanceDataset turns a mizan report into an export table: borç /
// alacak bakiyesi split from the signed balance, the main-account rows as
// they come (level filter applied by the report).
func TrialBalanceDataset(rep usecase.TrialBalanceReport) ioengine.Dataset {
	compare := rep.ComparedTo != nil
	ds := ioengine.Dataset{Resource: "trial_balance", Columns: trialBalanceColumns(compare)}
	for _, l := range rep.Lines {
		row := map[string]any{
			"code": l.Code, "name": l.Name, "debit": l.Debit, "credit": l.Credit,
			"debit_balance": "", "credit_balance": "",
		}
		if b := strings.TrimSpace(l.Balance); strings.HasPrefix(b, "-") {
			row["credit_balance"] = strings.TrimPrefix(b, "-")
		} else if b != "" && b != "0" && b != "0.00" {
			row["debit_balance"] = b
		}
		if compare {
			if l.PreviousBalance != nil {
				row["previous_balance"] = *l.PreviousBalance
			}
			if l.Change != nil {
				row["change"] = *l.Change
			}
		}
		ds.Rows = append(ds.Rows, row)
	}
	return ds
}

// TrialBalanceExport: GET .../account-plan/trial-balances/{trialBalance}/export?format=xlsx|pdf&compare=&level=
// — the mizan report as a file (#669 E).
func (h *Handler) TrialBalanceExport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	format := ioengine.ExportFormat(q.Get("format"))
	if format != ioengine.ExportPDF && format != ioengine.ExportXLSX {
		response.BadRequest(w, r, response.CodeValidationError, "format must be xlsx or pdf")
		return
	}
	level := 0
	if v := q.Get("level"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "level must be a number")
			return
		}
		level = n
	}
	rep, err := h.svc.TrialBalanceReport(r.Context(), subject(r), r.PathValue("trialBalance"), q.Get("compare"), level)
	if err != nil {
		writeError(w, r, err)
		return
	}
	locale := string(i18n.LocaleTR)
	if v := q.Get("locale"); v != "" {
		locale = v
	}
	ds := TrialBalanceDataset(rep)
	period := ""
	if rep.TrialBalance.Period != nil {
		period = *rep.TrialBalance.Period
	}
	title := strings.TrimSpace(i18n.Translate(i18n.Normalize(locale), "trial_balance.title") + " " + period)
	var body []byte
	if format == ioengine.ExportPDF {
		body, err = ioengine.EncodePDF(ds, locale, nil, title)
	} else {
		body, err = ioengine.EncodeXLSX(ds, locale, nil)
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not build the trial balance file")
		return
	}
	name := "mizan"
	if period != "" {
		name += "_" + period
	}
	w.Header().Set("Content-Type", ioengine.ContentTypeForExport(format))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.%s"`, name, ioengine.FileExtForExport(format)))
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
