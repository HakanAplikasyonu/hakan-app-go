package reconciliation

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

func (h *Handler) listCounterparties(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Counterparties(r.Context(), engagementctx.Must(r.Context()), r.PathValue("runRef"), strings.TrimSpace(r.URL.Query().Get("period")))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) decideCounterparty(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "counterpartyUuid")
	if !ok {
		return
	}
	var in DecisionInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.DecideCounterparty(r.Context(), engagementctx.Must(r.Context()), id, userID(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) reviewCounterparty(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "counterpartyUuid")
	if !ok {
		return
	}
	var in reviewInput
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	out, err := h.svc.SendCounterpartyToReview(r.Context(), engagementctx.Must(r.Context()), id, userID(r), in.Note)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) listDeclarations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out, err := h.svc.Declarations(r.Context(), engagementctx.Must(r.Context()), strings.TrimSpace(q.Get("period")), strings.TrimSpace(q.Get("form")))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) saveDeclaration(w http.ResponseWriter, r *http.Request) {
	var in DeclarationInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.SaveDeclaration(r.Context(), engagementctx.Must(r.Context()), userID(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// maxUpload caps a BA / BS listing.
const maxUpload = 10 << 20

func (h *Handler) importDeclarations(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+1<<20)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "multipart form with a file field is required")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "file field is required")
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxUpload+1))
	if err != nil || len(data) > maxUpload {
		response.BadRequest(w, r, "FILE_UNREADABLE", fmt.Sprintf("the file must be under %d MB", maxUpload>>20))
		return
	}
	out, err := h.svc.ImportDeclarations(r.Context(), engagementctx.Must(r.Context()), userID(r),
		strings.TrimSpace(r.FormValue("period")), strings.TrimSpace(r.FormValue("form")), hdr.Filename, data)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) deleteDeclaration(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "declarationUuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteDeclaration(r.Context(), engagementctx.Must(r.Context()), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listLetters(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Letters(r.Context(), engagementctx.Must(r.Context()), strings.TrimSpace(r.URL.Query().Get("counterparty_tax_id")))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) createLetter(w http.ResponseWriter, r *http.Request) {
	var in LetterInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.CreateLetter(r.Context(), engagementctx.Must(r.Context()), userID(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) letterPDF(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "letterUuid")
	if !ok {
		return
	}
	body, name, err := h.svc.LetterPDF(r.Context(), engagementctx.Must(r.Context()), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+name+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write(body)
}

type sendInput struct {
	Email *string `json:"email"`
}

func (h *Handler) sendLetter(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "letterUuid")
	if !ok {
		return
	}
	var in sendInput
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	out, err := h.svc.SendLetter(r.Context(), engagementctx.Must(r.Context()), id, in.Email)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) recordResponse(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "letterUuid")
	if !ok {
		return
	}
	var in ResponseInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.RecordResponse(r.Context(), engagementctx.Must(r.Context()), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type tokenInput struct {
	Token string `json:"token"`
}

func (h *Handler) publicLetter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var in tokenInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.PublicLetterByToken(r.Context(), in.Token)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) publicRespond(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var in struct {
		Token string `json:"token"`
		ResponseInput
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.PublicRespond(r.Context(), in.Token, in.ResponseInput)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
