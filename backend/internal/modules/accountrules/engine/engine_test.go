package engine

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestPickPrefersPriorityThenSpecificityThenEngagement(t *testing.T) {
	t.Parallel()
	x := Context{Direction: "incoming", CounterpartyTaxID: "1234567890", CounterpartyName: "KİRA A.Ş.", ItemName: "Ofis kirası Ekim", LineType: "service", TaxRate: decimal.NewFromInt(20)}
	rules := append([]Rule{
		{ID: 1, Version: 1, Role: RoleLine, Priority: 100, Conditions: Conditions{Direction: "incoming"}, AccountCode: "770", Ref: "account_rule:1@v1"},
		{ID: 2, Version: 3, Role: RoleLine, Priority: 100, Conditions: Conditions{Direction: "incoming", ItemNameContains: []string{"KİRA"}}, AccountCode: "770.01", Ref: "account_rule:2@v3"},
		{ID: 3, Version: 1, Role: RoleLine, Priority: 100, Conditions: Conditions{CounterpartyTaxID: "9999999999"}, AccountCode: "760"},
	}, Defaults...)
	a, ok := Pick(rules, RoleLine, x)
	if !ok || a.AccountCode != "770.01" || a.RuleID != 2 || a.RuleRef != "account_rule:2@v3" {
		t.Fatalf("pick = %+v", a)
	}
	// A lower priority number wins over specificity.
	rules = append(rules, Rule{ID: 4, Role: RoleLine, Priority: 10, AccountCode: "740"})
	if a, _ := Pick(rules, RoleLine, x); a.AccountCode != "740" {
		t.Fatalf("priority: %+v", a)
	}
	// VAT and defaults.
	if a, _ := Pick(Defaults, RoleVAT, x); a.AccountCode != "191" {
		t.Fatalf("vat default = %+v", a)
	}
	if a, _ := Pick(Defaults, RoleLine, Context{Direction: "outgoing", InvoiceType: "IADE"}); a.AccountCode != "610" {
		t.Fatalf("sales return default = %+v", a)
	}
	if a, _ := Pick(Defaults, RoleLine, x); a.AccountCode != "770" || a.RuleID != 0 {
		t.Fatalf("service default = %+v", a)
	}
}

func TestConditionsValidate(t *testing.T) {
	t.Parallel()
	for _, c := range []Conditions{{Direction: "x"}, {CounterpartyTaxID: "12"}, {TaxRate: "yirmi"}} {
		if err := c.Validate(); err == nil {
			t.Errorf("%+v must be invalid", c)
		}
	}
	c := Conditions{ItemNameContains: []string{" kira ", ""}}
	if err := c.Validate(); err != nil || len(c.ItemNameContains) != 1 || c.ItemNameContains[0] != "kira" {
		t.Fatalf("keywords = %+v %v", c, err)
	}
}

func TestProposeCounterparty(t *testing.T) {
	t.Parallel()
	plan := []PlanAccount{
		{Code: "320", Name: "SATICILAR"},
		{Code: "320.01", Name: "Yurtiçi"},
		{Code: "320.01.001", Name: "Kira A.Ş.", IsLeaf: true},
		{Code: "320.01.009", Name: "Elektrik", IsLeaf: true},
		{Code: "320.02.01", Name: "Yurtdışı", IsLeaf: true},
	}
	if p := ProposeCounterparty("320", "KİRA A.Ş.", plan); !p.Existing || p.AccountCode != "320.01.001" {
		t.Fatalf("existing = %+v", p)
	}
	if p := ProposeCounterparty("320", "Yeni Tedarikçi", plan); p.Existing || p.AccountCode != "320.01.010" {
		t.Fatalf("new = %+v", p)
	}
	if p := ProposeCounterparty("120", "Alıcı", nil); p.AccountCode != "120.01.001" {
		t.Fatalf("empty plan = %+v", p)
	}
}

// #310: punctuation and spacing differences between the mizan and the
// invoice must not open a duplicate cari account.
func TestProposeCounterpartyIgnoresPunctuation(t *testing.T) {
	t.Parallel()
	plan := []PlanAccount{
		{Code: "320.01.002", Name: "DENİZ AMBALAJ LTD. ŞTİ.", IsLeaf: true},
		{Code: "120.01.006", Name: "KORKMAZ GIDA SAN. VE TİC. A.Ş.", IsLeaf: true},
	}
	for _, name := range []string{"Deniz Ambalaj Ltd.Şti.", "Deniz  Ambalaj LTD ŞTİ", "deniz ambalaj ltd. şti."} {
		if p := ProposeCounterparty("320", name, plan); !p.Existing || p.AccountCode != "320.01.002" {
			t.Errorf("%q = %+v, want 320.01.002", name, p)
		}
	}
	if p := ProposeCounterparty("120", "Korkmaz Gıda San. ve Tic. A.Ş.", plan); !p.Existing || p.AccountCode != "120.01.006" {
		t.Errorf("Korkmaz = %+v", p)
	}
	// Different words stay different (no fuzzy match).
	if p := ProposeCounterparty("320", "Deniz Ambalaj Sanayi A.Ş.", plan); p.Existing {
		t.Errorf("Deniz Ambalaj Sanayi matched %+v", p)
	}
}

func TestPickEngagementRuleBeatsTemplate(t *testing.T) {
	t.Parallel()
	x := Context{Direction: "incoming", CounterpartyTaxID: "2981056178", ItemName: "Koli"}
	rules := append([]Rule{
		// Office template copied into the engagement, a low priority number.
		{ID: 1, Role: RoleLine, Priority: 10, Origin: OriginTemplate, Conditions: Conditions{Direction: "incoming"}, AccountCode: "770"},
		// Promoted from the accountant's decisions, default priority 100.
		{ID: 2, Role: RoleLine, Priority: 100, Origin: OriginCandidate, Conditions: Conditions{CounterpartyTaxID: "2981056178"}, AccountCode: "150.01"},
	}, Defaults...)
	if a, _ := Pick(rules, RoleLine, x); a.AccountCode != "150.01" || a.RuleID != 2 {
		t.Fatalf("engagement rule must win over the template: %+v", a)
	}
	// Without an engagement rule the template still beats the defaults.
	if a, _ := Pick(append([]Rule{rules[0]}, Defaults...), RoleLine, x); a.AccountCode != "770" {
		t.Fatalf("template over default: %+v", a)
	}
}

func TestNameKeyAbbreviations(t *testing.T) {
	t.Parallel()
	same := [][2]string{
		{"KORKMAZ GIDA SAN. VE TİC. A.Ş.", "Korkmaz Gıda Sanayi ve Ticaret Anonim Şirketi"},
		{"DENİZ AMBALAJ LTD. ŞTİ.", "Deniz Ambalaj Limited Şirketi"},
		{"Deniz Ambalaj Ltd.Şti.", "DENIZ AMBALAJ LTD STI"},
		{"Yeni Lojistik Taşımacılık A.S.", "YENİ LOJİSTİK TAŞIMACILIK AŞ"},
		{"Atlas İnş. Paz. Tic. Ltd. Şti.", "ATLAS İNŞAAT PAZARLAMA TİCARET LİMİTED ŞİRKETİ"},
	}
	for _, p := range same {
		if NameKey(p[0]) != NameKey(p[1]) {
			t.Errorf("%q / %q: %q != %q", p[0], p[1], NameKey(p[0]), NameKey(p[1]))
		}
	}
	if NameKey("Deniz Ambalaj Ltd. Şti.") == NameKey("Deniz Kağıt Ltd. Şti.") {
		t.Error("different words must not match")
	}
}

func TestVATSubAccount(t *testing.T) {
	t.Parallel()
	plan := []PlanAccount{
		{Code: "191", Name: "İNDİRİLECEK KDV"},
		{Code: "191.01", Name: "İNDİRİLECEK KDV %20", IsLeaf: true},
		{Code: "191.02", Name: "İndirilecek KDV % 10", IsLeaf: true},
		{Code: "191.03", Name: "KDV 1%", IsLeaf: true},
		{Code: "191.09", Name: "Diğer KDV", IsLeaf: true},
	}
	for rate, want := range map[int64]string{20: "191.01", 10: "191.02", 1: "191.03"} {
		a, ok := VATSubAccount("191", decimal.NewFromInt(rate), plan)
		if !ok || a.Code != want {
			t.Errorf("rate %d: %+v %v", rate, a, ok)
		}
	}
	if _, ok := VATSubAccount("191", decimal.NewFromInt(8), plan); ok {
		t.Error("no sub-account for 8%: stay on the main account")
	}
	// Two leaves with the same rate: ambiguous, stay on the main account.
	dup := append(plan, PlanAccount{Code: "191.04", Name: "KDV %20 (ithalat)", IsLeaf: true})
	if _, ok := VATSubAccount("191", decimal.NewFromInt(20), dup); ok {
		t.Error("ambiguous rate must not map")
	}
	if r, ok := RateInName("Hesaplanan KDV yüzde 10"); !ok || !r.Equal(decimal.NewFromInt(10)) {
		t.Errorf("yüzde: %v %v", r, ok)
	}
}
