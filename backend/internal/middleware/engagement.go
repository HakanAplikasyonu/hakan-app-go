package middleware

import (
	"errors"
	"net/http"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// RequireEngagement resolves the engagement named by the {engagementUuid}
// route wildcard inside the current organization and stores the
// engagementctx.Scope on the request context. It must run after
// RequireOrganization.
//
// The caller passes when they are assigned to the engagement or their tenant
// role grants tenant.engagements.read_all. An ended engagement is refused
// with 403 ENGAGEMENT_ENDED (only to callers who could otherwise reach it).
//
// Unknown engagement, an engagement of another organization, a malformed
// uuid and a missing assignment all get the same answer
// (403 NO_ENGAGEMENT_ACCESS), so the route cannot be used to probe which
// engagements exist.
func RequireEngagement(resolver engagementctx.Resolver) func(http.Handler) http.Handler {
	return requireEngagement(resolver, endedRefused)
}

// RequireEngagementArchive is RequireEngagement that also lets an ended
// engagement through while its read-only archive window is open
// (engagements.archive_until, issue #8). Use it on read-only (GET) routes
// whose data is filtered by engagement_document_visible, which itself keeps
// the old office to the period before the office change.
func RequireEngagementArchive(resolver engagementctx.Resolver) func(http.Handler) http.Handler {
	return requireEngagement(resolver, endedInArchive)
}

// RequireEngagementAllowEnded is RequireEngagement that also lets ended
// engagements through. Use it for routes that manage the engagement itself
// (detail, assignment list), not for mükellef data.
func RequireEngagementAllowEnded(resolver engagementctx.Resolver) func(http.Handler) http.Handler {
	return requireEngagement(resolver, endedAllowed)
}

type endedMode int

const (
	endedRefused endedMode = iota
	endedAllowed
	endedInArchive
)

func requireEngagement(resolver engagementctx.Resolver, ended endedMode) func(http.Handler) http.Handler {
	if resolver == nil {
		panic("middleware: RequireEngagement needs a resolver")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope, ok := orgctx.From(r.Context())
			if !ok {
				response.InternalErr(w, r, errors.New("RequireEngagement without RequireOrganization"), "")
				return
			}
			p, ok := authctx.PrincipalFrom(r.Context())
			if !ok {
				response.Unauthorized(w, r, "Authentication is required")
				return
			}
			id, err := uuid.Parse(r.PathValue(engagementctx.PathParam))
			if err != nil {
				writeNoEngagementAccess(w, r, err)
				return
			}
			access, err := resolver.ResolveEngagement(r.Context(), scope.ID, id, p.UserInternal)
			if err != nil {
				if errors.Is(err, engagementctx.ErrNotFound) {
					writeNoEngagementAccess(w, r, err)
					return
				}
				response.InternalErr(w, r, err, "")
				return
			}
			readAll := scope.Has(rbac.PermTenantEngagementsReadAll)
			if access.OrganizationID != scope.ID || (!access.Assigned && !readAll) {
				writeNoEngagementAccess(w, r, errors.New("not assigned to engagement"))
				return
			}
			inArchive := access.ArchiveUntil != nil && time.Now().Before(*access.ArchiveUntil)
			if access.Status != engagementctx.StatusActive &&
				(ended == endedRefused || (ended == endedInArchive && !inArchive)) {
				response.RecordFailure(w, engagementctx.ErrEnded)
				response.Error(w, r, http.StatusForbidden, response.CodeEngagementEnded, "Engagement has ended")
				return
			}
			// Narrow the request's row level security scope (opened by the
			// organization step) to this engagement's taxpayer (issue #166).
			if err := rls.Narrow(r.Context(), access.TaxpayerID); err != nil {
				response.InternalErr(w, r, err, "")
				return
			}
			next.ServeHTTP(w, r.WithContext(engagementctx.WithScope(r.Context(), engagementctx.Scope{
				ID:             access.ID,
				UUID:           access.UUID,
				OrganizationID: access.OrganizationID,
				TaxpayerID:     access.TaxpayerID,
				TaxpayerUUID:   access.TaxpayerUUID,
				Status:         access.Status,
				StartedAt:      access.StartedAt,
				ArchiveUntil:   access.ArchiveUntil,
				ViaReadAll:     !access.Assigned,
			})))
		})
	}
}

func writeNoEngagementAccess(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	response.Error(w, r, http.StatusForbidden, response.CodeNoEngagementAccess, "You do not have access to this engagement")
}
