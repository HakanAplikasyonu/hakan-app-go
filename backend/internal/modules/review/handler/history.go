package handler

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// CounterpartyHistory: GET /v1/tenant/engagements/{engagementUuid}/suggestions/history?counterparty=…
// The counterparty's past line account codes (#669 L), from all decisions.
func (h *Handler) CounterpartyHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := intconv.ParseInt32(q.Get("limit"), 5, 1, 20)
	items, err := h.svc.CounterpartyHistory(r.Context(), subject(r), q.Get("counterparty"), limit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}
