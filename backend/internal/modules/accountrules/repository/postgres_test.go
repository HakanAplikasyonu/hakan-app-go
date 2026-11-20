package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/usecase"
	reviewrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/google/uuid"
)

type fx struct {
	d      *testdb.DB
	org    int64
	tp     int64
	eng    int64
	user   int64
	rules  *usecase.Service
	review *reviewusecase.Service
}

func setup(t *testing.T) fx {
	t.Helper()
	d := testdb.Open(t)
	q := db.New(d.Pool)
	review := reviewusecase.New(reviewrepo.New(d.Pool, q)).Register(usecase.Kind, Applier{})
	f := fx{d: d, org: d.Organization(t), user: d.User(t), review: review}
	f.rules = usecase.New(New(d.Pool, q), review)
	return f
}

func (f *fx) newEngagement(t *testing.T, taxID string) {
	t.Helper()
	f.tp = f.d.Taxpayer(t, "legal", taxID)
	f.eng = f.d.Engagement(t, f.org, f.tp)
}

func (f *fx) sub() usecase.Subject {
	return usecase.Subject{OrganizationID: f.org, EngagementID: f.eng, TaxpayerID: f.tp}
}

var seq int

// invoice inserts an incoming invoice from supplier with the given lines.
func (f *fx) invoice(t *testing.T, period, supplierTaxID, supplierName string, lines ...string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	seq++
	var custID int64
	err := f.d.Pool.QueryRow(ctx, `
		INSERT INTO customers (taxpayer_id, source_system, source_id, capture_method, name, tax_number)
		VALUES ($1, 'test', $2, 'manual', $3, $4)
		ON CONFLICT (taxpayer_id, tax_number) WHERE tax_number IS NOT NULL DO UPDATE SET name = EXCLUDED.name
		RETURNING id`, f.tp, "c"+supplierTaxID, supplierName, supplierTaxID).Scan(&custID)
	if err != nil {
		t.Fatal(err)
	}
	var invID int64
	var id uuid.UUID
	err = f.d.Pool.QueryRow(ctx, `
		INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number, direction, type,
		                      issue_date, period, customer_id, customer_tax_number, customer_name, currency, tax_summary_source)
		VALUES ($1, 'test', $2, 'manual', $7, 'incoming', 'SATIS', ($3::text || '-15')::date, $3, $4, $5, $6, 'TRY', 'items')
		RETURNING id, uuid`, f.tp, fmt.Sprintf("INV%04d", seq), period, custID, supplierTaxID, supplierName, fmt.Sprintf("INV%04d", seq)).Scan(&invID, &id)
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range lines {
		if _, err := f.d.Pool.Exec(ctx, `
			INSERT INTO invoice_items (taxpayer_id, invoice_id, line_no, name, tax_rate, line_type)
			VALUES ($1, $2, $3, $4, 20, 'service')`, f.tp, invID, i+1, name); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func find(t *testing.T, vs []usecase.SuggestionView, target, field string) usecase.SuggestionView {
	t.Helper()
	for _, v := range vs {
		if v.TargetType == target && v.TargetField == field {
			return v
		}
	}
	t.Fatalf("no %s.%s suggestion in %+v", target, field, vs)
	return usecase.SuggestionView{}
}

// TestTemplateVersionsAndSuggestions covers the #25 acceptance criteria on a
// real database: a new engagement receives the office template; a rule
// change creates a new version and leaves earlier assignments and the old
// version untouched; suggestions reach the review queue with confidence,
// rationale and provenance.
func TestTemplateVersionsAndSuggestions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// Office template: rent goes to 770.01.
	tmpl, err := f.rules.CreateRule(ctx, f.org, 0, usecase.RuleInput{
		Name: "Kira", Conditions: engine.Conditions{Direction: "incoming", ItemNameContains: []string{"kira"}}, AccountCode: "770.01",
	}, f.user)
	if err != nil {
		t.Fatal(err)
	}
	f.newEngagement(t, "1111111111")
	rules, err := f.rules.Rules(ctx, f.org, f.eng, false)
	if err != nil || len(rules) != 1 || rules[0].Origin != engine.OriginTemplate || rules[0].AccountCode != "770.01" || rules[0].Scope != "engagement" {
		t.Fatalf("copied rules = %+v %v", rules, err)
	}
	if n, err := f.rules.ApplyTemplate(ctx, f.org, f.eng); err != nil || n != 0 {
		t.Fatalf("re-apply copied %d %v", n, err)
	}
	_ = tmpl

	inv := f.invoice(t, "2026-10", "2222222222", "Mülk Sahibi", "Ofis kirası Ekim", "Temizlik hizmeti")
	out, err := f.rules.SuggestInvoice(ctx, f.sub(), inv)
	if err != nil {
		t.Fatal(err)
	}
	// 2 lines × (account + VAT) + counterparty.
	if len(out) != 5 {
		t.Fatalf("suggestions = %+v", out)
	}
	rent := out[0]
	if rent.AccountCode != "770.01" || rent.Status != string(reviewusecase.StatusAutoAccepted) || rent.RuleRef != fmt.Sprintf("account_rule:%d@v1", rules[0].ID) {
		t.Fatalf("rent = %+v", rent)
	}
	clean := out[2]
	if clean.AccountCode != "770" || clean.Status != string(reviewusecase.StatusPending) || clean.RuleRef != "global:line_service" {
		t.Fatalf("cleaning = %+v", clean)
	}
	cari := find(t, out, usecase.TargetCustomer, usecase.FieldAccount)
	if cari.AccountCode != "320.01.001" || cari.Status != string(reviewusecase.StatusPending) {
		t.Fatalf("cari = %+v", cari)
	}
	sug, decisions, err := f.review.Get(ctx, reviewusecase.Subject{OrganizationID: f.org, EngagementID: f.eng, TaxpayerID: f.tp}, rent.UUID)
	if err != nil || len(decisions) != 1 || sug.Input.Provenance.Origin != reviewusecase.OriginRule || sug.Input.Rationale == "" {
		t.Fatalf("review row = %+v %+v %v", sug, decisions, err)
	}

	// New version: 770.02. v1 is retired and immutable; the assignment made
	// by v1 keeps its code and its rule version.
	v2, err := f.rules.UpdateRule(ctx, f.org, f.eng, rules[0].Key, usecase.RuleInput{
		Name: "Kira", Conditions: engine.Conditions{Direction: "incoming", ItemNameContains: []string{"kira"}}, AccountCode: "770.02",
	}, f.user)
	if err != nil || v2.Version != 2 || v2.Key != rules[0].Key {
		t.Fatalf("v2 = %+v %v", v2, err)
	}
	var code string
	var ruleID int64
	if err := f.d.Pool.QueryRow(ctx, `SELECT account_code, rule_id FROM account_assignments
		WHERE engagement_id = $1 AND target_type = 'invoice_item' AND field = 'account_code' AND account_code LIKE '770%'`, f.eng).Scan(&code, &ruleID); err != nil {
		t.Fatal(err)
	}
	if code != "770.01" || ruleID != rules[0].ID {
		t.Fatalf("assignment after v2 = %s rule %d", code, ruleID)
	}
	if _, err := f.d.Pool.Exec(ctx, `UPDATE account_rules SET account_code = '770.09' WHERE id = $1`, rules[0].ID); err == nil {
		t.Fatal("a rule version must be immutable")
	}
	vs, err := f.rules.Versions(ctx, f.org, f.eng, rules[0].Key)
	if err != nil || len(vs) != 2 || vs[1].Status != "retired" || vs[1].AccountCode != "770.01" {
		t.Fatalf("versions = %+v %v", vs, err)
	}
	// The next invoice uses v2.
	out2, err := f.rules.SuggestInvoice(ctx, f.sub(), f.invoice(t, "2026-11", "2222222222", "Mülk Sahibi", "Kira Kasım"))
	if err != nil || out2[0].AccountCode != "770.02" {
		t.Fatalf("after v2 = %+v %v", out2, err)
	}
	// Office template is untouched by the engagement change.
	if office, _ := f.rules.Rules(ctx, f.org, 0, false); len(office) != 1 || office[0].AccountCode != "770.01" {
		t.Fatalf("office template = %+v", office)
	}
}

// TestRuleCandidateAfterThreeCorrections: the accountant corrects the same
// supplier to the same account three times → "kural yapılsın mı?"; the
// promoted rule then codes the next invoice and the candidate disappears.
// Accepted codes are also remembered for the counterparty (history).
func TestRuleCandidateAfterThreeCorrections(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.newEngagement(t, "3333333333")
	rsub := reviewusecase.Subject{OrganizationID: f.org, EngagementID: f.eng, TaxpayerID: f.tp}
	val := json.RawMessage(`{"account_code":"760.01"}`)
	for i, period := range []string{"2026-07", "2026-08", "2026-09"} {
		out, err := f.rules.SuggestInvoice(ctx, f.sub(), f.invoice(t, period, "4444444444", "Reklam Ajansı", "Reklam hizmeti"))
		if err != nil {
			t.Fatal(err)
		}
		line := out[0]
		if i > 0 && (line.RuleRef != "history:counterparty" || line.AccountCode != "760.01") {
			t.Fatalf("history suggestion %d = %+v", i, line)
		}
		if line.AccountCode == "760.01" {
			_, err = f.review.Accept(ctx, rsub, line.UUID, f.user, "")
		} else {
			_, err = f.review.Correct(ctx, rsub, line.UUID, f.user, val, "reklam 760")
		}
		if err != nil {
			t.Fatal(err)
		}
		cs, err := f.rules.Candidates(ctx, f.sub())
		if err != nil {
			t.Fatal(err)
		}
		if i < 2 && len(cs) != 0 {
			t.Fatalf("candidate too early after %d: %+v", i+1, cs)
		}
		if i == 2 && (len(cs) != 1 || cs[0].CounterpartyKey != "4444444444" || cs[0].AccountCode != "760.01" || cs[0].Decisions != 3) {
			t.Fatalf("candidates = %+v", cs)
		}
	}
	cs, _ := f.rules.Candidates(ctx, f.sub())
	r, err := f.rules.Promote(ctx, f.sub(), usecase.PromoteInput{
		CounterpartyKey: cs[0].CounterpartyKey, CounterpartyName: cs[0].CounterpartyName, Direction: cs[0].Direction, AccountCode: cs[0].AccountCode,
	}, f.user)
	if err != nil || r.Origin != engine.OriginCandidate {
		t.Fatalf("promote = %+v %v", r, err)
	}
	if cs, _ := f.rules.Candidates(ctx, f.sub()); len(cs) != 0 {
		t.Fatalf("candidate must be covered now: %+v", cs)
	}
	out, err := f.rules.SuggestInvoice(ctx, f.sub(), f.invoice(t, "2026-10", "4444444444", "Reklam Ajansı", "Reklam"))
	if err != nil || out[0].RuleRef != r.Ref() || out[0].Status != string(reviewusecase.StatusAutoAccepted) {
		t.Fatalf("after promote = %+v %v", out, err)
	}
	// Revert removes the assignment again.
	if _, err := f.review.Revert(ctx, rsub, out[0].UUID, f.user, "yanlış"); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = f.d.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM account_assignments WHERE suggestion_id IN (SELECT id FROM suggestions WHERE uuid = $1)`, out[0].UUID).Scan(&n)
	if n != 0 {
		t.Fatalf("assignment after revert = %d", n)
	}
}

// TestCounterpartyAccountFromPlan: the cari account is found by name in the
// target plan (#30) or proposed next to the existing sub-accounts.
func TestCounterpartyAccountFromPlan(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.newEngagement(t, "5555555555")
	_, err := f.d.Pool.Exec(ctx, `
		WITH imp AS (
			INSERT INTO account_plan_imports (organization_id, engagement_id, source_format, source_kind, sha256)
			VALUES ($1, $2, 'csv', 'account_plan', repeat('0', 64)) RETURNING id)
		INSERT INTO account_plan_accounts (organization_id, engagement_id, import_id, code, name, main_code, level, is_leaf)
		SELECT $1, $2, imp.id, v.code, v.name, '320', v.level, v.leaf FROM imp,
		(VALUES ('320', 'SATICILAR', 1, false), ('320.01', 'Yurtiçi', 2, false),
		        ('320.01.001', 'Elektrik Dağıtım', 3, true), ('320.01.002', 'Su İdaresi', 3, true)) AS v(code, name, level, leaf)`,
		f.org, f.eng)
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.rules.SuggestInvoice(ctx, f.sub(), f.invoice(t, "2026-10", "6666666666", "SU İDARESİ", "Su"))
	if err != nil {
		t.Fatal(err)
	}
	if c := find(t, out, usecase.TargetCustomer, usecase.FieldAccount); c.AccountCode != "320.01.002" || c.RuleRef != "global:counterparty_plan_match" {
		t.Fatalf("existing cari = %+v", c)
	}
	out, err = f.rules.SuggestInvoice(ctx, f.sub(), f.invoice(t, "2026-10", "7777777777", "Yeni Tedarikçi", "Malzeme"))
	if err != nil {
		t.Fatal(err)
	}
	if c := find(t, out, usecase.TargetCustomer, usecase.FieldAccount); c.AccountCode != "320.01.003" || c.RuleRef != "global:counterparty_open" {
		t.Fatalf("new cari = %+v", c)
	}
}

// TestMemoryIsPerEngagement is the #572 acceptance: what the accountant
// teaches for taxpayer A (a rule, a rule change, the codes it produces) is
// not seen at taxpayer B of the same office, and B's own rule does not
// reach A or the office template.
func TestMemoryIsPerEngagement(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	f.newEngagement(t, "1111111111")
	a := f
	f.newEngagement(t, "3333333333")
	b := f

	ruleA, err := a.rules.CreateRule(ctx, a.org, a.eng, usecase.RuleInput{
		Name: "Kira (A)", Conditions: engine.Conditions{Direction: "incoming", ItemNameContains: []string{"kira"}}, AccountCode: "770.01",
	}, a.user)
	if err != nil {
		t.Fatal(err)
	}
	if rules, err := b.rules.Rules(ctx, b.org, b.eng, true); err != nil || len(rules) != 0 {
		t.Fatalf("A's rule leaked to B: %+v %v", rules, err)
	}
	if office, err := a.rules.Rules(ctx, a.org, 0, true); err != nil || len(office) != 0 {
		t.Fatalf("A's rule leaked to the office template: %+v %v", office, err)
	}

	// The same rent line: A's rule codes it, B falls back to the global rule.
	outA, err := a.rules.SuggestInvoice(ctx, a.sub(), a.invoice(t, "2026-10", "2222222222", "Mülk Sahibi", "Ofis kirası Ekim"))
	if err != nil || outA[0].AccountCode != "770.01" || outA[0].RuleRef != fmt.Sprintf("account_rule:%d@v1", ruleA.ID) {
		t.Fatalf("A rent = %+v %v", outA, err)
	}
	outB, err := b.rules.SuggestInvoice(ctx, b.sub(), b.invoice(t, "2026-10", "2222222222", "Mülk Sahibi", "Ofis kirası Ekim"))
	if err != nil || outB[0].RuleRef == outA[0].RuleRef || outB[0].AccountCode == "770.01" {
		t.Fatalf("B used A's rule: %+v %v", outB, err)
	}

	// B's own rule and A's rule live side by side; changing one leaves the other.
	if _, err := b.rules.CreateRule(ctx, b.org, b.eng, usecase.RuleInput{
		Name: "Kira (B)", Conditions: engine.Conditions{Direction: "incoming", ItemNameContains: []string{"kira"}}, AccountCode: "760.05",
	}, b.user); err != nil {
		t.Fatal(err)
	}
	if _, err := a.rules.UpdateRule(ctx, a.org, a.eng, ruleA.Key, usecase.RuleInput{
		Name: "Kira (A)", Conditions: engine.Conditions{Direction: "incoming", ItemNameContains: []string{"kira"}}, AccountCode: "770.02",
	}, a.user); err != nil {
		t.Fatal(err)
	}
	rulesA, _ := a.rules.Rules(ctx, a.org, a.eng, false)
	rulesB, _ := b.rules.Rules(ctx, b.org, b.eng, false)
	if len(rulesA) != 1 || rulesA[0].AccountCode != "770.02" || len(rulesB) != 1 || rulesB[0].AccountCode != "760.05" {
		t.Fatalf("after changes A = %+v, B = %+v", rulesA, rulesB)
	}
	// The assignments (what was coded) are per engagement too.
	var n int
	if err := f.d.Pool.QueryRow(ctx, `SELECT count(*) FROM account_assignments WHERE engagement_id = $1 AND rule_id = $2`, b.eng, ruleA.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("B holds %d assignments of A's rule (%v)", n, err)
	}
}
