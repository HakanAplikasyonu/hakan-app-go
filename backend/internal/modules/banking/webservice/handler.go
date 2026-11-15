package webservice

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

// Handler serves the bank connection routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts:
//
//	GET  /v1/tenant/bank-services
//	GET  /v1/tenant/engagements/{engagementUuid}/bank-connections
//	POST /v1/tenant/engagements/{engagementUuid}/bank-connections
//	GET  …/bank-connections/{connectionUuid}
//	GET  …/bank-connections/{connectionUuid}/instruction   (PDF)
//	POST …/bank-connections/{connectionUuid}/submitted
//	PUT  …/bank-connections/{connectionUuid}/credentials
//	POST …/bank-connections/{connectionUuid}/sync
func RegisterRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver, engResolver engagementctx.Resolver, h *Handler,
) {
	if orgResolver == nil || engResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(orgResolver)
	eng := middleware.RequireEngagement(engResolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantBankRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantBankWrite)
	mux.Handle("GET /v1/tenant/bank-services", middleware.Chain(http.HandlerFunc(h.banks), authn, org, read))
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/bank-connections"
	one := base + "/{connectionUuid}"
	mux.Handle("GET "+base, middleware.Chain(http.HandlerFunc(h.list), authn, org, read, eng))
	mux.Handle("POST "+base, middleware.Chain(http.HandlerFunc(h.create), authn, org, write, eng))
	mux.Handle("GET "+one, middleware.Chain(http.HandlerFunc(h.get), authn, org, read, eng))
	mux.Handle("GET "+one+"/instruction", middleware.Chain(http.HandlerFunc(h.instruction), authn, org, write, eng))
	mux.Handle("POST "+one+"/submitted", middleware.Chain(http.HandlerFunc(h.submitted), authn, org, write, eng))
	mux.Handle("PUT "+one+"/credentials", middleware.Chain(http.HandlerFunc(h.credentials), authn, org, write, eng))
	mux.Handle("POST "+one+"/sync", middleware.Chain(http.HandlerFunc(h.sync), authn, org, write, eng))
}

func (h *Handler) banks(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]any{"items": h.svc.Banks()})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	scope := engagementctx.Must(r.Context())
	items, err := h.svc.List(r.Context(), scope.TaxpayerID)
	if err != nil {
		response.InternalErr(w, r, err, "list failed")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type createReq struct {
	BankID      string   `json:"bank_id"`
	Environment string   `json:"environment"`
	IBANs       []string `json:"ibans"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	scope := engagementctx.Must(r.Context())
	var in createReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return
	}
	c, err := h.svc.Create(r.Context(), scope.TaxpayerID, in.BankID, in.Environment, in.IBANs)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, c)
}

func connID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("connectionUuid"))
	if err != nil {
		response.NotFound(w, r, "bank connection not found")
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

func (h *Handler) instruction(w http.ResponseWriter, r *http.Request) {
	id, ok := connID(w, r)
	if !ok {
		return
	}
	pdf, name, err := h.svc.Instruction(r.Context(), engagementctx.Must(r.Context()).TaxpayerID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(pdf)
}

func (h *Handler) submitted(w http.ResponseWriter, r *http.Request) {
	id, ok := connID(w, r)
	if !ok {
		return
	}
	c, err := h.svc.MarkSubmitted(r.Context(), engagementctx.Must(r.Context()).TaxpayerID, id)
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
	// A bank refusal is not an HTTP error: the connection is "failed" with
	// the reason and the Turkish text (20 §5.4).
	response.JSON(w, r, http.StatusOK, c)
}

func (h *Handler) sync(w http.ResponseWriter, r *http.Request) {
	id, ok := connID(w, r)
	if !ok {
		return
	}
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return
	}
	from, err1 := time.Parse("2006-01-02", in.From)
	to, err2 := time.Parse("2006-01-02", in.To)
	if err1 != nil || err2 != nil {
		response.BadRequest(w, r, "INVALID_RANGE", "from and to are YYYY-MM-DD")
		return
	}
	res, err := h.svc.Sync(r.Context(), engagementctx.Must(r.Context()).TaxpayerID, id, from, to)
	if err != nil {
		var be *Error
		if errors.As(err, &be) {
			response.ErrorWithDetails(w, r, http.StatusBadGateway, "BANK_REFUSED", be.Reason.Message(),
				[]response.Detail{{Field: "reason", Message: string(be.Reason)}, {Field: "code", Message: be.Code}})
			return
		}
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "bank connection not found")
	case errors.Is(err, ErrUnknownBank):
		response.Error(w, r, http.StatusUnprocessableEntity, "BANK_NOT_SUPPORTED", "this bank has no web-service adapter yet; use statement upload")
	case errors.Is(err, ErrInvalidIBAN):
		response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_IBAN", err.Error())
	case errors.Is(err, ErrBadCredentials):
		response.Error(w, r, http.StatusUnprocessableEntity, "CREDENTIALS_INCOMPLETE", err.Error())
	case errors.Is(err, ErrBadState):
		response.Conflict(w, r, "BANK_CONNECTION_STATE", "not allowed in the connection's current state")
	case errors.Is(err, ErrThrottled):
		response.Error(w, r, http.StatusTooManyRequests, "BANK_SYNC_THROTTLED", "a sync was requested less than 5 minutes ago")
	case errors.Is(err, ErrBadRange):
		response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_RANGE", err.Error())
	case errors.Is(err, ErrNoEgressIPs):
		response.Error(w, r, http.StatusServiceUnavailable, "BANK_EGRESS_NOT_CONFIGURED", "the fixed egress IPs are not configured (BANK_EGRESS_IPS)")
	case errors.Is(err, ErrNoVault):
		response.Error(w, r, http.StatusServiceUnavailable, "VAULT_NOT_CONFIGURED", "the credential vault is not configured")
	default:
		response.InternalErr(w, r, err, "bank connection failed")
	}
}
