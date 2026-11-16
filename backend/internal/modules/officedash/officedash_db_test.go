package officedash

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type fixture struct {
	d                *testdb.DB
	orgA, orgB       int64
	assigned, viewer int64
	engA1, engA2     int64
	tpA1, tpA2       int64
}

func setup(t *testing.T) fixture {
	t.Helper()
	d := testdb.Open(t)
	f := fixture{d: d, orgA: d.Organization(t), orgB: d.Organization(t), assigned: d.User(t), viewer: d.User(t)}
	d.Member(t, f.orgA, f.assigned, "member")
	d.Member(t, f.orgA, f.viewer, "owner")
	f.tpA1, f.tpA2 = d.Taxpayer(t, "legal", "1111111111"), d.Taxpayer(t, "legal", "2222222222")
	tpB := d.Taxpayer(t, "legal", "3333333333")
	f.engA1, f.engA2 = d.Engagement(t, f.orgA, f.tpA1), d.Engagement(t, f.orgA, f.tpA2)
	engB := d.Engagement(t, f.orgB, tpB)
	d.Assign(t, f.orgA, f.engA1, f.assigned)

	// engA1: 2 pending + 1 accepted suggestion, 1 blocker + 1 info open warning,
	// 1 resolved warning, 1 upload today, 1 upload three days ago.
	f.suggestion(t, f.orgA, f.engA1, f.tpA1, "pending")
	f.suggestion(t, f.orgA, f.engA1, f.tpA1, "pending")
	f.suggestion(t, f.orgA, f.engA1, f.tpA1, "accepted")
	f.warning(t, f.orgA, f.engA1, f.tpA1, "blocker", "open")
	f.warning(t, f.orgA, f.engA1, f.tpA1, "info", "open")
	f.warning(t, f.orgA, f.engA1, f.tpA1, "warning", "resolved")
	f.upload(t, f.orgA, f.engA1, f.tpA1, f.assigned, time.Now())
	f.upload(t, f.orgA, f.engA1, f.tpA1, f.assigned, time.Now().Add(-72*time.Hour))
	// engA2 (not assigned to `assigned`): 1 pending, 1 warning, 1 upload.
	f.suggestion(t, f.orgA, f.engA2, f.tpA2, "pending")
	f.warning(t, f.orgA, f.engA2, f.tpA2, "warning", "open")
	f.upload(t, f.orgA, f.engA2, f.tpA2, f.viewer, time.Now())
	// Office B must never show up.
	f.suggestion(t, f.orgB, engB, tpB, "pending")
	f.warning(t, f.orgB, engB, tpB, "blocker", "open")
	f.upload(t, f.orgB, engB, tpB, f.viewer, time.Now())
	return f
}

func (f fixture) suggestion(t *testing.T, org, eng, tp int64, status string) {
	t.Helper()
	_, err := f.d.Pool.Exec(context.Background(), `
		INSERT INTO suggestions (organization_id, engagement_id, taxpayer_id, kind, target_type, target_id, proposed_value, confidence, rationale, origin, model, status)
		VALUES ($1, $2, $3, 'account_code', 'invoice_item', $5, '"770.01"', 0.8, 'test', 'model', 'test-model', $4)`, org, eng, tp, status, testdb.Next())
	if err != nil {
		t.Fatalf("suggestion: %v", err)
	}
}

func (f fixture) warning(t *testing.T, org, eng, tp int64, severity, status string) {
	t.Helper()
	_, err := f.d.Pool.Exec(context.Background(), `
		INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, entity_type, entity_id, code, severity, producer, status)
		VALUES ($1, $2, $3, 'period', $6, 'TEST_CODE', $4, 'test', $5)`, org, eng, tp, severity, status, testdb.Next())
	if err != nil {
		t.Fatalf("warning: %v", err)
	}
}

func (f fixture) upload(t *testing.T, org, eng, tp, actor int64, at time.Time) {
	t.Helper()
	_, err := f.d.Pool.Exec(context.Background(), `
		INSERT INTO ubl_upload_jobs (organization_id, engagement_id, taxpayer_id, actor_id, file_name, file_key, sha256, size_bytes, created_at)
		VALUES ($1, $2, $3, $4, 'a.xml', 'k', repeat('a', 64), 1, $5)`, org, eng, tp, actor, at)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
}

func (f fixture) get(t *testing.T, org, user int64, perms ...string) (int, Dashboard) {
	t.Helper()
	h := &Handler{Q: database.QueriesFor(f.d.Pool)}
	r := httptest.NewRequest(http.MethodGet, "/v1/tenant/dashboard", nil)
	ctx := authctx.WithPrincipal(r.Context(), authctx.Principal{UserInternal: user})
	ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: org, Permissions: perms})
	w := httptest.NewRecorder()
	// As in the API (issue #166): one row level security scope per request.
	rls.Middleware(f.d.Pool, database.VisibleTaxpayers(f.d.Pool), nil)(http.HandlerFunc(h.Get)).ServeHTTP(w, r.WithContext(ctx))
	var env struct{ Data Dashboard }
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode %s: %v", w.Body, err)
		}
	}
	return w.Code, env.Data
}

var readAll = []string{
	rbac.PermTenantEngagementsRead, rbac.PermTenantEngagementsReadAll,
	rbac.PermTenantReviewRead, rbac.PermTenantDocTasksRead, rbac.PermTenantWarningsRead,
}

func TestDashboardCountsOfficeWide(t *testing.T) {
	f := setup(t)
	code, d := f.get(t, f.orgA, f.viewer, readAll...)
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if d.Review == nil || d.Review.Pending != 3 {
		t.Fatalf("review = %+v, want 3 pending", d.Review)
	}
	if d.Warnings == nil || d.Warnings.Open != 3 || d.Warnings.Blocker != 1 || d.Warnings.Warning != 1 || d.Warnings.Info != 1 {
		t.Fatalf("warnings = %+v", d.Warnings)
	}
	if d.DocTasks == nil || d.DocTasks.Open != 0 {
		t.Fatalf("doc tasks = %+v", d.DocTasks)
	}
	if d.Uploads == nil || d.Uploads.Invoices != 2 || d.Uploads.Total != 2 || len(d.Uploads.Recent) != 2 {
		t.Fatalf("uploads = %+v", d.Uploads)
	}
	for _, u := range d.Uploads.Recent {
		if u.Kind != "invoice" || u.TaxpayerTitle == "" {
			t.Fatalf("recent upload %+v", u)
		}
	}
}

func TestDashboardAssignedOnly(t *testing.T) {
	f := setup(t)
	code, d := f.get(t, f.orgA, f.assigned,
		rbac.PermTenantEngagementsRead, rbac.PermTenantReviewRead, rbac.PermTenantWarningsRead)
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if d.Review == nil || d.Review.Pending != 2 {
		t.Fatalf("review = %+v, want 2 (assigned engagement only)", d.Review)
	}
	if d.Warnings == nil || d.Warnings.Open != 2 || d.Warnings.Warning != 0 {
		t.Fatalf("warnings = %+v", d.Warnings)
	}
	if d.DocTasks != nil {
		t.Fatalf("doc tasks without tenant.doc_tasks.read = %+v, want null", d.DocTasks)
	}
	if d.Uploads.Total != 1 || len(d.Uploads.Recent) != 1 || d.Uploads.Recent[0].TaxpayerTitle != "Mükellef 1111111111" {
		t.Fatalf("uploads = %+v", d.Uploads)
	}
}

func TestDashboardEndedEngagementHidden(t *testing.T) {
	f := setup(t)
	f.d.EndEngagement(t, f.engA2)
	_, d := f.get(t, f.orgA, f.viewer, readAll...)
	if d.Review.Pending != 2 || d.Warnings.Open != 2 || d.Uploads.Total != 1 {
		t.Fatalf("after ending engagement: review %+v warnings %+v uploads %d", d.Review, d.Warnings, d.Uploads.Total)
	}
}

func TestStartOfDayIstanbul(t *testing.T) {
	// 21:30 UTC is 00:30 the next day in Istanbul (UTC+3).
	got := StartOfDay(time.Date(2026, 11, 16, 21, 30, 0, 0, time.UTC))
	want := time.Date(2026, 11, 16, 21, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("StartOfDay = %s, want %s", got.UTC(), want)
	}
}
