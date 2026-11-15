package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// TestRequireEngagementArchive: an ended engagement passes only while its
// archive window is open (issue #8); the strict chain never lets it through.
func TestRequireEngagementArchive(t *testing.T) {
	t.Parallel()
	const orgID, user = int64(7), int64(1)
	open, closed, none, active := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	future, past := time.Now().Add(24*time.Hour), time.Now().Add(-time.Hour)
	resolver := engagementResolverFunc(func(_ context.Context, _ int64, id uuid.UUID, _ int64) (engagementctx.Access, error) {
		a := engagementctx.Access{ID: 1, UUID: id, OrganizationID: orgID, Status: engagementctx.StatusEnded, Assigned: true}
		switch id {
		case open:
			a.ArchiveUntil = &future
		case closed:
			a.ArchiveUntil = &past
		case active:
			a.Status = engagementctx.StatusActive
		}
		return a, nil
	})
	run := func(mw func(http.Handler) http.Handler, id uuid.UUID) int {
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.SetPathValue(engagementctx.PathParam, id.String())
		req = withPrincipal(req, authctx.Principal{UserInternal: user})
		req = req.WithContext(orgctx.WithScope(req.Context(), orgctx.Scope{ID: orgID, Permissions: []string{rbac.PermTenantEngagementsRead}}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	archive := RequireEngagementArchive(resolver)
	strict := RequireEngagement(resolver)
	for name, tc := range map[string]struct {
		id              uuid.UUID
		archive, strict int
	}{
		"active":               {active, 200, 200},
		"ended, window open":   {open, 200, 403},
		"ended, window closed": {closed, 403, 403},
		"ended, no window":     {none, 403, 403},
	} {
		if got := run(archive, tc.id); got != tc.archive {
			t.Errorf("%s: archive chain = %d, want %d", name, got, tc.archive)
		}
		if got := run(strict, tc.id); got != tc.strict {
			t.Errorf("%s: strict chain = %d, want %d", name, got, tc.strict)
		}
	}
}
