package kolaybi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves /v1/tenant/engagements/{engagementUuid}/kolaybi-connections.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the KolayBi routes.
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
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/kolaybi-connections"
	one := base + "/{connectionUuid}"
	mux.Handle("GET "+base, middleware.Chain(http.HandlerFunc(h.list), authn, org, read, eng))
	mux.Handle("POST "+base, middleware.Chain(http.HandlerFunc(h.connect), authn, org, write, eng))
	mux.Handle("POST "+one+"/sync", middleware.Chain(http.HandlerFunc(h.sync), authn, org, write, eng))
	mux.Handle("PUT "+one+"/company", middleware.Chain(http.HandlerFunc(h.company), authn, org, write, eng))
}

func tp(r *http.Request) int64 { return engagementctx.Must(r.Context()).TaxpayerID }

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context(), tp(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) connect(w http.ResponseWriter, r *http.Request) {
	var in struct {
		APIKey    string `json:"api_key"`
		CompanyID string `json:"company_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return
	}
	c, cos, err := h.svc.Connect(r.Context(), tp(r), in.APIKey, in.CompanyID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if cos == nil {
		cos = []Company{}
	}
	response.JSON(w, r, http.StatusCreated, map[string]any{"connection": c, "companies": cos})
}

func (h *Handler) company(w http.ResponseWriter, r *http.Request) {
	id, ok := connID(w, r)
	if !ok {
		return
	}
	var in struct {
		CompanyID string `json:"company_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return
	}
	c, err := h.svc.SelectCompany(r.Context(), tp(r), id, in.CompanyID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

func connID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("connectionUuid"))
	if err != nil {
		response.NotFound(w, r, "connection not found")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) sync(w http.ResponseWriter, r *http.Request) {
	id, ok := connID(w, r)
	if !ok {
		return
	}
	var in struct{ From, To string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return
	}
	from, e1 := time.Parse("2006-01-02", in.From)
	to, e2 := time.Parse("2006-01-02", in.To)
	if e1 != nil || e2 != nil {
		response.BadRequest(w, r, "INVALID_RANGE", "from and to are YYYY-MM-DD")
		return
	}
	rep, err := h.svc.Sync(r.Context(), tp(r), id, from, to)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rep)
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotConfigured):
		response.Error(w, r, http.StatusServiceUnavailable, "KOLAYBI_NOT_CONFIGURED", "KOLAYBI_CHANNEL is not configured")
	case errors.Is(err, ErrNoVault):
		response.Error(w, r, http.StatusServiceUnavailable, "VAULT_NOT_CONFIGURED", "the credential vault is not configured")
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "connection not found")
	case errors.Is(err, ErrBadState):
		response.Conflict(w, r, "CONNECTION_STATE", "missing input or not allowed in the connection's current state")
	case errors.Is(err, ErrUnknownCo):
		response.Error(w, r, http.StatusUnprocessableEntity, "UNKNOWN_COMPANY", "the company is not accessible with this api key")
	case errors.Is(err, ErrThrottle):
		response.Error(w, r, http.StatusTooManyRequests, "SYNC_THROTTLED", "a sync was requested less than 5 minutes ago")
	case errors.Is(err, ErrBadRange):
		response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_RANGE", "the range is reversed or longer than 400 days")
	case errors.Is(err, ErrAuth):
		response.Error(w, r, http.StatusBadGateway, "KOLAYBI_AUTH_FAILED", "KolayBi refused the api key")
	default:
		response.InternalErr(w, r, err, "KolayBi request failed")
	}
}
