package handler

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/periodq"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

const xlsxType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// ExportExpenses: GET .../expenses/export (.xlsx, #384).
func (h *Handler) ExportExpenses(w http.ResponseWriter, r *http.Request) {
	period, ok := periodq.Parse(w, r)
	if !ok {
		return
	}
	qs := r.URL.Query()
	data, err := h.svc.ExportExpenses(r.Context(), engagementctx.Must(r.Context()), usecase.ExpenseFilter{
		Period: period, Status: qs.Get("status"), Category: qs.Get("category"), Q: qs.Get("q"),
	})
	writeXLSX(w, r, "masraflar", period, data, err)
}

// ExportExpensesLuca: GET .../expenses/luca?period=YYYY-MM — the period's
// expenses as a Luca fiş file (#780): .xlsx, or .zip when Luca's 50-voucher
// limit needs several files. The X-Expenses-* headers count what went in.
func (h *Handler) ExportExpensesLuca(w http.ResponseWriter, r *http.Request) {
	period, ok := periodq.Parse(w, r)
	if !ok {
		return
	}
	f, err := h.svc.ExportExpensesLuca(r.Context(), engagementctx.Must(r.Context()), period)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, f.Name))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Expenses-Exported", strconv.Itoa(f.Exported))
	w.Header().Set("X-Expenses-No-Breakdown", strconv.Itoa(f.NoBreakdown))
	w.Header().Set("X-Expenses-Skipped", strconv.Itoa(f.Skipped))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(f.Data)
}

// ExportRetailSales: GET .../retail-sales/export (.xlsx).
func (h *Handler) ExportRetailSales(w http.ResponseWriter, r *http.Request) {
	period, ok := periodq.Parse(w, r)
	if !ok {
		return
	}
	qs := r.URL.Query()
	data, err := h.svc.ExportRetailSales(r.Context(), engagementctx.Must(r.Context()), usecase.RetailFilter{
		Period: period, Status: qs.Get("status"), OKC: qs.Get("okc"),
	})
	writeXLSX(w, r, "z-raporlari", period, data, err)
}

// ImportExpenses: POST .../expenses/import (multipart file).
func (h *Handler) ImportExpenses(w http.ResponseWriter, r *http.Request) {
	name, data, ok := readSheetFile(w, r)
	if !ok {
		return
	}
	res, err := h.svc.ImportExpenses(r.Context(), engagementctx.Must(r.Context()), userID(r), name, data)
	replyAny(w, r, res, err)
}

// ImportRetailSales: POST .../retail-sales/import (multipart file).
func (h *Handler) ImportRetailSales(w http.ResponseWriter, r *http.Request) {
	name, data, ok := readSheetFile(w, r)
	if !ok {
		return
	}
	res, err := h.svc.ImportRetailSales(r.Context(), engagementctx.Must(r.Context()), name, data)
	replyAny(w, r, res, err)
}

func readSheetFile(w http.ResponseWriter, r *http.Request) (string, []byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxSheetBytes+1<<20)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "multipart form with a file is required (max 10 MB)")
		return "", nil, false
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return "", nil, false
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, usecase.MaxSheetBytes+1))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file could not be read")
		return "", nil, false
	}
	return hdr.Filename, data, true
}

func writeXLSX(w http.ResponseWriter, r *http.Request, base, period string, data []byte, err error) {
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if period == "" {
		period = time.Now().Format("2006-01-02")
	}
	w.Header().Set("Content-Type", xlsxType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.xlsx"`, base, period))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
