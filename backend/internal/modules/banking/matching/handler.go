package matching

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/posting"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

var rePeriod = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// Handler serves POST /v1/tenant/engagements/{engagementUuid}/bank-matching.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the route behind the active-engagement chain.
func RegisterRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver, engResolver engagementctx.Resolver, h *Handler,
) {
	if orgResolver == nil || engResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(orgResolver)
	eng := middleware.RequireEngagement(engResolver)
	write := middleware.RequireTenantPermission(rbac.PermTenantBankWrite)
	read := middleware.RequireTenantPermission(rbac.PermTenantBankRead)
	path := "POST /v1/tenant/engagements/{" + engagementctx.PathParam + "}/bank-matching"
	mux.Handle(path, middleware.Chain(http.HandlerFunc(h.run), authn, org, write, eng))
	// One movement (#528): its matches and candidates; match by hand, undo.
	tx := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/bank-transactions/{transactionUuid}"
	mux.Handle("GET "+tx+"/matching", middleware.Chain(http.HandlerFunc(h.transaction), authn, org, read, eng))
	mux.Handle("GET "+tx+"/invoice-search", middleware.Chain(http.HandlerFunc(h.search), authn, org, read, eng))
	mux.Handle("PUT "+tx+"/matches", middleware.Chain(http.HandlerFunc(h.match), authn, org, write, eng))
	mux.Handle("DELETE "+tx+"/matches", middleware.Chain(http.HandlerFunc(h.unmatch), authn, org, write, eng))
	// Book the movement to an account from the same screen (#565).
	mux.Handle("PUT "+tx+"/account", middleware.Chain(http.HandlerFunc(h.setAccount), authn, org, write, eng))
}

func subjectOf(r *http.Request) Subject {
	scope := engagementctx.Must(r.Context())
	return Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID}
}

func (h *Handler) transaction(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionUuid"))
	if err != nil {
		response.NotFound(w, r, "bank transaction not found")
		return
	}
	v, err := h.svc.Transaction(r.Context(), subjectOf(r), id)
	h.reply(w, r, v, err)
}

// search: GET .../invoice-search?q= (#565).
func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionUuid"))
	if err != nil {
		response.NotFound(w, r, "bank transaction not found")
		return
	}
	items, err := h.svc.Search(r.Context(), subjectOf(r), id, r.URL.Query().Get("q"))
	h.reply(w, r, map[string]any{"items": items}, err)
}

type matchBody struct {
	InvoiceUUIDs []uuid.UUID `json:"invoice_uuids"`
}

func (h *Handler) match(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionUuid"))
	if err != nil {
		response.NotFound(w, r, "bank transaction not found")
		return
	}
	var in matchBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || len(in.InvoiceUUIDs) == 0 || len(in.InvoiceUUIDs) > MaxCombination*2 {
		response.BadRequest(w, r, "VALIDATION_ERROR", "invoice_uuids must name 1-10 invoices")
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	v, err := h.svc.Match(r.Context(), subjectOf(r), id, in.InvoiceUUIDs, p.UserInternal)
	h.reply(w, r, v, err)
}

func (h *Handler) unmatch(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionUuid"))
	if err != nil {
		response.NotFound(w, r, "bank transaction not found")
		return
	}
	v, err := h.svc.Unmatch(r.Context(), subjectOf(r), id)
	h.reply(w, r, v, err)
}

type accountBody struct {
	AccountCode string `json:"account_code"`
	AccountName string `json:"account_name"`
}

// setAccount: PUT .../account (#565).
func (h *Handler) setAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionUuid"))
	if err != nil {
		response.NotFound(w, r, "bank transaction not found")
		return
	}
	var in accountBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || in.AccountCode == "" {
		response.BadRequest(w, r, "VALIDATION_ERROR", "account_code is required")
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	v, err := h.svc.SetAccount(r.Context(), subjectOf(r), id, p.UserInternal, in.AccountCode, in.AccountName)
	h.reply(w, r, v, err)
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request, v any, err error) {
	switch {
	case err == nil:
		response.JSON(w, r, http.StatusOK, v)
	case errors.Is(err, ErrTransactionNotFound):
		response.NotFound(w, r, "bank transaction not found")
	case errors.Is(err, ErrNotMatchable):
		response.Conflict(w, r, "BANK_TRANSACTION_NOT_MATCHABLE", "this movement is neither a collection nor a payment, or it is cancelled")
	case errors.Is(err, ErrInvalidInvoices), errors.Is(err, posting.ErrInvalid), errors.Is(err, reviewusecase.ErrInvalid):
		response.Error(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, posting.ErrNotFound):
		response.NotFound(w, r, "bank transaction not found")
	case errors.Is(err, posting.ErrDrafted):
		response.Conflict(w, r, "BANK_POSTING_DRAFTED", "the movement is already in a journal draft; reverse the draft first")
	case errors.Is(err, reviewusecase.ErrState), errors.Is(err, reviewusecase.ErrNotFound):
		response.Conflict(w, r, "SUGGESTION_STATE", "the suggestion was decided meanwhile; reload")
	case errors.Is(err, r.Context().Err()):
	default:
		response.InternalErr(w, r, err, "bank matching failed")
	}
}

// run: ?period=YYYY-MM. Matches the period's collections and payments to
// invoices (auto ≥ 80, the rest to the review queue) and refreshes the
// BNK_* period warnings.
func (h *Handler) run(w http.ResponseWriter, r *http.Request) {
	scope := engagementctx.Must(r.Context())
	period := r.URL.Query().Get("period")
	if !rePeriod.MatchString(period) {
		response.BadRequest(w, r, "INVALID_PERIOD", "period must be YYYY-MM")
		return
	}
	res, err := h.svc.Run(r.Context(), Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID}, period)
	if err != nil {
		if errors.Is(err, r.Context().Err()) {
			return
		}
		response.InternalErr(w, r, err, "bank matching failed")
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}
