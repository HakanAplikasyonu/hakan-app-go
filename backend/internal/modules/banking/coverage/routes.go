package coverage

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

const maxBodyBytes = 16 << 10

// Page bounds of the office to-do list.
const (
	defaultLimit = 20
	maxLimit     = 200
	maxOffset    = 100_000
)

// Handler serves the missing statement routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Module mounts the routes:
//   - GET /v1/tenant/bank-statement-gaps: the office to-do list (bank read).
//   - .../engagements/{engagementUuid}/bank-statement-status: the
//     taxpayer's monthly status (bank read), /requests opens document
//     requests (document task write), /marks records "geldi, hareket yok"
//     (bank write).
type Module struct {
	H           *Handler
	Engagements engagementctx.Resolver
}

// Name implements module.Module.
func (Module) Name() string { return "bankcoverage" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	r.Tenant("GET /v1/tenant/bank-statement-gaps", rbac.PermTenantBankRead, h.Gaps)
	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/bank-statement-status"
	r.Tenant("GET "+base, rbac.PermTenantBankRead, h.Status, eng...)
	r.Tenant("POST "+base+"/requests", rbac.PermTenantDocTasksWrite, h.Request, eng...)
	r.Tenant("POST "+base+"/marks", rbac.PermTenantBankWrite, h.Mark, eng...)
}

func userID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

// Status: GET .../bank-statement-status.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Status(r.Context(), engagementctx.Must(r.Context()))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Request: POST .../bank-statement-status/requests.
func (h *Handler) Request(w http.ResponseWriter, r *http.Request) {
	var in RequestInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Request(r.Context(), engagementctx.Must(r.Context()), userID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusOK
	if len(out.Created) > 0 {
		status = http.StatusCreated
	}
	response.JSON(w, r, status, out)
}

// Mark: POST .../bank-statement-status/marks.
func (h *Handler) Mark(w http.ResponseWriter, r *http.Request) {
	var in MarkInput
	if !decode(w, r, &in) {
		return
	}
	scope := engagementctx.Must(r.Context())
	if err := h.svc.Mark(r.Context(), scope, userID(r), in); err != nil {
		writeError(w, r, err)
		return
	}
	out, err := h.svc.Status(r.Context(), scope)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func pageParam(r *http.Request, name string, def, max int) (int, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, false
	}
	if n > max {
		n = max
	}
	return n, true
}

// Gaps: GET /v1/tenant/bank-statement-gaps?limit=&offset=. Without
// tenant.engagements.read_all only assigned engagements are listed.
func (h *Handler) Gaps(w http.ResponseWriter, r *http.Request) {
	limit, ok := pageParam(r, "limit", defaultLimit, maxLimit)
	if !ok || limit == 0 {
		response.BadRequest(w, r, response.CodeValidationError, "limit must be between 1 and 200")
		return
	}
	offset, ok := pageParam(r, "offset", 0, maxOffset)
	if !ok {
		response.BadRequest(w, r, response.CodeValidationError, "offset must be zero or more")
		return
	}
	s := orgctx.Must(r.Context())
	out, err := h.svc.Gaps(r.Context(), s.ID, s.Has(rbac.PermTenantEngagementsReadAll), userID(r), limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, ErrDisabled):
		response.Error(w, r, http.StatusConflict, "BANK_STATEMENT_TRACKING_DISABLED",
			"the office turned the missing statement follow-up off (setting bank_statement_tracking)")
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "bank account not found")
	case errors.Is(err, ErrNotMissing):
		response.Error(w, r, http.StatusConflict, "STATEMENT_NOT_MISSING",
			"the statement of this account and month is not missing")
	case errors.Is(err, ErrBadInput):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
