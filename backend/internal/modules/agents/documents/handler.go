package documents

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the agent document routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func actor(r *http.Request) Actor {
	org := orgctx.Must(r.Context())
	a := Actor{OrganizationID: org.ID, Has: org.Has}
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		a.UserID = p.UserInternal
	}
	return a
}

// Upload: POST /v1/tenant/agents/{agentUuid}/documents (multipart "file",
// "folder", optional "engagement_uuid").
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(r.PathValue("agentUuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "agentUuid is invalid")
		return
	}
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
	in := UploadInput{AgentUUID: agentID, Folder: r.FormValue("folder"), FileName: header.Filename, Data: data}
	if v := strings.TrimSpace(r.FormValue("engagement_uuid")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "engagement_uuid is invalid")
			return
		}
		in.EngagementUUID = &id
	}
	out, err := h.svc.Upload(r.Context(), actor(r), in)
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

// List: GET /v1/tenant/agent-documents?status=open|all|...&limit=&offset=.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := int32(50), int32(0)
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = int32(v) //nolint:gosec // bounded above
	}
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v >= 0 && v <= 1_000_000 {
		offset = int32(v) //nolint:gosec // bounded above
	}
	out, err := h.svc.List(r.Context(), orgctx.Must(r.Context()).ID, q.Get("status"), limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Resolve: POST /v1/tenant/agent-documents/{agentDocumentUuid}/resolve.
func (h *Handler) Resolve(w http.ResponseWriter, r *http.Request) {
	id, ok := docID(w, r)
	if !ok {
		return
	}
	var in ResolveInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	out, err := h.svc.Resolve(r.Context(), actor(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Dismiss: POST /v1/tenant/agent-documents/{agentDocumentUuid}/dismiss.
func (h *Handler) Dismiss(w http.ResponseWriter, r *http.Request) {
	id, ok := docID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Dismiss(r.Context(), actor(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func docID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("agentDocumentUuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "agentDocumentUuid is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "Agent document not found")
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrConflict):
		response.Conflict(w, r, response.CodeConflict, "The document is already imported")
	case errors.Is(err, ErrStorageMissing):
		response.Error(w, r, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "file storage is not configured")
	default:
		response.InternalErr(w, r, err, "Agent document request failed")
	}
}
