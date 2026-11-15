package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/apiquery"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Warnings is what the handler needs from the warnings service.
type Warnings interface {
	List(ctx context.Context, f repository.Filter) ([]repository.Row, int64, error)
	Decide(ctx context.Context, organizationID, engagementID int64, id uuid.UUID, status catalog.Status, userID int64) error
}

// WithWarnings attaches the warnings service.
func (h *Handler) WithWarnings(w Warnings) *Handler { h.warnings = w; return h }

// ListOfficeWarnings: GET /v1/tenant/warnings. Callers without
// tenant.engagements.read_all see only engagements assigned to them.
func (h *Handler) ListOfficeWarnings(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	h.listWarnings(w, r, repository.Filter{
		OrganizationID: s.ID, AllEngagements: s.Has(rbac.PermTenantEngagementsReadAll), UserID: userID(r),
	})
}

// ListEngagementWarnings: GET /v1/tenant/engagements/{engagementUuid}/warnings.
// Engagement access was checked by the middleware.
func (h *Handler) ListEngagementWarnings(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	h.listWarnings(w, r, repository.Filter{
		OrganizationID: s.ID, EngagementID: engagementctx.Must(r.Context()).ID, AllEngagements: true,
	})
}

// AcknowledgeWarning: POST .../warnings/{warningUuid}/acknowledge.
func (h *Handler) AcknowledgeWarning(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, catalog.StatusAcknowledged)
}

// ResolveWarning: POST .../warnings/{warningUuid}/resolve.
func (h *Handler) ResolveWarning(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, catalog.StatusResolved)
}

func (h *Handler) listWarnings(w http.ResponseWriter, r *http.Request, f repository.Filter) {
	q := r.URL.Query()
	limit, offset := apiquery.DefaultLimit, int32(0)
	if v, ok := parseInt32(q.Get("limit")); ok && v > 0 && v <= apiquery.MaxLimit {
		limit = v
	}
	if v, ok := parseInt32(q.Get("offset")); ok && v >= 0 {
		offset = v
	}
	f.Period, f.Code, f.Severity = q.Get("period"), q.Get("code"), q.Get("severity")
	f.Status, f.EntityType = q.Get("status"), q.Get("entity_type")
	f.Limit, f.Offset = limit, offset
	rows, total, err := h.warnings.List(r.Context(), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(rows, total, limit, offset))
}

func (h *Handler) decide(w http.ResponseWriter, r *http.Request, status catalog.Status) {
	s := orgctx.Must(r.Context())
	id, err := uuid.Parse(r.PathValue("warningUuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "warningUuid is invalid")
		return
	}
	err = h.warnings.Decide(r.Context(), s.ID, engagementctx.Must(r.Context()).ID, id, status, userID(r))
	if errors.Is(err, usecase.ErrWarningNotFound) {
		response.NotFound(w, r, "Open warning not found")
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"uuid": id.String(), "status": string(status)})
}

func userID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func parseInt32(s string) (int32, bool) {
	var n int32
	if s == "" || len(s) > 6 {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + c - '0'
	}
	return n, true
}
