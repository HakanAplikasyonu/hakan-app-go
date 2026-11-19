package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

type fakeWarnings struct {
	got     repository.Filter
	decided catalog.Status
	eng     int64
	note    string
}

func (f *fakeWarnings) List(_ context.Context, flt repository.Filter) ([]repository.Row, int64, error) {
	f.got = flt
	return nil, 0, nil
}

func (f *fakeWarnings) Decide(_ context.Context, _, eng int64, _ uuid.UUID, s catalog.Status, _ int64, note string) error {
	f.decided, f.eng, f.note = s, eng, note
	return nil
}

func (f *fakeWarnings) Groups(_ context.Context, flt repository.Filter) ([]repository.Group, error) {
	f.got = flt
	return nil, nil
}

func (f *fakeWarnings) AcknowledgeBulk(_ context.Context, flt repository.Filter, _ *usecase.BulkInput) (int64, error) {
	f.got = flt
	return 0, nil
}

func req(method, target string, scope orgctx.Scope, eng int64) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	ctx := orgctx.WithScope(r.Context(), scope)
	ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: 77})
	if eng > 0 {
		ctx = engagementctx.WithScope(ctx, engagementctx.Scope{ID: eng})
	}
	return r.WithContext(ctx)
}

func TestOfficeListUsesAssignmentsUnlessReadAll(t *testing.T) {
	fw := &fakeWarnings{}
	h := New(nil).WithWarnings(fw)
	rec := httptest.NewRecorder()
	h.ListOfficeWarnings(rec, req(http.MethodGet, "/v1/tenant/warnings?period=2026-10&severity=blocker&limit=500", orgctx.Scope{ID: 1}, 0))
	if rec.Code != http.StatusOK || fw.got.AllEngagements || fw.got.UserID != 77 || fw.got.Period != "2026-10" ||
		fw.got.Severity != "blocker" || fw.got.Limit != 20 {
		t.Fatalf("status %d filter %+v", rec.Code, fw.got)
	}
	rec = httptest.NewRecorder()
	h.ListOfficeWarnings(rec, req(http.MethodGet, "/v1/tenant/warnings", orgctx.Scope{ID: 1, Permissions: []string{rbac.PermTenantEngagementsReadAll}}, 0))
	if !fw.got.AllEngagements {
		t.Fatal("read_all caller should see every engagement")
	}
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("body = %s", rec.Body)
	}
}

func TestEngagementListAndDecide(t *testing.T) {
	fw := &fakeWarnings{}
	h := New(nil).WithWarnings(fw)
	rec := httptest.NewRecorder()
	h.ListEngagementWarnings(rec, req(http.MethodGet, "/x?status=open", orgctx.Scope{ID: 1}, 9))
	if fw.got.EngagementID != 9 || !fw.got.AllEngagements || fw.got.Status != "open" {
		t.Fatalf("filter = %+v", fw.got)
	}
	r := req(http.MethodPost, "/x", orgctx.Scope{ID: 1}, 9)
	r.SetPathValue("warningUuid", uuid.NewString())
	rec = httptest.NewRecorder()
	h.AcknowledgeWarning(rec, r)
	if rec.Code != http.StatusOK || fw.decided != catalog.StatusAcknowledged || fw.eng != 9 {
		t.Fatalf("status %d decided %s", rec.Code, fw.decided)
	}
	// The reason travels with the decision (#529); unknown fields are refused.
	r = req(http.MethodPost, "/x", orgctx.Scope{ID: 1}, 9)
	r.Body = io.NopCloser(strings.NewReader(`{"note":"fark doğru, sözleşmeye göre"}`))
	r.ContentLength = -1
	r.SetPathValue("warningUuid", uuid.NewString())
	rec = httptest.NewRecorder()
	h.ResolveWarning(rec, r)
	if rec.Code != http.StatusOK || fw.decided != catalog.StatusResolved || fw.note != "fark doğru, sözleşmeye göre" {
		t.Fatalf("with note: status %d decided %s note %q", rec.Code, fw.decided, fw.note)
	}
	r = req(http.MethodPost, "/x", orgctx.Scope{ID: 1}, 9)
	r.Body = io.NopCloser(strings.NewReader(`{"reason":"x"}`))
	r.ContentLength = -1
	r.SetPathValue("warningUuid", uuid.NewString())
	rec = httptest.NewRecorder()
	h.ResolveWarning(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d", rec.Code)
	}
	r = req(http.MethodPost, "/x", orgctx.Scope{ID: 1}, 9)
	r.SetPathValue("warningUuid", "nope")
	rec = httptest.NewRecorder()
	h.ResolveWarning(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad uuid status = %d", rec.Code)
	}
}

func TestListRejectsBadFilters(t *testing.T) {
	h := New(nil).WithWarnings(usecase.NewWarnings(nil))
	for _, q := range []string{"period=2026-13", "code=NOPE", "severity=x", "status=x", "entity_type=x"} {
		rec := httptest.NewRecorder()
		h.ListOfficeWarnings(rec, req(http.MethodGet, "/v1/tenant/warnings?"+q, orgctx.Scope{ID: 1}, 0))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d", q, rec.Code)
		}
	}
}
