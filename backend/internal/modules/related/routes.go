package related

import (
	"errors"
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves GET .../related/{kind}/{recordUuid}.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Module mounts the route. Scope runs the request in one row level security
// transaction (rls.Middleware): the source tables are enforced.
type Module struct {
	H           *Handler
	Engagements engagementctx.Resolver
	Scope       func(http.Handler) http.Handler
}

// Name implements module.Module.
func (Module) Name() string { return "related" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{middleware.RequireEngagement(m.Engagements)}
	if m.Scope != nil {
		eng = append(eng, m.Scope)
	}
	r.Tenant("GET /v1/tenant/engagements/{engagementUuid}/related/{kind}/{recordUuid}",
		rbac.PermTenantEngagementsRead, h.List, eng...)
}

// List: GET /v1/tenant/engagements/{engagementUuid}/related/{kind}/{recordUuid}.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !IsSourceKind(kind) {
		response.BadRequest(w, r, "INVALID_KIND", "unknown record kind")
		return
	}
	id, err := uuid.Parse(r.PathValue("recordUuid"))
	if err != nil {
		response.BadRequest(w, r, "INVALID_UUID", "invalid record uuid")
		return
	}
	scope := orgctx.Must(r.Context())
	if !scope.Has(Permission(kind)) {
		response.Forbidden(w, r, "Missing permission: "+Permission(kind))
		return
	}
	e := engagementctx.Must(r.Context())
	res, err := h.svc.List(r.Context(), Subject{
		OrganizationID: e.OrganizationID, EngagementID: e.ID, TaxpayerID: e.TaxpayerID,
	}, kind, id, scope.Has)
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "record not found")
	case err != nil:
		response.InternalErr(w, r, err, "")
	default:
		response.JSON(w, r, http.StatusOK, res)
	}
}
