package easytrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves /v1/tenant/engagements/{engagementUuid}/easytrade-connections.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the Easytrade routes.
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
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/easytrade-connections"
	one := base + "/{connectionUuid}"
	mux.Handle("GET "+base, middleware.Chain(http.HandlerFunc(h.list), authn, org, read, eng))
	mux.Handle("POST "+base, middleware.Chain(http.HandlerFunc(h.connect), authn, org, write, eng))
	mux.Handle("POST "+base+"/test", middleware.Chain(http.HandlerFunc(h.test), authn, org, write, eng))
	mux.Handle("PATCH "+one, middleware.Chain(http.HandlerFunc(h.schedule), authn, org, write, eng))
	mux.Handle("POST "+one+"/sync", middleware.Chain(http.HandlerFunc(h.sync), authn, org, write, eng))
	mux.Handle("GET "+one+"/sync-runs", middleware.Chain(http.HandlerFunc(h.runs), authn, org, read, eng))
	mux.Handle("GET "+one+"/financial-transactions", middleware.Chain(http.HandlerFunc(h.financial), authn, org, read, eng))
	mux.Handle("GET "+one+"/z-reports", middleware.Chain(http.HandlerFunc(h.zReports), authn, org, read, eng))
	mux.Handle("GET "+one+"/vat-reconciliation", middleware.Chain(http.HandlerFunc(h.reconcile), authn, org, read, eng))
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
		Email     string `json:"email"`
		Password  string `json:"password"`
		CompanyID string `json:"company_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return
	}
	c, err := h.svc.Connect(r.Context(), tp(r), in.Email, in.Password, in.CompanyID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, c)
}

// test checks the email, password and ID against Easytrade without storing
// anything ("Bağlantıyı test et").
func (h *Handler) test(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email     string `json:"email"`
		Password  string `json:"password"`
		CompanyID string `json:"company_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return
	}
	res, err := h.svc.Test(r.Context(), in.Email, in.Password, in.CompanyID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
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

// schedule sets the connection's sync interval; null or 0 turns the
// schedule off (manual sync only).
func (h *Handler) schedule(w http.ResponseWriter, r *http.Request) {
	id, ok := connID(w, r)
	if !ok {
		return
	}
	var in struct {
		SyncIntervalMinutes *int `json:"sync_interval_minutes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "invalid JSON body")
		return
	}
	var every time.Duration
	if in.SyncIntervalMinutes != nil {
		every = time.Duration(*in.SyncIntervalMinutes) * time.Minute
	}
	c, err := h.svc.SetSchedule(r.Context(), tp(r), id, every)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

func (h *Handler) runs(w http.ResponseWriter, r *http.Request) {
	id, ok := connID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Runs(r.Context(), tp(r), id, 20)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func periodParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	p := r.URL.Query().Get("period")
	if !periodOK(p) {
		response.BadRequest(w, r, "INVALID_PERIOD", "period is YYYY-MM")
		return "", false
	}
	return p, true
}

func (h *Handler) financial(w http.ResponseWriter, r *http.Request) {
	id, ok := connID(w, r)
	if !ok {
		return
	}
	period, ok := periodParam(w, r)
	if !ok {
		return
	}
	out, err := h.svc.FinancialTransactions(r.Context(), tp(r), id, period)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) zReports(w http.ResponseWriter, r *http.Request) {
	id, ok := connID(w, r)
	if !ok {
		return
	}
	period, ok := periodParam(w, r)
	if !ok {
		return
	}
	out, err := h.svc.ZReports(r.Context(), tp(r), id, period)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) reconcile(w http.ResponseWriter, r *http.Request) {
	id, ok := connID(w, r)
	if !ok {
		return
	}
	period := r.URL.Query().Get("period")
	if _, err := time.Parse("2006-01", period); err != nil {
		response.BadRequest(w, r, "INVALID_PERIOD", "period is YYYY-MM")
		return
	}
	rec, err := h.svc.Reconcile(r.Context(), tp(r), id, period)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rec)
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNoVault):
		response.Error(w, r, http.StatusServiceUnavailable, "VAULT_NOT_CONFIGURED", "the credential vault is not configured")
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "connection not found")
	case errors.Is(err, ErrBadState):
		response.Conflict(w, r, "CONNECTION_STATE", "missing input or not allowed in the connection's current state")
	case errors.Is(err, ErrThrottle):
		response.Error(w, r, http.StatusTooManyRequests, "SYNC_THROTTLED", "a sync was requested less than 5 minutes ago")
	case errors.Is(err, ErrBusy):
		response.Conflict(w, r, "SYNC_RUNNING", "a sync of this connection is already running")
	case errors.Is(err, ErrBadInterval):
		response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_INTERVAL", "sync_interval_minutes is 15 to 10080, or null to turn the schedule off")
	case errors.Is(err, ErrBadCompanyID):
		response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_COMPANY_ID", "the Easytrade ID is a number")
	case upstreamErr(w, r, err):
	case errors.Is(err, ErrInvalidInput):
		response.Error(w, r, http.StatusUnprocessableEntity, "EASYTRADE_INVALID_INPUT", "Easytrade rejected the login input")
	case errors.Is(err, ErrBadLogin):
		response.Error(w, r, http.StatusUnprocessableEntity, "EASYTRADE_BAD_CREDENTIALS", "Easytrade refused the email or password")
	case errors.Is(err, ErrNoCompany):
		response.Error(w, r, http.StatusUnprocessableEntity, "EASYTRADE_COMPANY_UNKNOWN", "Easytrade did not name the user's company; enter the ID")
	case errors.Is(err, ErrCompanyMismatch):
		response.Error(w, r, http.StatusUnprocessableEntity, "EASYTRADE_COMPANY_MISMATCH", "the ID is not the Easytrade user's company")
	case errors.Is(err, ErrForbidden):
		response.Error(w, r, http.StatusUnprocessableEntity, "EASYTRADE_FORBIDDEN", "the Easytrade user has no permission in the company")
	case errors.Is(err, ErrTimeout):
		response.Error(w, r, http.StatusGatewayTimeout, "EASYTRADE_TIMEOUT", "Easytrade did not answer in time")
	case errors.Is(err, ErrUnreachable):
		response.Error(w, r, http.StatusBadGateway, "EASYTRADE_UNREACHABLE", "Easytrade could not be reached")
	case errors.Is(err, ErrUpstream):
		response.Error(w, r, http.StatusBadGateway, "EASYTRADE_UNAVAILABLE", "Easytrade answered with an error; the schedule retries")
	case errors.Is(err, ErrBadRange):
		response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_RANGE", "the range is reversed or longer than 400 days")
	case errors.Is(err, ErrLicense):
		response.Error(w, r, http.StatusBadGateway, "EASYTRADE_LICENSE_EXPIRED", "the taxpayer's Easytrade license has expired")
	case errors.Is(err, ErrAuth):
		response.Error(w, r, http.StatusBadGateway, "EASYTRADE_AUTH_FAILED", "Easytrade refused the login or token")
	case errors.Is(err, ErrNoBranch):
		response.Error(w, r, http.StatusBadGateway, "EASYTRADE_NO_BRANCH", "the company has no active branch in Easytrade")
	default:
		response.InternalErr(w, r, err, "Easytrade request failed")
	}
}

// upstreamErr writes an error that carries what Easytrade answered and
// reports whether err was one. The message is Turkish and names the server
// (host only) and its HTTP status, e.g. "Easytrade (pro.easytradetr.com)
// 401: E-posta veya şifre hatalı."; details carry the parts: code
// EASYTRADE_HOST (message = host), EASYTRADE_STATUS (message = HTTP status,
// absent when there was no answer) and, on a 422, EASYTRADE_FIELD (field =
// Easytrade's field name, message = "<Turkish name>: <text>").
//
// Codes: a refused sign-in is EASYTRADE_BAD_CREDENTIALS (401),
// EASYTRADE_INVALID_INPUT (422 on email or password),
// EASYTRADE_LICENSE_EXPIRED (403 LICENSE_EXPIRED, 502 like a sync) or
// EASYTRADE_FORBIDDEN (other 403); any other refused sign-in is
// EASYTRADE_LOGIN_REJECTED; an unreadable answer EASYTRADE_BAD_RESPONSE; an
// error answer elsewhere EASYTRADE_UNAVAILABLE; no answer
// EASYTRADE_TIMEOUT / EASYTRADE_UNREACHABLE.
func upstreamErr(w http.ResponseWriter, r *http.Request, err error) bool {
	var (
		le *LoginError
		he *HTTPError
		be *BadResponseError
		ne *NetError
	)
	var (
		httpStatus     = http.StatusUnprocessableEntity
		code, host     string
		status         int
		reason, prefix string
		field          *response.Detail
	)
	switch {
	case errors.As(err, &le):
		host, status, reason = le.Host, le.Status, le.UserMessage()
		code = "EASYTRADE_BAD_CREDENTIALS"
		switch {
		case errors.Is(le, ErrInvalidInput):
			code = "EASYTRADE_INVALID_INPUT"
		case errors.Is(le, ErrLicense):
			httpStatus, code = http.StatusBadGateway, "EASYTRADE_LICENSE_EXPIRED"
		case errors.Is(le, ErrForbidden):
			code = "EASYTRADE_FORBIDDEN"
		}
		if le.Field != "" && le.FieldMessage != "" {
			field = &response.Detail{Field: le.Field, Message: le.Reason(), Code: "EASYTRADE_FIELD"}
		}
	case errors.As(err, &he):
		host, status = he.Host, he.Status
		httpStatus, code = http.StatusBadGateway, "EASYTRADE_UNAVAILABLE"
		reason = he.Message
		if he.Path == "/auth/login" {
			httpStatus, code = http.StatusUnprocessableEntity, "EASYTRADE_LOGIN_REJECTED"
			if reason == "" {
				reason = "Easytrade giriş isteğini reddetti."
			}
		} else if reason == "" {
			reason = "Easytrade hata döndürdü; zamanlanmış senkron yeniden dener."
		}
		prefix = fmt.Sprintf("Easytrade (%s) %d: ", orUnknown(host), status)
	case errors.As(err, &be):
		host, status = be.Host, be.Status
		httpStatus, code = http.StatusBadGateway, "EASYTRADE_BAD_RESPONSE"
		reason = "Easytrade'in yanıtı okunamadı (beklenen biçimde değil)."
		prefix = fmt.Sprintf("Easytrade (%s) %d: ", orUnknown(host), status)
	case errors.As(err, &ne):
		host = ne.Host
		if errors.Is(ne, ErrTimeout) {
			httpStatus, code = http.StatusGatewayTimeout, "EASYTRADE_TIMEOUT"
			reason = "zamanında yanıt vermedi (zaman aşımı). Biraz sonra yeniden deneyin."
		} else {
			httpStatus, code = http.StatusBadGateway, "EASYTRADE_UNREACHABLE"
			reason = "sunucuya ulaşılamıyor; yanıt gelmedi."
		}
		prefix = fmt.Sprintf("Easytrade (%s): ", orUnknown(host))
	default:
		return false
	}
	details := []response.Detail{{Field: "easytrade_host", Message: host, Code: "EASYTRADE_HOST"}}
	if status > 0 {
		details = append(details, response.Detail{Field: "easytrade_status", Message: strconv.Itoa(status), Code: "EASYTRADE_STATUS"})
	}
	if field != nil {
		details = append(details, *field)
	}
	response.ErrorWithDetails(w, r, httpStatus, code, prefix+reason, details)
	return true
}

func orUnknown(host string) string {
	if host == "" {
		return "?"
	}
	return host
}
