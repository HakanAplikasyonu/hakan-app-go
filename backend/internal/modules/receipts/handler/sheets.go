package handler

import (
	"fmt"
	"io"
	"net/http"
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
