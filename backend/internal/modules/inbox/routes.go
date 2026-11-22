package inbox

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/periodq"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the inbox route.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Module mounts the route.
type Module struct{ H *Handler }

// Name implements module.Module.
func (Module) Name() string { return "inbox" }

// Routes implements module.Module. The inbox sits beside the suggestion
// queue (/review), so it needs the same review read permission. The
// engagement view is the same route with ?engagement=; the engagement must
// be one the caller sees (read_all or an assignment), else the list is
// empty.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	r.Tenant("GET /v1/tenant/inbox", rbac.PermTenantReviewRead, h.List)
}

// List: GET /v1/tenant/inbox?period=&type=&engagement=&limit=&offset=&cursor=.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	f := Filter{OrganizationID: s.ID, AllEngagements: s.Has(rbac.PermTenantEngagementsReadAll)}
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		f.UserID = p.UserInternal
	}
	var ok bool
	if f.Period, ok = periodq.Parse(w, r); !ok {
		return
	}
	q := r.URL.Query()
	f.Type = strings.TrimSpace(q.Get("type"))
	if !ValidType(f.Type) {
		response.BadRequest(w, r, response.CodeValidationError, "type must be one of "+strings.Join(Types, ", "))
		return
	}
	if v := strings.TrimSpace(q.Get("engagement")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "engagement is invalid")
			return
		}
		f.Engagement = &id
	}
	f.Limit = int32Param(q.Get("limit"))
	f.Offset = int32Param(q.Get("offset"))
	// cursor is the opaque next_cursor of the previous page (an offset).
	if v := strings.TrimSpace(q.Get("cursor")); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n < 0 {
			response.BadRequest(w, r, response.CodeValidationError, "cursor is invalid")
			return
		}
		f.Offset = int32(n)
	}
	out, err := h.svc.List(r.Context(), f)
	if err != nil {
		response.RecordFailure(w, err)
		if errors.Is(err, ErrInvalid) {
			response.BadRequest(w, r, response.CodeValidationError, err.Error())
			return
		}
		response.InternalErr(w, r, err, "Unexpected server error")
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// int32Param reads a paging number; a malformed one counts as absent.
func int32Param(v string) int32 {
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}
