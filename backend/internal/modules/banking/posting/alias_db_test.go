package posting

import (
	"context"
	"errors"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

const payerIBAN = "TR290006200000006299001234"

type aliasFixture struct {
	d   *testdb.DB
	q   *db.Queries
	svc *Service
	sub Subject
}

func setupAliases(t *testing.T) aliasFixture {
	t.Helper()
	d := testdb.Open(t)
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", "9000000017")
	eng := d.Engagement(t, org, tp)
	return aliasFixture{d: d, q: db.New(d.Pool), svc: NewService(d.Pool), sub: Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}}
}

func TestAliasLearnResolveAndPromote(t *testing.T) {
	f := setupAliases(t)
	ctx := context.Background()
	payer := Counterparty{Name: "X Yazılım Teknolojileri A.Ş.", IBAN: payerIBAN, Direction: banking.DirIn}
	if m, err := Resolve(ctx, f.q, f.sub, payer); err != nil || m != nil {
		t.Fatalf("empty memory: %+v %v", m, err)
	}
	// The accountant books the payment to Oğuz Yılmaz's cari.
	if err := Learn(ctx, f.q, f.sub, payer, Decision{Correction: true, AccountCode: "120.01.015", AccountName: "Oğuz Yılmaz", SuggestionID: 0}); err != nil {
		t.Fatal(err)
	}
	m, err := Resolve(ctx, f.q, f.sub, payer)
	if err != nil || m == nil || m.Alias.AccountCode != "120.01.015" || m.Alias.MatchType != MatchIBAN {
		t.Fatalf("match %+v err %v", m, err)
	}
	// The same payer written differently, without IBAN: found by name.
	byName := Counterparty{Name: "X YAZILIM TEKNOLOJILERI AS", Direction: banking.DirIn}
	m, err = Resolve(ctx, f.q, f.sub, byName)
	if err != nil || m == nil || m.Alias.MatchType != MatchName || m.Confidence.String() != "0.85" {
		t.Fatalf("by name %+v err %v", m, err)
	}
	// An outgoing payment to the same firm is not this alias (direction).
	if m, _ := Resolve(ctx, f.q, f.sub, Counterparty{Name: payer.Name, Direction: banking.DirOut}); m != nil {
		t.Fatalf("direction ignored: %+v", m)
	}
	// Two more decisions: a rule candidate (office default: 3).
	for i := 0; i < 2; i++ {
		if err := Learn(ctx, f.q, f.sub, payer, Decision{AccountCode: "120.01.015"}); err != nil {
			t.Fatal(err)
		}
	}
	cs, err := f.svc.Candidates(ctx, f.sub)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[0].Decisions != 3 || cs[0].Question == "" { // IBAN and name aliases
		t.Fatalf("candidates %+v", cs)
	}
	// Office counts corrections only: 1 < 3, no candidates.
	if _, err := f.q.UpsertAccountRuleSettings(ctx, db.UpsertAccountRuleSettingsParams{
		OrganizationID: f.sub.OrganizationID, RuleCandidateMinCount: 3, RuleCandidateDecisions: "corrections_only"}); err != nil {
		t.Fatal(err)
	}
	if cs2, _ := f.svc.Candidates(ctx, f.sub); len(cs2) != 0 {
		t.Fatalf("corrections only: %+v", cs2)
	}
	a, err := f.svc.Promote(ctx, f.sub, cs[0].UUID, 0)
	if err != nil || a.Status != StatusRule || a.Origin != OriginCandidate {
		t.Fatalf("promote %+v %v", a, err)
	}
	m, _ = Resolve(ctx, f.q, f.sub, payer)
	if m.Confidence.String() != "0.97" {
		t.Fatalf("rule confidence %s", m.Confidence)
	}
	// A correction to another code demotes and restarts the count.
	if err := Learn(ctx, f.q, f.sub, payer, Decision{Correction: true, AccountCode: "120.01.020"}); err != nil {
		t.Fatal(err)
	}
	m, _ = Resolve(ctx, f.q, f.sub, payer)
	if m.Alias.AccountCode != "120.01.020" || m.Alias.Status != StatusLearned || m.Alias.Corrections != 1 || m.Alias.Accepts != 0 {
		t.Fatalf("after correction %+v", m.Alias)
	}
	// Another engagement of the office does not see the memory.
	tp2 := f.d.Taxpayer(t, "legal", "9000000025")
	other := Subject{OrganizationID: f.sub.OrganizationID, EngagementID: f.d.Engagement(t, f.sub.OrganizationID, tp2), TaxpayerID: tp2}
	if m, _ := Resolve(ctx, f.q, other, payer); m != nil {
		t.Fatalf("leaked to another engagement: %+v", m)
	}
}

func TestAliasTaxIDWritesCariMemoryAndUnlearn(t *testing.T) {
	f := setupAliases(t)
	ctx := context.Background()
	// A review suggestion row stands in for the decision (FK-free column).
	payer := Counterparty{Name: "Kalem Kırtasiye", TaxID: "9100000021", Direction: banking.DirOut}
	if err := Learn(ctx, f.q, f.sub, payer, Decision{AccountCode: "320.01.007", AccountName: "Kalem Kırtasiye", SuggestionID: 4242}); err != nil {
		t.Fatal(err)
	}
	got, err := f.q.GetCounterpartyAccount(ctx, db.GetCounterpartyAccountParams{OrganizationID: f.sub.OrganizationID,
		EngagementID: f.sub.EngagementID, CounterpartyTaxID: "9100000021", MainCode: "320"})
	if err != nil || got.AccountCode != "320.01.007" {
		t.Fatalf("cari memory %+v %v", got, err)
	}
	// Bank fees are not a cari: no counterparty_accounts row.
	if err := Learn(ctx, f.q, f.sub, Counterparty{TaxID: "9100000039", Direction: banking.DirOut}, Decision{AccountCode: "780.01"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.GetCounterpartyAccount(ctx, db.GetCounterpartyAccountParams{OrganizationID: f.sub.OrganizationID,
		EngagementID: f.sub.EngagementID, CounterpartyTaxID: "9100000039", MainCode: "780"}); err == nil {
		t.Fatal("780 must not reach the cari memory")
	}
	if err := Unlearn(ctx, f.q, f.sub, 4242); err != nil {
		t.Fatal(err)
	}
	if m, _ := Resolve(ctx, f.q, f.sub, payer); m != nil {
		t.Fatalf("unlearned alias still found: %+v", m.Alias)
	}
	if _, err := f.q.GetCounterpartyAccount(ctx, db.GetCounterpartyAccountParams{OrganizationID: f.sub.OrganizationID,
		EngagementID: f.sub.EngagementID, CounterpartyTaxID: "9100000021", MainCode: "320"}); err == nil {
		t.Fatal("cari memory not forgotten")
	}
}

func TestAliasManual(t *testing.T) {
	f := setupAliases(t)
	ctx := context.Background()
	bad := []AliasInput{
		{MatchType: "iban", MatchValue: "TR00", AccountCode: "120.01"},
		{MatchType: "tax_id", MatchValue: "123", AccountCode: "120.01"},
		{MatchType: "name", MatchValue: "A.Ş.", AccountCode: "120.01"},
		{MatchType: "phone", MatchValue: "x", AccountCode: "120.01"},
		{MatchType: "name", MatchValue: "Oğuz", AccountCode: "12"},
	}
	for i, in := range bad {
		if _, err := f.svc.CreateAlias(ctx, f.sub, in, 0); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%d: err = %v", i, err)
		}
	}
	dir := "A"
	a, err := f.svc.CreateAlias(ctx, f.sub, AliasInput{MatchType: "name", MatchValue: "X Yazılım Teknolojileri A.Ş.",
		Direction: &dir, AccountCode: "120.01.015", AccountName: "Oğuz Yılmaz"}, 0)
	if err != nil || a.Status != StatusRule || a.MatchValue != "X YAZILIM TEKNOLOJILERI" {
		t.Fatalf("alias %+v %v", a, err)
	}
	// Same key again: redefined, not duplicated.
	a2, err := f.svc.CreateAlias(ctx, f.sub, AliasInput{MatchType: "name", MatchValue: "x yazılım teknolojileri",
		Direction: &dir, AccountCode: "120.01.016"}, 0)
	if err != nil || a2.UUID != a.UUID || a2.AccountCode != "120.01.016" {
		t.Fatalf("redefine %+v %v", a2, err)
	}
	code, learned := "120.01.017", StatusLearned
	a3, err := f.svc.UpdateAlias(ctx, f.sub, a.UUID, AliasPatch{AccountCode: &code, Status: &learned}, 0)
	if err != nil || a3.AccountCode != code || a3.Status != StatusLearned {
		t.Fatalf("patch %+v %v", a3, err)
	}
	list, err := f.svc.Aliases(ctx, f.sub, "")
	if err != nil || len(list) != 1 {
		t.Fatalf("list %+v %v", list, err)
	}
	if err := f.svc.DeleteAlias(ctx, f.sub, a.UUID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteAlias(ctx, f.sub, a.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}
