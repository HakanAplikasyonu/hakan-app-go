package billing

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the billing routes.
type Handler struct {
	svc *Service
	now func() time.Time
}

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc, now: time.Now} }

// RegisterRoutes mounts platform (super admin) and office routes:
//
//	GET  /v1/platform/billing/plans            PUT /v1/platform/billing/plans/{code}
//	GET  /v1/platform/organizations/{uuid}/billing
//	PUT  /v1/platform/organizations/{uuid}/billing
//	POST /v1/platform/organizations/{uuid}/billing/statements          {period}
//	POST /v1/platform/organizations/{uuid}/billing/statements/{period}/status {status}
//	GET  /v1/tenant/billing
func RegisterRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader, orgResolver orgctx.Resolver, h *Handler) {
	authn := middleware.Authenticate(tokens, loader)
	read := middleware.RequirePermission(rbac.PermPlatformOrganizationsRead)
	write := middleware.RequirePermission(rbac.PermPlatformOrganizationsWrite)
	mux.Handle("GET /v1/platform/billing/plans", middleware.Chain(http.HandlerFunc(h.plans), authn, read))
	mux.Handle("PUT /v1/platform/billing/plans/{code}", middleware.Chain(http.HandlerFunc(h.savePlan), authn, write))
	org := "/v1/platform/organizations/{uuid}/billing"
	mux.Handle("GET "+org, middleware.Chain(http.HandlerFunc(h.platformOverview), authn, read))
	mux.Handle("PUT "+org, middleware.Chain(http.HandlerFunc(h.setAccount), authn, write))
	mux.Handle("POST "+org+"/statements", middleware.Chain(http.HandlerFunc(h.generate), authn, write))
	mux.Handle("POST "+org+"/statements/{period}/status", middleware.Chain(http.HandlerFunc(h.setStatus), authn, write))
	if orgResolver != nil {
		mux.Handle("GET /v1/tenant/billing", middleware.Chain(http.HandlerFunc(h.tenantOverview), authn,
			middleware.RequireOrganization(orgResolver), middleware.RequireTenantPermission(rbac.PermTenantSettingsRead)))
	}
}

func (h *Handler) plans(w http.ResponseWriter, r *http.Request) {
	ps, err := h.svc.Plans(r.Context())
	if err != nil {
		response.InternalErr(w, r, err, "billing failed")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": ps})
}

func (h *Handler) savePlan(w http.ResponseWriter, r *http.Request) {
	var p Plan
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&p); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return
	}
	p.Code = r.PathValue("code")
	out, err := h.svc.SavePlan(r.Context(), p)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) org(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "organization not found")
		return 0, false
	}
	o, err := db.New(h.svc.pool).GetOrganizationByUUID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "organization not found")
		return 0, false
	} else if err != nil {
		response.InternalErr(w, r, err, "billing failed")
		return 0, false
	}
	return o.ID, true
}

func (h *Handler) platformOverview(w http.ResponseWriter, r *http.Request) {
	id, ok := h.org(w, r)
	if !ok {
		return
	}
	ov, err := h.svc.Overview(r.Context(), id, h.now())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, ov)
}

func (h *Handler) setAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := h.org(w, r)
	if !ok {
		return
	}
	var in struct {
		PlanCode       string `json:"plan_code"`
		ReadOnly       bool   `json:"read_only"`
		ReadOnlyReason string `json:"read_only_reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return
	}
	var by int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		by = p.UserInternal
	}
	acc, err := h.svc.SetAccount(r.Context(), id, in.PlanCode, in.ReadOnly, in.ReadOnlyReason, by)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, acc)
}

func (h *Handler) generate(w http.ResponseWriter, r *http.Request) {
	id, ok := h.org(w, r)
	if !ok {
		return
	}
	var in struct {
		Period string `json:"period"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return
	}
	if _, _, err := PeriodBounds(in.Period); err != nil {
		response.BadRequest(w, r, "INVALID_PERIOD", "period is YYYY-MM")
		return
	}
	st, err := h.svc.Generate(r.Context(), id, in.Period)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

func (h *Handler) setStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := h.org(w, r)
	if !ok {
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return
	}
	st, err := h.svc.SetStatus(r.Context(), id, r.PathValue("period"), in.Status)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

func (h *Handler) tenantOverview(w http.ResponseWriter, r *http.Request) {
	ov, err := h.svc.Overview(r.Context(), orgctx.Must(r.Context()).ID, h.now())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, ov)
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidPlan):
		response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_BILLING_INPUT", "invalid plan, fee, VAT rate or status")
	case errors.Is(err, ErrFinal):
		response.Conflict(w, r, "BILLING_STATEMENT_FINAL", "the statement of this period is final or void")
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "no draft statement for this period")
	default:
		response.InternalErr(w, r, err, "billing failed")
	}
}
