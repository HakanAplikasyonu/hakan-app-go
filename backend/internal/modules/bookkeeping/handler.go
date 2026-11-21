package bookkeeping

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the defter türü and işletme hesabı routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the office routes behind the active-engagement chain.
// A nil resolver registers nothing.
func RegisterRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver, engResolver engagementctx.Resolver, h *Handler,
) {
	if orgResolver == nil || engResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(orgResolver)
	eng := middleware.RequireEngagement(engResolver)
	route := func(method, path, perm string, fn http.HandlerFunc) {
		mux.Handle(method+" /v1/tenant/engagements/{"+engagementctx.PathParam+"}"+path,
			middleware.Chain(fn, authn, org, middleware.RequireTenantPermission(perm), eng))
	}
	// Defter türü mükellef ayarıdır (#780).
	route("PUT", "/ledger-type", rbac.PermTenantEngagementsWrite, h.setLedgerType)
	route("GET", "/operating-account", rbac.PermTenantInvoicesRead, h.book)
	route("PUT", "/operating-account/categories", rbac.PermTenantInvoicesWrite, h.setCategory)
	route("PUT", "/operating-account/inventory", rbac.PermTenantInvoicesWrite, h.setInventory)
}

func userID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func (h *Handler) setLedgerType(w http.ResponseWriter, r *http.Request) {
	var in LedgerType
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.SetLedgerType(r.Context(), engagementctx.Must(r.Context()), in.LedgerType)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) book(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out, err := h.svc.Book(r.Context(), engagementctx.Must(r.Context()), q.Get("period"), q.Get("year"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) setCategory(w http.ResponseWriter, r *http.Request) {
	var in CategoryInput
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.SetCategory(r.Context(), engagementctx.Must(r.Context()), userID(r), in); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) setInventory(w http.ResponseWriter, r *http.Request) {
	var in InventoryInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.SetInventory(r.Context(), engagementctx.Must(r.Context()), userID(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "not found")
	case errors.Is(err, ErrBalanceType):
		response.Conflict(w, r, "LEDGER_NOT_SINGLE_ENTRY", "the operating account is kept only for işletme defteri and serbest meslek taxpayers")
	default:
		response.InternalErr(w, r, err, "operating account request failed")
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "request body is not valid JSON for this endpoint")
		return false
	}
	return true
}
