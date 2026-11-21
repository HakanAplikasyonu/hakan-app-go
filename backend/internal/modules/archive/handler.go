package archive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the archive routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts /v1/tenant/engagements/{engagementUuid}/archive-documents
// behind the active-engagement chain. A nil resolver registers nothing.
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
		mux.Handle(method+" /v1/tenant/engagements/{"+engagementctx.PathParam+"}/archive-documents"+path,
			middleware.Chain(fn, authn, org, middleware.RequireTenantPermission(perm), eng))
	}
	read, write := rbac.PermTenantDocumentsRead, rbac.PermTenantDocumentsWrite
	route("GET", "", read, h.list)
	route("POST", "", write, h.upload)
	route("GET", "/{archiveDocumentUuid}/file", read, h.file)
	route("PATCH", "/{archiveDocumentUuid}", write, h.update)
	route("DELETE", "/{archiveDocumentUuid}", write, h.remove)
	// "Önemli belgeler" ofis ayarı (#780 B).
	mux.Handle("GET /v1/tenant/important-document-settings",
		middleware.Chain(http.HandlerFunc(h.getSettings), authn, org, middleware.RequireTenantPermission(read)))
	mux.Handle("PUT /v1/tenant/important-document-settings",
		middleware.Chain(http.HandlerFunc(h.putSettings), authn, org, middleware.RequireTenantPermission(rbac.PermTenantSettingsWrite)))
}

// RegisterTaxpayerRoutes mounts the mükellef panel routes: the documents the
// office marked "Mükellef de görsün", read only, for the taxpayer's owner
// (contracts, tax and SGK papers; like the payroll PDFs).
func RegisterTaxpayerRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader,
	tpResolver taxpayerctx.Resolver, h *Handler,
) {
	if tpResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	tp := middleware.RequireTaxpayer(tpResolver)
	mux.Handle("GET /v1/taxpayer/archive-documents", middleware.Chain(http.HandlerFunc(h.myList), authn, tp))
	mux.Handle("GET /v1/taxpayer/archive-documents/{archiveDocumentUuid}/file", middleware.Chain(http.HandlerFunc(h.myFile), authn, tp))
	mux.Handle("GET /v1/taxpayer/important-documents", middleware.Chain(http.HandlerFunc(h.myImportant), authn, tp))
}

// myImportant: GET /v1/taxpayer/important-documents (#780 B). The files
// download through /v1/taxpayer/archive-documents/{uuid}/file.
func (h *Handler) myImportant(w http.ResponseWriter, r *http.Request) {
	s, ok := ownerOnly(w, r)
	if !ok {
		return
	}
	out, err := h.svc.TaxpayerImportant(r.Context(), s.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// update: PATCH {important_kind, visible_to_taxpayer, title, note, valid_until}.
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var p Patch
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "request body must be a JSON object")
		return
	}
	d, err := h.svc.Update(r.Context(), engagementctx.Must(r.Context()).TaxpayerID, id, p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

// getSettings: GET /v1/tenant/important-document-settings.
func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Settings(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		response.InternalErr(w, r, err, "could not load important document settings")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// putSettings: PUT /v1/tenant/important-document-settings {"enabled": bool}.
func (h *Handler) putSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Enabled == nil {
		response.BadRequest(w, r, "INVALID_BODY", "enabled is required")
		return
	}
	var uid int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		uid = p.UserInternal
	}
	st, err := h.svc.SaveSettings(r.Context(), orgctx.Must(r.Context()).ID, uid, *in.Enabled)
	if err != nil {
		response.InternalErr(w, r, err, "could not save important document settings")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// ownerOnly: the archive's shared documents are for the taxpayer's owner.
func ownerOnly(w http.ResponseWriter, r *http.Request) (taxpayerctx.Scope, bool) {
	s := taxpayerctx.Must(r.Context())
	if s.Role != taxpayerctx.RoleOwner {
		response.Forbidden(w, r, "archive documents are visible to the taxpayer's owner")
		return s, false
	}
	return s, true
}

func (h *Handler) myList(w http.ResponseWriter, r *http.Request) {
	s, ok := ownerOnly(w, r)
	if !ok {
		return
	}
	h.listFor(w, r, s.ID, true)
}

func (h *Handler) myFile(w http.ResponseWriter, r *http.Request) {
	s, ok := ownerOnly(w, r)
	if !ok {
		return
	}
	h.fileFor(w, r, s.ID, true)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "not found")
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrExists):
		response.Conflict(w, r, "DOCUMENT_EXISTS", err.Error())
	case errors.Is(err, ErrUnsupported):
		response.Error(w, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_FILE", err.Error())
	case errors.Is(err, ErrStorageMissing):
		response.ServiceUnavailable(w, r, "ARCHIVE_UNAVAILABLE", err.Error())
	default:
		response.InternalErr(w, r, err, "document archive request failed")
	}
}

func intParam(w http.ResponseWriter, r *http.Request, name string, def, lo, hi int) (int, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		response.BadRequest(w, r, response.CodeValidationError, fmt.Sprintf("%s must be %d..%d", name, lo, hi))
		return 0, false
	}
	return n, true
}

// list: GET ?category=&isg_kind=&q=&expiring=true&important=true&limit=&offset=.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	h.listFor(w, r, engagementctx.Must(r.Context()).TaxpayerID, false)
}

func (h *Handler) listFor(w http.ResponseWriter, r *http.Request, taxpayerID int64, onlyVisible bool) {
	limit, ok := intParam(w, r, "limit", 50, 1, 200)
	if !ok {
		return
	}
	offset, ok := intParam(w, r, "offset", 0, 0, 1_000_000)
	if !ok {
		return
	}
	q := r.URL.Query()
	search := strings.TrimSpace(q.Get("q"))
	if len([]rune(search)) > 100 {
		response.BadRequest(w, r, response.CodeValidationError, "q is too long")
		return
	}
	expiring := false
	switch q.Get("expiring") {
	case "", "false":
	case "true":
		expiring = true
	default:
		response.BadRequest(w, r, response.CodeValidationError, "expiring must be true or false")
		return
	}
	important := false
	switch q.Get("important") {
	case "", "false":
	case "true":
		important = true
	default:
		response.BadRequest(w, r, response.CodeValidationError, "important must be true or false")
		return
	}
	p, err := h.svc.List(r.Context(), taxpayerID, Filter{
		Category: strings.TrimSpace(q.Get("category")), ISGKind: strings.TrimSpace(q.Get("isg_kind")), Q: search, ExpiringOnly: expiring,
		OnlyVisible: onlyVisible, OnlyImportant: important,
		Limit: int32(limit), Offset: int32(offset), //nolint:gosec // bounded above
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

// upload: multipart "file" (PDF / JPEG / PNG), "category", "isg_kind",
// "title", "note", "document_date", "valid_until", "visible_to_taxpayer",
// "important_kind" (#780 B).
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		response.Error(w, r, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", "file is too large or the form is invalid")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "FILE_REQUIRED", "multipart field \"file\" is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "FILE_UNREADABLE", "could not read the file")
		return
	}
	var uid int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		uid = p.UserInternal
	}
	f := func(k string) string { return strings.TrimSpace(r.FormValue(k)) }
	d, err := h.svc.Upload(r.Context(), engagementctx.Must(r.Context()).TaxpayerID, uid, header.Filename, data, Input{
		Category: f("category"), ISGKind: f("isg_kind"), Title: f("title"), Note: f("note"),
		DocumentDate: f("document_date"), ValidUntil: f("valid_until"), VisibleToTaxpayer: f("visible_to_taxpayer") == "true",
		ImportantKind: f("important_kind"),
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, d)
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("archiveDocumentUuid"))
	if err != nil {
		response.NotFound(w, r, "not found")
		return uuid.UUID{}, false
	}
	return id, true
}

func (h *Handler) file(w http.ResponseWriter, r *http.Request) {
	h.fileFor(w, r, engagementctx.Must(r.Context()).TaxpayerID, false)
}

func (h *Handler) fileFor(w http.ResponseWriter, r *http.Request, taxpayerID int64, onlyVisible bool) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	f, err := h.svc.Open(r.Context(), taxpayerID, id, onlyVisible)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, f.Name)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safe, url.PathEscape(f.Name)))
	_, _ = io.Copy(w, f.Body)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), engagementctx.Must(r.Context()).TaxpayerID, id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
