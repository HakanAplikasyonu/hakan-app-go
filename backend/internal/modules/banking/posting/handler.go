package posting

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strconv"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the alias memory (and, from the posting service, the
// movement → account workflow).
type Handler struct {
	aliases *Service
	flow    *Workflow
}

// NewHandler builds the handler. flow may be nil: only the alias routes.
func NewHandler(aliases *Service, flow *Workflow) *Handler {
	return &Handler{aliases: aliases, flow: flow}
}

// RegisterRoutes mounts the routes behind the active-engagement chain.
func RegisterRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver, engResolver engagementctx.Resolver, h *Handler,
) {
	if orgResolver == nil || engResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(orgResolver)
	eng := middleware.RequireEngagement(engResolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantBankRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantBankWrite)
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/bank-counterparty-aliases"
	mux.Handle("GET "+base, middleware.Chain(http.HandlerFunc(h.listAliases), authn, org, read, eng))
	mux.Handle("POST "+base, middleware.Chain(http.HandlerFunc(h.createAlias), authn, org, write, eng))
	mux.Handle("GET "+base+"/candidates", middleware.Chain(http.HandlerFunc(h.candidates), authn, org, read, eng))
	mux.Handle("PATCH "+base+"/{aliasUuid}", middleware.Chain(http.HandlerFunc(h.updateAlias), authn, org, write, eng))
	mux.Handle("DELETE "+base+"/{aliasUuid}", middleware.Chain(http.HandlerFunc(h.deleteAlias), authn, org, write, eng))
	mux.Handle("POST "+base+"/{aliasUuid}/promote", middleware.Chain(http.HandlerFunc(h.promote), authn, org, write, eng))
	if h.flow == nil {
		return
	}
	journals := middleware.RequireTenantPermission(rbac.PermTenantJournalsWrite)
	p := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/bank-postings"
	mux.Handle("GET "+p, middleware.Chain(http.HandlerFunc(h.listPostings), authn, org, read, eng))
	mux.Handle("POST "+p+"/suggest", middleware.Chain(http.HandlerFunc(h.suggest), authn, org, write, eng))
	mux.Handle("POST "+p+"/approve", middleware.Chain(http.HandlerFunc(h.approve), authn, org, write, eng))
	mux.Handle("POST "+p+"/drafts", middleware.Chain(http.HandlerFunc(h.drafts), authn, org, write, journals, eng))
	mux.Handle("PUT "+p+"/{transactionUuid}", middleware.Chain(http.HandlerFunc(h.setAccount), authn, org, write, eng))
	mux.Handle("PUT "+p+"/{transactionUuid}/commission", middleware.Chain(http.HandlerFunc(h.setCommission), authn, org, write, eng))
	l := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/bank-ledger-accounts"
	mux.Handle("GET "+l, middleware.Chain(http.HandlerFunc(h.ledgerAccounts), authn, org, read, eng))
	mux.Handle("PUT "+l+"/{cashBoxUuid}", middleware.Chain(http.HandlerFunc(h.setLedgerAccount), authn, org, write, eng))
}

func subjectOf(r *http.Request) Subject {
	s := engagementctx.Must(r.Context())
	return Subject{OrganizationID: s.OrganizationID, EngagementID: s.ID, TaxpayerID: s.TaxpayerID}
}

func userOf(r *http.Request) int64 {
	p, _ := authctx.PrincipalFrom(r.Context())
	return p.UserInternal
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		response.BadRequest(w, r, "VALIDATION_ERROR", "Invalid request body")
		return false
	}
	return true
}

func writeErr(w http.ResponseWriter, r *http.Request, err error, what string) {
	switch {
	case errors.Is(err, ErrInvalid):
		response.Error(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, what+" not found")
	case errors.Is(err, ErrNoPosting):
		response.Error(w, r, http.StatusConflict, "BANK_POSTING_MISSING", "pick the movement's account first")
	case errors.Is(err, ErrDrafted):
		response.Error(w, r, http.StatusConflict, "BANK_POSTING_DRAFTED", "the movement is already in a journal draft; reverse the draft first")
	case errors.Is(err, reviewusecase.ErrInvalid):
		response.Error(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, reviewusecase.ErrState), errors.Is(err, reviewusecase.ErrNotFound):
		response.Error(w, r, http.StatusConflict, "SUGGESTION_STATE", "the suggestion was decided meanwhile; reload")
	default:
		response.InternalErr(w, r, err, what+" failed")
	}
}

func (h *Handler) listAliases(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "" && status != StatusLearned && status != StatusRule {
		response.BadRequest(w, r, "INVALID_STATUS", "status must be learned or rule")
		return
	}
	limit, offset := int32(AliasPageMax), int32(0)
	q := r.URL.Query()
	if v := q.Get("limit"); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n < 1 || n > AliasPageMax {
			response.BadRequest(w, r, response.CodeValidationError, "limit must be 1..500")
			return
		}
		limit = int32(n)
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n < 0 {
			response.BadRequest(w, r, response.CodeValidationError, "offset must be 0 or more")
			return
		}
		offset = int32(n)
	}
	items, total, err := h.aliases.AliasesPage(r.Context(), subjectOf(r), status, limit, offset)
	if err != nil {
		writeErr(w, r, err, "alias list")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) createAlias(w http.ResponseWriter, r *http.Request) {
	var in AliasInput
	if !decode(w, r, &in) {
		return
	}
	a, err := h.aliases.CreateAlias(r.Context(), subjectOf(r), in, userOf(r))
	if err != nil {
		writeErr(w, r, err, "alias")
		return
	}
	response.JSON(w, r, http.StatusCreated, a)
}

func aliasID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("aliasUuid"))
	if err != nil {
		response.NotFound(w, r, "alias not found")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) updateAlias(w http.ResponseWriter, r *http.Request) {
	id, ok := aliasID(w, r)
	if !ok {
		return
	}
	var in AliasPatch
	if !decode(w, r, &in) {
		return
	}
	a, err := h.aliases.UpdateAlias(r.Context(), subjectOf(r), id, in, userOf(r))
	if err != nil {
		writeErr(w, r, err, "alias")
		return
	}
	response.JSON(w, r, http.StatusOK, a)
}

func (h *Handler) deleteAlias(w http.ResponseWriter, r *http.Request) {
	id, ok := aliasID(w, r)
	if !ok {
		return
	}
	if err := h.aliases.DeleteAlias(r.Context(), subjectOf(r), id); err != nil {
		writeErr(w, r, err, "alias")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) candidates(w http.ResponseWriter, r *http.Request) {
	items, err := h.aliases.Candidates(r.Context(), subjectOf(r))
	if err != nil {
		writeErr(w, r, err, "candidates")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) promote(w http.ResponseWriter, r *http.Request) {
	id, ok := aliasID(w, r)
	if !ok {
		return
	}
	a, err := h.aliases.Promote(r.Context(), subjectOf(r), id, userOf(r))
	if err != nil {
		writeErr(w, r, err, "alias")
		return
	}
	response.JSON(w, r, http.StatusOK, a)
}

var periodRe = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

func (h *Handler) listPostings(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	period, status := qs.Get("period"), qs.Get("status")
	if period != "" && !periodRe.MatchString(period) {
		response.BadRequest(w, r, "INVALID_PERIOD", "period must be YYYY-MM")
		return
	}
	if status != "" && !slices.Contains(Statuses, status) {
		response.BadRequest(w, r, "INVALID_STATUS", "status must be unassigned, suggested, approved or drafted")
		return
	}
	limit, offset := int32(50), int32(0)
	if v, err := strconv.Atoi(qs.Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = int32(v) //nolint:gosec // bounded
	}
	if v, err := strconv.Atoi(qs.Get("offset")); err == nil && v >= 0 && v <= 1_000_000 {
		offset = int32(v) //nolint:gosec // bounded
	}
	page, err := h.flow.List(r.Context(), subjectOf(r), period, status, limit, offset)
	if err != nil {
		writeErr(w, r, err, "bank postings")
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

type periodBody struct {
	Period string `json:"period"`
}

func (h *Handler) suggest(w http.ResponseWriter, r *http.Request) {
	var in periodBody
	if !decode(w, r, &in) {
		return
	}
	if !periodRe.MatchString(in.Period) {
		response.BadRequest(w, r, "INVALID_PERIOD", "period must be YYYY-MM")
		return
	}
	res, err := h.flow.Run(r.Context(), subjectOf(r), in.Period)
	if err != nil {
		writeErr(w, r, err, "bank posting suggestions")
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

func (h *Handler) approve(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TransactionUUIDs []uuid.UUID `json:"transaction_uuids"`
	}
	if !decode(w, r, &in) {
		return
	}
	res, err := h.flow.Approve(r.Context(), subjectOf(r), userOf(r), in.TransactionUUIDs)
	if err != nil {
		writeErr(w, r, err, "approval")
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

func (h *Handler) setAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionUuid"))
	if err != nil {
		response.NotFound(w, r, "bank transaction not found")
		return
	}
	var in accountValue
	if !decode(w, r, &in) {
		return
	}
	if err := h.flow.SetAccount(r.Context(), subjectOf(r), userOf(r), id, in.AccountCode, in.AccountName); err != nil {
		writeErr(w, r, err, "bank transaction")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setCommission: PUT .../bank-postings/{transactionUuid}/commission
// {amount, account_code} nets a POS collection (#565); amount null removes it.
func (h *Handler) setCommission(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionUuid"))
	if err != nil {
		response.NotFound(w, r, "bank transaction not found")
		return
	}
	var in CommissionInput
	if !decode(w, r, &in) {
		return
	}
	if err := h.flow.SetCommission(r.Context(), subjectOf(r), id, in); err != nil {
		writeErr(w, r, err, "bank transaction")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) drafts(w http.ResponseWriter, r *http.Request) {
	var in periodBody
	if !decode(w, r, &in) {
		return
	}
	if in.Period != "" && !periodRe.MatchString(in.Period) {
		response.BadRequest(w, r, "INVALID_PERIOD", "period must be YYYY-MM")
		return
	}
	res, err := h.flow.CreateDrafts(r.Context(), engagementctx.Must(r.Context()), userOf(r), in.Period)
	if err != nil {
		writeErr(w, r, err, "journal drafts")
		return
	}
	response.JSON(w, r, http.StatusCreated, res)
}

func (h *Handler) ledgerAccounts(w http.ResponseWriter, r *http.Request) {
	items, err := h.flow.LedgerAccounts(r.Context(), subjectOf(r))
	if err != nil {
		writeErr(w, r, err, "ledger accounts")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) setLedgerAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("cashBoxUuid"))
	if err != nil {
		response.NotFound(w, r, "cash box not found")
		return
	}
	var in struct {
		AccountCode string `json:"account_code"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := h.flow.SetLedgerAccount(r.Context(), subjectOf(r), userOf(r), id, in.AccountCode); err != nil {
		writeErr(w, r, err, "cash box")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
