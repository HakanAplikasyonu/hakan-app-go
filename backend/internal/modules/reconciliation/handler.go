package reconciliation

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the reconciliation routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the office routes behind the active-engagement chain
// (/v1/tenant/engagements/{engagementUuid}/reconciliation/...). A nil
// resolver registers nothing.
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
		mux.Handle(method+" /v1/tenant/engagements/{"+engagementctx.PathParam+"}/reconciliation"+path,
			middleware.Chain(fn, authn, org, middleware.RequireTenantPermission(perm), eng))
	}
	read, write := rbac.PermTenantReconciliationRead, rbac.PermTenantReconciliationWrite
	route("GET", "/runs", read, h.listRuns)
	route("POST", "/runs", write, h.run)
	route("GET", "/runs/{runRef}", read, h.getRun)
	route("PATCH", "/items/{itemUuid}", write, h.decide)
	route("POST", "/items/{itemUuid}/review", write, h.review)
	route("GET", "/runs/{runRef}/counterparties", read, h.listCounterparties)
	route("PATCH", "/counterparties/{counterpartyUuid}", write, h.decideCounterparty)
	route("POST", "/counterparties/{counterpartyUuid}/review", write, h.reviewCounterparty)
	route("GET", "/babs", read, h.listDeclarations)
	route("POST", "/babs", write, h.saveDeclaration)
	route("POST", "/babs/imports", write, h.importDeclarations)
	route("DELETE", "/babs/{declarationUuid}", write, h.deleteDeclaration)
	route("GET", "/letters", read, h.listLetters)
	route("POST", "/letters", write, h.createLetter)
	route("GET", "/letters/{letterUuid}/pdf", read, h.letterPDF)
	route("POST", "/letters/{letterUuid}/send", write, h.sendLetter)
	route("POST", "/letters/{letterUuid}/response", write, h.recordResponse)
}

// RegisterPublicRoutes mounts the counterparty's response link (no sign in;
// the token, sent in the body so it stays out of URLs and logs, opens one
// letter).
func RegisterPublicRoutes(mux *http.ServeMux, h *Handler) {
	if h == nil {
		return
	}
	mux.HandleFunc("POST /v1/public/reconciliation-letters/preview", h.publicLetter)
	mux.HandleFunc("POST /v1/public/reconciliation-letters/respond", h.publicRespond)
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

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "request body is not valid JSON for this endpoint")
		return false
	}
	return true
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "not found")
	case errors.Is(err, ErrInvalid), errors.Is(err, reviewusecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrNoTrialBalance):
		response.Error(w, r, http.StatusUnprocessableEntity, "RECONCILIATION_NO_TRIAL_BALANCE",
			"no trial balance with amounts is uploaded for the period")
	case errors.Is(err, ErrMatched):
		response.Conflict(w, r, "RECONCILIATION_NO_DIFFERENCE", err.Error())
	case errors.Is(err, ErrInReview):
		response.Conflict(w, r, "RECONCILIATION_IN_REVIEW", err.Error())
	case errors.Is(err, ErrNoReview):
		response.ServiceUnavailable(w, r, "RECONCILIATION_UNAVAILABLE", err.Error())
	case errors.Is(err, ErrBadRecipient), errors.Is(err, ErrNoRecipient):
		response.BadRequest(w, r, "RECONCILIATION_BAD_RECIPIENT", err.Error())
	case errors.Is(err, ErrLetterAnswered):
		response.Conflict(w, r, "RECONCILIATION_LETTER_ANSWERED", err.Error())
	case errors.Is(err, ErrLetterExpired):
		response.Error(w, r, http.StatusGone, "RECONCILIATION_LETTER_EXPIRED", err.Error())
	case errors.Is(err, ErrLettersDisabled):
		response.ServiceUnavailable(w, r, "RECONCILIATION_UNAVAILABLE", err.Error())
	case errors.Is(err, ErrNoBABSHeader):
		response.Error(w, r, http.StatusUnprocessableEntity, "BABS_HEADER_NOT_FOUND", err.Error())
	case errors.Is(err, parse.ErrUnsupported), errors.Is(err, parse.ErrEmpty), errors.Is(err, parse.ErrTooLarge), errors.Is(err, parse.ErrTooManyRows):
		response.BadRequest(w, r, "FILE_UNREADABLE", err.Error())
	default:
		response.InternalErr(w, r, err, "reconciliation request failed")
	}
}

func (h *Handler) listRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.svc.Runs(r.Context(), engagementctx.Must(r.Context()), strings.TrimSpace(r.URL.Query().Get("period")))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": runs})
}

func (h *Handler) run(w http.ResponseWriter, r *http.Request) {
	var in RunInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Run(r.Context(), engagementctx.Must(r.Context()), userID(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) getRun(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Detail(r.Context(), engagementctx.Must(r.Context()), r.PathValue("runRef"), strings.TrimSpace(r.URL.Query().Get("period")))
	if errors.Is(err, ErrNoRun) {
		// Henüz karşılaştırma yok: boş durum, hata değil (#639).
		response.JSON(w, r, http.StatusOK, map[string]any{"run": nil, "items": []ItemView{}})
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) decide(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "itemUuid")
	if !ok {
		return
	}
	var in DecisionInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Decide(r.Context(), engagementctx.Must(r.Context()), id, userID(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type reviewInput struct {
	Note *string `json:"note"`
}

func (h *Handler) review(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "itemUuid")
	if !ok {
		return
	}
	var in reviewInput
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	out, err := h.svc.SendToReview(r.Context(), engagementctx.Must(r.Context()), id, userID(r), in.Note)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
