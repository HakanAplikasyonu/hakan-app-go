package statement

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Office template routes (issue #383):
//   GET    /v1/tenant/bank-statement-templates
//   POST   /v1/tenant/bank-statement-templates
//   DELETE /v1/tenant/bank-statement-templates/{templateUuid}
//   POST   /v1/tenant/bank-statement-templates/analyze (multipart "file")

func (h *Handler) listTemplates(w http.ResponseWriter, r *http.Request) {
	items, err := h.im.Templates(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		response.InternalErr(w, r, err, "could not list templates")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items, "fields": Fields})
}

func (h *Handler) saveTemplate(w http.ResponseWriter, r *http.Request) {
	var in TemplateInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, "VALIDATION_ERROR", "Invalid request body")
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	v, err := h.im.SaveTemplate(r.Context(), orgctx.Must(r.Context()).ID, p.UserInternal, in)
	if errors.Is(err, ErrInvalidTemplate) {
		response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_TEMPLATE", err.Error())
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not save template")
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

func (h *Handler) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("templateUuid"))
	if err != nil {
		response.NotFound(w, r, "template not found")
		return
	}
	err = h.im.DeleteTemplate(r.Context(), orgctx.Must(r.Context()).ID, id)
	if errors.Is(err, ErrTemplateNotFound) {
		response.NotFound(w, r, "template not found")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not delete template")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) analyze(w http.ResponseWriter, r *http.Request) {
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
	if err != nil || len(data) == 0 || len(data) > MaxFileBytes {
		response.Error(w, r, http.StatusBadRequest, "FILE_UNREADABLE", "could not read the file")
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	a, err := h.im.Analyze(r.Context(), orgctx.Must(r.Context()).ID, p.UserInternal, header.Filename, data)
	switch {
	case errors.Is(err, ErrPDFUnreadable),
		errors.Is(err, ErrLayoutUnknown) && Detect(header.Filename, data) == "pdf":
		// #780: a PDF without a readable movement table cannot be mapped
		// by hand either; the office downloads Excel or CSV instead.
		response.Error(w, r, http.StatusUnprocessableEntity, "PDF_UNREADABLE", "the PDF cannot be read; download the statement as Excel or CSV")
	case errors.Is(err, ErrNoTemplateNeeded):
		response.Error(w, r, http.StatusUnprocessableEntity, "TEMPLATE_NOT_NEEDED", "MT940 files are read without a template")
	case errors.Is(err, ErrLayoutUnknown):
		response.Error(w, r, http.StatusUnprocessableEntity, "STATEMENT_LAYOUT_UNKNOWN", "no header row with dates below it was found")
	case errors.Is(err, ErrLegacyXLS):
		response.Error(w, r, http.StatusUnsupportedMediaType, "STATEMENT_XLS_LEGACY", "binary Excel 97-2003 file; save it as .xlsx or download CSV")
	case errors.Is(err, ErrUnsupported):
		response.Error(w, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_FILE", "upload .xlsx, .xls, CSV or a text PDF statement")
	case errors.Is(err, ErrScannedPDF):
		response.Error(w, r, http.StatusUnprocessableEntity, "SCANNED_PDF", "the PDF has no text layer; scanned statements are not supported yet")
	case err != nil:
		response.Error(w, r, http.StatusUnprocessableEntity, "STATEMENT_UNREADABLE", err.Error())
	default:
		response.JSON(w, r, http.StatusOK, a)
	}
}
