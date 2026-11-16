package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/usecase"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
)

// decide builds one correction and two acceptances of 760.01 for one
// supplier, as in TestRuleCandidateAfterThreeCorrections.
func (f *fx) decide(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	rsub := reviewusecase.Subject{OrganizationID: f.org, EngagementID: f.eng, TaxpayerID: f.tp}
	val := json.RawMessage(`{"account_code":"760.01"}`)
	for _, period := range []string{"2026-07", "2026-08", "2026-09"} {
		out, err := f.rules.SuggestInvoice(ctx, f.sub(), f.invoice(t, period, "4444444444", "Reklam Ajansı", "Reklam hizmeti"))
		if err != nil {
			t.Fatal(err)
		}
		line := out[0]
		if line.AccountCode == "760.01" {
			_, err = f.review.Accept(ctx, rsub, line.UUID, f.user, "")
		} else {
			_, err = f.review.Correct(ctx, rsub, line.UUID, f.user, val, "reklam 760")
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestRuleCandidateSettings (issue #333, user decision of 16 November): the
// office's minimum count and decision kinds drive the candidate list; the
// preview compares today's and a proposed setting without storing it;
// invalid settings are refused.
func TestRuleCandidateSettings(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.newEngagement(t, "3333333333")
	f.decide(t) // 1 correction + 2 manual acceptances

	set, err := f.rules.RuleSettings(ctx, f.org)
	if err != nil || !set.Default || set.MinCount != 3 || set.Decisions != usecase.DecisionsCorrectionsAndAccepts {
		t.Fatalf("default settings = %+v %v", set, err)
	}
	if cs, err := f.rules.Candidates(ctx, f.sub()); err != nil || len(cs) != 1 {
		t.Fatalf("default candidates = %+v %v", cs, err)
	}

	// Preview: corrections only (1 here) never reaches 2.
	pv, err := f.rules.PreviewRuleSettings(ctx, f.org, usecase.RuleSettings{MinCount: 2, Decisions: usecase.DecisionsCorrectionsOnly})
	if err != nil || pv.Current.Candidates != 1 || pv.Current.Engagements != 1 || pv.Proposed.Candidates != 0 {
		t.Fatalf("preview = %+v %v", pv, err)
	}
	if s2, _ := f.rules.RuleSettings(ctx, f.org); !s2.Default {
		t.Fatalf("preview stored the setting: %+v", s2)
	}
	// A lower count with acceptances still counted keeps the candidate.
	if pv, err = f.rules.PreviewRuleSettings(ctx, f.org, usecase.RuleSettings{MinCount: 2, Decisions: usecase.DecisionsCorrectionsAndAccepts}); err != nil || pv.Proposed.Candidates != 1 {
		t.Fatalf("preview 2 / both = %+v %v", pv, err)
	}

	saved, err := f.rules.SaveRuleSettings(ctx, f.org, usecase.RuleSettings{MinCount: 2, Decisions: usecase.DecisionsCorrectionsOnly}, f.user)
	if err != nil || saved.MinCount != 2 || saved.Decisions != usecase.DecisionsCorrectionsOnly || saved.Default {
		t.Fatalf("save = %+v %v", saved, err)
	}
	if cs, err := f.rules.Candidates(ctx, f.sub()); err != nil || len(cs) != 0 {
		t.Fatalf("candidates under corrections_only / 2 = %+v %v", cs, err)
	}
	// Another office keeps the default.
	other := f.d.Organization(t)
	if s, _ := f.rules.RuleSettings(ctx, other); !s.Default {
		t.Fatalf("other office = %+v", s)
	}

	for _, bad := range []usecase.RuleSettings{
		{MinCount: 1, Decisions: usecase.DecisionsCorrectionsOnly},
		{MinCount: 11, Decisions: usecase.DecisionsCorrectionsOnly},
		{MinCount: 3, Decisions: "auto_accepts"},
	} {
		if _, err := f.rules.SaveRuleSettings(ctx, f.org, bad, f.user); !errors.Is(err, usecase.ErrInvalidSettings) {
			t.Errorf("save %+v: %v", bad, err)
		}
		if _, err := f.rules.PreviewRuleSettings(ctx, f.org, bad); !errors.Is(err, usecase.ErrInvalidSettings) {
			t.Errorf("preview %+v: %v", bad, err)
		}
	}
}

// TestRulePreview (issue #333): an unsaved rule's fit over the last 6
// months — documents and lines matched, approved ones, approved with a
// different code — for one engagement and office-wide; invalid rules are
// refused; nothing is stored.
func TestRulePreview(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.newEngagement(t, "3333333333")
	f.decide(t)                                                    // three invoices of 4444444444, lines approved as 760.01
	f.invoice(t, "2026-09", "5555555555", "Kırtasiye", "Kalem")    // another supplier, no decision
	f.invoice(t, "2025-01", "4444444444", "Reklam Ajansı", "Eski") // older than 6 months
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

	rule := usecase.RuleInput{Role: engine.RoleLine, AccountCode: "770.01",
		Conditions: engine.Conditions{CounterpartyTaxID: "4444444444"}}
	p, err := f.rules.PreviewRule(ctx, f.org, f.eng, rule, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Months != 6 || p.Documents != 3 || p.Lines != 3 || p.Approved != 3 || p.ApprovedDifferent != 3 || len(p.Samples) != 3 || p.Samples[0].ApprovedCode != "760.01" {
		t.Fatalf("preview 770.01 = %+v", p)
	}
	rule.AccountCode = "760.01"
	if p, err = f.rules.PreviewRule(ctx, f.org, f.eng, rule, now); err != nil || p.Approved != 3 || p.ApprovedDifferent != 0 {
		t.Fatalf("preview 760.01 = %+v %v", p, err)
	}
	// Office-wide (template rule): the same, over every active engagement.
	if p, err = f.rules.PreviewRule(ctx, f.org, 0, rule, now); err != nil || p.Documents != 3 || p.Lines != 3 {
		t.Fatalf("office preview = %+v %v", p, err)
	}
	// Item name condition: the stationery line only.
	kalem := usecase.RuleInput{Role: engine.RoleLine, AccountCode: "770.01", Conditions: engine.Conditions{ItemNameContains: []string{"kalem"}}}
	if p, err = f.rules.PreviewRule(ctx, f.org, f.eng, kalem, now); err != nil || p.Lines != 1 || p.Approved != 0 {
		t.Fatalf("preview kalem = %+v %v", p, err)
	}
	// Counterparty rule: one decision per invoice (no customer decision yet).
	cp := usecase.RuleInput{Role: engine.RoleCounterparty, AccountCode: "320.01", Conditions: engine.Conditions{CounterpartyTaxID: "4444444444"}}
	if p, err = f.rules.PreviewRule(ctx, f.org, f.eng, cp, now); err != nil || p.Documents != 3 || p.Lines != 3 {
		t.Fatalf("preview counterparty = %+v %v", p, err)
	}
	if _, err := f.rules.PreviewRule(ctx, f.org, f.eng, usecase.RuleInput{AccountCode: "x"}, now); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("invalid rule: %v", err)
	}
	if rules, _ := f.rules.Rules(ctx, f.org, f.eng, true); len(rules) != 0 {
		t.Fatalf("preview stored rules: %+v", rules)
	}
}
