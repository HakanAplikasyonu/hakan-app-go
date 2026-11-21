package automation

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/communication"
)

func at(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, Istanbul) }

// Ayın birden çok günü (15'i ve 29'u): yalnız o günlerde, saat geldikten
// sonra çalışır; her gün ayrı anahtar alır.
func TestMatchCalendarMultipleDays(t *testing.T) {
	tr := Trigger{Mode: ModeDays, Days: []int{15, 29}, Hour: 9, Tax: TaxKDV}
	if _, ok := MatchCalendar(tr, at(2026, 11, 15, 8)); ok {
		t.Fatal("before the hour must not fire")
	}
	a, ok := MatchCalendar(tr, at(2026, 11, 15, 9))
	if !ok || a.Period != "2026-10" || a.RunKey != "cal:2026-11:d15" {
		t.Fatalf("15th = %+v %v", a, ok)
	}
	if !a.Due.Equal(date(2026, 11, 30)) { // 28 Kasım 2026 cumartesi → pazartesi 30
		t.Fatalf("due = %v", a.Due)
	}
	b, ok := MatchCalendar(tr, at(2026, 11, 29, 17))
	if !ok || b.RunKey == a.RunKey {
		t.Fatalf("29th = %+v %v", b, ok)
	}
	if _, ok := MatchCalendar(tr, at(2026, 11, 16, 12)); ok {
		t.Fatal("16th must not fire")
	}
	// Aynı gün ikinci tik aynı anahtarı üretir: tekil anahtar ikinci
	// gönderimi engeller.
	again, _ := MatchCalendar(tr, at(2026, 11, 15, 23))
	if again.RunKey != a.RunKey {
		t.Fatalf("same day, other key: %s vs %s", again.RunKey, a.RunKey)
	}
}

// 31 ayın son günüdür; 30 günlük ayda 30 ve 31 birlikte seçiliyse tek
// çalışma olur.
func TestMatchCalendarLastDay(t *testing.T) {
	tr := Trigger{Mode: ModeDays, Days: []int{30, 31}, Hour: 0}
	occ, ok := MatchCalendar(tr, at(2026, 11, 30, 10))
	if !ok || occ.RunKey != "cal:2026-11:d30" {
		t.Fatalf("30 Nov = %+v %v", occ, ok)
	}
	if occ, ok := MatchCalendar(Trigger{Mode: ModeDays, Days: []int{31}}, at(2027, 2, 28, 10)); !ok || occ.RunKey != "cal:2027-02:d28" {
		t.Fatalf("28 Feb = %+v %v", occ, ok)
	}
}

// Geçici vergi duyurusu yalnız beyan aylarında (mayıs, ağustos, kasım) ve
// son çeyreğin dönemiyle çalışır.
func TestMatchCalendarGecici(t *testing.T) {
	tr := Trigger{Mode: ModeDays, Days: []int{1}, Hour: 9, Tax: TaxGecici}
	if _, ok := MatchCalendar(tr, at(2026, 10, 1, 10)); ok {
		t.Fatal("October is not a filing month")
	}
	occ, ok := MatchCalendar(tr, at(2026, 11, 1, 10))
	if !ok || occ.Period != "2026-09" || !occ.Due.Equal(date(2026, 11, 17)) {
		t.Fatalf("November = %+v %v", occ, ok)
	}
	if FormatPeriod(TaxGecici, occ.Period) != "2026 3. dönem" {
		t.Fatalf("period label = %s", FormatPeriod(TaxGecici, occ.Period))
	}
}

// "KDV son gününden 1 gün önce": son gün hafta sonuna denk gelirse
// pazartesiye kayar, hatırlatma da pazar gününe.
func TestMatchCalendarBeforeDue(t *testing.T) {
	tr := Trigger{Mode: ModeBeforeDue, BeforeDueDays: 1, Hour: 10, Tax: TaxKDV}
	// Ekim 2026 KDV'si: 28 Kasım cumartesi → 30 Kasım pazartesi.
	occ, ok := MatchCalendar(tr, at(2026, 11, 29, 10))
	if !ok || occ.Period != "2026-10" || occ.RunKey != "due:kdv:2026-10" {
		t.Fatalf("before due = %+v %v", occ, ok)
	}
	if _, ok := MatchCalendar(tr, at(2026, 11, 27, 10)); ok {
		t.Fatal("27th is not one day before the shifted due date")
	}
	// Eylül 2026 KDV'si: 28 Ekim çarşamba; 27 Ekim.
	if occ, ok := MatchCalendar(tr, at(2026, 10, 27, 11)); !ok || occ.Period != "2026-09" {
		t.Fatalf("October = %+v %v", occ, ok)
	}
	// Ocak 2026 KDV'si: 28 Şubat 2026 cumartesi → 2 Mart; hatırlatma 1 Mart.
	if occ, ok := MatchCalendar(tr, at(2026, 3, 1, 12)); !ok || occ.Period != "2026-01" {
		t.Fatalf("over the month = %+v %v", occ, ok)
	}
	m := Trigger{Mode: ModeBeforeDue, BeforeDueDays: 3, Hour: 9, Tax: TaxMuhtasar}
	// Ekim 2026 muhtasarı: 26 Kasım perşembe; 3 gün önce 23 Kasım.
	if occ, ok := MatchCalendar(m, at(2026, 11, 23, 9)); !ok || occ.RunKey != "due:muhtasar:2026-10" {
		t.Fatalf("muhtasar = %+v %v", occ, ok)
	}
}

func TestMatchPayment(t *testing.T) {
	tr := Trigger{PaymentDate: "2026-12-31", PaymentDaysBefore: 2, Hour: 9, PaymentAmount: "1500"}
	occ, ok := MatchPayment(tr, at(2026, 12, 29, 9))
	if !ok || occ.RunKey != "pay:2026-12-31" || occ.Period != "2026-12" {
		t.Fatalf("payment = %+v %v", occ, ok)
	}
	if _, ok := MatchPayment(tr, at(2026, 12, 30, 9)); ok {
		t.Fatal("wrong day")
	}
}

// Olay anahtarı: dedupe_key varsa o (aynı mükellef + dönem tek çalışma),
// yoksa olay ve kaynak kayıt.
func TestEventRunKey(t *testing.T) {
	a := EventRunKey(EventVATStatusChanged, "vat.status_changed:e1:2026-11:n1", "ev-1")
	b := EventRunKey(EventVATStatusChanged, "vat.status_changed:e1:2026-11:n1", "ev-2")
	if a != b {
		t.Fatalf("same dedupe key, different run keys: %s %s", a, b)
	}
	if EventRunKey(EventReturnFiled, "", "r1") != "evt:tax_return.filed:r1" {
		t.Fatal("fallback key")
	}
}

func TestRenderVariables(t *testing.T) {
	got := Render("Sayın :mukellef, :donem :beyan tutarı :tutar, son gün :son_gun. :ofis", map[string]string{
		"mukellef": "Atlas Ltd.", "donem": "Ekim 2026", "beyan": "KDV", "tutar": FormatAmount("12450.5"),
		"son_gun": FormatDate(date(2026, 11, 30)), "ofis": "Demo SMMM",
	})
	want := "Sayın Atlas Ltd., Ekim 2026 KDV tutarı 12.450,50 TL, son gün 30 Kasım 2026. Demo SMMM"
	if got != want {
		t.Fatalf("render = %q", got)
	}
	// Değeri olmayan değişken boşalır, boşluk katlanır; saat "10:00" bozulmaz.
	if got := Render("Banka :banka ekstresi saat 10:00", map[string]string{}); got != "Banka ekstresi saat 10:00" {
		t.Fatalf("missing var = %q", got)
	}
	if FormatAmount("1234567") != "1.234.567,00 TL" || FormatAmount("") != "" {
		t.Fatal("amount format")
	}
}

// SMS uzunluğu: TR kodlamada ç ğ ı ş Ğ İ Ş iki karakter, operatör eki 5;
// tek SMS 155, çoklu 148.
func TestSMSLength(t *testing.T) {
	l := communication.SMSLength("çğış", "TR")
	if l.Chars != 5+8 || l.Parts != 1 {
		t.Fatalf("turkish chars = %+v", l)
	}
	l = communication.SMSLength(strings.Repeat("a", 150), "TR")
	if l.Chars != 155 || l.Parts != 1 {
		t.Fatalf("single = %+v", l)
	}
	l = communication.SMSLength(strings.Repeat("a", 151), "TR")
	if l.Chars != 156 || l.Parts != 2 {
		t.Fatalf("multi = %+v", l)
	}
	// Önizleme örnek mükellefle sayar: "Örnek Gıda Ticaret Ltd. Şti."
	// (28 harf; ı ve Ş ikişer) + operatör eki = 35.
	if p := SMSPreviewLength(":mukellef"); p.Chars != 35 || p.Parts != 1 {
		t.Fatalf("preview = %+v", p)
	}
}

func valid() Input {
	return Input{Name: "28'i KDV", TriggerKind: TriggerCalendar,
		Trigger:  Trigger{Mode: ModeDays, Days: []int{28}, Hour: 9, Tax: TaxKDV},
		Channels: []string{ChannelSMS}, Body: "Sayın :mukellef, KDV son günü :son_gun."}
}

func TestValidate(t *testing.T) {
	in := valid()
	if err := Validate(&in); err != nil {
		t.Fatalf("valid: %v", err)
	}
	if in.ApprovalMode != ApprovalAuto || in.TargetKind != TargetAll {
		t.Fatalf("defaults = %+v", in)
	}
	cases := map[string]func(*Input){
		"campaign":      func(i *Input) { i.Body = "Bu aya özel %20 indirim" },
		"whatsapp":      func(i *Input) { i.Channels = []string{"whatsapp"} },
		"no channel":    func(i *Input) { i.Channels = nil },
		"email no subj": func(i *Input) { i.Channels = []string{ChannelEmail} },
		"bad day":       func(i *Input) { i.Trigger.Days = []int{32} },
		"before no tax": func(i *Input) { i.Trigger = Trigger{Mode: ModeBeforeDue, BeforeDueDays: 1} },
		"soon event":    func(i *Input) { i.TriggerKind, i.Trigger = TriggerEvent, Trigger{Event: EventMonthlyReport} },
		"resend non pay": func(i *Input) {
			i.TriggerKind, i.Trigger = TriggerEvent, Trigger{Event: EventReturnChecked, ResendAfterDays: 3}
		},
		"group TODO":     func(i *Input) { i.TargetKind = TargetGroup },
		"selected empty": func(i *Input) { i.TargetKind = TargetSelected },
		"unopened no mail": func(i *Input) {
			h := 4
			i.SMSIfUnopenedHours = &h
		},
		"payment not one": func(i *Input) {
			i.TriggerKind, i.Trigger = TriggerPayment, Trigger{PaymentDate: "2026-12-31"}
		},
	}
	for name, mut := range cases {
		in := valid()
		mut(&in)
		err := Validate(&in)
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if name == "campaign" && !errors.Is(err, communication.ErrCampaignText) {
			t.Errorf("campaign: %v", err)
		}
	}
	in = valid()
	in.TriggerKind, in.Trigger = TriggerEvent, Trigger{Event: EventVATStatusChanged, DelayMinHours: 2, DelayMaxHours: 48, ResendAfterDays: 5}
	in.Channels, in.Subject = []string{ChannelSMS, ChannelEmail}, "KDV durumu"
	h := 6
	in.SMSIfUnopenedHours = &h
	if err := Validate(&in); err != nil {
		t.Fatalf("event rule: %v", err)
	}
}

// Hazır kurallar geçerli kurallardır (kampanya dili yok, uzunluk sınırda).
func TestTemplatesValid(t *testing.T) {
	codes := map[string]bool{}
	for _, tp := range Templates {
		if codes[tp.Code] {
			t.Fatalf("duplicate template %s", tp.Code)
		}
		codes[tp.Code] = true
		in := Input{Name: tp.Name, TriggerKind: tp.TriggerKind, Trigger: tp.Trigger, OnlyDue: tp.OnlyDue,
			Channels: tp.Channels, Subject: tp.Subject, Body: tp.Body, ApprovalMode: tp.ApprovalMode}
		if err := Validate(&in); err != nil {
			t.Errorf("template %s: %v", tp.Code, err)
		}
	}
	if len(Templates) != 7 {
		t.Fatalf("templates = %d", len(Templates))
	}
}
