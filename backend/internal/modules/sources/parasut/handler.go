package parasut

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

// Handler serves /v1/tenant/engagements/{engagementUuid}/parasut-connections.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the Paraşüt connection routes.
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
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/parasut-connections"
	one := base + "/{connectionUuid}"
	mux.Handle("GET "+base, middleware.Chain(http.HandlerFunc(h.list), authn, org, read, eng))
	mux.Handle("POST "+base, middleware.Chain(http.HandlerFunc(h.create), authn, org, write, eng))
	mux.Handle("POST "+one+"/authorize", middleware.Chain(http.HandlerFunc(h.authorize), authn, org, write, eng))
	mux.Handle("GET "+one+"/companies", middleware.Chain(http.HandlerFunc(h.companies), authn, org, read, eng))
	mux.Handle("PUT "+one+"/company", middleware.Chain(http.HandlerFunc(h.company), authn, org, write, eng))
	mux.Handle("POST "+one+"/sync", middleware.Chain(http.HandlerFunc(h.sync), authn, org, write, eng))
}

func tp(r *http.Request) int64 { return engagementctx.Must(r.Context()).TaxpayerID }

func id(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	v, err := uuid.Parse(r.PathValue("connectionUuid"))
	if err != nil {
		response.NotFound(w, r, "connection not found")
		return uuid.Nil, false
	}
	return v, true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(v); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return false
	}
	return true
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context(), tp(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Create(r.Context(), tp(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, c)
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) {
	cid, ok := id(w, r)
	if !ok {
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Code == "" {
		response.BadRequest(w, r, "CODE_REQUIRED", "code is required")
		return
	}
	c, cos, err := h.svc.Authorize(r.Context(), tp(r), cid, in.Code)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if cos == nil {
		cos = []Company{}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"connection": c, "companies": cos})
}

func (h *Handler) companies(w http.ResponseWriter, r *http.Request) {
	cid, ok := id(w, r)
	if !ok {
		return
	}
	cos, err := h.svc.Companies(r.Context(), tp(r), cid)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": cos})
}

func (h *Handler) company(w http.ResponseWriter, r *http.Request) {
	cid, ok := id(w, r)
	if !ok {
		return
	}
	var in struct {
		CompanyID string `json:"company_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, err := h.svc.SelectCompany(r.Context(), tp(r), cid, in.CompanyID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

func (h *Handler) sync(w http.ResponseWriter, r *http.Request) {
	cid, ok := id(w, r)
	if !ok {
		return
	}
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if !decode(w, r, &in) {
		return
	}
	from, e1 := time.Parse("2006-01-02", in.From)
	to, e2 := time.Parse("2006-01-02", in.To)
	if e1 != nil || e2 != nil {
		response.BadRequest(w, r, "INVALID_RANGE", "from and to are YYYY-MM-DD")
		return
	}
	rep, err := h.svc.Sync(r.Context(), tp(r), cid, from, to)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rep)
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotConfigured):
		response.Error(w, r, http.StatusServiceUnavailable, "PARASUT_NOT_CONFIGURED", "PARASUT_CLIENT_ID / _SECRET / _REDIRECT_URL are not configured")
	case errors.Is(err, ErrNoVault):
		response.Error(w, r, http.StatusServiceUnavailable, "VAULT_NOT_CONFIGURED", "the credential vault is not configured")
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "connection not found")
	case errors.Is(err, ErrBadState):
		response.Conflict(w, r, "CONNECTION_STATE", "not allowed in the connection's current state")
	case errors.Is(err, ErrUnknownCo):
		response.Error(w, r, http.StatusUnprocessableEntity, "UNKNOWN_COMPANY", "the company is not accessible with this Paraşüt login")
	case errors.Is(err, ErrThrottled):
		response.Error(w, r, http.StatusTooManyRequests, "SYNC_THROTTLED", "a sync was requested less than 5 minutes ago")
	case errors.Is(err, ErrBadRange):
		response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_RANGE", "the range is reversed or longer than 400 days")
	case errors.Is(err, ErrAuth):
		response.Error(w, r, http.StatusBadGateway, "PARASUT_AUTH_FAILED", "Paraşüt refused the authorization; reconnect")
	default:
		response.InternalErr(w, r, err, "Paraşüt request failed")
	}
}
