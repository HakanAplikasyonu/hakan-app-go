package petitions

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves the petition routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the petition routes (issue #56). Petitions belong to
// an active engagement (RequireEngagement): office work product about one
// mükellef.
func RegisterRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver, engResolver engagementctx.Resolver, h *Handler) {
	if orgResolver == nil || engResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(orgResolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantEngagementsWrite)
	eng := middleware.RequireEngagement(engResolver)

	mux.Handle("GET /v1/tenant/petition-templates", middleware.Chain(http.HandlerFunc(h.templates), authn, org, read))
	mux.Handle("POST /v1/tenant/engagements/{engagementUuid}/petitions/preview", middleware.Chain(http.HandlerFunc(h.preview), authn, org, read, eng))
	mux.Handle("GET /v1/tenant/engagements/{engagementUuid}/petitions", middleware.Chain(http.HandlerFunc(h.list), authn, org, read, eng))
	mux.Handle("POST /v1/tenant/engagements/{engagementUuid}/petitions", middleware.Chain(http.HandlerFunc(h.create), authn, org, write, eng))
	mux.Handle("GET /v1/tenant/engagements/{engagementUuid}/petitions/{uuid}/pdf", middleware.Chain(http.HandlerFunc(h.pdf), authn, org, read, eng))
}

type templateList struct {
	Items       []Template `json:"items"`
	DraftNotice string     `json:"draft_notice"`
}

func (h *Handler) templates(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, templateList{Items: Templates(), DraftNotice: DraftNotice})
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Preview(r.Context(), engagementctx.Must(r.Context()), userID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Create(r.Context(), engagementctx.Must(r.Context()), userID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

type petitionList struct {
	Items  []Petition `json:"items"`
	Limit  int32      `json:"limit"`
	Offset int32      `json:"offset"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, offset := int32(50), int32(0)
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 100 {
		limit = int32(v)
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = int32(v)
	}
	items, err := h.svc.List(r.Context(), engagementctx.Must(r.Context()), limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, petitionList{Items: items, Limit: limit, Offset: offset})
}

func (h *Handler) pdf(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return
	}
	p, err := h.svc.Get(r.Context(), engagementctx.Must(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	body, err := PDF(p.Rendered)
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="dilekce-`+p.UUID.String()+`.pdf"`)
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write(body)
}

func userID(r *http.Request) int64 {
	p, _ := authctx.PrincipalFrom(r.Context())
	return p.UserInternal
}

func decode(w http.ResponseWriter, r *http.Request) (Input, bool) {
	var in Input
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return in, false
	}
	return in, true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "Resource was not found")
	default:
		response.InternalErr(w, r, err, "")
	}
}
