package posting

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the invoice → journal draft routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the routes. Reading the link needs invoice read;
// booking writes a journal draft, so it needs journal write.
func RegisterRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver, engResolver engagementctx.Resolver, h *Handler,
) {
	if orgResolver == nil || engResolver == nil || h == nil {
		return
	}
	chain := func(perm string, fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn,
			middleware.Authenticate(tokens, loader),
			middleware.RequireOrganization(orgResolver),
			middleware.RequireTenantPermission(perm),
			middleware.RequireEngagement(engResolver),
		)
	}
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}"
	mux.Handle("GET "+base+"/invoices/{invoiceUuid}/journal-draft", chain(rbac.PermTenantInvoicesRead, h.get))
	mux.Handle("POST "+base+"/invoices/{invoiceUuid}/journal-draft", chain(rbac.PermTenantJournalsWrite, h.post))
	mux.Handle("POST "+base+"/invoice-postings", chain(rbac.PermTenantJournalsWrite, h.postPeriod))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	inv, ok := h.invoice(w, r)
	if !ok {
		return
	}
	ref, err := h.svc.Link(r.Context(), s, inv)
	if err != nil {
		response.InternalErr(w, r, err, "could not load the invoice's journal draft")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"draft": ref})
}

func (h *Handler) post(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	id, err := uuid.Parse(r.PathValue("invoiceUuid"))
	if err != nil {
		response.NotFound(w, r, "invoice not found")
		return
	}
	inv, err := h.svc.Invoice(r.Context(), s, id)
	if errors.Is(err, ErrNotFound) {
		response.NotFound(w, r, "invoice not found")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not load invoice")
		return
	}
	res, err := h.svc.Post(r.Context(), s, p.UserInternal, inv)
	if err != nil {
		response.InternalErr(w, r, err, "could not book the invoice")
		return
	}
	status := http.StatusOK
	if res.Outcome == OutcomeCreated {
		status = http.StatusCreated
	}
	response.JSON(w, r, status, res)
}

// postPeriod books every unbooked invoice of the entry period.
func (h *Handler) postPeriod(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	var body struct {
		Period string `json:"period"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_BODY", "body must be {\"period\": \"YYYY-MM\"}")
		return
	}
	period := strings.TrimSpace(body.Period)
	if _, err := time.Parse("2006-01", period); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_PERIOD", "period must be YYYY-MM")
		return
	}
	items, err := h.svc.PostPeriod(r.Context(), s, p.UserInternal, period)
	if err != nil {
		response.InternalErr(w, r, err, "could not book the invoices")
		return
	}
	var created, refused int
	for _, it := range items {
		switch it.Outcome {
		case OutcomeCreated:
			created++
		case OutcomeRefused:
			refused++
		}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{
		"period": period, "created": created, "refused": refused, "items": items,
	})
}

// invoice resolves the path invoice to its internal id.
func (h *Handler) invoice(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := uuid.Parse(r.PathValue("invoiceUuid"))
	if err != nil {
		response.NotFound(w, r, "invoice not found")
		return 0, false
	}
	inv, err := h.svc.Invoice(r.Context(), engagementctx.Must(r.Context()), id)
	if errors.Is(err, ErrNotFound) {
		response.NotFound(w, r, "invoice not found")
		return 0, false
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not load invoice")
		return 0, false
	}
	return inv.ID, true
}
