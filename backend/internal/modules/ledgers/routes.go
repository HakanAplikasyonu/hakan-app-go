package ledgers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ledgers/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// PathParam is the file path parameter.
const PathParam = "ledgerUuid"

// Handler serves the Defterler routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Module mounts the routes. Scope runs requests in one row level security
// transaction (rls.Middleware).
type Module struct {
	H           *Handler
	Engagements engagementctx.Resolver
	Scope       func(http.Handler) http.Handler
}

// Name implements module.Module.
func (Module) Name() string { return "ledgers" }

// Routes implements module.Module. The books are taxpayer documents:
// reading needs tenant.documents.read, uploading, deleting and the Defter
// dönemi setting tenant.documents.write.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	if m.Scope != nil {
		eng = append(eng, m.Scope)
	}
	read, write := rbac.PermTenantDocumentsRead, rbac.PermTenantDocumentsWrite
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/ledgers"
	one := base + "/{" + PathParam + "}"
	r.Tenant("GET "+base, read, h.List, eng...)
	r.Tenant("POST "+base, write, h.Upload, eng...)
	r.Tenant("PUT "+base+"/settings", write, h.PutSettings, eng...)
	r.Tenant("GET "+one, read, h.Get, eng...)
	r.Tenant("GET "+one+"/file", read, h.File, eng...)
	r.Tenant("DELETE "+one, write, h.Delete, eng...)
}

func subject(r *http.Request) Subject {
	e := engagementctx.Must(r.Context())
	return Subject{OrganizationID: e.OrganizationID, EngagementID: e.ID, StartedAt: e.StartedAt}
}

func userID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func fileID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(PathParam))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "ledgerUuid is invalid")
		return uuid.Nil, false
	}
	return id, true
}

// List: GET .../ledgers.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.List(r.Context(), subject(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Upload: POST .../ledgers (multipart: file, period, kind).
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, parse.MaxBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file must be an Excel, CSV, XML or ZIP file up to 20 MB")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, parse.MaxBytes+1))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file could not be read")
		return
	}
	out, err := h.svc.Upload(r.Context(), subject(r), UploadInput{
		FileName: hdr.Filename, Data: data, Period: strings.TrimSpace(r.FormValue("period")),
		Kind: strings.TrimSpace(r.FormValue("kind")), UserID: userID(r),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if out.Replayed {
		status = http.StatusOK
	}
	response.JSON(w, r, status, out)
}

// PutSettings: PUT .../ledgers/settings.
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var in Settings
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	out, err := h.svc.SaveSettings(r.Context(), subject(r), in, userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Get: GET .../ledgers/{ledgerUuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := fileID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Get(r.Context(), subject(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// File: GET .../ledgers/{ledgerUuid}/file (download).
func (h *Handler) File(w http.ResponseWriter, r *http.Request) {
	id, ok := fileID(w, r)
	if !ok {
		return
	}
	rc, it, err := h.svc.File(r.Context(), subject(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	safe := strings.Map(func(c rune) rune {
		if c < 0x20 || c == 0x7f || c == '"' || c == '\\' || c > 0x7e {
			return '_'
		}
		return c
	}, it.FileName)
	w.Header().Set("Content-Type", it.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safe))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

// Delete: DELETE .../ledgers/{ledgerUuid}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := fileID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), subject(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "Ledger file not found")
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrUnsupported):
		response.BadRequest(w, r, response.CodeValidationError, "file must be an Excel (xlsx, xls), CSV, XML or ZIP file")
	case errors.Is(err, ErrStorageMissing):
		response.Error(w, r, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "file storage is not configured")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
