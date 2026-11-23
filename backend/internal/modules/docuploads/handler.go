package docuploads

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/upload"
	receiptshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the typed upload routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Module mounts the routes.
//
//   - GET/PUT /v1/tenant/document-upload-settings: the office setting.
//   - GET/POST /v1/tenant/engagements/{engagementUuid}/document-uploads:
//     the engagement's typed uploads and a new one (multipart).
type Module struct {
	H           *Handler
	Engagements engagementctx.Resolver
	// Scope runs each request in one row level security transaction.
	Scope func(http.Handler) http.Handler
}

// Name implements module.Module.
func (Module) Name() string { return "docuploads" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	read, write := rbac.PermTenantDocumentsRead, rbac.PermTenantDocumentsWrite
	r.Tenant("GET /v1/tenant/document-upload-settings", read, h.GetSettings)
	r.Tenant("PUT /v1/tenant/document-upload-settings", rbac.PermTenantSettingsWrite, h.PutSettings)
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
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/document-uploads"
	r.Tenant("GET "+base, read, h.List, eng...)
	r.Tenant("POST "+base, write, h.Create, eng...)
}

func userID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

// List: GET .../document-uploads?period=YYYY-MM.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context(), engagementctx.Must(r.Context()), strings.TrimSpace(r.URL.Query().Get("period")))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Create: POST .../document-uploads (multipart: file, doc_type, period,
// period_auto, document_date, lock_choice, target_period).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context())
	st, err := h.svc.Settings(r.Context(), org.ID)
	if err != nil {
		response.InternalErr(w, r, err, "could not load document upload settings")
		return
	}
	if !st.Enabled {
		writeError(w, r, ErrDisabled)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileBytes+1<<20)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		response.Error(w, r, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", fmt.Sprintf("multipart form with a file is required (max %d MB)", MaxFileBytes>>20))
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file could not be read")
		return
	}
	in := Input{
		DocType: strings.TrimSpace(r.FormValue("doc_type")), Period: strings.TrimSpace(r.FormValue("period")),
		PeriodAuto: r.FormValue("period_auto") == "true", DocumentDate: r.FormValue("document_date"),
		LockChoice: strings.TrimSpace(r.FormValue("lock_choice")), TargetPeriod: r.FormValue("target_period"),
		FileName: hdr.Filename, Data: data,
	}
	// A Z report / expense becomes a receipt draft: receipt write as well.
	if (in.DocType == TypeZReport || in.DocType == TypeExpense) && !org.Has(rbac.PermTenantReceiptsWrite) {
		response.Forbidden(w, r, "tenant.receipts.write is required to upload a Z report or an expense receipt")
		return
	}
	v, err := h.svc.Upload(r.Context(), engagementctx.Must(r.Context()), userID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

// GetSettings: GET /v1/tenant/document-upload-settings.
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Settings(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		response.InternalErr(w, r, err, "could not load document upload settings")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// PutSettings: PUT /v1/tenant/document-upload-settings {"enabled": bool}.
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Enabled == nil {
		response.BadRequest(w, r, "INVALID_BODY", "enabled is required")
		return
	}
	st, err := h.svc.SaveSettings(r.Context(), orgctx.Must(r.Context()).ID, userID(r), *in.Enabled)
	if err != nil {
		response.InternalErr(w, r, err, "could not save document upload settings")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrPeriodLocked):
		response.Conflict(w, r, "PERIOD_LOCKED", "The chosen month is locked: reject, take it in as a correction or apply it to another month")
	case errors.Is(err, ErrTargetLocked):
		response.Conflict(w, r, "TARGET_PERIOD_LOCKED", "The selected month must be another, open month")
	case errors.Is(err, ErrNeedsImage):
		response.Error(w, r, http.StatusUnsupportedMediaType, "IMAGE_REQUIRED", "Z reports and expense receipts are read from a photo (JPEG, PNG, WebP)")
	case errors.Is(err, ErrUnsupported):
		response.Error(w, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_FILE", "Upload a PDF, JPEG or PNG file (e-invoice: XML or ZIP)")
	case errors.Is(err, ErrExists):
		response.Conflict(w, r, "DOCUMENT_EXISTS", "The same file was uploaded before")
	case errors.Is(err, ErrDisabled):
		response.Conflict(w, r, "DOCUMENT_UPLOADS_OFF", "The office turned typed document upload off (Ayarlar → Belgeler)")
	case errors.Is(err, upload.ErrEmpty), errors.Is(err, upload.ErrTooLarge), errors.Is(err, upload.ErrUnsupported):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, upload.ErrNoOwnTaxID):
		response.Conflict(w, r, "NO_OWN_TAX_ID", "The taxpayer has no VKN/TCKN on record")
	default:
		receiptshandler.WriteError(w, r, err)
	}
}
