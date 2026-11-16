package upload

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/periodq"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves /v1/tenant/engagements/{engagementUuid}/ubl-uploads.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the upload routes behind the active-engagement chain.
// A nil orgResolver (tenancy off) or nil handler registers nothing.
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
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/ubl-uploads"
	mux.Handle("POST "+base, middleware.Chain(http.HandlerFunc(h.upload), authn, org, write, eng))
	mux.Handle("GET "+base, middleware.Chain(http.HandlerFunc(h.list), authn, org, read, eng))
	mux.Handle("GET "+base+"/{jobUuid}", middleware.Chain(http.HandlerFunc(h.get), authn, org, read, eng))
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	scope := engagementctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		response.Error(w, r, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", "file is too large or the form is invalid")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "FILE_REQUIRED", "multipart field \"file\" is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, MaxUploadBytes+1))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "FILE_UNREADABLE", "could not read the file")
		return
	}
	job, err := h.svc.Upload(r.Context(), scope, p.UserInternal, header.Filename, data)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}

type page struct {
	Items  []Job `json:"items"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	scope := engagementctx.Must(r.Context())
	limit, offset := int32(20), int32(0)
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 100 {
		limit = int32(v) //nolint:gosec // bounded above
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 && v <= 1_000_000 {
		offset = int32(v) //nolint:gosec // bounded above
	}
	period, ok := periodq.Parse(w, r)
	if !ok {
		return
	}
	items, err := h.svc.List(r.Context(), scope, period, limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page{Items: items, Limit: limit, Offset: offset})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	scope := engagementctx.Must(r.Context())
	id, err := uuid.Parse(r.PathValue("jobUuid"))
	if err != nil {
		response.NotFound(w, r, "upload job not found")
		return
	}
	job, err := h.svc.Get(r.Context(), scope, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, job)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "upload job not found")
	case errors.Is(err, ErrEmpty):
		response.Error(w, r, http.StatusBadRequest, "FILE_EMPTY", "the file is empty")
	case errors.Is(err, ErrTooLarge):
		response.Error(w, r, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", "file is too large")
	case errors.Is(err, ErrUnsupported):
		response.Error(w, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_FILE", "upload an XML, ZIP or base64 file")
	default:
		response.InternalErr(w, r, err, "upload failed")
	}
}
