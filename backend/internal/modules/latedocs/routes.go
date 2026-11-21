package latedocs

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

const maxBodyBytes = 8 << 10

// Handler serves the late document routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Module mounts the routes. Scope runs engagement requests in one row level
// security transaction (rls.Middleware).
type Module struct {
	H           *Handler
	Engagements engagementctx.Resolver
	Scope       func(http.Handler) http.Handler
}

// Name implements module.Module.
func (Module) Name() string { return "latedocs" }

// Routes implements module.Module. Reading needs the period read permission;
// deciding needs the change request permission (the same people who decide
// what happens to data arriving for a locked period).
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	r.Tenant("GET /v1/tenant/late-documents/summary", rbac.PermTenantPeriodsRead, h.Summary)
	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	if m.Scope != nil {
		eng = append(eng, m.Scope)
	}
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/late-documents"
	r.Tenant("GET "+base, rbac.PermTenantPeriodsRead, h.List, eng...)
	r.Tenant("POST "+base+"/{lateDocumentUuid}/decision", rbac.PermTenantChangeRequestsDecide, h.Decide, eng...)
}

func subject(r *http.Request) Subject {
	e := engagementctx.Must(r.Context())
	return Subject{OrganizationID: e.OrganizationID, EngagementID: e.ID, TaxpayerID: e.TaxpayerID}
}

func userID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

// List: GET .../late-documents?status=&period=&document_uuid=.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := Filter{Status: q.Get("status"), Period: q.Get("period")}
	if v := q.Get("document_uuid"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "document_uuid is invalid")
			return
		}
		f.DocumentUUID = &id
	}
	out, err := h.svc.List(r.Context(), subject(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Decide: POST .../late-documents/{lateDocumentUuid}/decision.
func (h *Handler) Decide(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("lateDocumentUuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "lateDocumentUuid is invalid")
		return
	}
	var in Decision
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	out, err := h.svc.Decide(r.Context(), subject(r), id, in, userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Summary: GET /v1/tenant/late-documents/summary. Without
// tenant.engagements.read_all only assigned engagements are counted.
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	out, err := h.svc.Summary(r.Context(), s.ID, s.Has(rbac.PermTenantEngagementsReadAll), userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "Late document not found")
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
