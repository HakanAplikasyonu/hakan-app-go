package taxpayers

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ratelimit"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// ImportHandler serves the bulk taxpayer import (issue #273).
type ImportHandler struct {
	im      *Importer
	limiter *ratelimit.Limiter
}

// Per office and limitWindow. Sending bypasses the add-mükellef endpoint's
// limit (limitRequestsPerOrg), so the consent e-mails a list can trigger are
// bounded here: at most limitImportSendsPerOrg lists of MaxImportRows.
const (
	limitImportUploadsPerOrg = 30
	limitImportSendsPerOrg   = 3
)

// NewImportHandler builds the handler.
func NewImportHandler(im *Importer) *ImportHandler { return &ImportHandler{im: im} }

// WithLimiter rate-limits uploads and sends per office. nil disables it.
func (h *ImportHandler) WithLimiter(l *ratelimit.Limiter) *ImportHandler {
	h.limiter = l
	return h
}

func (h *ImportHandler) allow(w http.ResponseWriter, r *http.Request, action string, orgID int64, limit int) bool {
	return (&MemberHandler{limiter: h.limiter}).allow(w, r, action, strconv.FormatInt(orgID, 10), limit)
}

// RegisterImportRoutes mounts /v1/tenant/taxpayer-imports.
func RegisterImportRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader, orgResolver orgctx.Resolver, h *ImportHandler) {
	if orgResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(orgResolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantEngagementsWrite)
	mux.Handle("GET /v1/tenant/taxpayer-imports", middleware.Chain(http.HandlerFunc(h.list), authn, org, read))
	mux.Handle("POST /v1/tenant/taxpayer-imports", middleware.Chain(http.HandlerFunc(h.upload), authn, org, write))
	mux.Handle("GET /v1/tenant/taxpayer-imports/{uuid}", middleware.Chain(http.HandlerFunc(h.get), authn, org, read))
	mux.Handle("POST /v1/tenant/taxpayer-imports/{uuid}/send", middleware.Chain(http.HandlerFunc(h.send), authn, org, write))
}

func (h *ImportHandler) upload(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	if !h.allow(w, r, "taxpayer_import_upload_org", s.ID, limitImportUploadsPerOrg) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, parse.MaxBytes+1<<20)
	file, header, err := r.FormFile("file")
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "FILE_REQUIRED", "multipart field \"file\" is required (.xlsx or .csv, up to 20 MB)")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "FILE_UNREADABLE", "could not read the file")
		return
	}
	b, err := h.im.Upload(r.Context(), s.ID, p.UserInternal, header.Filename, data)
	if err != nil {
		writeImportError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, b)
}

func (h *ImportHandler) list(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	items, err := h.im.List(r.Context(), s.ID)
	if err != nil {
		writeImportError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *ImportHandler) get(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	b, err := h.im.Get(r.Context(), s.ID, id)
	if err != nil {
		writeImportError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, b)
}

func (h *ImportHandler) send(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if !h.allow(w, r, "taxpayer_import_send_org", s.ID, limitImportSendsPerOrg) {
		return
	}
	b, err := h.im.Send(r.Context(), s.ID, s.Name, p.UserInternal, id)
	if err != nil {
		writeImportError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, b)
}

func writeImportError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "Import not found")
	case errors.Is(err, ErrImportSent):
		response.Error(w, r, http.StatusConflict, "IMPORT_ALREADY_SENT", "the invitations of this import were already sent")
	case errors.Is(err, ErrImportNoHeader):
		response.Error(w, r, http.StatusUnprocessableEntity, "IMPORT_NO_HEADER",
			"no header row with a tax id (VKN/TCKN) column and a title (Unvan) column was found")
	case errors.Is(err, ErrImportTooLarge):
		response.Error(w, r, http.StatusUnprocessableEntity, "IMPORT_TOO_LARGE", err.Error())
	case errors.Is(err, parse.ErrEmpty), errors.Is(err, parse.ErrTooLarge), errors.Is(err, parse.ErrUnsupported):
		response.Error(w, r, http.StatusUnprocessableEntity, "IMPORT_FILE_INVALID", err.Error())
	default:
		response.InternalErr(w, r, err, "Could not process the import")
	}
}
