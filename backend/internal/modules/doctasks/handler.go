package doctasks

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

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/apiquery"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

const maxBodyBytes = 64 << 10

// Handler serves the office and mükellef document task routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ---------------------------------------------------------------------------
// Office: /v1/tenant/engagements/{engagementUuid}/document-tasks
// ---------------------------------------------------------------------------

type detectBody struct {
	Period string `json:"period"`
}

// Detect: POST .../document-tasks/detect.
func (h *Handler) Detect(w http.ResponseWriter, r *http.Request) {
	var body detectBody
	if !decode(w, r, &body) {
		return
	}
	rep, err := h.svc.DetectEngagement(r.Context(), engagementctx.Must(r.Context()), strings.TrimSpace(body.Period))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rep)
}

type generateBody struct {
	Period       string      `json:"period"`
	WarningUUIDs []uuid.UUID `json:"warning_uuids"`
	Detect       *bool       `json:"detect"`
	DueDays      int         `json:"due_days"`
	Note         string      `json:"note"`
}

// Generate: POST .../document-tasks/generate.
func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	var body generateBody
	if !decode(w, r, &body) {
		return
	}
	in := GenerateInput{
		Period: strings.TrimSpace(body.Period), WarningUUIDs: body.WarningUUIDs,
		DueDays: body.DueDays, Note: body.Note,
	}
	// Detection runs by default when a period is given.
	in.Detect = in.Period != ""
	if body.Detect != nil {
		in.Detect = *body.Detect
	}
	if len(in.WarningUUIDs) > 1000 {
		response.BadRequest(w, r, response.CodeValidationError, "warning_uuids accepts at most 1000 items")
		return
	}
	res, err := h.svc.Generate(r.Context(), engagementctx.Must(r.Context()), userID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

// Missing: GET .../document-tasks/missing?period=YYYY-MM (#780).
func (h *Handler) Missing(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Missing(r.Context(), engagementctx.Must(r.Context()), r.URL.Query().Get("period"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// RequestMissing: POST .../document-tasks/missing (#780).
func (h *Handler) RequestMissing(w http.ResponseWriter, r *http.Request) {
	var body MissingRequestInput
	if !decodeOptional(w, r, &body) {
		return
	}
	res, err := h.svc.RequestMissing(r.Context(), engagementctx.Must(r.Context()), userID(r), body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

// List: GET .../document-tasks.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	f, ok := filter(w, r)
	if !ok {
		return
	}
	f.Period = r.URL.Query().Get("period")
	if f.Period != "" && !ValidPeriod(f.Period) {
		writeError(w, r, ErrBadPeriod)
		return
	}
	items, total, err := h.svc.List(r.Context(), engagementctx.Must(r.Context()), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// Get: GET .../document-tasks/{taskUuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "taskUuid")
	if !ok {
		return
	}
	t, err := h.svc.Get(r.Context(), engagementctx.Must(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

type cancelBody struct {
	Reason string `json:"reason"`
}

// Cancel: POST .../document-tasks/{taskUuid}/cancel.
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "taskUuid")
	if !ok {
		return
	}
	var body cancelBody
	if !decodeOptional(w, r, &body) {
		return
	}
	t, err := h.svc.Cancel(r.Context(), engagementctx.Must(r.Context()), id, userID(r), body.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

// CreateRequest: POST .../document-tasks (issue #240).
func (h *Handler) CreateRequest(w http.ResponseWriter, r *http.Request) {
	var body RequestInput
	if !decode(w, r, &body) {
		return
	}
	t, err := h.svc.CreateRequest(r.Context(), engagementctx.Must(r.Context()), userID(r), body)
	if errors.Is(err, ErrRequestExists) {
		// Açık talep zaten var: yenisi açılmadı, var olan döner.
		response.JSON(w, r, http.StatusOK, t)
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, t)
}

// Send: POST .../document-tasks/{taskUuid}/send. Sends the open task now on
// the chosen channels (push, sms, email): a reminder by default, or the
// request again with "reminder": false.
func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "taskUuid")
	if !ok {
		return
	}
	var body SendInput
	if !decodeOptional(w, r, &body) {
		return
	}
	t, err := h.svc.Send(r.Context(), engagementctx.Must(r.Context()), userID(r), id, body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

// Complete: POST .../document-tasks/{taskUuid}/complete.
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "taskUuid")
	if !ok {
		return
	}
	t, err := h.svc.Complete(r.Context(), engagementctx.Must(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

// Rerequest: POST .../document-tasks/{taskUuid}/rerequest.
func (h *Handler) Rerequest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "taskUuid")
	if !ok {
		return
	}
	var body RerequestInput
	if !decode(w, r, &body) {
		return
	}
	t, err := h.svc.Rerequest(r.Context(), engagementctx.Must(r.Context()), id, body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

// OfficeFile: GET .../document-tasks/{taskUuid}/uploads/{uploadUuid}/file.
func (h *Handler) OfficeFile(w http.ResponseWriter, r *http.Request) {
	taskID, ok := pathUUID(w, r, "taskUuid")
	if !ok {
		return
	}
	uploadID, ok := pathUUID(w, r, "uploadUuid")
	if !ok {
		return
	}
	f, err := h.svc.OfficeFile(r.Context(), engagementctx.Must(r.Context()), taskID, uploadID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeFile(w, f)
}

// ---------------------------------------------------------------------------
// Mükellef: /v1/taxpayer/document-tasks (X-Taxpayer)
// ---------------------------------------------------------------------------

// ListMine: GET /v1/taxpayer/document-tasks. Default status is open.
func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	f, ok := filter(w, r)
	if !ok {
		return
	}
	// The panel's period selector (#333): the task's own period.
	f.Period = r.URL.Query().Get("period")
	if f.Period != "" && !ValidPeriod(f.Period) {
		writeError(w, r, ErrBadPeriod)
		return
	}
	items, total, err := h.svc.ListMine(r.Context(), taxpayerctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// GetMine: GET /v1/taxpayer/document-tasks/{taskUuid}.
func (h *Handler) GetMine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "taskUuid")
	if !ok {
		return
	}
	t, err := h.svc.GetMine(r.Context(), taxpayerctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

// UploadMine: POST /v1/taxpayer/document-tasks/{taskUuid}/uploads
// (multipart field "file").
func (h *Handler) UploadMine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "taskUuid")
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes+(1<<20))
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
	data, err := io.ReadAll(io.LimitReader(file, MaxUploadBytes+1))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "FILE_UNREADABLE", "could not read the file")
		return
	}
	res, err := h.svc.Upload(r.Context(), taxpayerctx.Must(r.Context()).ID, id, userID(r), header.Filename, data)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, res)
}

// MyFile: GET /v1/taxpayer/document-tasks/{taskUuid}/uploads/{uploadUuid}/file.
func (h *Handler) MyFile(w http.ResponseWriter, r *http.Request) {
	taskID, ok := pathUUID(w, r, "taskUuid")
	if !ok {
		return
	}
	uploadID, ok := pathUUID(w, r, "uploadUuid")
	if !ok {
		return
	}
	f, err := h.svc.MyFile(r.Context(), taxpayerctx.Must(r.Context()).ID, taskID, uploadID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeFile(w, f)
}

// ---------------------------------------------------------------------------

func filter(w http.ResponseWriter, r *http.Request) (Filter, bool) {
	q := r.URL.Query()
	f := Filter{Limit: apiquery.DefaultLimit}
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= int(apiquery.MaxLimit) {
		f.Limit = int32(v) //nolint:gosec // bounded above
	}
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v >= 0 && v <= 1_000_000 {
		f.Offset = int32(v) //nolint:gosec // bounded above
	}
	switch st := q.Get("status"); st {
	case "":
		f.Status = StatusOpen
	case "all":
	case StatusOpen, StatusSubmitted, StatusMatched, StatusCancelled:
		f.Status = st
	default:
		response.BadRequest(w, r, response.CodeValidationError, "status must be open, submitted, matched, cancelled or all")
		return f, false
	}
	return f, true
}

func writeFile(w http.ResponseWriter, f File) {
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
	// Always a download: uploaded content is never rendered inline.
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safe, url.PathEscape(f.Name)))
	_, _ = io.Copy(w, f.Body)
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
		response.NotFound(w, r, "Document task not found")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

// decodeOptional accepts an empty body.
func decodeOptional(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "Document task not found")
	case errors.Is(err, ErrClosed):
		response.Conflict(w, r, "TASK_CLOSED", "The task is no longer open")
	case errors.Is(err, ErrChannelsDisabled):
		response.Conflict(w, r, "DOC_REQUEST_CHANNELS_DISABLED", "Document request channels are turned off in office settings")
	case errors.Is(err, ErrRecentlySent):
		response.Conflict(w, r, "RECENTLY_REMINDED", "A reminder was sent moments ago; try again in a few minutes")
	case errors.Is(err, ErrNotSubmitted):
		response.Conflict(w, r, "TASK_NOT_SUBMITTED", "Only a submitted task can be requested again")
	case errors.Is(err, ErrBadPeriod):
		response.BadRequest(w, r, response.CodeValidationError, "period must be YYYY-MM")
	case errors.Is(err, ErrBadInput):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrEmpty):
		response.Error(w, r, http.StatusBadRequest, "FILE_EMPTY", "the file is empty")
	case errors.Is(err, ErrTooLarge):
		response.Error(w, r, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", "file is too large (max 20 MB)")
	case errors.Is(err, ErrUnsupported):
		response.Error(w, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_FILE", "upload a photo (JPEG, PNG, WebP, HEIC) or a PDF")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
