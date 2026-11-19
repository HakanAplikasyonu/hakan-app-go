package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/activity"
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
	Decide(ctx context.Context, organizationID, engagementID int64, id uuid.UUID, status catalog.Status, userID int64, note string) error
	Groups(ctx context.Context, f repository.Filter) ([]repository.Group, error)
	AcknowledgeBulk(ctx context.Context, f repository.Filter, in *usecase.BulkInput) (int64, error)
}

// WithWarnings attaches the warnings service.
func (h *Handler) WithWarnings(w Warnings) *Handler { h.warnings = w; return h }

// WithActivity attaches the activity log (bulk acknowledgements); rec may be nil.
func (h *Handler) WithActivity(rec *activity.Recorder) *Handler { h.activity = rec; return h }

type groupsResponse struct {
	Items []repository.Group `json:"items"`
}

// ListOfficeWarningGroups: GET /v1/tenant/warnings/groups. Same visibility
// as ListOfficeWarnings.
func (h *Handler) ListOfficeWarningGroups(w http.ResponseWriter, r *http.Request) {
	h.groups(w, r, officeFilter(r))
}

// ListEngagementWarningGroups: GET /v1/tenant/engagements/{engagementUuid}/warnings/groups.
func (h *Handler) ListEngagementWarningGroups(w http.ResponseWriter, r *http.Request) {
	h.groups(w, r, engagementFilter(r))
}

// AcknowledgeOfficeWarnings: POST /v1/tenant/warnings/acknowledge-bulk.
// Only engagements the caller may see are touched.
func (h *Handler) AcknowledgeOfficeWarnings(w http.ResponseWriter, r *http.Request) {
	h.acknowledgeBulk(w, r, officeFilter(r), nil)
}

// AcknowledgeEngagementWarnings: POST /v1/tenant/engagements/{engagementUuid}/warnings/acknowledge-bulk.
func (h *Handler) AcknowledgeEngagementWarnings(w http.ResponseWriter, r *http.Request) {
	e := engagementctx.Must(r.Context())
	h.acknowledgeBulk(w, r, engagementFilter(r), &e.UUID)
}

func officeFilter(r *http.Request) repository.Filter {
	s := orgctx.Must(r.Context())
	return repository.Filter{OrganizationID: s.ID, AllEngagements: s.Has(rbac.PermTenantEngagementsReadAll), UserID: userID(r)}
}

func engagementFilter(r *http.Request) repository.Filter {
	s := orgctx.Must(r.Context())
	return repository.Filter{OrganizationID: s.ID, EngagementID: engagementctx.Must(r.Context()).ID, AllEngagements: true, UserID: userID(r)}
}

func (h *Handler) groups(w http.ResponseWriter, r *http.Request, f repository.Filter) {
	q := r.URL.Query()
	f.Period, f.Severity, f.Status = q.Get("period"), q.Get("severity"), q.Get("status")
	items, err := h.warnings.Groups(r.Context(), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, groupsResponse{Items: items})
}

type bulkResponse struct {
	Acknowledged int64 `json:"acknowledged"`
}

func (h *Handler) acknowledgeBulk(w http.ResponseWriter, r *http.Request, f repository.Filter, engagementUUID *uuid.UUID) {
	var in usecase.BulkInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	n, err := h.warnings.AcknowledgeBulk(r.Context(), f, &in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil && n > 0 {
		actor := f.UserID
		payload := map[string]any{"code": in.Code, "period": in.Period, "note": in.Note, "acknowledged": n}
		if engagementUUID != nil {
			payload["engagement_uuid"] = engagementUUID.String()
		}
		h.activity.Record(r.Context(), &actor, "warnings.acknowledge_bulk", "data_warning", nil, payload, r)
	}
	response.JSON(w, r, http.StatusOK, bulkResponse{Acknowledged: n})
}

// ListOfficeWarnings: GET /v1/tenant/warnings. Callers without
// tenant.engagements.read_all see only engagements assigned to them.
func (h *Handler) ListOfficeWarnings(w http.ResponseWriter, r *http.Request) {
	h.listWarnings(w, r, officeFilter(r))
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
	// Optional body {"note": "..."}: the reason (#529). An empty body keeps
	// older clients (mobile) working.
	var in struct {
		Note string `json:"note"`
	}
	if r.ContentLength != 0 {
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil && !errors.Is(err, io.EOF) {
			response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
			return
		}
	}
	err = h.warnings.Decide(r.Context(), s.ID, engagementctx.Must(r.Context()).ID, id, status, userID(r), in.Note)
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
