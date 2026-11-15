package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	aiusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/summaries/facts"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type fakeAI struct {
	answers []string
	calls   int
}

func (f *fakeAI) Enabled() bool { return true }
func (f *fakeAI) Complete(_ context.Context, _ aiusecase.Call) (aiusecase.Result, error) {
	a := f.answers[f.calls%len(f.answers)]
	f.calls++
	return aiusecase.Result{Model: "claude-test", Message: provider.Message{Role: provider.RoleAssistant, Content: []provider.Block{provider.TextBlock(a)}}}, nil
}

// TestEveryNumberMatchesPortalData is the #35 acceptance test.
func TestEveryNumberMatchesPortalData(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", fmt.Sprintf("%010d", 5000000000+testdb.Next()))
	eng := d.Engagement(t, org, tp)
	n := 0
	inv := func(dir, date, base, vat string) {
		n++
		if _, err := d.Pool.Exec(ctx, `INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number,
			direction, type, issue_date, period, currency, tax_summary_source, tax_exclusive_amount, tax_amount, payable_amount)
			VALUES ($1, 'test', $2::text, 'manual', $2::text, $3::text, 'SATIS', $4::date, to_char($4::date, 'YYYY-MM'), 'TRY', 'items',
			        $5::numeric, $6::numeric, $5::numeric + $6::numeric)`, tp, fmt.Sprintf("S-%d", n), dir, date, base, vat); err != nil {
			t.Fatal(err)
		}
	}
	inv("outgoing", "2026-09-05", "80000", "16000")
	inv("outgoing", "2026-10-03", "50000", "10000")
	inv("outgoing", "2026-10-20", "45250.75", "9050.15")
	inv("incoming", "2026-10-07", "30000", "6000")
	inv("incoming", "2026-09-07", "20000", "4000")
	if _, err := d.Pool.Exec(ctx, `INSERT INTO expenses (taxpayer_id, source_system, source_id, capture_method, expense_date, period, amount, approval_status)
		VALUES ($1, 'test', 'e1', 'manual', '2026-10-09', '2026-10', 1250.40, 'approved'),
		       ($1, 'test', 'e2', 'manual', '2026-10-10', '2026-10', 999, 'pending')`, tp); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, period, entity_type, code, severity, producer)
		VALUES ($1, $2, $3, '2026-10', 'period', 'BNK_GAP', 'blocker', 'control:test')`, org, eng, tp); err != nil {
		t.Fatal(err)
	}

	// The model answers once with a made-up number (rejected), then properly.
	ai := &fakeAI{answers: []string{
		"Satışlar yaklaşık 95 bin TL. {{F1}}",
		"Ekim döneminde {{F3}} satış faturasıyla {{F1}} satış matrahı ve {{F2}} hesaplanan KDV oluştu; satış matrahı önceki döneme göre {{F11}} değişti. Alışlarda {{F4}} matrah ve {{F5}} indirilecek KDV var; {{F7}} görünüyor. Onaylı masraflar {{F8}}. Açık {{F14}} engelleyici uyarı inceleniyor.",
	}}
	svc := New(db.New(d.Pool), ai, nil)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}
	v, err := svc.Generate(ctx, sc, 0, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if ai.calls != 2 || v.Model == nil || v.Status != "draft" {
		t.Fatalf("calls=%d view=%+v", ai.calls, v)
	}

	// Independent ground truth straight from SQL.
	truth := func(q string, args ...any) decimal.Decimal {
		var s string
		if err := d.Pool.QueryRow(ctx, q, args...).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return decimal.RequireFromString(s)
	}
	sumOut := truth(`SELECT sum(tax_exclusive_amount)::text FROM invoices WHERE taxpayer_id=$1 AND period='2026-10' AND direction='outgoing'`, tp)
	vatOut := truth(`SELECT sum(tax_amount)::text FROM invoices WHERE taxpayer_id=$1 AND period='2026-10' AND direction='outgoing'`, tp)
	sumIn := truth(`SELECT sum(tax_exclusive_amount)::text FROM invoices WHERE taxpayer_id=$1 AND period='2026-10' AND direction='incoming'`, tp)
	vatIn := truth(`SELECT sum(tax_amount)::text FROM invoices WHERE taxpayer_id=$1 AND period='2026-10' AND direction='incoming'`, tp)
	exp := truth(`SELECT sum(amount)::text FROM expenses WHERE taxpayer_id=$1 AND period='2026-10' AND approval_status='approved'`, tp)
	prevOut := truth(`SELECT sum(tax_exclusive_amount)::text FROM invoices WHERE taxpayer_id=$1 AND period='2026-09' AND direction='outgoing'`, tp)
	change := sumOut.Sub(prevOut).Div(prevOut).Mul(decimal.NewFromInt(100))
	want := []string{
		"2 satış faturasıyla",
		facts.Render(sumOut, facts.UnitTRY) + " satış matrahı",
		facts.Render(vatOut, facts.UnitTRY) + " hesaplanan KDV",
		facts.Render(change, facts.UnitPercent) + " değişti",
		facts.Render(sumIn, facts.UnitTRY) + " matrah",
		facts.Render(vatIn, facts.UnitTRY) + " indirilecek",
		facts.Render(vatOut.Sub(vatIn), facts.UnitTRY) + " görünüyor",
		facts.Render(exp, facts.UnitTRY) + ".",
		"Açık 1 engelleyici",
	}
	for _, w := range want {
		if !strings.Contains(v.Body, w) {
			t.Errorf("body lacks %q\nbody: %s", w, v.Body)
		}
	}
	if bad := facts.Verify(v.Body, v.Facts, v.Period); len(bad) != 0 {
		t.Fatalf("numbers without a fact: %v", bad)
	}
	t.Log(v.Body)

	// The accountant edits in a made-up number: approval is refused.
	if _, err := svc.Edit(ctx, sc, v.UUID, v.Body+" Gelecek ay 200.000 TL bekleniyor."); err != nil {
		t.Fatal(err)
	}
	var unmatched UnmatchedError
	if _, err := svc.Approve(ctx, sc, v.UUID, 0); !errors.As(err, &unmatched) || unmatched.Numbers[0] != "200.000" {
		t.Fatalf("approve with made-up number: %v", err)
	}
	if _, err := svc.Edit(ctx, sc, v.UUID, v.Body+" Ekim 2026 verileri kesinleşti."); err != nil {
		t.Fatal(err)
	}
	a, err := svc.Approve(ctx, sc, v.UUID, 0)
	if err != nil || a.Status != "approved" || !a.Edited {
		t.Fatalf("approve: %v %+v", err, a)
	}
	if _, err := svc.Edit(ctx, sc, v.UUID, "x"); !errors.Is(err, ErrState) {
		t.Fatalf("edit approved: %v", err)
	}

	// Without AI: the fixed template, also fully backed by facts.
	f, err := New(db.New(d.Pool), nil, nil).Generate(ctx, sc, 0, "2026-10")
	if err != nil || f.Model != nil {
		t.Fatalf("fallback: %v %+v", err, f)
	}
	if err := facts.CheckTemplate(f.Template, f.Facts); err != nil {
		t.Fatalf("fallback template: %v", err)
	}
	if bad := facts.Verify(f.Body, f.Facts, f.Period); len(bad) != 0 {
		t.Fatalf("fallback numbers: %v (%s)", bad, f.Body)
	}
	list, err := svc.List(ctx, sc, "2026-10")
	if err != nil || len(list) != 2 {
		t.Fatalf("list %d %v", len(list), err)
	}
}
