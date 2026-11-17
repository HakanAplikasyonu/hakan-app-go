package taxreturns

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the tax return routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the office routes behind the active-engagement chain
// (/v1/tenant/engagements/{engagementUuid}/tax-returns/...). A nil resolver
// registers nothing.
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
		mux.Handle(method+" /v1/tenant/engagements/{"+engagementctx.PathParam+"}/tax-returns"+path,
			middleware.Chain(fn, authn, org, middleware.RequireTenantPermission(perm), eng))
	}
	read, write := rbac.PermTenantTaxReturnsRead, rbac.PermTenantTaxReturnsWrite
	route("GET", "", read, h.list)
	route("GET", "/{returnKind}/{period}", read, h.get)
	route("PUT", "/{returnKind}/{period}", write, h.saveDraft)
	route("POST", "/{returnKind}/{period}/prepare", write, h.prepare)
	route("POST", "/{returnKind}/{period}/reopen", write, h.reopen)
	route("POST", "/{returnKind}/{period}/file", write, h.file)
	route("GET", "/{returnKind}/{period}/export", read, h.export)
}

func userID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var un *UnacknowledgedError
	switch {
	case errors.As(err, &un):
		details := make([]response.Detail, 0, len(un.Codes))
		for _, c := range un.Codes {
			details = append(details, response.Detail{Field: "acknowledged", Code: c, Message: "warning is not acknowledged"})
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, "TAX_RETURN_UNACKNOWLEDGED",
			"acknowledge every checklist warning before preparing", details)
	case errors.Is(err, ErrInvalidKind):
		response.NotFound(w, r, "not found")
	case errors.Is(err, ErrInvalidPeriod):
		response.BadRequest(w, r, "INVALID_PERIOD", "period must be YYYY-MM")
	case errors.Is(err, ErrInvalidInput):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrNotDraft):
		response.Conflict(w, r, "TAX_RETURN_NOT_DRAFT", "the return is prepared or filed; reopen it first")
	case errors.Is(err, ErrNotPrepared):
		response.Conflict(w, r, "TAX_RETURN_NOT_PREPARED", "the return must be prepared first")
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "not found")
	default:
		response.InternalErr(w, r, err, "tax return request failed")
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

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	y := strings.TrimSpace(r.URL.Query().Get("year"))
	if y != "" {
		if n, err := strconv.Atoi(y); err != nil || n < 2000 || n > 2100 {
			response.BadRequest(w, r, response.CodeValidationError, "year must be YYYY")
			return
		}
	}
	items, err := h.svc.List(r.Context(), s, y)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	p, err := h.svc.Preview(r.Context(), s, r.PathValue("returnKind"), r.PathValue("period"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

func (h *Handler) saveDraft(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	var in DraftInput
	if !decode(w, r, &in) {
		return
	}
	p, err := h.svc.SaveDraft(r.Context(), s, userID(r), r.PathValue("returnKind"), r.PathValue("period"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

func (h *Handler) prepare(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	var in struct {
		Acknowledged []string `json:"acknowledged"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	p, err := h.svc.Prepare(r.Context(), s, userID(r), r.PathValue("returnKind"), r.PathValue("period"), in.Acknowledged)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

func (h *Handler) reopen(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	p, err := h.svc.Reopen(r.Context(), s, r.PathValue("returnKind"), r.PathValue("period"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

func (h *Handler) file(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	var in FileInput
	if !decode(w, r, &in) {
		return
	}
	p, err := h.svc.File(r.Context(), s, userID(r), r.PathValue("returnKind"), r.PathValue("period"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	data, name, err := h.svc.Export(r.Context(), s, r.PathValue("returnKind"), r.PathValue("period"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}
