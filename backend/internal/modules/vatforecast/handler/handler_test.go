package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/shopspring/decimal"
)

type store struct {
	periods []string
	filter  usecase.OfficeFilter
}

func (s *store) Lines(_ context.Context, _ int64, p string) ([]usecase.Line, error) {
	s.periods = append(s.periods, p)
	return []usecase.Line{{Direction: usecase.DirectionOutgoing, TaxRate: decimal.NewFromInt(20), VAT: decimal.RequireFromString("123.4"), InvoiceCount: 1}}, nil
}
func (s *store) Excluded(context.Context, int64, string) (usecase.Excluded, error) {
	return usecase.Excluded{Included: 1}, nil
}
func (s *store) History(context.Context, int64, string, string) ([]usecase.PeriodTotal, error) {
	return nil, nil
}
func (s *store) OpeningAtOrBefore(context.Context, int64, int64, string) (*usecase.Opening, error) {
	return nil, nil
}
func (s *store) ListOpenings(context.Context, int64, int64) ([]usecase.Opening, error) {
	return []usecase.Opening{}, nil
}
func (s *store) UpsertOpening(_ context.Context, _, _ int64, o usecase.Opening, _ int64) (usecase.Opening, error) {
	return o, nil
}
func (s *store) DeleteOpening(context.Context, int64, int64, string) (bool, error) { return false, nil }
func (s *store) TrackingSettings(context.Context, int64) (usecase.TrackingSettings, error) {
	return usecase.DefaultTrackingSettings(), nil
}
func (s *store) SaveTrackingSettings(_ context.Context, _, _ int64, in usecase.TrackingSettings) (usecase.TrackingSettings, error) {
	return in, nil
}
func (s *store) ActiveEngagements(_ context.Context, f usecase.OfficeFilter) ([]usecase.EngagementRef, int64, error) {
	s.filter = f
	return nil, 0, nil
}

type noConfig struct{}

func (noConfig) Resolve(context.Context, engine.Subject, []string) (map[string]engine.Effective, error) {
	return map[string]engine.Effective{}, nil
}

func newHandler() (*Handler, *store) {
	s := &store{}
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, usecase.Istanbul)
	svc := usecase.New(s, noConfig{}, nil).WithClock(func() time.Time { return now })
	return New(svc, nil), s
}

func req(method, target, body string, perms ...string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := orgctx.WithScope(r.Context(), orgctx.Scope{ID: 1, Permissions: perms})
	ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: 77})
	ctx = engagementctx.WithScope(ctx, engagementctx.Scope{ID: 5, OrganizationID: 1, TaxpayerID: 9})
	return r.WithContext(ctx)
}

func TestGetDefaultsToCurrentPeriodAndRejectsBadPeriod(t *testing.T) {
	h, s := newHandler()
	rec := httptest.NewRecorder()
	h.Get(rec, req(http.MethodGet, "/x", ""))
	if rec.Code != http.StatusOK || len(s.periods) != 1 || s.periods[0] != "2026-11" {
		t.Fatalf("status %d periods %v", rec.Code, s.periods)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"payable":"123.40"`) || !strings.Contains(body, `"due_date":"2026-12-28"`) {
		t.Fatalf("body %s", body)
	}
	rec = httptest.NewRecorder()
	h.Get(rec, req(http.MethodGet, "/x?period=2026-13", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad period: %d", rec.Code)
	}
}

func TestPutOpeningValidates(t *testing.T) {
	h, _ := newHandler()
	for _, body := range []string{`{"amount":"-5"}`, `{"amount":"5","x":1}`, `not json`} {
		rec := httptest.NewRecorder()
		r := req(http.MethodPut, "/x", body)
		r.SetPathValue("period", "2026-10")
		h.PutOpening(rec, r)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	r := req(http.MethodPut, "/x", `{"amount":"250.5","note":"2025 sonu beyanname"}`)
	r.SetPathValue("period", "2026-10")
	h.PutOpening(rec, r)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"amount":"250.50"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	r = req(http.MethodDelete, "/x", "")
	r.SetPathValue("period", "2026-10")
	h.DeleteOpening(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing: %d", rec.Code)
	}
}

func TestOfficeUsesAssignmentsUnlessReadAll(t *testing.T) {
	h, s := newHandler()
	rec := httptest.NewRecorder()
	h.Office(rec, req(http.MethodGet, "/x?limit=500", ""))
	if rec.Code != http.StatusOK || s.filter.AllEngagements || s.filter.UserID != 77 || s.filter.Limit != 20 {
		t.Fatalf("%d %+v", rec.Code, s.filter)
	}
	h.Office(httptest.NewRecorder(), req(http.MethodGet, "/x", "", rbac.PermTenantEngagementsReadAll))
	if !s.filter.AllEngagements {
		t.Fatal("read_all caller should see every engagement")
	}
}
