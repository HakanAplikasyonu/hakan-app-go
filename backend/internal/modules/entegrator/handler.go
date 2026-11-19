package entegrator

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the integrator connection routes.
type Handler struct{ svc *Connections }

// NewHandler builds the handler.
func NewHandler(svc *Connections) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts:
//
//	GET  /v1/tenant/einvoice-integrators
//	GET  /v1/tenant/engagements/{engagementUuid}/einvoice-connections
//	POST /v1/tenant/engagements/{engagementUuid}/einvoice-connections
//	GET  …/einvoice-connections/{connectionUuid}
//	PUT  …/einvoice-connections/{connectionUuid}/credentials
//	POST …/einvoice-connections/{connectionUuid}/sync
func RegisterRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver, engResolver engagementctx.Resolver, h *Handler,
) {
	if orgResolver == nil || engResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(orgResolver)
	eng := middleware.RequireEngagement(engResolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantInvoicesRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantInvoicesWrite)
	mux.Handle("GET /v1/tenant/einvoice-integrators", middleware.Chain(http.HandlerFunc(h.catalog), authn, org, read))
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/einvoice-connections"
	one := base + "/{connectionUuid}"
	mux.Handle("GET "+base, middleware.Chain(http.HandlerFunc(h.list), authn, org, read, eng))
	mux.Handle("POST "+base, middleware.Chain(http.HandlerFunc(h.create), authn, org, write, eng))
	mux.Handle("GET "+one, middleware.Chain(http.HandlerFunc(h.get), authn, org, read, eng))
	mux.Handle("PUT "+one+"/credentials", middleware.Chain(http.HandlerFunc(h.credentials), authn, org, write, eng))
	mux.Handle("POST "+one+"/sync", middleware.Chain(http.HandlerFunc(h.sync), authn, org, write, eng))
}

func (h *Handler) catalog(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]any{"items": Catalog()})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context(), engagementctx.Must(r.Context()).TaxpayerID)
	if err != nil {
		response.InternalErr(w, r, err, "list failed")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IntegratorID string `json:"integrator_id"`
		Environment  string `json:"environment"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return
	}
	c, err := h.svc.Create(r.Context(), engagementctx.Must(r.Context()).TaxpayerID, in.IntegratorID, in.Environment)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, c)
}

func connID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("connectionUuid"))
	if err != nil {
		response.NotFound(w, r, "integrator connection not found")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := connID(w, r)
	if !ok {
		return
	}
	c, err := h.svc.Get(r.Context(), engagementctx.Must(r.Context()).TaxpayerID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

func (h *Handler) credentials(w http.ResponseWriter, r *http.Request) {
	id, ok := connID(w, r)
	if !ok {
		return
	}
	var in struct {
		Values map[string]string `json:"values"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return
	}
	c, err := h.svc.SetCredentials(r.Context(), engagementctx.Must(r.Context()).TaxpayerID, id, in.Values)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	// The integrator's answer comes with the first sync: the connection is
	// "waiting" until the worker marks it active or auth_failed.
	response.JSON(w, r, http.StatusOK, c)
}

func (h *Handler) sync(w http.ResponseWriter, r *http.Request) {
	id, ok := connID(w, r)
	if !ok {
		return
	}
	c, err := h.svc.Sync(r.Context(), engagementctx.Must(r.Context()).TaxpayerID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, c)
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrConnectionNotFound):
		response.NotFound(w, r, "integrator connection not found")
	case errors.Is(err, ErrUnknownIntegrator):
		response.Error(w, r, http.StatusUnprocessableEntity, "INTEGRATOR_NOT_SUPPORTED", "unknown e-invoice integrator")
	case errors.Is(err, ErrDuplicate):
		response.Conflict(w, r, "INTEGRATOR_CONNECTION_EXISTS", "the taxpayer already has a connection to this integrator")
	case errors.Is(err, ErrBadCredentials):
		response.Error(w, r, http.StatusUnprocessableEntity, "CREDENTIALS_INCOMPLETE", err.Error())
	case errors.Is(err, ErrNotReady):
		response.Conflict(w, r, "INTEGRATOR_CONNECTION_STATE", "enter the web-service user first")
	case errors.Is(err, ErrNoVault):
		response.Error(w, r, http.StatusServiceUnavailable, "VAULT_NOT_CONFIGURED", "the credential vault is not configured")
	case errors.Is(err, ErrNoQueue):
		response.Conflict(w, r, "QUEUE_NOT_CONFIGURED", "syncs need the job queue (QUEUE_ENABLED); it is off on this server")
	default:
		response.InternalErr(w, r, err, "integrator connection failed")
	}
}
