package exchangerates

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the office rate setting.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Module mounts the routes. Scope runs requests in one row level security
// transaction (rls.Middleware).
type Module struct {
	H     *Handler
	Scope func(http.Handler) http.Handler
}

// Name implements module.Module.
func (Module) Name() string { return "exchangerates" }

// Routes implements module.Module: reading needs tax_returns.read (the
// invoice and KDV2 screens), writing settings.write.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	var office []func(http.Handler) http.Handler
	if m.Scope != nil {
		office = append(office, m.Scope)
	}
	r.Tenant("GET /v1/tenant/exchange-rate-settings", rbac.PermTenantTaxReturnsRead, h.Get, office...)
	r.Tenant("PUT /v1/tenant/exchange-rate-settings", rbac.PermTenantSettingsWrite, h.Put, office...)
}

// Get: GET /v1/tenant/exchange-rate-settings.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Settings(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		response.InternalErr(w, r, err, "could not load the exchange rate setting")
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Put: PUT /v1/tenant/exchange-rate-settings.
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	var in Settings
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "body must be {\"rate_kind\": \"…\", \"auto_fill\": true}")
		return
	}
	var userID int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		userID = p.UserInternal
	}
	out, err := h.svc.SaveSettings(r.Context(), orgctx.Must(r.Context()).ID, in, userID)
	if errors.Is(err, ErrInvalid) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not save the exchange rate setting")
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
