package matching

import (
	"errors"
	"net/http"
	"regexp"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
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
	path := "POST /v1/tenant/engagements/{" + engagementctx.PathParam + "}/bank-matching"
	mux.Handle(path, middleware.Chain(http.HandlerFunc(h.run), authn, org, write, eng))
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
