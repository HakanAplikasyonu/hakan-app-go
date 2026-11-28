package handler

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/periodq"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/apiquery"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
)

// RetailSales: GET .../retail-sales (#384).
func (h *Handler) RetailSales(w http.ResponseWriter, r *http.Request) {
	period, ok := periodq.Parse(w, r)
	if !ok {
		return
	}
	qs := r.URL.Query()
	page, err := h.svc.RetailSales(r.Context(), engagementctx.Must(r.Context()), usecase.RetailFilter{
		Period: period, Status: qs.Get("status"), OKC: qs.Get("okc"),
		Limit:  intconv.ParseInt32(qs.Get("limit"), apiquery.DefaultLimit, 1, apiquery.MaxLimit),
		Offset: intconv.ParseInt32(qs.Get("offset"), 0, 0, 1_000_000),
	})
	replyAny(w, r, page, err)
}

// RetailSale: GET .../retail-sales/{retailSaleUuid}.
func (h *Handler) RetailSale(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "retailSaleUuid")
	if !ok {
		return
	}
	v, err := h.svc.RetailSale(r.Context(), engagementctx.Must(r.Context()), id)
	replyAny(w, r, v, err)
}

// UpdateRetailSale: PATCH .../retail-sales/{retailSaleUuid}.
func (h *Handler) UpdateRetailSale(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "retailSaleUuid")
	if !ok {
		return
	}
	var in usecase.RetailInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.UpdateRetailSale(r.Context(), engagementctx.Must(r.Context()), id, in)
	replyAny(w, r, v, err)
}

// ApproveRetailSale / RejectRetailSale: POST .../retail-sales/{retailSaleUuid}/approve|reject.
func (h *Handler) ApproveRetailSale(w http.ResponseWriter, r *http.Request) {
	h.decideRetail(w, r, true)
}
func (h *Handler) RejectRetailSale(w http.ResponseWriter, r *http.Request) {
	h.decideRetail(w, r, false)
}

func (h *Handler) decideRetail(w http.ResponseWriter, r *http.Request, approve bool) {
	id, ok := pathID(w, r, "retailSaleUuid")
	if !ok {
		return
	}
	var rej usecase.Rejection
	if !approve {
		if rej, ok = readRejection(w, r); !ok {
			return
		}
	}
	v, err := h.svc.DecideRetailSaleWithReason(r.Context(), engagementctx.Must(r.Context()), id, userID(r), approve, rej)
	if err == nil && !approve {
		h.auditRejection(r, "retail_sale", id, rej)
	}
	replyAny(w, r, v, err)
}

// RunZContinuity: POST .../retail-sales/continuity.
func (h *Handler) RunZContinuity(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.RunZContinuity(r.Context(), engagementctx.Must(r.Context()))
	replyAny(w, r, map[string]any{"reports_with_findings": n}, err)
}
