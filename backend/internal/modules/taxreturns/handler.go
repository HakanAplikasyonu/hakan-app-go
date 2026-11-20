package taxreturns

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	// Ofis ayarı (#710): son kontrolün sorumluda olup olmadığı.
	mux.Handle("GET /v1/tenant/tax-return-settings",
		middleware.Chain(http.HandlerFunc(h.getSettings), authn, org, middleware.RequireTenantPermission(read)))
	mux.Handle("PUT /v1/tenant/tax-return-settings",
		middleware.Chain(http.HandlerFunc(h.putSettings), authn, org, middleware.RequireTenantPermission(rbac.PermTenantSettingsWrite)))
	route("GET", "", read, h.list)
	route("GET", "/{returnKind}/{period}", read, h.get)
	route("PUT", "/{returnKind}/{period}", write, h.saveDraft)
	route("POST", "/{returnKind}/{period}/prepare", write, h.prepare)
	route("POST", "/{returnKind}/{period}/reopen", write, h.reopen)
	// Son kontrol (#669): kontrol edildi / verildi yalnız sorumlu; ofis
	// ayarı kapalıysa hazırlayan da işaretler (#710, finalCheck).
	route("POST", "/{returnKind}/{period}/check", write, h.finalCheck(h.check))
	route("POST", "/{returnKind}/{period}/file", write, h.finalCheck(h.file))
	route("GET", "/{returnKind}/{period}/export", read, h.export)
	route("PUT", "/{returnKind}/{period}/accrual-document", write, h.uploadAccrual)
	route("GET", "/{returnKind}/{period}/accrual-document", read, h.accrualFile)
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
	case errors.Is(err, ErrNotFiled):
		response.Conflict(w, r, "TAX_RETURN_NOT_FILED", "the accrual slip is attached after the return is filed")
	case errors.Is(err, ErrNotPDF):
		response.Error(w, r, http.StatusUnsupportedMediaType, "NOT_PDF", "the accrual slip must be a PDF file")
	case errors.Is(err, ErrStorageMissing):
		response.Error(w, r, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "file storage is not configured")
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

// finalCheck lets fn through for the responsible (tenant.tax_returns.approve)
// and, when the office turned the final check off (#710), for whoever
// prepares the return; otherwise 403 as before (#669).
func (h *Handler) finalCheck(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := orgctx.Must(r.Context())
		if !scope.Has(rbac.PermTenantTaxReturnsApprove) {
			st, err := h.svc.Settings(r.Context(), scope.ID)
			if err != nil {
				response.InternalErr(w, r, err, "could not load tax return settings")
				return
			}
			if st.FinalCheckRequired {
				response.Forbidden(w, r, "Missing permission: "+rbac.PermTenantTaxReturnsApprove)
				return
			}
		}
		fn(w, r)
	}
}

// getSettings: GET /v1/tenant/tax-return-settings (#710).
func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Settings(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		response.InternalErr(w, r, err, "could not load tax return settings")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// putSettings: PUT /v1/tenant/tax-return-settings {"final_check_required": bool}.
func (h *Handler) putSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		FinalCheckRequired *bool `json:"final_check_required"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.FinalCheckRequired == nil {
		response.BadRequest(w, r, "INVALID_BODY", "final_check_required is required")
		return
	}
	st, err := h.svc.SaveSettings(r.Context(), orgctx.Must(r.Context()).ID, userID(r), Settings{FinalCheckRequired: *in.FinalCheckRequired})
	if err != nil {
		response.InternalErr(w, r, err, "could not save tax return settings")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

func (h *Handler) check(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	p, err := h.svc.Check(r.Context(), s, userID(r), r.PathValue("returnKind"), r.PathValue("period"))
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
	export, contentType := h.svc.Export, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	switch r.URL.Query().Get("format") {
	case "", "xlsx":
	case "pdf": // beyan özeti (#669 H)
		export, contentType = h.svc.ExportPDF, "application/pdf"
	default:
		response.BadRequest(w, r, response.CodeValidationError, "format must be xlsx or pdf")
		return
	}
	data, name, err := export(r.Context(), s, r.PathValue("returnKind"), r.PathValue("period"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// uploadAccrual: multipart "file" (the GİB tahakkuk PDF) of a filed return.
func (h *Handler) uploadAccrual(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, MaxAccrualBytes+(1<<20))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		response.Error(w, r, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", "file is too large or the form is invalid")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "FILE_REQUIRED", "multipart field \"file\" is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, MaxAccrualBytes+1))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "FILE_UNREADABLE", "could not read the file")
		return
	}
	p, err := h.svc.UploadAccrual(r.Context(), s, userID(r), r.PathValue("returnKind"), r.PathValue("period"), header.Filename, data)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

func (h *Handler) accrualFile(w http.ResponseWriter, r *http.Request) {
	f, err := h.svc.OpenAccrual(r.Context(), engagementctx.Must(r.Context()), r.PathValue("returnKind"), r.PathValue("period"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	w.Header().Set("Content-Type", "application/pdf")
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
