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
	"github.com/shopspring/decimal"

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
	approve := rbac.PermTenantJournalsApprove
	mux.Handle("GET /v1/tenant/export-drivers", tenant(read, h.drivers))
	// Office-wide export settings (#466): integrators Luca fetches itself.
	mux.Handle("GET /v1/tenant/journal-export-settings", tenant(read, h.getOfficeSettings))
	mux.Handle("PUT /v1/tenant/journal-export-settings", tenant(export, h.putOfficeSettings))
	// Office-wide transfer overview (#574): pending entries per taxpayer
	// period and the latest transfers with their trial balance check.
	mux.Handle("GET /v1/tenant/journal-transfers", tenant(read, h.listOfficeTransfers))
	mux.Handle("GET /v1/tenant/journal-transfers/pending", tenant(read, h.listOfficePending))
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}"
	mux.Handle("GET "+base+"/journal-drafts", engaged(read, h.listDrafts))
	mux.Handle("POST "+base+"/journal-drafts", engaged(write, h.createDraft))
	mux.Handle("GET "+base+"/journal-drafts/{draftUuid}", engaged(read, h.getDraft))
	// Onay sorumlu müşavirde (#566): hazırlayan stajyer onaylayamaz.
	mux.Handle("POST "+base+"/journal-drafts/{draftUuid}/approve", engaged(approve, h.approve))
	// Onayı gerekçeyle geri al (#566): onaylayan yetki geri de alır.
	mux.Handle("POST "+base+"/journal-drafts/{draftUuid}/unapprove", engaged(approve, h.unapprove))
	mux.Handle("PATCH "+base+"/journal-drafts/{draftUuid}/lines/{lineNo}", engaged(write, h.setLineAccount))
	mux.Handle("POST "+base+"/journal-drafts/{draftUuid}/reverse", engaged(export, h.reverse))
	mux.Handle("GET "+base+"/journal-exports", engaged(read, h.listBatches))
	mux.Handle("POST "+base+"/journal-exports", scoped(export, h.export))
	// Pre-export check and the document → file trail (#466).
	mux.Handle("POST "+base+"/journal-exports/check", scoped(export, h.check))
	mux.Handle("GET "+base+"/journal-export-documents", engaged(read, h.listDocuments))
	mux.Handle("GET "+base+"/journal-exports/{batchUuid}", engaged(read, h.getBatch))
	mux.Handle("GET "+base+"/journal-exports/{batchUuid}/files/{fileName}", engaged(read, h.download))
	mux.Handle("GET "+base+"/journal-export-settings", scoped(read, h.getSettings))
	mux.Handle("PUT "+base+"/journal-export-settings", scoped(export, h.putSettings))

	// Transfers to the accounting program (#391). The office asks, closes,
	// cancels and retries (export); the desktop agent of the office claims
	// and reports delivery with its own agent permission.
	agent := rbac.PermTenantAgentsWrite
	mux.Handle("POST "+base+"/journal-exports/{batchUuid}/transfers", engaged(export, h.requestTransfer))
	mux.Handle("GET "+base+"/journal-transfers", engaged(read, h.listTransfers))
	mux.Handle("GET "+base+"/journal-transfers/{transferUuid}", engaged(read, h.getTransfer))
	mux.Handle("POST "+base+"/journal-transfers/{transferUuid}/claim", engaged(agent, h.claimTransfer))
	mux.Handle("POST "+base+"/journal-transfers/{transferUuid}/delivery", engaged(agent, h.deliverTransfer))
	mux.Handle("POST "+base+"/journal-transfers/{transferUuid}/complete", engaged(export, h.completeTransfer))
	mux.Handle("POST "+base+"/journal-transfers/{transferUuid}/cancel", engaged(export, h.cancelTransfer))
	mux.Handle("POST "+base+"/journal-transfers/{transferUuid}/retry", engaged(export, h.retryTransfer))
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
	// Gövde isteğe bağlı: {"confirm_warnings": true, "note": "..."} (#669).
	var in struct {
		ConfirmWarnings bool   `json:"confirm_warnings"`
		Note            string `json:"note"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
			response.BadRequest(w, r, "INVALID_BODY", "body must be {\"confirm_warnings\": true, \"note\": \"...\"}")
			return
		}
	}
	if len(in.Note) > 1000 {
		response.BadRequest(w, r, "INVALID_BODY", "note must be at most 1000 characters")
		return
	}
	s := engagementctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	if err := h.svc.ApproveWith(r.Context(), s, p.UserInternal, id, Confirmation{Confirmed: in.ConfirmWarnings, Note: in.Note}); err != nil {
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

// unapprove: POST .../journal-drafts/{draftUuid}/unapprove {"reason": "..."}.
func (h *Handler) unapprove(w http.ResponseWriter, r *http.Request) {
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
	if err := h.svc.Unapprove(r.Context(), s, p.UserInternal, id, in.Reason); err != nil {
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

// setLineAccount: kontrol ekranında satırın hesap kodu ve / veya tutarı
// düzeltilir (#566). Gövde: {"account"?, "debit"?, "credit"?}; en az biri.
func (h *Handler) setLineAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "draftUuid")
	if !ok {
		return
	}
	line, err := strconv.ParseInt(r.PathValue("lineNo"), 10, 32)
	if err != nil || line < 1 {
		response.BadRequest(w, r, "INVALID_LINE", "lineNo must be a positive integer")
		return
	}
	var in struct {
		Account string           `json:"account"`
		Debit   *decimal.Decimal `json:"debit"`
		Credit  *decimal.Decimal `json:"credit"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "body must be {\"account\": \"770.01\"} and / or {\"debit\": \"100.00\", \"credit\": \"0\"}")
		return
	}
	amounts := in.Debit != nil || in.Credit != nil
	if strings.TrimSpace(in.Account) == "" && !amounts {
		response.BadRequest(w, r, "INVALID_BODY", "account or debit / credit is required")
		return
	}
	s := engagementctx.Must(r.Context())
	if amounts {
		var debit, credit decimal.Decimal
		if in.Debit != nil {
			debit = *in.Debit
		}
		if in.Credit != nil {
			credit = *in.Credit
		}
		if err := h.svc.SetLineAmounts(r.Context(), s, id, int32(line), debit, credit); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	if strings.TrimSpace(in.Account) != "" {
		if err := h.svc.SetLineAccount(r.Context(), s, id, int32(line), in.Account); err != nil {
			h.fail(w, r, err)
			return
		}
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

// check is the pre-export check: same body as export, nothing is written.
func (h *Handler) check(w http.ResponseWriter, r *http.Request) {
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
	v, err := h.svc.Check(r.Context(), engagementctx.Must(r.Context()), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// listDocuments is the document → file trail of the export history.
func (h *Handler) listDocuments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := paging(q.Get("limit"), q.Get("offset"))
	f := DocumentFilter{Q: q.Get("q"), Limit: limit, Offset: offset}
	if b := strings.TrimSpace(q.Get("batch")); b != "" {
		id, err := uuid.Parse(b)
		if err != nil {
			response.BadRequest(w, r, "INVALID_BATCH", "batch must be an export uuid")
			return
		}
		f.Batch = &id
	}
	items, err := h.svc.ListDocuments(r.Context(), engagementctx.Must(r.Context()), f)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items, "limit": limit, "offset": offset})
}

func (h *Handler) getOfficeSettings(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context())
	st, err := h.svc.GetOfficeSettings(r.Context(), org.ID)
	if err != nil {
		response.InternalErr(w, r, err, "could not load office export settings")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

func (h *Handler) putOfficeSettings(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context())
	var in OfficeSettings
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "request body is not valid office export settings")
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	st, err := h.svc.SaveOfficeSettings(r.Context(), org.ID, p.UserInternal, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
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
	var in SettingsInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "request body is not valid export settings")
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	st, err := h.svc.SaveSettings(r.Context(), engagementctx.Must(r.Context()), p.UserInternal, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// fail maps service errors to HTTP. A blocked export lists its issues as
// error details (code = issue code, field = entry/line).
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var blocked *BlockedError
	var draftBlocked *DraftBlockedError
	var needsConfirm *DraftNeedsConfirmationError
	switch {
	case errors.As(err, &needsConfirm):
		details := make([]response.Detail, 0, len(needsConfirm.Issues))
		for _, is := range needsConfirm.Issues {
			details = append(details, response.Detail{Code: is.Code})
		}
		response.ErrorWithDetails(w, r, http.StatusConflict, "JOURNAL_NEEDS_CONFIRMATION",
			"the draft has warnings (missing document / possible duplicate); approve with confirm_warnings", details)
	case errors.As(err, &draftBlocked):
		details := make([]response.Detail, 0, len(draftBlocked.Issues))
		for _, is := range draftBlocked.Issues {
			field := ""
			if is.LineNo > 0 {
				field = fmt.Sprintf("line %d", is.LineNo)
			}
			details = append(details, response.Detail{Field: field, Code: is.Code, Message: is.Account})
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, "JOURNAL_BLOCKED", "the draft has blocking issues", details)
	case errors.As(err, &blocked):
		details := make([]response.Detail, 0, len(blocked.Issues))
		for _, is := range blocked.Issues {
			details = append(details, response.Detail{Field: issueField(is), Code: is.Code, Message: issueMessage(is)})
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, "EXPORT_BLOCKED", "export blocked by validation issues", details)
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "not found")
	case errors.Is(err, ErrInvalidIntegrator):
		response.BadRequest(w, r, "INVALID_INTEGRATOR", err.Error())
	case errors.Is(err, ErrUnbalanced):
		response.Error(w, r, http.StatusUnprocessableEntity, "JOURNAL_UNBALANCED", err.Error())
	case errors.Is(err, ErrReasonRequired):
		response.BadRequest(w, r, "REASON_REQUIRED", "a reason is required")
	case errors.Is(err, ErrLineFromSource):
		response.Conflict(w, r, "LINE_FROM_SOURCE", err.Error())
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, "INVALID_DRAFT", err.Error())
	case errors.Is(err, ErrNotApproved):
		response.Conflict(w, r, "NOT_APPROVED", "entry is not approved")
	case errors.Is(err, ErrAlreadyExported):
		response.Conflict(w, r, "ALREADY_EXPORTED", "entry already exported; reverse it first")
	case errors.Is(err, ErrNothingToExport):
		response.Error(w, r, http.StatusUnprocessableEntity, "NOTHING_TO_EXPORT", "no approved entries to export")
	case errors.Is(err, ErrTransferExists):
		response.Conflict(w, r, "TRANSFER_EXISTS", "the export already has an open or imported transfer")
	case errors.Is(err, ErrTransferState):
		response.Conflict(w, r, "TRANSFER_STATE", "the transfer is not in a state that allows this")
	case errors.Is(err, ErrTransferDriver):
		response.Error(w, r, http.StatusUnprocessableEntity, "TRANSFER_UNSUPPORTED", "exports of this program cannot be transferred")
	case errors.Is(err, ErrTransferReversed):
		response.Error(w, r, http.StatusUnprocessableEntity, "TRANSFER_REVERSED", "every entry of the export was reversed")
	case errors.Is(err, ErrAgentUnknown):
		response.Error(w, r, http.StatusUnprocessableEntity, "AGENT_UNKNOWN", "desktop agent not found or revoked")
	case errors.Is(err, ErrTransferInvalid):
		response.BadRequest(w, r, "INVALID_TRANSFER", err.Error())
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
