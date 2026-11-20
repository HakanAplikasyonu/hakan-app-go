package matching

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/posting"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/activity"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

var rePeriod = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// Handler serves POST /v1/tenant/engagements/{engagementUuid}/bank-matching.
type Handler struct {
	svc      *Service
	activity *activity.Recorder
}

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// WithActivity records who matched, unmatched or booked a movement, when and
// with what (#565 audit trail). Without it nothing is recorded.
func (h *Handler) WithActivity(rec *activity.Recorder) *Handler {
	h.activity = rec
	return h
}

// Audit actions of the single-movement screen.
const (
	ActionMatched = "bank.transaction.matched"
	// ActionRematched: the movement had matches and was matched again in
	// one step (#669 G); the payload names what it replaced.
	ActionRematched = "bank.transaction.rematched"
	ActionUnmatched = "bank.transaction.unmatched"
	ActionBooked    = "bank.transaction.booked"
)

// audit writes the event after a successful change. The payload names the
// engagement, the movement (date, amount) and what it was tied to.
func (h *Handler) audit(r *http.Request, action string, v TransactionView, extra map[string]any) {
	if h.activity == nil {
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	payload := map[string]any{
		"engagement_uuid":  subjectOf(r).EngagementUUID.String(),
		"transaction_date": v.Date, "amount": v.Amount, "currency": v.Currency, "debit_credit": v.DebitCredit,
	}
	for k, val := range extra {
		payload[k] = val
	}
	id := v.UUID
	h.activity.Record(r.Context(), &p.UserInternal, action, "bank_transaction", &id, payload, r)
}

func invoiceNumbers(ms []MatchedInvoice) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.InvoiceNumber)
	}
	return out
}

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
	// Existing journal lines / cari movements to tie the movement to (#669 G).
	mux.Handle("GET "+tx+"/journal-line-search", middleware.Chain(http.HandlerFunc(h.searchLines), authn, org, read, eng))
	// Who matched, unmatched or booked the movement, when (#565).
	mux.Handle("GET "+tx+"/history", middleware.Chain(http.HandlerFunc(h.history), authn, org, read, eng))
	mux.Handle("PUT "+tx+"/matches", middleware.Chain(http.HandlerFunc(h.match), authn, org, write, eng))
	mux.Handle("DELETE "+tx+"/matches", middleware.Chain(http.HandlerFunc(h.unmatch), authn, org, write, eng))
	// Book the movement to an account from the same screen (#565).
	mux.Handle("PUT "+tx+"/account", middleware.Chain(http.HandlerFunc(h.setAccount), authn, org, write, eng))
}

func subjectOf(r *http.Request) Subject {
	scope := engagementctx.Must(r.Context())
	return Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID, EngagementUUID: scope.UUID}
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

// search: GET .../invoice-search?q=&from=&to=&min=&max= (#565).
func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionUuid"))
	if err != nil {
		response.NotFound(w, r, "bank transaction not found")
		return
	}
	qs := r.URL.Query()
	f := SearchFilter{Term: qs.Get("q")}
	for _, d := range []struct {
		key string
		dst **time.Time
	}{{"from", &f.From}, {"to", &f.To}} {
		if v := strings.TrimSpace(qs.Get(d.key)); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				response.BadRequest(w, r, "VALIDATION_ERROR", d.key+" must be YYYY-MM-DD")
				return
			}
			*d.dst = &t
		}
	}
	for _, a := range []struct {
		key string
		dst **decimal.Decimal
	}{{"min", &f.Min}, {"max", &f.Max}} {
		if v := strings.TrimSpace(qs.Get(a.key)); v != "" {
			n, err := decimal.NewFromString(v)
			if err != nil || n.IsNegative() {
				response.BadRequest(w, r, "VALIDATION_ERROR", a.key+" must be a non-negative amount")
				return
			}
			*a.dst = &n
		}
	}
	items, err := h.svc.SearchFiltered(r.Context(), subjectOf(r), id, f)
	h.reply(w, r, map[string]any{"items": items}, err)
}

// history: GET .../history (#565).
func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionUuid"))
	if err != nil {
		response.NotFound(w, r, "bank transaction not found")
		return
	}
	items, err := h.svc.History(r.Context(), subjectOf(r), id)
	h.reply(w, r, map[string]any{"items": items}, err)
}

// searchLines: GET .../journal-line-search?q=&cari=1 (#669 G).
func (h *Handler) searchLines(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionUuid"))
	if err != nil {
		response.NotFound(w, r, "bank transaction not found")
		return
	}
	cari := r.URL.Query().Get("cari")
	items, err := h.svc.SearchLines(r.Context(), subjectOf(r), id, r.URL.Query().Get("q"), cari == "1" || cari == "true")
	h.reply(w, r, map[string]any{"items": items}, err)
}

type matchBody struct {
	InvoiceUUIDs []uuid.UUID `json:"invoice_uuids"`
	JournalLines []LineRef   `json:"journal_lines"`
}

// matchedPayload names what the movement is tied to (invoices, lines).
func matchedPayload(v TransactionView) map[string]any {
	return map[string]any{"invoices": invoiceNumbers(v.Matches), "journal_lines": lineLabels(v.JournalMatches)}
}

func hasMatches(v TransactionView) bool { return len(v.Matches)+len(v.JournalMatches) > 0 }

func (h *Handler) match(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionUuid"))
	if err != nil {
		response.NotFound(w, r, "bank transaction not found")
		return
	}
	var in matchBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || len(in.InvoiceUUIDs)+len(in.JournalLines) == 0 ||
		len(in.InvoiceUUIDs) > MaxCombination*2 || len(in.JournalLines) > MaxCombination*2 {
		response.BadRequest(w, r, "VALIDATION_ERROR", "name 1-10 invoices (invoice_uuids) and / or 1-10 journal lines (journal_lines)")
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	// What a re-match replaces is read first: afterwards it is gone.
	before, _ := h.svc.Transaction(r.Context(), subjectOf(r), id)
	v, err := h.svc.Match(r.Context(), subjectOf(r), id, in.InvoiceUUIDs, in.JournalLines, p.UserInternal)
	if err == nil {
		action, extra := ActionMatched, matchedPayload(v)
		if hasMatches(before) {
			action = ActionRematched
			prev := matchedPayload(before)
			extra["replaced_invoices"], extra["replaced_journal_lines"] = prev["invoices"], prev["journal_lines"]
		}
		h.audit(r, action, v, extra)
	}
	h.reply(w, r, v, err)
}

func (h *Handler) unmatch(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionUuid"))
	if err != nil {
		response.NotFound(w, r, "bank transaction not found")
		return
	}
	// What the undo removes is read first: afterwards it is gone.
	before, _ := h.svc.Transaction(r.Context(), subjectOf(r), id)
	v, err := h.svc.Unmatch(r.Context(), subjectOf(r), id)
	if err == nil && hasMatches(before) {
		h.audit(r, ActionUnmatched, v, matchedPayload(before))
	}
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
	if err == nil && v.Posting != nil {
		h.audit(r, ActionBooked, v, map[string]any{"account_code": v.Posting.AccountCode, "account_name": v.Posting.AccountName})
	}
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
	case errors.Is(err, ErrInvalidInvoices), errors.Is(err, ErrInvalidLines), errors.Is(err, posting.ErrInvalid), errors.Is(err, reviewusecase.ErrInvalid):
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
