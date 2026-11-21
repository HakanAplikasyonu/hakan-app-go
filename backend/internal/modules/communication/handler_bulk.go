package communication

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// İletişim sayfası (#780 B): mükellef grupları ve elle toplu gönderim.

// decodeLarge reads a body of up to 128 KiB (500 engagement uuids).
func decodeLarge(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func (h *Handler) failBulk(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, response.CodeNotFound, "Kayıt bulunamadı.")
		return
	}
	h.fail(w, r, err)
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.Error(w, r, http.StatusNotFound, response.CodeNotFound, "Kayıt bulunamadı.")
		return uuid.Nil, false
	}
	return id, true
}

// ListGroups: GET /v1/tenant/communication/groups.
func (h *Handler) ListGroups(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListGroups(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		h.failBulk(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// GetGroup: GET /v1/tenant/communication/groups/{groupUuid}.
func (h *Handler) GetGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "groupUuid")
	if !ok {
		return
	}
	g, err := h.svc.GetGroup(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		h.failBulk(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, g)
}

// CreateGroup: POST /v1/tenant/communication/groups.
func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var in GroupInput
	if !decodeLarge(w, r, &in) {
		return
	}
	g, err := h.svc.CreateGroup(r.Context(), orgctx.Must(r.Context()).ID, actor(r), in)
	if err != nil {
		h.failBulk(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, g)
}

// UpdateGroup: PUT /v1/tenant/communication/groups/{groupUuid}.
func (h *Handler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "groupUuid")
	if !ok {
		return
	}
	var in GroupInput
	if !decodeLarge(w, r, &in) {
		return
	}
	g, err := h.svc.UpdateGroup(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		h.failBulk(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, g)
}

// DeleteGroup: DELETE /v1/tenant/communication/groups/{groupUuid}.
func (h *Handler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "groupUuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteGroup(r.Context(), orgctx.Must(r.Context()).ID, id); err != nil {
		h.failBulk(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Filters: GET /v1/tenant/communication/filters.
func (h *Handler) Filters(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Filters(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		h.failBulk(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// PreviewBulk: POST /v1/tenant/communication/bulk-sends/preview.
func (h *Handler) PreviewBulk(w http.ResponseWriter, r *http.Request) {
	var in BulkInput
	if !decodeLarge(w, r, &in) {
		return
	}
	v, err := h.svc.Preview(r.Context(), orgctx.Must(r.Context()).ID, actor(r), in)
	if err != nil {
		h.failBulk(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// StartBulk: POST /v1/tenant/communication/bulk-sends (202, runs in the
// background; poll GET …/bulk-sends/{bulkUuid}).
func (h *Handler) StartBulk(w http.ResponseWriter, r *http.Request) {
	var in BulkInput
	if !decodeLarge(w, r, &in) {
		return
	}
	v, err := h.svc.StartBulk(r.Context(), orgctx.Must(r.Context()).ID, actor(r), in)
	if err != nil {
		h.failBulk(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, v)
}

// ListBulk: GET /v1/tenant/communication/bulk-sends (the user's last five).
func (h *Handler) ListBulk(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.RecentBulk(r.Context(), orgctx.Must(r.Context()).ID, actor(r))
	if err != nil {
		h.failBulk(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// GetBulk: GET /v1/tenant/communication/bulk-sends/{bulkUuid}.
func (h *Handler) GetBulk(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "bulkUuid")
	if !ok {
		return
	}
	v, err := h.svc.GetBulk(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		h.failBulk(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}
