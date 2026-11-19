package copilot

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	aiusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type fixture struct {
	d                *testdb.DB
	org, user, other int64
	tp1, tp2         int64
	eng1, eng2       int64
	eng1UUID         uuid.UUID
	svc              *Service
}

func setup(t *testing.T) fixture {
	t.Helper()
	d := testdb.Open(t)
	ctx := context.Background()
	f := fixture{d: d, org: d.Organization(t), user: d.User(t), other: d.User(t)}
	f.tp1 = d.Taxpayer(t, "legal", fmt.Sprintf("%010d", 6100000000+testdb.Next()))
	f.tp2 = d.Taxpayer(t, "legal", fmt.Sprintf("%010d", 6100000000+testdb.Next()))
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.Pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`UPDATE taxpayers SET title = 'Atlas Gıda Sanayi A.Ş.' WHERE id = $1`, f.tp1)
	exec(`UPDATE taxpayers SET title = 'Deniz Lojistik Ltd. Şti.' WHERE id = $1`, f.tp2)
	f.eng1, f.eng2 = d.Engagement(t, f.org, f.tp1), d.Engagement(t, f.org, f.tp2)
	d.Member(t, f.org, f.user, "accountant")
	d.Member(t, f.org, f.other, "accountant")
	d.Assign(t, f.org, f.eng1, f.user)
	if err := d.Pool.QueryRow(ctx, `SELECT uuid FROM engagements WHERE id = $1`, f.eng1).Scan(&f.eng1UUID); err != nil {
		t.Fatal(err)
	}
	// Atlas, October: two drafts to approve, one approved, one pending
	// suggestion and a failed report email.
	for i, st := range []string{"draft", "draft", "approved"} {
		exec(`INSERT INTO journal_drafts (organization_id, engagement_id, taxpayer_id, period, entry_no, entry_date, status, approved_at)
		      VALUES ($1, $2, $3, '2026-10', $4, '2026-10-31', $5, CASE WHEN $5 = 'draft' THEN NULL ELSE NOW() END)`,
			f.org, f.eng1, f.tp1, i+1, st)
	}
	exec(`INSERT INTO suggestions (organization_id, engagement_id, taxpayer_id, kind, target_type, period, proposed_value, confidence, rationale, origin)
	      VALUES ($1, $2, $3, 'account_code', 'invoice_line', '2026-10', '"770"', 0.5, 'test', 'input')`, f.org, f.eng1, f.tp1)
	exec(`INSERT INTO report_deliveries (organization_id, engagement_id, taxpayer_id, kind, period, recipient, status, report)
	      VALUES ($1, $2, $3, 'month', '2026-10', 'a@b.c', 'failed', '{}')`, f.org, f.eng1, f.tp1)
	f.svc = NewService(database.QueriesFor(d.Pool))
	f.svc.now = func() time.Time { return time.Date(2026, 11, 17, 10, 0, 0, 0, istanbul) }
	return f
}

// scoped runs fn as the request would: the office scope of user with the
// taxpayers visible to them.
func (f fixture) scoped(t *testing.T, user int64, readAll bool, fn func(ctx context.Context)) {
	t.Helper()
	tps := []int64{f.tp1}
	if readAll {
		tps = append(tps, f.tp2)
	}
	err := rls.WithScope(context.Background(), f.d.Pool, rls.Scope{OrganizationID: f.org, UserID: user, TaxpayerIDs: tps, EngagementsReadAll: readAll},
		func(ctx context.Context, _ pgx.Tx) error {
			fn(ctx)
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowDB(t *testing.T) {
	f := setup(t)
	v := Viewer{OrganizationID: f.org, UserID: f.user, Has: allPerms}
	f.scoped(t, f.user, false, func(ctx context.Context) {
		page, err := f.svc.Workflow(ctx, v, Filter{})
		if err != nil {
			t.Fatal(err)
		}
		// Only the assigned engagement; the default period is last month.
		if page.Period != "2026-10" || page.Total != 1 || len(page.Items) != 1 || page.Items[0].EngagementUUID != f.eng1UUID {
			t.Fatalf("page: %+v", page)
		}
		r := page.Items[0]
		if s := stepOf(t, r.Steps, StepReview); s.Status != StatusTodo || s.Count != 1 {
			t.Fatalf("review: %+v", s)
		}
		if s := stepOf(t, r.Steps, StepJournals); actionOf(s) != ActionJournalsApprove || s.Count != 2 {
			t.Fatalf("journals: %+v", s)
		}
		if s := stepOf(t, r.Steps, StepTransfer); actionOf(s) != ActionExportTransfer || s.Count != 1 {
			t.Fatalf("transfer: %+v", s)
		}
		// Fişler open: the failed report waits for them (#562).
		if s := stepOf(t, r.Steps, StepReport); s.Status != StatusWaiting {
			t.Fatalf("report: %+v", s)
		}
		if r.NextStep == nil || *r.NextStep != StepReview {
			t.Fatalf("next: %v", r.NextStep)
		}
		// Another month has nothing but the report.
		page, _ = f.svc.Workflow(ctx, v, Filter{Period: "2026-09"})
		if *page.Items[0].NextStep != StepReport {
			t.Fatalf("september: %+v", page.Items[0])
		}
		if _, err := f.svc.Workflow(ctx, v, Filter{Period: "2026-13"}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("bad period: %v", err)
		}
	})
	f.scoped(t, f.other, true, func(ctx context.Context) {
		page, err := f.svc.Workflow(ctx, Viewer{OrganizationID: f.org, UserID: f.other, ReadAll: true}, Filter{Search: "deniz"})
		if err != nil || page.Total != 1 || page.Items[0].TaxpayerTitle != "Deniz Lojistik Ltd. Şti." {
			t.Fatalf("read all + search: %+v %v", page, err)
		}
	})
	// An unassigned member without read_all sees nothing.
	f.scoped(t, f.other, false, func(ctx context.Context) {
		page, err := f.svc.Workflow(ctx, Viewer{OrganizationID: f.org, UserID: f.other}, Filter{})
		if err != nil || page.Total != 0 {
			t.Fatalf("unassigned: %+v %v", page, err)
		}
	})
}

func TestPlansDB(t *testing.T) {
	f := setup(t)
	v := Viewer{OrganizationID: f.org, UserID: f.user, Has: allPerms}
	plans := NewPlans(f.svc, nil, nil)
	var id uuid.UUID
	f.scoped(t, f.user, false, func(ctx context.Context) {
		pl, err := plans.Create(ctx, v, "Atlas Gıda ekim fişlerini Luca'ya aktar", "")
		if err != nil {
			t.Fatal(err)
		}
		if pl.Status != PlanProposed || pl.Planner != PlannerRules || pl.Period == nil || *pl.Period != "2026-10" || len(pl.Steps) != 2 ||
			pl.Steps[0].Action != ActionJournalsApprove || pl.Steps[1].Action != ActionExportTransfer || pl.Steps[1].Driver != "luca" ||
			pl.Steps[1].Index != 1 || pl.Steps[0].EngagementUUID != f.eng1UUID {
			t.Fatalf("plan: %+v", pl)
		}
		id = pl.UUID
		// Deniz is not reachable for this user: not found.
		pl, err = plans.Create(ctx, v, "Deniz Lojistik raporunu gönder", "2026-10")
		if err != nil || len(pl.Steps) != 0 || pl.Status != PlanDone || pl.Notes[0].Code != NoteEngagementNotFound {
			t.Fatalf("foreign: %+v %v", pl, err)
		}
		if _, err := plans.Create(ctx, v, "  ", ""); !errors.Is(err, ErrInvalid) {
			t.Fatalf("empty: %v", err)
		}
		pl, err = plans.RecordStep(ctx, v, id, 0, StepResult{Status: StepOK})
		if err != nil || pl.Status != PlanRunning || pl.Steps[0].Status != StepOK || pl.Steps[0].FinishedAt == nil {
			t.Fatalf("record: %+v %v", pl, err)
		}
		if _, err := plans.RecordStep(ctx, v, id, 0, StepResult{Status: StepFailed}); !errors.Is(err, ErrState) {
			t.Fatalf("again: %v", err)
		}
		if _, err := plans.RecordStep(ctx, v, id, 9, StepResult{Status: StepOK}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("index: %v", err)
		}
		if _, err := plans.RecordStep(ctx, v, id, 1, StepResult{Status: "maybe"}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("status: %v", err)
		}
		pl, err = plans.Cancel(ctx, v, id)
		if err != nil || pl.Status != PlanCancelled || pl.Steps[1].Status != StepSkipped {
			t.Fatalf("cancel: %+v %v", pl, err)
		}
		list, err := plans.List(ctx, v, 10)
		if err != nil || len(list) != 2 {
			t.Fatalf("list: %d %v", len(list), err)
		}
	})
	// Another member of the office does not see the user's plans.
	f.scoped(t, f.other, true, func(ctx context.Context) {
		ov := Viewer{OrganizationID: f.org, UserID: f.other, ReadAll: true}
		if _, err := plans.Get(ctx, ov, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("other user: %v", err)
		}
		if list, _ := plans.List(ctx, ov, 10); len(list) != 0 {
			t.Fatalf("other list: %d", len(list))
		}
	})
	// Another office does not see the plan even with the same user.
	other := f.d.Organization(t)
	err := rls.WithScope(context.Background(), f.d.Pool, rls.Scope{OrganizationID: other, UserID: f.user}, func(ctx context.Context, _ pgx.Tx) error {
		if _, err := plans.Get(ctx, Viewer{OrganizationID: f.org, UserID: f.user}, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("other office (RLS): %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestAIPlannerDB: the model reads the engagements, hands in a plan with
// propose_plan, and the plan is validated: a step on an engagement the user
// does not reach is dropped.
func TestAIPlannerDB(t *testing.T) {
	f := setup(t)
	fake := &provider.Fake{}
	ai := aiusecase.New(&aiusecase.Backend{Provider: fake, Model: "test-model"}, nil, nil, aiusecase.Options{})
	plans := NewPlans(f.svc, NewAIPlanner(ai), nil)
	fake.Responses = []provider.Response{
		provider.FakeToolCall("t1", "list_engagements", map[string]any{"search": "Atlas"}),
		provider.FakeToolCall("t2", "engagement_status", map[string]any{"engagement_uuid": f.eng1UUID.String(), "period": "2026-10"}),
		provider.FakeToolCall("t3", "propose_plan", map[string]any{
			"period": "2026-10",
			"steps": []map[string]any{
				{"action": ActionJournalsApprove, "engagement_uuid": f.eng1UUID.String()},
				{"action": ActionExportTransfer, "engagement_uuid": f.eng1UUID.String(), "driver": "luca"},
				{"action": ActionReportSend, "engagement_uuid": uuid.NewString()},
			},
			"message": "Önce iki taslak fiş onaylanmalı, sonra Luca'ya aktarılır.",
		}),
		provider.FakeText("Plan hazır."),
	}
	scope := orgctx.Scope{ID: f.org, Permissions: []string{rbac.PermTenantEngagementsRead}}
	v := Viewer{OrganizationID: f.org, UserID: f.user, Has: allPerms}
	f.scoped(t, f.user, false, func(ctx context.Context) {
		pl, err := plans.Create(orgctx.WithScope(ctx, scope), v, "Atlas Gıda ekim fişlerini Luca'ya aktar", "")
		if err != nil {
			t.Fatal(err)
		}
		if pl.Planner != PlannerAI || len(pl.Steps) != 2 || pl.Steps[1].Driver != "luca" {
			t.Fatalf("plan: %+v", pl)
		}
		codes := map[string]bool{}
		for _, n := range pl.Notes {
			codes[n.Code] = true
		}
		if !codes[NoteAssistant] || !codes[NoteEngagementNotFound] {
			t.Fatalf("notes: %+v", pl.Notes)
		}
	})
	if fake.RequestCount() != 4 {
		t.Fatalf("calls: %d", fake.RequestCount())
	}
	// A provider error falls back to the rules.
	fake.Err = errors.New("down")
	f.scoped(t, f.user, false, func(ctx context.Context) {
		pl, err := plans.Create(orgctx.WithScope(ctx, scope), v, "Atlas ekim raporunu gönder", "")
		if err != nil || pl.Planner != PlannerRules || len(pl.Steps) != 1 || pl.Notes[0].Code != NoteAIUnavailable {
			t.Fatalf("fallback: %+v %v", pl, err)
		}
	})
}
