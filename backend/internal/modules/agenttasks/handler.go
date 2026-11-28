package agenttasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Downloader reads a stored file back.
type Downloader interface {
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// Auditor records an audit event (activity.Recorder).
type Auditor interface {
	Record(ctx context.Context, actorID *int64, action, resource string, resourceUUID *uuid.UUID, payload map[string]any, r *http.Request)
}

// Audit actions.
const (
	ActionExportStarted = "agent_task.luca_export_started"
	ActionApproved      = "agent_task.approved"
	ActionCancelled     = "agent_task.cancelled"
	ActionResolved      = "agent_task.resolved"
	ActionFinished      = "agent_task.finished"
	auditResource       = "agent_task"
)

// Handler serves the routes.
type Handler struct {
	svc   *Service
	files Downloader
	audit Auditor
}

// NewHandler builds the handler.
func NewHandler(svc *Service, files Downloader) *Handler { return &Handler{svc: svc, files: files} }

// WithAudit turns on the audit record (who opened, approved, finished).
func (h *Handler) WithAudit(a Auditor) *Handler {
	h.audit = a
	return h
}

// Module mounts the routes. Scope runs requests in one row level security
// transaction (rls.Middleware): the file builders read enforced tables.
type Module struct {
	H     *Handler
	Scope func(http.Handler) http.Handler
}

// Name implements module.Module.
func (Module) Name() string { return "agenttasks" }

// Routes implements module.Module. Office routes: the panel reads tasks,
// opens a "Luca'ya aktar" run, approves a cari task, cancels, closes a
// needs_check task. Agent routes (tenant.agents.write) take a task, report
// progress, download its files and send the results.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	var mw []func(http.Handler) http.Handler
	if m.Scope != nil {
		mw = append(mw, m.Scope)
	}
	read, export, agent := rbac.PermTenantJournalsRead, rbac.PermTenantJournalsExport, rbac.PermTenantAgentsWrite
	r.Tenant("GET /v1/tenant/desktop-helper-settings", read, h.getSettings, mw...)
	r.Tenant("PUT /v1/tenant/desktop-helper-settings", export, h.putSettings, mw...)
	base := "/v1/tenant/agent-tasks"
	r.Tenant("GET "+base, read, h.list, mw...)
	r.Tenant("POST "+base+"/luca-export", export, h.startExport, mw...)
	task := base + "/{agentTaskUuid}"
	r.Tenant("GET "+task, read, h.get, mw...)
	r.Tenant("POST "+task+"/approve", export, h.approve, mw...)
	r.Tenant("POST "+task+"/cancel", export, h.cancel, mw...)
	r.Tenant("POST "+task+"/resolve", export, h.resolve, mw...)
	r.Tenant("POST "+task+"/claim", agent, h.claim, mw...)
	r.Tenant("POST "+task+"/progress", agent, h.progress, mw...)
	r.Tenant("POST "+task+"/result", agent, h.result, mw...)
	r.Tenant("GET "+task+"/files/{agentFileUuid}/content", agent, h.fileContent, mw...)
	r.Tenant("POST "+task+"/files/{agentFileUuid}/result", agent, h.fileResult, mw...)
}

func scopeOf(r *http.Request) Scope {
	org := orgctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	return Scope{OrganizationID: org.ID, UserID: p.UserInternal, ReadAll: org.Has(rbac.PermTenantEngagementsReadAll)}
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) bool {
	if h.svc == nil {
		response.Error(w, r, http.StatusServiceUnavailable, "AGENT_TASKS_UNAVAILABLE", "the desktop helper is not available")
		return false
	}
	return true
}

// decode reads a small JSON body; an empty body leaves dst as is.
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dst)
	if err != nil && err != io.EOF {
		response.BadRequest(w, r, "INVALID_BODY", "request body is not valid JSON")
		return false
	}
	return true
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.NotFound(w, r, "not found")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "task not found")
	case errors.Is(err, ErrAgentUnknown):
		response.Error(w, r, http.StatusNotFound, "AGENT_UNKNOWN", "desktop agent not found or revoked")
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrDisabled):
		response.Conflict(w, r, "HELPER_DISABLED", "the desktop helper is turned off for this office")
	case errors.Is(err, ErrAgentBusy):
		response.Conflict(w, r, "AGENT_BUSY", "the agent already works on another task")
	case errors.Is(err, ErrNotReady):
		response.Conflict(w, r, "TASK_NOT_READY", "an earlier task of the run is not finished")
	case errors.Is(err, ErrNotApproved):
		response.Conflict(w, r, "TASK_NOT_APPROVED", "the task is not approved")
	case errors.Is(err, ErrState):
		response.Conflict(w, r, "TASK_STATE", "the task is not in a state that allows this")
	default:
		response.InternalErr(w, r, err, "could not process the task")
	}
}

func (h *Handler) record(r *http.Request, action string, t Task, payload map[string]any) {
	if h.audit == nil {
		return
	}
	sc := scopeOf(r)
	if payload == nil {
		payload = map[string]any{}
	}
	payload["kind"] = t.Kind
	payload["engagement_uuid"] = t.EngagementUUID.String()
	payload["status"] = t.Status
	if t.Period != nil {
		payload["period"] = *t.Period
	}
	if t.RunUUID != nil {
		payload["run_uuid"] = t.RunUUID.String()
	}
	if t.Agent != nil {
		payload["agent_uuid"] = t.Agent.UUID.String()
	}
	actor := sc.UserID
	id := t.UUID
	h.audit.Record(r.Context(), &actor, action, auditResource, &id, payload, r)
}

// getSettings: GET /v1/tenant/desktop-helper-settings.
func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	st, err := h.svc.GetSettings(r.Context(), scopeOf(r))
	if err != nil {
		response.InternalErr(w, r, err, "could not read the helper settings")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// putSettings: PUT /v1/tenant/desktop-helper-settings.
func (h *Handler) putSettings(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	var in Settings
	if !decode(w, r, &in) {
		return
	}
	st, err := h.svc.PutSettings(r.Context(), scopeOf(r), in)
	if err != nil {
		response.InternalErr(w, r, err, "could not save the helper settings")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

var listStatuses = map[string]bool{
	StatusQueued: true, StatusClaimed: true, StatusWaitingUser: true, StatusRunning: true, StatusDone: true,
	StatusPartial: true, StatusFailed: true, StatusNeedsCheck: true, StatusCancelled: true,
}

func paging(limit, offset string) (int32, int32) {
	l, err := strconv.Atoi(limit)
	if err != nil || l <= 0 {
		l = 50
	}
	if l > 200 {
		l = 200
	}
	o, err := strconv.Atoi(offset)
	if err != nil || o < 0 {
		o = 0
	}
	if o > 1_000_000 {
		o = 1_000_000
	}
	return int32(l), int32(o)
}

// list: GET /v1/tenant/agent-tasks.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	q := r.URL.Query()
	f := Filter{Status: strings.TrimSpace(q.Get("status")), Kind: strings.TrimSpace(q.Get("kind"))}
	if f.Status != "" && !listStatuses[f.Status] {
		response.BadRequest(w, r, "INVALID_STATUS", "unknown task status")
		return
	}
	if f.Kind != "" && f.Kind != KindCariSync && f.Kind != KindBulkExport {
		response.BadRequest(w, r, "INVALID_KIND", "unknown task kind")
		return
	}
	f.OpenOnly = q.Get("open") == "true" || q.Get("open") == "1"
	for name, dst := range map[string]**uuid.UUID{"run_uuid": &f.RunUUID, "engagement_uuid": &f.EngagementUUID} {
		if v := strings.TrimSpace(q.Get(name)); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				response.BadRequest(w, r, response.CodeValidationError, fmt.Sprintf("%s must be a UUID", name))
				return
			}
			*dst = &id
		}
	}
	f.Limit, f.Offset = paging(q.Get("limit"), q.Get("offset"))
	page, err := h.svc.List(r.Context(), scopeOf(r), f)
	if err != nil {
		response.InternalErr(w, r, err, "could not list tasks")
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

// startExport: POST /v1/tenant/agent-tasks/luca-export.
func (h *Handler) startExport(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	var in ExportInput
	if !decode(w, r, &in) {
		return
	}
	run, err := h.svc.StartExport(r.Context(), scopeOf(r), in)
	if errors.Is(err, ErrState) {
		response.Conflict(w, r, "EXPORT_OPEN", "the chosen taxpayers already have an open export for this period")
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	for _, t := range run.Items {
		h.record(r, ActionExportStarted, t, nil)
	}
	response.JSON(w, r, http.StatusCreated, run)
}

// get: GET /v1/tenant/agent-tasks/{agentTaskUuid}.
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	id, ok := pathUUID(w, r, "agentTaskUuid")
	if !ok {
		return
	}
	t, err := h.svc.Get(r.Context(), scopeOf(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

func (h *Handler) approve(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	id, ok := pathUUID(w, r, "agentTaskUuid")
	if !ok {
		return
	}
	t, err := h.svc.Approve(r.Context(), scopeOf(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.record(r, ActionApproved, t, map[string]any{"counterparties": len(t.Counterparties)})
	response.JSON(w, r, http.StatusOK, t)
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	id, ok := pathUUID(w, r, "agentTaskUuid")
	if !ok {
		return
	}
	t, err := h.svc.Cancel(r.Context(), scopeOf(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.record(r, ActionCancelled, t, nil)
	response.JSON(w, r, http.StatusOK, t)
}

func (h *Handler) resolve(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	id, ok := pathUUID(w, r, "agentTaskUuid")
	if !ok {
		return
	}
	var in ResolveInput
	if !decode(w, r, &in) {
		return
	}
	t, err := h.svc.Resolve(r.Context(), scopeOf(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.record(r, ActionResolved, t, map[string]any{"resolution": derefString(t.Resolution)})
	response.JSON(w, r, http.StatusOK, t)
}

func (h *Handler) claim(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	id, ok := pathUUID(w, r, "agentTaskUuid")
	if !ok {
		return
	}
	var in struct {
		AgentUUID string `json:"agent_uuid"`
	}
	if !decode(w, r, &in) {
		return
	}
	agent, err := uuid.Parse(in.AgentUUID)
	if err != nil {
		response.BadRequest(w, r, "AGENT_REQUIRED", "agent_uuid is required")
		return
	}
	t, err := h.svc.Claim(r.Context(), scopeOf(r), id, agent)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

func (h *Handler) progress(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	id, ok := pathUUID(w, r, "agentTaskUuid")
	if !ok {
		return
	}
	var in ProgressInput
	if !decode(w, r, &in) {
		return
	}
	t, err := h.svc.Progress(r.Context(), scopeOf(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

func (h *Handler) result(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	id, ok := pathUUID(w, r, "agentTaskUuid")
	if !ok {
		return
	}
	var in ResultInput
	if !decode(w, r, &in) {
		return
	}
	t, err := h.svc.Finish(r.Context(), scopeOf(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ok2, failed := 0, 0
	for _, f := range t.Files {
		switch f.Status {
		case FileOK:
			ok2++
		case FileFailed:
			failed++
		}
	}
	h.record(r, ActionFinished, t, map[string]any{
		"files": len(t.Files), "files_ok": ok2, "files_failed": failed,
		"counterparties": len(t.Counterparties), "error_code": t.ErrorCode,
	})
	response.JSON(w, r, http.StatusOK, t)
}

// fileContent: GET .../files/{agentFileUuid}/content — the file's bytes with
// X-Content-SHA256 (the agent checks it against files[].sha256).
func (h *Handler) fileContent(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	id, ok := pathUUID(w, r, "agentTaskUuid")
	if !ok {
		return
	}
	fid, ok := pathUUID(w, r, "agentFileUuid")
	if !ok {
		return
	}
	f, err := h.svc.File(r.Context(), scopeOf(r), id, fid)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if h.files == nil {
		response.Error(w, r, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "file storage is not available")
		return
	}
	body, _, err := h.files.Download(r.Context(), f.StorageKey)
	if err != nil {
		response.InternalErr(w, r, err, "could not read the file")
		return
	}
	defer func() { _ = body.Close() }()
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(f.Name, `"`, "")))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-SHA256", f.SHA256)
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

func (h *Handler) fileResult(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	id, ok := pathUUID(w, r, "agentTaskUuid")
	if !ok {
		return
	}
	fid, ok := pathUUID(w, r, "agentFileUuid")
	if !ok {
		return
	}
	var in FileResultInput
	if !decode(w, r, &in) {
		return
	}
	t, err := h.svc.ReportFile(r.Context(), scopeOf(r), id, fid, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
