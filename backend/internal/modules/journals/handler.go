package journals

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the journal draft and export routes (issue #143).
type Handler struct {
	svc   *Service
	files FileStore
}

// NewHandler builds the handler. files reads stored export files back.
func NewHandler(svc *Service, files FileStore) *Handler { return &Handler{svc: svc, files: files} }

// RegisterRoutes mounts the routes. Engagement routes go through the
// active-engagement chain. A nil resolver (tenancy off) or handler registers
// nothing.
func RegisterRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver, engResolver engagementctx.Resolver, h *Handler,
	scope func(http.Handler) http.Handler,
) {
	if orgResolver == nil || engResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(orgResolver)
	eng := middleware.RequireEngagement(engResolver)
	tenant := func(perm string, fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, middleware.RequireTenantPermission(perm))
	}
	engaged := func(perm string, fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, middleware.RequireTenantPermission(perm), eng)
	}
	// Routes reading export_settings (enforced, issue #166) run in one row
	// level security transaction.
	scoped := func(perm string, fn http.HandlerFunc) http.Handler {
		if scope == nil {
			return engaged(perm, fn)
		}
		return middleware.Chain(fn, authn, org, middleware.RequireTenantPermission(perm), eng, scope)
	}
	read, write, export := rbac.PermTenantJournalsRead, rbac.PermTenantJournalsWrite, rbac.PermTenantJournalsExport
	mux.Handle("GET /v1/tenant/export-drivers", tenant(read, h.drivers))
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}"
	mux.Handle("GET "+base+"/journal-drafts", engaged(read, h.listDrafts))
	mux.Handle("POST "+base+"/journal-drafts", engaged(write, h.createDraft))
	mux.Handle("GET "+base+"/journal-drafts/{draftUuid}", engaged(read, h.getDraft))
	mux.Handle("POST "+base+"/journal-drafts/{draftUuid}/approve", engaged(write, h.approve))
	mux.Handle("POST "+base+"/journal-drafts/{draftUuid}/reverse", engaged(export, h.reverse))
	mux.Handle("GET "+base+"/journal-exports", engaged(read, h.listBatches))
	mux.Handle("POST "+base+"/journal-exports", scoped(export, h.export))
	mux.Handle("GET "+base+"/journal-exports/{batchUuid}", engaged(read, h.getBatch))
	mux.Handle("GET "+base+"/journal-exports/{batchUuid}/files/{fileName}", engaged(read, h.download))
	mux.Handle("GET "+base+"/journal-export-settings", scoped(read, h.getSettings))
	mux.Handle("PUT "+base+"/journal-export-settings", scoped(export, h.putSettings))
}

func (h *Handler) drivers(w http.ResponseWriter, r *http.Request) {
	ds := Drivers()
	sort.Slice(ds, func(i, j int) bool { return ds[i].Name < ds[j].Name })
	response.JSON(w, r, http.StatusOK, map[string]any{"items": ds})
}

func (h *Handler) listDrafts(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	q := r.URL.Query()
	limit, offset := paging(q.Get("limit"), q.Get("offset"))
	status := strings.TrimSpace(q.Get("status"))
	switch status {
	case "", "draft", "approved", "exported":
	default:
		response.BadRequest(w, r, "INVALID_STATUS", "status must be draft, approved or exported")
		return
	}
	page, err := h.svc.ListDrafts(r.Context(), s, DraftFilter{Period: strings.TrimSpace(q.Get("period")), Status: status, Limit: limit, Offset: offset})
	if err != nil {
		response.InternalErr(w, r, err, "could not list journal drafts")
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

func (h *Handler) createDraft(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	var in DraftInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "request body is not a valid journal draft")
		return
	}
	d, err := h.svc.CreateDraft(r.Context(), s, p.UserInternal, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, d)
}

func (h *Handler) getDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "draftUuid")
	if !ok {
		return
	}
	d, err := h.svc.GetDraft(r.Context(), engagementctx.Must(r.Context()), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

func (h *Handler) approve(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "draftUuid")
	if !ok {
		return
	}
	s := engagementctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	if err := h.svc.Approve(r.Context(), s, p.UserInternal, id); err != nil {
		h.fail(w, r, err)
		return
	}
	d, err := h.svc.GetDraft(r.Context(), s, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

func (h *Handler) reverse(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "draftUuid")
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "body must be {\"reason\": \"...\"}")
		return
	}
	s := engagementctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	if err := h.svc.Reverse(r.Context(), s, p.UserInternal, id, in.Reason); err != nil {
		h.fail(w, r, err)
		return
	}
	d, err := h.svc.GetDraft(r.Context(), s, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

func (h *Handler) listBatches(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := paging(q.Get("limit"), q.Get("offset"))
	items, err := h.svc.ListBatches(r.Context(), engagementctx.Must(r.Context()), limit, offset)
	if err != nil {
		response.InternalErr(w, r, err, "could not list exports")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items, "limit": limit, "offset": offset})
}

func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	var in ExportInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "request body is not a valid export request")
		return
	}
	if strings.TrimSpace(in.Driver) == "" {
		response.BadRequest(w, r, "DRIVER_REQUIRED", "driver is required (see GET /v1/tenant/export-drivers)")
		return
	}
	if in.Period == "" && len(in.Drafts) == 0 {
		response.BadRequest(w, r, "SELECTION_REQUIRED", "give a period or the drafts to export")
		return
	}
	b, err := h.svc.Export(r.Context(), s, p.UserInternal, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, b)
}

func (h *Handler) getBatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "batchUuid")
	if !ok {
		return
	}
	b, err := h.svc.GetBatch(r.Context(), engagementctx.Must(r.Context()), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, b)
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "batchUuid")
	if !ok {
		return
	}
	if h.files == nil {
		response.NotFound(w, r, "file storage is not configured")
		return
	}
	f, rc, err := h.svc.OpenFile(r.Context(), engagementctx.Must(r.Context()), id, r.PathValue("fileName"), h.files)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	ct := f.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", f.Name))
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = io.Copy(w, rc)
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.GetSettings(r.Context(), engagementctx.Must(r.Context()))
	if err != nil {
		response.InternalErr(w, r, err, "could not load export settings")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

func (h *Handler) putSettings(w http.ResponseWriter, r *http.Request) {
	var in Settings
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "request body is not valid export settings")
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	st, err := h.svc.SaveSettings(r.Context(), engagementctx.Must(r.Context()), p.UserInternal, in)
	if err != nil {
		response.InternalErr(w, r, err, "could not save export settings")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// fail maps service errors to HTTP. A blocked export lists its issues as
// error details (code = issue code, field = entry/line).
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var blocked *BlockedError
	switch {
	case errors.As(err, &blocked):
		details := make([]response.Detail, 0, len(blocked.Issues))
		for _, is := range blocked.Issues {
			details = append(details, response.Detail{Field: issueField(is), Code: is.Code, Message: issueMessage(is)})
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, "EXPORT_BLOCKED", "export blocked by validation issues", details)
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "not found")
	case errors.Is(err, ErrUnbalanced):
		response.Error(w, r, http.StatusUnprocessableEntity, "JOURNAL_UNBALANCED", err.Error())
	case errors.Is(err, ErrReasonRequired):
		response.BadRequest(w, r, "REASON_REQUIRED", "a reason is required to reverse an export")
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, "INVALID_DRAFT", err.Error())
	case errors.Is(err, ErrNotApproved):
		response.Conflict(w, r, "NOT_APPROVED", "entry is not approved")
	case errors.Is(err, ErrAlreadyExported):
		response.Conflict(w, r, "ALREADY_EXPORTED", "entry already exported; reverse it first")
	case errors.Is(err, ErrNothingToExport):
		response.Error(w, r, http.StatusUnprocessableEntity, "NOTHING_TO_EXPORT", "no approved entries to export")
	case errors.Is(err, accountingexport.ErrUnknownDriver), errors.Is(err, accountingexport.ErrTemplate):
		response.BadRequest(w, r, "UNKNOWN_DRIVER", err.Error())
	default:
		response.InternalErr(w, r, err, "journal operation failed")
	}
}

func issueField(is accountingexport.Issue) string {
	switch {
	case is.EntryNo != "" && is.Line > 0:
		return fmt.Sprintf("entry:%s/line:%d", is.EntryNo, is.Line)
	case is.EntryNo != "":
		return "entry:" + is.EntryNo
	}
	return ""
}

func issueMessage(is accountingexport.Issue) string {
	if len(is.Params) == 0 {
		return is.Severity
	}
	b, _ := json.Marshal(is.Params)
	return string(b)
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.NotFound(w, r, "not found")
		return uuid.UUID{}, false
	}
	return id, true
}

func paging(l, o string) (limit, offset int32) {
	limit = 50
	if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 200 {
		limit = int32(v) //nolint:gosec // bounded
	}
	if v, err := strconv.Atoi(o); err == nil && v >= 0 && v <= 1_000_000 {
		offset = int32(v) //nolint:gosec // bounded
	}
	return limit, offset
}
