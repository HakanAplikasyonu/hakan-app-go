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
		// Another month: nothing but the Luca mizan, the VAT notice and the
		// report (#780).
		page, _ = f.svc.Workflow(ctx, v, Filter{Period: "2026-09"})
		if *page.Items[0].NextStep != StepLucaImport {
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

// #780: ay sonu kapanış — onay bekleyen masraflar, Luca'dan bordro / mizan,
// mizan farkı, KDV bildirimi; ofis sayaçları ve adım süzgeci.
func TestCloseChainDB(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.d.Pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	v := Viewer{OrganizationID: f.org, UserID: f.other, ReadAll: true, Has: allPerms}
	row := func(ctx context.Context, period string) Row {
		t.Helper()
		page, err := f.svc.Workflow(ctx, v, Filter{Period: period, Search: "Deniz"})
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("deniz: %+v %v", page, err)
		}
		return page.Items[0]
	}
	// Deniz, December: a pending expense and an employee.
	exec(`INSERT INTO expenses (taxpayer_id, source_system, source_id, capture_method, period)
	      VALUES ($1, 'manual', 'copilot-780', 'manual', '2026-12')`, f.tp2)
	exec(`INSERT INTO employees (taxpayer_id, full_name) VALUES ($1, 'Ayşe Demir')`, f.tp2)
	f.scoped(t, f.other, true, func(ctx context.Context) {
		r := row(ctx, "2026-12")
		if s := stepOf(t, r.Steps, StepJournals); s.Status != StatusTodo || actionOf(s) != ActionOpenExpenses || s.Count != 1 {
			t.Fatalf("expenses: %+v", s)
		}
		if s := stepOf(t, r.Steps, StepLucaImport); s.Status != StatusTodo || actionOf(s) != ActionOpenPayroll || s.Count != 2 {
			t.Fatalf("payroll: %+v", s)
		}
		if s := stepOf(t, r.Steps, StepVAT); s.Status != StatusTodo {
			t.Fatalf("vat: %+v", s)
		}
	})
	exec(`UPDATE expenses SET approval_status = 'approved' WHERE taxpayer_id = $1`, f.tp2)
	exec(`INSERT INTO payroll_runs (taxpayer_id, period) VALUES ($1, '2026-12')`, f.tp2)
	exec(`INSERT INTO account_plan_imports (organization_id, engagement_id, source_format, source_kind, sha256, period)
	      VALUES ($1, $2, 'xlsx', 'trial_balance', repeat('a', 64), '2026-12')`, f.org, f.eng2)
	exec(`INSERT INTO reconciliation_runs (organization_id, engagement_id, taxpayer_id, period, window_from, window_to, summary)
	      VALUES ($1, $2, $3, '2026-12', '2026-12', '2026-12', '{"differences":1,"missing_in_ledger":1}')`, f.org, f.eng2, f.tp2)
	f.scoped(t, f.other, true, func(ctx context.Context) {
		r := row(ctx, "2026-12")
		if s := stepOf(t, r.Steps, StepJournals); s.Status != StatusNA {
			t.Fatalf("expenses approved: %+v", s)
		}
		if s := stepOf(t, r.Steps, StepLucaImport); s.Status != StatusBlocked || actionOf(s) != ActionOpenReconciliation || s.Count != 2 {
			t.Fatalf("differences: %+v", s)
		}
	})
	exec(`INSERT INTO reconciliation_runs (organization_id, engagement_id, taxpayer_id, period, window_from, window_to, summary)
	      VALUES ($1, $2, $3, '2026-12', '2026-12', '2026-12', '{"differences":0}')`, f.org, f.eng2, f.tp2)
	exec(`INSERT INTO vat_notices (organization_id, engagement_id, taxpayer_id, period, outcome, output_vat, input_vat, carried_in, payable, carry_out)
	      VALUES ($1, $2, $3, '2026-12', 'no_payment', 0, 0, 0, 0, 0)`, f.org, f.eng2, f.tp2)
	f.scoped(t, f.other, true, func(ctx context.Context) {
		r := row(ctx, "2026-12")
		if s := stepOf(t, r.Steps, StepLucaImport); s.Status != StatusDone {
			t.Fatalf("luca done: %+v", s)
		}
		if s := stepOf(t, r.Steps, StepVAT); s.Status != StatusDone || s.Count != 1 {
			t.Fatalf("vat done: %+v", s)
		}
		if r.NextStep == nil || *r.NextStep != StepReport {
			t.Fatalf("next: %v", r.NextStep)
		}
		// Office counters: Atlas waits for its mizan, Deniz for its report.
		page, err := f.svc.Workflow(ctx, v, Filter{Period: "2026-12", Summary: true})
		if err != nil || page.Summary == nil || page.Summary.Scanned != 2 || page.Total != 2 {
			t.Fatalf("summary: %+v %v", page, err)
		}
		by := map[string]int64{}
		for _, c := range page.Summary.Steps {
			by[c.Key] = c.Count
		}
		if by[StepLucaImport] != 1 || by[StepReport] != 1 {
			t.Fatalf("counters: %+v", page.Summary.Steps)
		}
		page, err = f.svc.Workflow(ctx, v, Filter{Period: "2026-12", Step: StepReport})
		if err != nil || page.Total != 1 || page.Items[0].TaxpayerTitle != "Deniz Lojistik Ltd. Şti." || page.Summary != nil {
			t.Fatalf("step filter: %+v %v", page, err)
		}
		if _, err := f.svc.Workflow(ctx, v, Filter{Period: "2026-12", Step: "bogus"}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("bad step: %v", err)
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

// TestLedgerTargetsTransfersDB (Oğuz'un demo geri bildirimi): the chain read
// from the database follows the ledger type, names the open document
// request and ignores the failed transfer of a reversed export.
func TestLedgerTargetsTransfersDB(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	fixture := func(q string, args ...any) {
		t.Helper()
		if err := rls.System(ctx, f.d.Pool, "copilot test fixture", func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, q, args...)
			return err
		}); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	v := Viewer{OrganizationID: f.org, UserID: f.other, ReadAll: true, Has: allPerms}
	row := func(search, period string) Row {
		t.Helper()
		var out Row
		f.scoped(t, f.other, true, func(ctx context.Context) {
			page, err := f.svc.Workflow(ctx, v, Filter{Period: period, Search: search})
			if err != nil || len(page.Items) != 1 {
				t.Fatalf("%s: %+v %v", search, page, err)
			}
			out = page.Items[0]
		})
		return out
	}

	// Deniz keeps an işletme defteri and has an unassigned bank movement:
	// before the fix the next work was bank_suggest, which the API refuses
	// with LEDGER_NOT_BALANCE_SHEET.
	fixture(`UPDATE engagements SET ledger_type = 'isletme' WHERE id = $1`, f.eng2)
	fixture(`WITH box AS (INSERT INTO cash_boxes (taxpayer_id, name, currency) VALUES ($1, 'Ziraat', 'TRY') RETURNING id)
	         INSERT INTO bank_transactions (taxpayer_id, cash_box_id, source_system, source_id, capture_method, booked_at,
	             transaction_date, debit_credit, amount, currency, description, period)
	         SELECT $1, box.id, 'bank_test', 'copilot-m1', 'api', DATE '2026-08-10', DATE '2026-08-10', 'A', 100, 'TRY', 'x', '2026-08'
	         FROM box`, f.tp2)
	// Two open document requests: the older one is the step's target.
	fixture(`INSERT INTO document_tasks (organization_id, engagement_id, taxpayer_id, code, kind, period, status, title, note,
	             due_at, next_reminder_at, created_by)
	         VALUES ($1, $2, $3, 'DOC_REQUEST', 'document_request', '2026-08', 'open', 'Kira sözleşmesi', '', now() + interval '2 day', now(), $4),
	                ($1, $2, $3, 'DOC_REQUEST', 'document_request', '2026-08', 'open', 'Ağustos ekstresi', '', now() + interval '1 day', now(), $4)`,
		f.org, f.eng2, f.tp2, f.user)
	var first uuid.UUID
	if err := rls.System(ctx, f.d.Pool, "copilot test fixture", func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT uuid FROM document_tasks WHERE engagement_id = $1 AND title = 'Ağustos ekstresi'`, f.eng2).Scan(&first)
	}); err != nil {
		t.Fatal(err)
	}
	r := row("Deniz", "2026-08")
	if r.LedgerType != "isletme" {
		t.Fatalf("ledger type: %q", r.LedgerType)
	}
	// The statement is in: the bank step is done, with no account
	// suggestion offered.
	if s := stepOf(t, r.Steps, StepBank); s.Status != StatusDone || s.Count != 1 || s.Action != nil {
		t.Fatalf("işletme bank: %+v", s)
	}
	d := stepOf(t, r.Steps, StepDocuments)
	if d.Status != StatusTodo || d.Count != 2 || d.Target == nil || *d.Target != first {
		t.Fatalf("documents target: %+v (want %s)", d, first)
	}
	// A bank account on file but no statement for September: the step
	// asks for the statement (screen only).
	r = row("Deniz", "2026-09")
	if s := stepOf(t, r.Steps, StepBank); s.Status != StatusTodo || actionOf(s) != ActionOpenBank {
		t.Fatalf("işletme bank without statement: %+v", s)
	}
	// Back to bilanço: the same movement is bank work again.
	fixture(`UPDATE engagements SET ledger_type = 'bilanco' WHERE id = $1`, f.eng2)
	r = row("Deniz", "2026-08")
	if s := stepOf(t, r.Steps, StepBank); actionOf(s) != ActionBankSuggest || r.LedgerType != "bilanco" {
		t.Fatalf("bilanço bank: %+v %q", s, r.LedgerType)
	}

	// Atlas, July: the first export failed and was reversed; the entry was
	// exported again and imported. Every entry is in Luca: no blocker.
	fixture(`INSERT INTO journal_drafts (organization_id, engagement_id, taxpayer_id, period, entry_no, entry_date, status, approved_at)
	         VALUES ($1, $2, $3, '2026-07', 1, '2026-07-31', 'exported', NOW())`, f.org, f.eng1, f.tp1)
	fixture(`WITH d AS (SELECT id FROM journal_drafts WHERE engagement_id = $2 AND period = '2026-07'),
	         b1 AS (INSERT INTO export_batches (organization_id, engagement_id, taxpayer_id, driver, template_version, template_verified, period, created_by)
	                VALUES ($1, $2, $3, 'luca', 'v1', TRUE, '2026-07', $4) RETURNING id),
	         b2 AS (INSERT INTO export_batches (organization_id, engagement_id, taxpayer_id, driver, template_version, template_verified, period, created_by)
	                VALUES ($1, $2, $3, 'luca', 'v1', TRUE, '2026-07', $4) RETURNING id),
	         i1 AS (INSERT INTO export_batch_items (organization_id, engagement_id, export_batch_id, journal_draft_id, reversed_at, reversed_by, reversal_reason)
	                SELECT $1, $2, b1.id, d.id, NOW(), $4, 'Luca aktarımı başarısız' FROM b1, d),
	         i2 AS (INSERT INTO export_batch_items (organization_id, engagement_id, export_batch_id, journal_draft_id)
	                SELECT $1, $2, b2.id, d.id FROM b2, d),
	         t1 AS (INSERT INTO accounting_transfers (organization_id, engagement_id, export_batch_id, driver, status, requested_by)
	                SELECT $1, $2, b1.id, 'luca', 'failed', $4 FROM b1)
	         INSERT INTO accounting_transfers (organization_id, engagement_id, export_batch_id, driver, status, requested_by)
	         SELECT $1, $2, b2.id, 'luca', 'imported', $4 FROM b2`, f.org, f.eng1, f.tp1, f.user)
	r = row("Atlas", "2026-07")
	if s := stepOf(t, r.Steps, StepTransfer); s.Status != StatusDone || s.Count != 1 {
		t.Fatalf("reversed failed export still blocks: %+v", s)
	}
	if s := stepOf(t, r.Steps, StepJournals); s.Status != StatusDone {
		t.Fatalf("journals: %+v", s)
	}
}
