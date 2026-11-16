package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/activity"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/google/uuid"
)

type bulkWorld struct {
	d                   *testdb.DB
	h                   *Handler
	org, otherOrg       int64
	engA1, engA2, engA3 engagementctx.Scope
	engB                engagementctx.Scope
	assigned, manager   int64
}

const (
	codeTax     = "TAX_TOTAL_MISMATCH"
	codePayable = "PAYABLE_MISMATCH"
	codeTaxID   = "INVALID_TAX_ID"
)

func setupBulk(t *testing.T) *bulkWorld {
	t.Helper()
	d := testdb.Open(t)
	ctx := context.Background()
	w := &bulkWorld{d: d, org: d.Organization(t), otherOrg: d.Organization(t)}
	w.assigned, w.manager = d.User(t), d.User(t)
	d.Member(t, w.org, w.assigned, "member")
	d.Member(t, w.org, w.manager, "accountant")
	scope := func(org int64, tax string) engagementctx.Scope {
		tp := d.Taxpayer(t, "legal", tax)
		id := d.Engagement(t, org, tp)
		var u uuid.UUID
		if err := d.Pool.QueryRow(ctx, `SELECT uuid FROM engagements WHERE id = $1`, id).Scan(&u); err != nil {
			t.Fatal(err)
		}
		return engagementctx.Scope{ID: id, UUID: u, OrganizationID: org, TaxpayerID: tp, Status: engagementctx.StatusActive}
	}
	w.engA1, w.engA2, w.engA3 = scope(w.org, "9000000101"), scope(w.org, "9000000102"), scope(w.org, "9000000103")
	w.engB = scope(w.otherOrg, "9000000104")
	d.Assign(t, w.org, w.engA1.ID, w.assigned)
	d.Assign(t, w.org, w.engA3.ID, w.assigned)

	ins := func(s engagementctx.Scope, entity int64, code, severity, status, period, updated string) {
		t.Helper()
		_, err := d.Pool.Exec(ctx, `
			INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, period, entity_type, entity_id,
				code, severity, params, producer, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'invoice', $5, $6, $7, '{}', 'test', $8, $9, $9)`,
			s.OrganizationID, s.ID, s.TaxpayerID, period, entity, code, severity, status, updated)
		if err != nil {
			t.Fatal(err)
		}
	}
	ins(w.engA1, 1, codeTax, "warning", "open", "2026-10", "2026-11-01T10:00:00Z")
	ins(w.engA1, 2, codeTax, "warning", "open", "2026-10", "2026-11-03T10:00:00Z")
	ins(w.engA1, 3, codeTax, "warning", "open", "2026-09", "2026-10-03T10:00:00Z")
	ins(w.engA1, 4, codeTax, "warning", "acknowledged", "2026-10", "2026-11-02T10:00:00Z")
	ins(w.engA1, 5, codePayable, "blocker", "open", "2026-10", "2026-11-01T10:00:00Z")
	ins(w.engA1, 6, codeTaxID, "info", "open", "2026-10", "2026-11-01T10:00:00Z")
	ins(w.engA2, 1, codeTax, "warning", "open", "2026-10", "2026-11-05T10:00:00Z")
	ins(w.engA2, 2, codePayable, "blocker", "open", "2026-10", "2026-11-01T10:00:00Z")
	ins(w.engA3, 1, codeTax, "warning", "open", "2026-10", "2026-11-01T10:00:00Z")
	ins(w.engB, 1, codeTax, "warning", "open", "2026-10", "2026-11-01T10:00:00Z")
	// engA3 ends after its warning was raised: bulk decisions skip it.
	d.EndEngagement(t, w.engA3.ID)
	w.engA3.Status = "ended"

	q := database.NewQueries(d.Pool)
	w.h = New(nil).WithWarnings(usecase.NewWarnings(repository.NewWarnings(q))).WithActivity(activity.NewRecorder(q, nil))
	return w
}

// do runs fn as the API would: one row level security transaction per
// request. eng is nil for office routes.
func (w *bulkWorld) do(t *testing.T, fn http.HandlerFunc, method, target, body string, user int64, readAll bool, eng *engagementctx.Scope) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, bytes.NewReader([]byte(body)))
	ctx := authctx.WithPrincipal(r.Context(), authctx.Principal{UserInternal: user})
	perms := []string{rbac.PermTenantWarningsRead, rbac.PermTenantWarningsWrite}
	if readAll {
		perms = append(perms, rbac.PermTenantEngagementsReadAll)
	}
	ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: w.org, Permissions: perms})
	if eng != nil {
		ctx = engagementctx.WithScope(ctx, *eng)
	}
	rec := httptest.NewRecorder()
	rls.Middleware(w.d.Pool, nil, nil)(fn).ServeHTTP(rec, r.WithContext(ctx))
	return rec
}

type groupDTO struct {
	Code        string
	Severity    string
	Count       int64
	Engagements *int64
	LatestAt    string `json:"latest_at"`
}

func decodeData[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var env struct{ Data T }
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return env.Data
}

func (w *bulkWorld) groups(t *testing.T, fn http.HandlerFunc, target string, user int64, readAll bool, eng *engagementctx.Scope) []groupDTO {
	t.Helper()
	rec := w.do(t, fn, http.MethodGet, target, "", user, readAll, eng)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s", target, rec.Code, rec.Body)
	}
	return decodeData[struct{ Items []groupDTO }](t, rec).Items
}

func (w *bulkWorld) status(t *testing.T, eng int64, code, period string) map[string]int {
	t.Helper()
	rows, err := w.d.Pool.Query(context.Background(), `
		SELECT status, count(*) FROM data_warnings
		WHERE engagement_id = $1 AND code = $2 AND ($3 = '' OR period = $3) GROUP BY status`, eng, code, period)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			t.Fatal(err)
		}
		out[s] = n
	}
	return out
}

func ptr(n int64) *int64 { return &n }

func sameGroups(got []groupDTO, want []groupDTO) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		g, x := got[i], want[i]
		if g.Code != x.Code || g.Severity != x.Severity || g.Count != x.Count ||
			(g.Engagements == nil) != (x.Engagements == nil) || (g.Engagements != nil && *g.Engagements != *x.Engagements) {
			return false
		}
	}
	return true
}

// TestWarningGroupsAndBulkAcknowledge is issue #333's warning part: counts by
// code, the period and status filters, visibility without read_all, and bulk
// acknowledgement that leaves other codes, engagements and offices alone.
func TestWarningGroupsAndBulkAcknowledge(t *testing.T) {
	w := setupBulk(t)

	// Office view, read_all: every engagement of the office, open by default.
	got := w.groups(t, w.h.ListOfficeWarningGroups, "/v1/tenant/warnings/groups", w.manager, true, nil)
	want := []groupDTO{
		{Code: codePayable, Severity: "blocker", Count: 2, Engagements: ptr(2)},
		{Code: codeTax, Severity: "warning", Count: 5, Engagements: ptr(3)},
		{Code: codeTaxID, Severity: "info", Count: 1, Engagements: ptr(1)},
	}
	if !sameGroups(got, want) {
		t.Fatalf("office groups = %+v", got)
	}
	if got[1].LatestAt != "2026-11-05T10:00:00Z" {
		t.Fatalf("latest_at = %s", got[1].LatestAt)
	}
	// Period and severity filters.
	got = w.groups(t, w.h.ListOfficeWarningGroups, "/v1/tenant/warnings/groups?period=2026-09", w.manager, true, nil)
	if !sameGroups(got, []groupDTO{{Code: codeTax, Severity: "warning", Count: 1, Engagements: ptr(1)}}) {
		t.Fatalf("period groups = %+v", got)
	}
	got = w.groups(t, w.h.ListOfficeWarningGroups, "/v1/tenant/warnings/groups?severity=blocker", w.manager, true, nil)
	if len(got) != 1 || got[0].Code != codePayable {
		t.Fatalf("severity groups = %+v", got)
	}
	// Without read_all: only assigned engagements (A1, A3).
	got = w.groups(t, w.h.ListOfficeWarningGroups, "/v1/tenant/warnings/groups", w.assigned, false, nil)
	want = []groupDTO{
		{Code: codePayable, Severity: "blocker", Count: 1, Engagements: ptr(1)},
		{Code: codeTax, Severity: "warning", Count: 4, Engagements: ptr(2)},
		{Code: codeTaxID, Severity: "info", Count: 1, Engagements: ptr(1)},
	}
	if !sameGroups(got, want) {
		t.Fatalf("assigned groups = %+v", got)
	}
	got = w.groups(t, w.h.ListOfficeWarningGroups, "/v1/tenant/warnings/groups", w.manager, false, nil)
	if len(got) != 0 {
		t.Fatalf("unassigned groups = %+v", got)
	}
	// Engagement view: no engagements count; status filter.
	got = w.groups(t, w.h.ListEngagementWarningGroups, "/x", w.assigned, false, &w.engA1)
	want = []groupDTO{
		{Code: codePayable, Severity: "blocker", Count: 1},
		{Code: codeTax, Severity: "warning", Count: 3},
		{Code: codeTaxID, Severity: "info", Count: 1},
	}
	if !sameGroups(got, want) {
		t.Fatalf("engagement groups = %+v", got)
	}
	got = w.groups(t, w.h.ListEngagementWarningGroups, "/x?status=acknowledged", w.assigned, false, &w.engA1)
	if !sameGroups(got, []groupDTO{{Code: codeTax, Severity: "warning", Count: 1}}) {
		t.Fatalf("acknowledged groups = %+v", got)
	}
	if got := w.groups(t, w.h.ListEngagementWarningGroups, "/x?status=all", w.assigned, false, &w.engA1); got[1].Count != 4 {
		t.Fatalf("all-status groups = %+v", got)
	}
	for _, q := range []string{"?period=2026-13", "?severity=fatal", "?status=closed"} {
		if rec := w.do(t, w.h.ListOfficeWarningGroups, http.MethodGet, "/v1/tenant/warnings/groups"+q, "", w.manager, true, nil); rec.Code != http.StatusBadRequest {
			t.Fatalf("groups %s = %d", q, rec.Code)
		}
	}

	// Bulk validation.
	for _, body := range []string{
		`{}`, `{"code":"NOT_A_CODE"}`, `{"code":"` + codeTax + `","period":"2026-1"}`,
		`{"code":"` + codeTax + `","extra":1}`, `{"code":"` + codeTax + `","note":"` + strings.Repeat("x", 501) + `"}`, `[`,
	} {
		if rec := w.do(t, w.h.AcknowledgeEngagementWarnings, http.MethodPost, "/x", body, w.assigned, false, &w.engA1); rec.Code != http.StatusBadRequest {
			t.Fatalf("bulk %s = %d %s", body, rec.Code, rec.Body)
		}
	}

	ack := func(fn http.HandlerFunc, body string, user int64, readAll bool, eng *engagementctx.Scope) int64 {
		t.Helper()
		rec := w.do(t, fn, http.MethodPost, "/x", body, user, readAll, eng)
		if rec.Code != http.StatusOK {
			t.Fatalf("bulk %s = %d %s", body, rec.Code, rec.Body)
		}
		return decodeData[struct{ Acknowledged int64 }](t, rec).Acknowledged
	}
	// Engagement + period: the two open 2026-10 rows of A1 only.
	if n := ack(w.h.AcknowledgeEngagementWarnings, `{"code":"`+codeTax+`","period":"2026-10","note":"toplu görüldü"}`, w.assigned, false, &w.engA1); n != 2 {
		t.Fatalf("engagement bulk = %d", n)
	}
	if s := w.status(t, w.engA1.ID, codeTax, "2026-10"); s["open"] != 0 || s["acknowledged"] != 3 {
		t.Fatalf("A1 2026-10 = %v", s)
	}
	if s := w.status(t, w.engA1.ID, codeTax, "2026-09"); s["open"] != 1 {
		t.Fatalf("A1 2026-09 = %v", s)
	}
	var by, withAt int
	if err := w.d.Pool.QueryRow(context.Background(), `
		SELECT count(*) FILTER (WHERE resolved_by = $2), count(*) FILTER (WHERE resolved_at IS NOT NULL)
		FROM data_warnings WHERE engagement_id = $1 AND code = $3 AND period = '2026-10' AND entity_id IN (1, 2)`,
		w.engA1.ID, w.assigned, codeTax).Scan(&by, &withAt); err != nil || by != 2 || withAt != 2 {
		t.Fatalf("decision fields: by=%d at=%d %v", by, withAt, err)
	}
	var note string
	if err := w.d.Pool.QueryRow(context.Background(), `
		SELECT payload->>'note' FROM activity_events WHERE organization_id = $1 AND action = 'warnings.acknowledge_bulk'`,
		w.org).Scan(&note); err != nil || note != "toplu görüldü" {
		t.Fatalf("activity note = %q %v", note, err)
	}
	// Nothing left: 0, not an error.
	if n := ack(w.h.AcknowledgeEngagementWarnings, `{"code":"`+codeTax+`","period":"2026-10"}`, w.assigned, false, &w.engA1); n != 0 {
		t.Fatalf("repeat bulk = %d", n)
	}

	// Office-wide without read_all: A1's 2026-09 row; A2 is not visible, A3 has ended.
	if n := ack(w.h.AcknowledgeOfficeWarnings, `{"code":"`+codeTax+`"}`, w.assigned, false, nil); n != 1 {
		t.Fatalf("assigned office bulk = %d", n)
	}
	if s := w.status(t, w.engA2.ID, codeTax, ""); s["open"] != 1 {
		t.Fatalf("A2 touched: %v", s)
	}
	if s := w.status(t, w.engA3.ID, codeTax, ""); s["open"] != 1 {
		t.Fatalf("ended A3 touched: %v", s)
	}
	// Office-wide with read_all: A2 now; the other office stays open.
	if n := ack(w.h.AcknowledgeOfficeWarnings, `{"code":"`+codeTax+`"}`, w.manager, true, nil); n != 1 {
		t.Fatalf("read_all office bulk = %d", n)
	}
	var otherOpen int
	if err := w.d.Pool.QueryRow(context.Background(), `
		SELECT count(*) FROM data_warnings WHERE engagement_id = $1 AND status = 'open'`, w.engB.ID).Scan(&otherOpen); err != nil || otherOpen != 1 {
		t.Fatalf("other office open = %d %v", otherOpen, err)
	}
	// Other codes untouched.
	for _, e := range []int64{w.engA1.ID, w.engA2.ID} {
		if s := w.status(t, e, codePayable, ""); s["open"] != 1 || len(s) != 1 {
			t.Fatalf("payable on %d = %v", e, s)
		}
	}
	if s := w.status(t, w.engA1.ID, codeTaxID, ""); s["open"] != 1 {
		t.Fatalf("tax id = %v", s)
	}
}
