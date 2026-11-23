package lucakoza

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves /v1/tenant/luca-koza-settings and
// /v1/tenant/engagements/{engagementUuid}/luca-koza-connections.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the Luca Koza routes.
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
	settings := middleware.RequireTenantPermission(rbac.PermTenantSettingsWrite)
	mux.Handle("GET /v1/tenant/luca-koza-settings", middleware.Chain(http.HandlerFunc(h.getSettings), authn, org, read))
	mux.Handle("PUT /v1/tenant/luca-koza-settings", middleware.Chain(http.HandlerFunc(h.putSettings), authn, org, settings))
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/luca-koza-connections"
	one := base + "/{connectionUuid}"
	mux.Handle("GET "+base, middleware.Chain(http.HandlerFunc(h.list), authn, org, read, eng))
	mux.Handle("POST "+base, middleware.Chain(http.HandlerFunc(h.connect), authn, org, write, eng))
	mux.Handle("PUT "+one+"/company", middleware.Chain(http.HandlerFunc(h.company), authn, org, write, eng))
	mux.Handle("POST "+one+"/sync", middleware.Chain(http.HandlerFunc(h.sync), authn, org, write, eng))
}

func scope(r *http.Request) (orgID, taxpayerID int64) {
	s := engagementctx.Must(r.Context())
	return s.OrganizationID, s.TaxpayerID
}

func actor(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Settings(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		response.InternalErr(w, r, err, "could not load luca koza settings")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

func (h *Handler) putSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil || in.Enabled == nil {
		response.BadRequest(w, r, "INVALID_BODY", "enabled is required")
		return
	}
	st, err := h.svc.SaveSettings(r.Context(), orgctx.Must(r.Context()).ID, actor(r), *in.Enabled)
	if err != nil {
		response.InternalErr(w, r, err, "could not save luca koza settings")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	org, tp := scope(r)
	items, err := h.svc.List(r.Context(), org, tp)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) connect(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BaseURL   string `json:"base_url"`
		OrgCode   string `json:"org_code"`
		Username  string `json:"username"`
		Password  string `json:"password"`
		CompanyID string `json:"company_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return
	}
	org, tp := scope(r)
	c, cos, err := h.svc.Connect(r.Context(), org, tp,
		Credentials{BaseURL: in.BaseURL, OrgCode: in.OrgCode, Username: in.Username, Password: in.Password}, in.CompanyID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, map[string]any{"connection": c, "companies": cos})
}

func connID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("connectionUuid"))
	if err != nil {
		response.NotFound(w, r, "connection not found")
		return uuid.Nil, false
	}
	return id, true
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
	org, tp := scope(r)
	c, err := h.svc.SelectCompany(r.Context(), org, tp, id, in.CompanyID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

func (h *Handler) sync(w http.ResponseWriter, r *http.Request) {
	id, ok := connID(w, r)
	if !ok {
		return
	}
	var in struct {
		From     string `json:"from"`
		To       string `json:"to"`
		Password string `json:"password"`
	}
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
	org, tp := scope(r)
	rep, err := h.svc.Sync(r.Context(), org, tp, id, from, to, in.Password)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rep)
}

// writeErr answers in Turkish where the office reads the text as is (the
// panel shows a Turkish server message unchanged). Koza's own failures are
// 422: the address and the sign-in are the office's input.
func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var kz *KozaError
	switch {
	case errors.Is(err, ErrOff):
		response.Conflict(w, r, "LUCA_KOZA_OFF", "Luca Koza bağlantısı ofis ayarlarında kapalı. Ayarlar'dan açın.")
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "connection not found")
	case errors.Is(err, ErrBadState):
		response.Conflict(w, r, "CONNECTION_STATE", "missing input or not allowed in the connection's current state")
	case errors.Is(err, ErrMissing):
		response.BadRequest(w, r, "MISSING_FIELDS", "base_url, org_code, username and password are required")
	case errors.Is(err, ErrUnknownCo):
		response.Error(w, r, http.StatusUnprocessableEntity, "UNKNOWN_COMPANY", "the company is not in this user's list")
	case errors.Is(err, ErrThrottle):
		response.Error(w, r, http.StatusTooManyRequests, "SYNC_THROTTLED", "a sync was requested less than 5 minutes ago")
	case errors.Is(err, ErrBadRange):
		response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_RANGE", "the range is reversed or longer than 400 days")
	case errors.As(err, &kz) && kz.Step == "login":
		response.Error(w, r, http.StatusUnprocessableEntity, "KOZA_LOGIN_FAILED", "Luca Koza girişi reddetti: "+kz.Message)
	case errors.As(err, &kz):
		response.Error(w, r, http.StatusUnprocessableEntity, "KOZA_REFUSED", "Luca Koza isteği reddetti: "+kz.Message)
	case errors.Is(err, ErrAddress):
		response.Error(w, r, http.StatusUnprocessableEntity, "KOZA_ADDRESS_INVALID", "Luca Koza adresi kullanılamıyor: http:// ya da https:// ile başlayan, internetten erişilen bir adres girin.")
	case errors.Is(err, ErrUnreachable):
		response.Error(w, r, http.StatusUnprocessableEntity, "KOZA_UNREACHABLE", "Luca Koza'ya ulaşılamadı. Adresi ve sunucunun açık olduğunu kontrol edin.")
	case errors.Is(err, ErrAnswer):
		response.Error(w, r, http.StatusUnprocessableEntity, "KOZA_ANSWER", "Luca Koza'nın yanıtı okunamadı. Adresin Luca Koza sunucusu olduğunu kontrol edin.")
	default:
		response.InternalErr(w, r, err, "Luca Koza request failed")
	}
}
