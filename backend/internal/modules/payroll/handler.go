package payroll

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
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the payroll routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the office routes behind the active-engagement chain
// (/v1/tenant/engagements/{engagementUuid}/payroll/...). The GET routes are
// also what the office mobile app reads. A nil resolver registers nothing.
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
		mux.Handle(method+" /v1/tenant/engagements/{"+engagementctx.PathParam+"}/payroll"+path,
			middleware.Chain(fn, authn, org, middleware.RequireTenantPermission(perm), eng))
	}
	read, write := rbac.PermTenantPayrollRead, rbac.PermTenantPayrollWrite
	route("GET", "/employees", read, h.listEmployees)
	route("POST", "/employees", write, h.createEmployee)
	route("PATCH", "/employees/{employeeUuid}", write, h.updateEmployee)
	route("POST", "/imports", write, h.importFile)
	route("GET", "/runs", read, h.listRuns)
	route("GET", "/runs/{runUuid}", read, h.getRun)
	route("DELETE", "/runs/{runUuid}", write, h.deleteRun)
	route("GET", "/runs/{runUuid}/export", read, h.exportRun)
	route("POST", "/runs/{runUuid}/journal-draft", write, h.journalDraft)
	route("POST", "/bank-match", write, h.bankMatch)
	route("GET", "/sgk-premiums", read, h.listPremiums)
	route("POST", "/sgk-premiums", write, h.createPremium)
	route("PATCH", "/sgk-premiums/{premiumUuid}", write, h.updatePremium)
	route("GET", "/documents", read, h.listDocuments)
	route("POST", "/documents", write, h.uploadDocument)
	route("GET", "/documents/{documentUuid}/file", read, h.officeFile)
	route("DELETE", "/documents/{documentUuid}", write, h.deleteDocument)
}

// RegisterTaxpayerRoutes mounts the mükellef panel and mobile app routes:
// the taxpayer's payroll PDFs, read only, for the taxpayer's owner role.
func RegisterTaxpayerRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader,
	tpResolver taxpayerctx.Resolver, h *Handler,
) {
	if tpResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	tp := middleware.RequireTaxpayer(tpResolver)
	mux.Handle("GET /v1/taxpayer/payroll/documents", middleware.Chain(http.HandlerFunc(h.myDocuments), authn, tp))
	mux.Handle("GET /v1/taxpayer/payroll/documents/{documentUuid}/file", middleware.Chain(http.HandlerFunc(h.myFile), authn, tp))
}

func userID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.NotFound(w, r, "not found")
		return uuid.UUID{}, false
	}
	return id, true
}

// paging reads limit (1..200, default 50) and offset.
func paging(w http.ResponseWriter, r *http.Request) (int32, int32, bool) {
	q := r.URL.Query()
	limit, offset := int32(50), int32(0)
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			response.BadRequest(w, r, response.CodeValidationError, "limit must be 1..200")
			return 0, 0, false
		}
		limit = int32(n) //nolint:gosec // bounded
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 1_000_000 {
			response.BadRequest(w, r, response.CodeValidationError, "offset must be 0..1000000")
			return 0, 0, false
		}
		offset = int32(n) //nolint:gosec // bounded
	}
	return limit, offset, true
}

func yearParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	y := strings.TrimSpace(r.URL.Query().Get("year"))
	if y == "" {
		return "", true
	}
	if n, err := strconv.Atoi(y); err != nil || n < 2000 || n > 2100 {
		response.BadRequest(w, r, response.CodeValidationError, "year must be YYYY")
		return "", false
	}
	return y, true
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "not found")
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrInvalidOverride), errors.Is(err, journals.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrDuplicate):
		response.Conflict(w, r, "EMPLOYEE_EXISTS", "an employee with this national id exists")
	case errors.Is(err, ErrRunLocked):
		response.Conflict(w, r, "PAYROLL_RUN_LOCKED", "the payroll's journal entry is approved; reverse it first")
	case errors.Is(err, ErrJournalExists):
		response.Conflict(w, r, "PAYROLL_JOURNAL_EXISTS", "the payroll already has a journal draft")
	case errors.Is(err, ErrDocumentExists):
		response.Conflict(w, r, "PAYROLL_DOCUMENT_EXISTS", err.Error())
	case errors.Is(err, ErrNotPDF):
		response.Error(w, r, http.StatusUnsupportedMediaType, "NOT_PDF", "payroll documents must be PDF files")
	case errors.Is(err, ErrMappingMissing):
		response.Error(w, r, http.StatusUnprocessableEntity, "PAYROLL_MAPPING_INCOMPLETE", "name, gross and net columns must be mapped")
	case errors.Is(err, ErrPeriodRequired):
		response.Error(w, r, http.StatusUnprocessableEntity, "PAYROLL_PERIOD_REQUIRED", "the file does not name the period; send period=YYYY-MM")
	case errors.Is(err, ErrNoPayrollSheet):
		response.Error(w, r, http.StatusUnprocessableEntity, "PAYROLL_SHEET_NOT_FOUND", "no sheet looks like a payroll")
	case errors.Is(err, parse.ErrUnsupported), errors.Is(err, parse.ErrEmpty), errors.Is(err, parse.ErrTooLarge), errors.Is(err, parse.ErrTooManyRows):
		response.BadRequest(w, r, "FILE_UNREADABLE", err.Error())
	case errors.Is(err, ErrStorageMissing), errors.Is(err, ErrJournalsDisabled):
		response.ServiceUnavailable(w, r, "PAYROLL_UNAVAILABLE", err.Error())
	default:
		response.InternalErr(w, r, err, "payroll request failed")
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

// ---- employees ----

func (h *Handler) listEmployees(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	limit, offset, ok := paging(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	switch status {
	case "", "all":
		status = ""
	case "active", "exited":
	default:
		response.BadRequest(w, r, response.CodeValidationError, "status must be active, exited or all")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 100 {
		response.BadRequest(w, r, response.CodeValidationError, "q is too long")
		return
	}
	p, err := h.svc.ListEmployees(r.Context(), s.TaxpayerID, EmployeeFilter{Status: status, Q: q, Limit: limit, Offset: offset})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

func (h *Handler) createEmployee(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	var in EmployeeInput
	if !decode(w, r, &in) {
		return
	}
	e, err := h.svc.CreateEmployee(r.Context(), s.TaxpayerID, userID(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, e)
}

func (h *Handler) updateEmployee(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "employeeUuid")
	if !ok {
		return
	}
	var in EmployeeInput
	if !decode(w, r, &in) {
		return
	}
	e, err := h.svc.UpdateEmployee(r.Context(), engagementctx.Must(r.Context()).TaxpayerID, id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, e)
}

// ---- import ----

const maxImportBytes = 20 << 20

// importFile: multipart "file" (xlsx / csv), optional "period" (YYYY-MM),
// optional "mapping" (JSON {"B":"gross", ...}) and ?preview=true.
func (h *Handler) importFile(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes+(1<<20))
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
	data, err := io.ReadAll(io.LimitReader(file, maxImportBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxImportBytes {
		response.Error(w, r, http.StatusBadRequest, "FILE_UNREADABLE", "could not read the file")
		return
	}
	opt := ImportOptions{Preview: r.URL.Query().Get("preview") == "true", Period: strings.TrimSpace(r.FormValue("period"))}
	if opt.Period != "" && !periodRe.MatchString(opt.Period) {
		response.BadRequest(w, r, response.CodeValidationError, "period must be YYYY-MM")
		return
	}
	if m := strings.TrimSpace(r.FormValue("mapping")); m != "" {
		if err := json.Unmarshal([]byte(m), &opt.Override); err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "mapping must be a JSON object of column letter to field")
			return
		}
	}
	res, err := h.svc.Import(r.Context(), s, userID(r), header.Filename, data, opt)
	if err != nil {
		if errors.Is(err, ErrMappingMissing) || errors.Is(err, ErrPeriodRequired) {
			// The preview tells the client what to fix.
			response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, codeOf(err), err.Error(), missingDetails(res))
			return
		}
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if res.Committed {
		status = http.StatusCreated
	}
	response.JSON(w, r, status, res)
}

func codeOf(err error) string {
	if errors.Is(err, ErrPeriodRequired) {
		return "PAYROLL_PERIOD_REQUIRED"
	}
	return "PAYROLL_MAPPING_INCOMPLETE"
}

func missingDetails(res ImportResult) []response.Detail {
	out := []response.Detail{}
	for _, f := range res.Missing {
		out = append(out, response.Detail{Field: f, Message: "column is not mapped", Code: "unmapped"})
	}
	if res.Period == "" {
		out = append(out, response.Detail{Field: "period", Message: "period is not in the file", Code: "required"})
	}
	return out
}

// ---- runs ----

func (h *Handler) listRuns(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := paging(w, r)
	if !ok {
		return
	}
	year, ok := yearParam(w, r)
	if !ok {
		return
	}
	p, err := h.svc.ListRuns(r.Context(), engagementctx.Must(r.Context()), year, limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

func (h *Handler) getRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "runUuid")
	if !ok {
		return
	}
	d, err := h.svc.GetRun(r.Context(), engagementctx.Must(r.Context()), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

func (h *Handler) deleteRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "runUuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteRun(r.Context(), engagementctx.Must(r.Context()), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) exportRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "runUuid")
	if !ok {
		return
	}
	data, name, err := h.svc.ExportRun(r.Context(), engagementctx.Must(r.Context()), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

func (h *Handler) journalDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "runUuid")
	if !ok {
		return
	}
	var a Accounts
	if r.ContentLength != 0 {
		if !decode(w, r, &a) {
			return
		}
	}
	d, err := h.svc.CreateJournalDraft(r.Context(), engagementctx.Must(r.Context()), userID(r), id, a)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, d)
}

func (h *Handler) bankMatch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Period string `json:"period"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !periodRe.MatchString(in.Period) {
		response.BadRequest(w, r, response.CodeValidationError, "period must be YYYY-MM")
		return
	}
	res, err := h.svc.MatchBank(r.Context(), engagementctx.Must(r.Context()), userID(r), in.Period)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

// ---- SGK ----

func (h *Handler) listPremiums(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := paging(w, r)
	if !ok {
		return
	}
	year, ok := yearParam(w, r)
	if !ok {
		return
	}
	p, err := h.svc.ListPremiums(r.Context(), engagementctx.Must(r.Context()).TaxpayerID, year, limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

func (h *Handler) createPremium(w http.ResponseWriter, r *http.Request) {
	var in PremiumInput
	if !decode(w, r, &in) {
		return
	}
	p, err := h.svc.CreatePremium(r.Context(), engagementctx.Must(r.Context()).TaxpayerID, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, p)
}

func (h *Handler) updatePremium(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "premiumUuid")
	if !ok {
		return
	}
	var in PremiumInput
	if !decode(w, r, &in) {
		return
	}
	if in.Period != "" {
		response.BadRequest(w, r, response.CodeValidationError, "period cannot be changed")
		return
	}
	p, err := h.svc.UpdatePremium(r.Context(), engagementctx.Must(r.Context()).TaxpayerID, id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

// ---- documents ----

func (h *Handler) listDocuments(w http.ResponseWriter, r *http.Request) {
	h.documents(w, r, engagementctx.Must(r.Context()).TaxpayerID, false)
}

func (h *Handler) documents(w http.ResponseWriter, r *http.Request, taxpayerID int64, onlyVisible bool) {
	limit, offset, ok := paging(w, r)
	if !ok {
		return
	}
	period := strings.TrimSpace(r.URL.Query().Get("period"))
	if period != "" && !periodRe.MatchString(period) {
		response.Error(w, r, http.StatusBadRequest, "INVALID_PERIOD", "period must be YYYY-MM")
		return
	}
	p, err := h.svc.ListDocuments(r.Context(), taxpayerID, onlyVisible, period, limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

// uploadDocument: multipart "file" (PDF), "period", optional "kind",
// "title", "employee_uuid" and "visible_to_taxpayer" (default true).
func (h *Handler) uploadDocument(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, MaxDocumentBytes+(1<<20))
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
	data, err := io.ReadAll(io.LimitReader(file, MaxDocumentBytes+1))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "FILE_UNREADABLE", "could not read the file")
		return
	}
	in := DocumentInput{Period: strings.TrimSpace(r.FormValue("period")), Kind: strings.TrimSpace(r.FormValue("kind")),
		Title: strings.TrimSpace(r.FormValue("title")), VisibleToTaxpayer: r.FormValue("visible_to_taxpayer") != "false"}
	if v := strings.TrimSpace(r.FormValue("employee_uuid")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "employee_uuid is not a uuid")
			return
		}
		in.EmployeeUUID = &id
	}
	d, err := h.svc.UploadDocument(r.Context(), s.TaxpayerID, userID(r), header.Filename, data, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, d)
}

func (h *Handler) officeFile(w http.ResponseWriter, r *http.Request) {
	h.file(w, r, engagementctx.Must(r.Context()).TaxpayerID, false)
}

func (h *Handler) file(w http.ResponseWriter, r *http.Request, taxpayerID int64, onlyVisible bool) {
	id, ok := pathUUID(w, r, "documentUuid")
	if !ok {
		return
	}
	f, err := h.svc.OpenDocument(r.Context(), taxpayerID, id, onlyVisible)
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

func (h *Handler) deleteDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "documentUuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteDocument(r.Context(), engagementctx.Must(r.Context()).TaxpayerID, id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- mükellef panel ----

// ownerOnly: payroll PDFs carry every employee's salary; the taxpayer's
// employee role does not see them.
func ownerOnly(w http.ResponseWriter, r *http.Request) (taxpayerctx.Scope, bool) {
	s := taxpayerctx.Must(r.Context())
	if s.Role != taxpayerctx.RoleOwner {
		response.Forbidden(w, r, "payroll documents are visible to the taxpayer's owner")
		return s, false
	}
	return s, true
}

func (h *Handler) myDocuments(w http.ResponseWriter, r *http.Request) {
	s, ok := ownerOnly(w, r)
	if !ok {
		return
	}
	h.documents(w, r, s.ID, true)
}

func (h *Handler) myFile(w http.ResponseWriter, r *http.Request) {
	s, ok := ownerOnly(w, r)
	if !ok {
		return
	}
	h.file(w, r, s.ID, true)
}
