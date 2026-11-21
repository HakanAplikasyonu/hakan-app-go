package automation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/communication"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/events"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type fakeNotifier struct{ n int }

func (f *fakeNotifier) Enqueue(_ context.Context, _ notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	f.n++
	return nil, nil
}

type fixture struct {
	d      *testdb.DB
	svc    *Service
	org    int64
	eng    int64
	engID  uuid.UUID
	now    time.Time
	notify *fakeNotifier
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	d := testdb.Open(t)
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", "1234567890")
	eng := d.Engagement(t, org, tp)
	if _, err := d.Pool.Exec(ctx, `UPDATE taxpayers SET contact_phone = '0532 123 45 67', contact_email = 'mukellef@example.test' WHERE id = $1`, tp); err != nil {
		t.Fatal(err)
	}
	// NetGSM hesabı deneme modunda: kayıt yazılır, hiçbir istek gitmez.
	if _, err := d.Pool.Exec(ctx, `INSERT INTO office_sms_settings (organization_id, enabled, test_mode, usercode, msgheader)
		VALUES ($1, TRUE, TRUE, '8501234567', 'OFISADI')`, org); err != nil {
		t.Fatal(err)
	}
	var engID uuid.UUID
	if err := d.Pool.QueryRow(ctx, `SELECT uuid FROM engagements WHERE id = $1`, eng).Scan(&engID); err != nil {
		t.Fatal(err)
	}
	comm := communication.NewService(d.Pool, nil, communication.Options{BaseURL: "http://127.0.0.1:1", PerMinute: 6000})
	f := &fixture{d: d, org: org, eng: eng, engID: engID, notify: &fakeNotifier{},
		now: time.Date(2026, 11, 28, 10, 0, 0, 0, Istanbul)}
	f.svc = New(d.Pool, comm, f.notify, "https://app.example.test", nil).WithClock(func() time.Time { return f.now })
	return f
}

func (f *fixture) runs(t *testing.T, status string) int {
	t.Helper()
	var n int
	if err := f.d.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM automation_runs WHERE organization_id = $1 AND ($2 = '' OR status = $2)`, f.org, status).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Hazır kurallar pasif kurulur, ikinci listede çoğalmaz; silinen geri gelmez.
func TestTemplatesInstalledPassive(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	ov, err := f.svc.Overview(ctx, f.org)
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.Items) != len(Templates) || ov.Stats.Active != 0 || !ov.Enabled {
		t.Fatalf("overview = %d items, %+v, enabled=%v", len(ov.Items), ov.Stats, ov.Enabled)
	}
	if err := f.svc.Delete(ctx, f.org, 0, ov.Items[0].UUID); err != nil {
		t.Fatal(err)
	}
	ov, err = f.svc.Overview(ctx, f.org)
	if err != nil || len(ov.Items) != len(Templates)-1 {
		t.Fatalf("after delete = %d, %v", len(ov.Items), err)
	}
}

// Takvim kuralı: aynı gün iki tik tek çalışma yazar (kural + mükellef +
// dönem tekil), deneme modunda SMS kaydı düşer, gönderim sayısı artar,
// kural geçmişi tutulur. Kampanya dili reddedilir.
func TestCalendarRuleRunsOncePerPeriod(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	if _, err := f.svc.Create(ctx, f.org, 0, Input{Name: "Kampanya", TriggerKind: TriggerCalendar,
		Trigger: Trigger{Mode: ModeDays, Days: []int{28}}, Channels: []string{ChannelSMS}, Body: "Kampanya: %20 indirim"}); !errors.Is(err, communication.ErrCampaignText) {
		t.Fatalf("campaign accepted: %v", err)
	}
	rule, err := f.svc.Create(ctx, f.org, 0, Input{Name: "28'i KDV", TriggerKind: TriggerCalendar,
		Trigger:  Trigger{Mode: ModeDays, Days: []int{28}, Hour: 9, Tax: TaxKDV},
		Channels: []string{ChannelSMS, ChannelInapp}, Body: "Sayın :mukellef, :donem KDV son günü :son_gun. :ofis", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := f.svc.Tick(ctx, f.d.Pool); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.runs(t, RunSent); n != 1 || f.runs(t, "") != 1 {
		t.Fatalf("runs sent=%d total=%d", n, f.runs(t, ""))
	}
	var body, status string
	if err := f.d.Pool.QueryRow(ctx, `SELECT body, status FROM sms_messages WHERE organization_id = $1`, f.org).Scan(&body, &status); err != nil {
		t.Fatal(err)
	}
	if status != communication.StatusDryRun || body != "Sayın Mükellef 1234567890, Ekim 2026 KDV son günü 30 Kasım 2026. Ofis "+orgSuffix(t, f)+"\n" {
		t.Fatalf("sms = %q %s", body, status)
	}
	h, err := f.svc.RuleHistory(ctx, f.org, rule.UUID)
	if err != nil || len(h.Runs) != 1 || len(h.Edits) != 1 || h.Rule.SendCount != 1 {
		t.Fatalf("history = %+v, %v", h, err)
	}
	// Uygulama içi kanal: mükellefin kullanıcısı yok → o kanal başarısız,
	// SMS gittiği için çalışma gönderildi sayılır.
	var res map[string]channelResult
	_ = json.Unmarshal(h.Runs[0].Result, &res)
	if res[ChannelInapp].Status != communication.StatusFailed || res[ChannelSMS].Status != communication.StatusDryRun {
		t.Fatalf("result = %+v", res)
	}
	// Ertesi gün: gün tutmuyor, yeni çalışma yok.
	f.now = f.now.Add(24 * time.Hour)
	if _, err := f.svc.Tick(ctx, f.d.Pool); err != nil {
		t.Fatal(err)
	}
	if f.runs(t, "") != 1 {
		t.Fatalf("next day wrote a run")
	}
}

func orgSuffix(t *testing.T, f *fixture) string {
	t.Helper()
	var name string
	if err := f.d.Pool.QueryRow(context.Background(), `SELECT name FROM organizations WHERE id = $1`, f.org).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name[len("Ofis "):]
}

// Ana anahtar kapalıyken kural çalışmaz; "yalnız ödemesi olanlar" ödemesi
// olmayanı atlar.
func TestMasterSwitchAndOnlyDue(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	if _, err := f.svc.SetEnabled(ctx, f.org, 0, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Create(ctx, f.org, 0, Input{Name: "KDV ödeme", TriggerKind: TriggerCalendar, OnlyDue: true,
		Trigger:  Trigger{Mode: ModeDays, Days: []int{28}, Tax: TaxKDV},
		Channels: []string{ChannelSMS}, Body: "KDV :tutar", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Tick(ctx, f.d.Pool); err != nil {
		t.Fatal(err)
	}
	if f.runs(t, "") != 0 {
		t.Fatal("ran while switched off")
	}
	if _, err := f.svc.SetEnabled(ctx, f.org, 0, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Tick(ctx, f.d.Pool); err != nil {
		t.Fatal(err)
	}
	if f.runs(t, RunSkipped) != 1 {
		t.Fatalf("no payable must be skipped: %d", f.runs(t, RunSkipped))
	}
}

// Taslak modu: çalışma Bekleyen taslaklar'a düşer; ofis onaylayınca gider,
// ikinci onay bir şey yapmaz.
func TestDraftApproval(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	if _, err := f.svc.Create(ctx, f.org, 0, Input{Name: "KDV raporu", TriggerKind: TriggerCalendar, ApprovalMode: ApprovalDraft,
		Trigger:  Trigger{Mode: ModeDays, Days: []int{28}, Tax: TaxKDV, CurrentPeriod: true},
		Channels: []string{ChannelEmail}, Subject: ":donem KDV raporu", Body: "Sayın :mukellef, rapor hazır: :link", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Tick(ctx, f.d.Pool); err != nil {
		t.Fatal(err)
	}
	drafts, err := f.svc.Runs(ctx, f.org, RunFilter{Status: RunDraft})
	if err != nil || len(drafts) != 1 || drafts[0].Subject != "Kasım 2026 KDV raporu" {
		t.Fatalf("drafts = %+v, %v", drafts, err)
	}
	res, err := f.svc.Decide(ctx, f.org, 0, []uuid.UUID{drafts[0].UUID}, true)
	if err != nil || res.Sent != 1 {
		t.Fatalf("approve = %+v, %v", res, err)
	}
	res, err = f.svc.Decide(ctx, f.org, 0, []uuid.UUID{drafts[0].UUID}, false)
	if err != nil || res.Rejected != 0 {
		t.Fatalf("second decision = %+v, %v", res, err)
	}
	var n int
	if err := f.d.Pool.QueryRow(ctx, `SELECT count(*) FROM mail_messages WHERE organization_id = $1 AND source_rule LIKE 'automation:%'`, f.org).Scan(&n); err != nil || n != 1 {
		t.Fatalf("mail records = %d, %v", n, err)
	}
}

// KDV olayı: aynı dedupe_key ile iki kez gelse de tek çalışma yazılır.
func TestVATEventDedupe(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	if _, err := f.svc.Create(ctx, f.org, 0, Input{Name: "KDV durumu", TriggerKind: TriggerEvent,
		Trigger:  Trigger{Event: EventVATStatusChanged},
		Channels: []string{ChannelSMS}, Body: "Sayın :mukellef, :donem KDV ödemeniz :tutar, son gün :son_gun.", Active: true}); err != nil {
		t.Fatal(err)
	}
	org := f.org
	for i := 0; i < 2; i++ {
		e := events.New(EventVATStatusChanged).WithOrganization(org).WithPayload(map[string]any{
			"engagement_uuid": f.engID.String(), "taxpayer_title": "Demo Gıda Ltd. Şti.", "period": "2026-11",
			"payable": "12450.00", "due_date": "2026-12-28",
			"dedupe_key": "vat.status_changed:" + f.engID.String() + ":2026-11:n1",
		})
		e.OccurredAt = f.now
		if err := f.svc.HandleEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if f.runs(t, "") != 1 {
		t.Fatalf("runs = %d", f.runs(t, ""))
	}
	if _, err := f.svc.Tick(ctx, f.d.Pool); err != nil {
		t.Fatal(err)
	}
	var body string
	if err := f.d.Pool.QueryRow(ctx, `SELECT body FROM sms_messages WHERE organization_id = $1`, f.org).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if body != "Sayın Demo Gıda Ltd. Şti., Kasım 2026 KDV ödemeniz 12.450,00 TL, son gün 28 Aralık 2026.\n" {
		t.Fatalf("sms = %q", body)
	}
}
