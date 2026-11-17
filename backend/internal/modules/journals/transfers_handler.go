package journals

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

var transferStatuses = map[string]bool{
	TransferQueued: true, TransferClaimed: true, TransferDelivered: true,
	TransferImported: true, TransferFailed: true, TransferCancelled: true,
}

func (h *Handler) requestTransfer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "batchUuid")
	if !ok {
		return
	}
	var in struct {
		Method string `json:"method"`
	}
	if !decodeOptional(w, r, &in) {
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	t, err := h.svc.RequestTransfer(r.Context(), engagementctx.Must(r.Context()), p.UserInternal, id, in.Method)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, t)
}

func (h *Handler) listTransfers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := paging(q.Get("limit"), q.Get("offset"))
	status := strings.TrimSpace(q.Get("status"))
	if status != "" && !transferStatuses[status] {
		response.BadRequest(w, r, "INVALID_STATUS", "unknown transfer status")
		return
	}
	page, err := h.svc.ListTransfers(r.Context(), engagementctx.Must(r.Context()), TransferFilter{Status: status, Limit: limit, Offset: offset})
	if err != nil {
		response.InternalErr(w, r, err, "could not list transfers")
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

func (h *Handler) getTransfer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "transferUuid")
	if !ok {
		return
	}
	t, err := h.svc.GetTransfer(r.Context(), engagementctx.Must(r.Context()), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

func (h *Handler) claimTransfer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "transferUuid")
	if !ok {
		return
	}
	var in struct {
		AgentUUID string `json:"agent_uuid"`
	}
	if !decodeOptional(w, r, &in) {
		return
	}
	agent, err := uuid.Parse(in.AgentUUID)
	if err != nil {
		response.BadRequest(w, r, "AGENT_REQUIRED", "agent_uuid is required")
		return
	}
	t, err := h.svc.ClaimTransfer(r.Context(), engagementctx.Must(r.Context()), id, agent)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

func (h *Handler) deliverTransfer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "transferUuid")
	if !ok {
		return
	}
	var in DeliveryInput
	if !decodeOptional(w, r, &in) {
		return
	}
	t, err := h.svc.ReportDelivery(r.Context(), engagementctx.Must(r.Context()), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

func (h *Handler) completeTransfer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "transferUuid")
	if !ok {
		return
	}
	var in CompleteInput
	if !decodeOptional(w, r, &in) {
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	t, err := h.svc.CompleteTransfer(r.Context(), engagementctx.Must(r.Context()), p.UserInternal, id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

func (h *Handler) cancelTransfer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "transferUuid")
	if !ok {
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	t, err := h.svc.CancelTransfer(r.Context(), engagementctx.Must(r.Context()), p.UserInternal, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

func (h *Handler) retryTransfer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "transferUuid")
	if !ok {
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	t, err := h.svc.RetryTransfer(r.Context(), engagementctx.Must(r.Context()), p.UserInternal, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, t)
}

// decodeOptional reads a small JSON body; an empty body leaves dst as is.
func decodeOptional(w http.ResponseWriter, r *http.Request, dst any) bool {
	err := json.NewDecoder(io.LimitReader(r.Body, 256<<10)).Decode(dst)
	if err != nil && err != io.EOF {
		response.BadRequest(w, r, "INVALID_BODY", "request body is not valid JSON")
		return false
	}
	return true
}
