package posting

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
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
}

// NewHandler builds the handler.
func NewHandler(aliases *Service) *Handler { return &Handler{aliases: aliases} }

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
	items, err := h.aliases.Aliases(r.Context(), subjectOf(r), status)
	if err != nil {
		writeErr(w, r, err, "alias list")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
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
