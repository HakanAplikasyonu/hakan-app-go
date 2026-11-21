package reportrules

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func ip(n int) *int { return &n }

func on(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 7, 0, 0, 0, usecase.Istanbul)
}

func TestFireDate(t *testing.T) {
	cases := []struct {
		name string
		rule Rule
		at   time.Time
		want string
	}{
		{"15'i ara rapor", Rule{TriggerKind: TriggerDay, Day: ip(15)}, on(2026, 11, 3), "2026-11-15"},
		{"29'u, şubatta son gün", Rule{TriggerKind: TriggerDay, Day: ip(29)}, on(2027, 2, 1), "2027-02-28"},
		{"31'i, kasımda 30'u", Rule{TriggerKind: TriggerDay, Day: ip(31)}, on(2026, 11, 1), "2026-11-30"},
		{"ay sonundan 3 gün önce", Rule{TriggerKind: TriggerBeforeMonthEnd, DaysBefore: ip(3)}, on(2026, 11, 1), "2026-11-27"},
		{"ay sonu (0 gün önce)", Rule{TriggerKind: TriggerBeforeMonthEnd, DaysBefore: ip(0)}, on(2026, 12, 9), "2026-12-31"},
	}
	for _, c := range cases {
		if got := c.rule.FireDate(c.at).Format(time.DateOnly); got != c.want {
			t.Errorf("%s: FireDate = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestNextRun(t *testing.T) {
	r := Rule{TriggerKind: TriggerDay, Day: ip(15)}
	if got := r.NextRun(on(2026, 11, 15)).Format(time.DateOnly); got != "2026-11-15" {
		t.Errorf("same day: %s", got)
	}
	if got := r.NextRun(on(2026, 11, 16)).Format(time.DateOnly); got != "2026-12-15" {
		t.Errorf("after: %s", got)
	}
	end := Rule{TriggerKind: TriggerBeforeMonthEnd, DaysBefore: ip(2)}
	if got := end.NextRun(on(2026, 12, 30)).Format(time.DateOnly); got != "2027-01-29" {
		t.Errorf("before month end: %s", got)
	}
}

func TestValidate(t *testing.T) {
	g := "kdv-hassas"
	ok := []Rule{
		{TriggerKind: TriggerDay, Day: ip(29), Audience: AudienceOffice, Scope: ScopeAll},
		{TriggerKind: TriggerBeforeMonthEnd, DaysBefore: ip(3), Audience: AudienceTaxpayerApproval, Scope: ScopePayableOnly},
		{TriggerKind: TriggerDay, Day: ip(15), Audience: AudienceTaxpayerDirect, Scope: ScopeSelected, EngagementUUIDs: []uuid.UUID{uuid.New()}},
		{TriggerKind: TriggerDay, Day: ip(25), Audience: AudienceOffice, Scope: ScopeGroup, GroupRef: &g},
	}
	for i, r := range ok {
		if err := r.Validate(); err != nil {
			t.Errorf("ok %d: %v", i, err)
		}
	}
	bad := []Rule{
		{TriggerKind: TriggerDay, Day: ip(0), Audience: AudienceOffice, Scope: ScopeAll},
		{TriggerKind: TriggerDay, Day: ip(32), Audience: AudienceOffice, Scope: ScopeAll},
		{TriggerKind: TriggerDay, Day: ip(10), DaysBefore: ip(1), Audience: AudienceOffice, Scope: ScopeAll},
		{TriggerKind: TriggerBeforeMonthEnd, DaysBefore: ip(28), Audience: AudienceOffice, Scope: ScopeAll},
		{TriggerKind: "weekly", Audience: AudienceOffice, Scope: ScopeAll},
		{TriggerKind: TriggerDay, Day: ip(5), Audience: "everyone", Scope: ScopeAll},
		{TriggerKind: TriggerDay, Day: ip(5), Audience: AudienceOffice, Scope: ScopeSelected},
		{TriggerKind: TriggerDay, Day: ip(5), Audience: AudienceOffice, Scope: ScopeGroup},
	}
	for i, r := range bad {
		if err := r.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("bad %d: err = %v", i, err)
		}
	}
}

func TestInScopeAndKey(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	sel := Rule{Scope: ScopeSelected, EngagementUUIDs: []uuid.UUID{a}}
	if !sel.InScope(Target{Engagement: a}) || sel.InScope(Target{Engagement: b}) {
		t.Fatal("selected scope")
	}
	if !(Rule{Scope: ScopeAll}).InScope(Target{Engagement: b}) || !(Rule{Scope: ScopePayableOnly}).InScope(Target{Engagement: b}) {
		t.Fatal("all / payable_only scope")
	}
	if (Rule{Scope: ScopeGroup}).InScope(Target{Engagement: b}) {
		t.Fatal("group has no members yet")
	}
	if DefaultRule(29).Key() != "default" || DefaultRule(29).Trigger() != "day:29" {
		t.Fatal("default rule")
	}
	if (Rule{TriggerKind: TriggerBeforeMonthEnd, DaysBefore: ip(3)}).Trigger() != "before_month_end:3" {
		t.Fatal("trigger")
	}
}

func TestSummary(t *testing.T) {
	label := "Ara rapor"
	r := Rule{TriggerKind: TriggerDay, Day: ip(15), Audience: AudienceTaxpayerApproval, Label: &label}
	title, body := Summary(r, "2026-11", Result{Taxpayers: 12, Payable: 3}, decimal.RequireFromString("12450"))
	if title != "KDV raporu hazır: Kasım 2026 (Ara rapor)" {
		t.Errorf("title = %q", title)
	}
	for _, want := range []string{"12 mükellefin", "3 mükellefte ödeme çıkıyor", "12.450,00 TL", "onayınızı bekliyor"} {
		if !strings.Contains(body, want) {
			t.Errorf("body %q lacks %q", body, want)
		}
	}
	_, body = Summary(Rule{Audience: AudienceTaxpayerDirect}, "2026-11", Result{Taxpayers: 4, Notified: 3, Skipped: 1}, decimal.Zero)
	if !strings.Contains(body, "Hiçbirinde ödeme çıkmıyor") || !strings.Contains(body, "3 mükellefe bildirim gönderildi") || !strings.Contains(body, "1 mükellefin panel kullanıcısı") {
		t.Errorf("direct body = %q", body)
	}
}
