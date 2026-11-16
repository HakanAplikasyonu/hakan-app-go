package taxpayers

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

const maxBodyBytes = 4 << 10

// Handler serves the platform taxpayer catalog and the office-side taxpayer
// profile of an engagement.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

type page struct {
	Items  []Taxpayer `json:"items"`
	Total  int64      `json:"total"`
	Limit  int32      `json:"limit"`
	Offset int32      `json:"offset"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	items, total, err := h.svc.List(r.Context(), r.URL.Query().Get("q"), limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Create(r.Context(), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in PatchInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Patch(r.Context(), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// engagementTaxpayer returns the taxpayer of the engagement in the route. It
// runs behind RequireEngagement and re-applies the access rule in SQL, so a
// missed middleware cannot widen access.
func (h *Handler) engagementTaxpayer(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	e := engagementctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	out, err := h.svc.ForOfficeMember(r.Context(), s.ID, p.UserInternal, s.Has(rbac.PermTenantEngagementsReadAll), e.TaxpayerID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.RecordFailure(w, err)
			response.Error(w, r, http.StatusForbidden, response.CodeNoEngagementAccess, "No access to this engagement")
			return
		}
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, ErrAlreadyExists):
		response.Conflict(w, r, response.CodeTaxpayerAlreadyExists, "A taxpayer with this tax id already exists")
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "Resource was not found")
	case errors.Is(err, ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

func pageParams(r *http.Request) (int32, int32) {
	limit, offset := int32(50), int32(0)
	limit = intconv.ParseInt32(r.URL.Query().Get("limit"), limit, 1, 100)
	offset = intconv.ParseInt32(r.URL.Query().Get("offset"), offset, 0, math.MaxInt32)
	return limit, offset
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue(name)))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, name+" is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}
