package doctasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/communication"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
)

// TestDocumentRequestChannels (Oğuz'un geri bildirimi): a request sent on
// push, SMS and e-mail leaves one record per channel in the taxpayer's
// communication history; "send now" reminds on the chosen channels; the
// scheduled reminder goes out N days before due, once; with the office
// setting off the old behaviour returns.
func TestDocumentRequestChannels(t *testing.T) {
	w := setupWorld(t)
	ctx := context.Background()
	org := w.scope.OrganizationID
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := w.d.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.d.Pool.QueryRow(ctx, `SELECT uuid FROM engagements WHERE id = $1`, w.scope.ID).Scan(&w.scope.UUID); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE taxpayers SET contact_phone = '5321234567', contact_email = 'muhasebe@ornek.com.tr' WHERE id = $1`, w.taxpayerID)
	// SMS açık, deneme modunda: kayıt yazılır, NetGSM'e gitmez.
	exec(`INSERT INTO office_sms_settings (organization_id, enabled, test_mode, msgheader) VALUES ($1, TRUE, TRUE, 'OFISADI')`, org)
	comm := communication.NewService(w.d.Pool, nil, communication.Options{})
	w.svc.WithChannels(CommunicationChannels{Svc: comm})

	count := func(table, rule, ref string) int {
		t.Helper()
		var n int
		if err := w.d.Pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE organization_id = $1 AND engagement_id = $2
			AND source_rule = $3 AND source_ref = $4`, org, w.scope.ID, rule, ref).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	if _, err := w.svc.CreateRequest(ctx, w.scope, 0, RequestInput{Title: "Kira", DueDate: "2026-11-20", Channels: []string{"fax"}}); !errors.Is(err, ErrBadInput) {
		t.Fatalf("unknown channel: %v", err)
	}
	two := 2
	task, err := w.svc.CreateRequest(ctx, w.scope, 0, RequestInput{Title: "Kira kontratı", DueDate: "2026-11-20",
		Channels: []string{"email", "push", "sms", "push"}, RemindDaysBefore: &two})
	if err != nil {
		t.Fatal(err)
	}
	if len(task.Channels) != 3 || task.Channels[0] != ChannelPush || task.NextReminderAt != nil || task.RemindDaysBefore == nil || *task.RemindDaysBefore != 2 {
		t.Fatalf("created: %+v", task)
	}
	if task.RemindAt == nil || task.RemindAt.In(istanbul).Format("2006-01-02 15:04") != "2026-11-18 10:00" {
		t.Fatalf("remind at = %v", task.RemindAt)
	}
	ref := task.UUID.String()
	for _, table := range []string{"push_messages", "sms_messages", "mail_messages"} {
		if n := count(table, SourceRequest, ref); n != 1 {
			t.Fatalf("%s records = %d", table, n)
		}
	}
	want := map[string]string{ChannelPush: "sent", ChannelSMS: "dry_run", ChannelEmail: "dry_run"}
	if len(task.Deliveries) != 3 {
		t.Fatalf("deliveries: %+v", task.Deliveries)
	}
	for _, d := range task.Deliveries {
		if want[d.Channel] != d.Status || d.Kind != "request" {
			t.Fatalf("delivery %+v", d)
		}
	}
	// Push yalnız uygulama bildirimi (e-posta kanalı ayrı kayıt).
	last := w.notif.got[len(w.notif.got)-1]
	if len(last.Channels) != 1 || last.Channels[0] != notifmodel.ChannelInapp || *last.UserID != w.member {
		t.Fatalf("push notification: %+v", last)
	}
	// İletişim geçmişi üç kanalı da gösterir.
	hist, err := comm.History(ctx, org, w.scope.ID, "", 50)
	if err != nil || len(hist) != 3 {
		t.Fatalf("history: %+v %v", hist, err)
	}
	// Detay ve liste kanal durumunu okur.
	got, err := w.svc.Get(ctx, w.scope, task.UUID)
	if err != nil || len(got.Deliveries) != 3 {
		t.Fatalf("get deliveries: %+v %v", got.Deliveries, err)
	}

	// Şimdi hatırlat: yalnız SMS; hemen ikincisi reddedilir.
	sent, err := w.svc.Send(ctx, w.scope, 0, task.UUID, SendInput{Channels: []string{"sms"}})
	if err != nil {
		t.Fatal(err)
	}
	if count("sms_messages", SourceReminder, ref) != 1 || sent.ChannelRemindedAt == nil || *sent.ReminderCount != 1 {
		t.Fatalf("send now: %+v", sent)
	}
	if _, err := w.svc.Send(ctx, w.scope, 0, task.UUID, SendInput{}); !errors.Is(err, ErrRecentlySent) {
		t.Fatalf("second send: %v", err)
	}

	// Zamanlanmış hatırlatma: 18 Kasım 10:00'dan sonra bir kez, üç kanaldan.
	later := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	w.clock = &later
	rep, err := w.svc.SendReminders(ctx)
	if err != nil || rep.Notified != 3 {
		t.Fatalf("reminders: %+v %v", rep, err)
	}
	if count("push_messages", SourceReminder, ref) != 1 || count("mail_messages", SourceReminder, ref) != 1 || count("sms_messages", SourceReminder, ref) != 2 {
		t.Fatal("scheduled reminder records missing")
	}
	again, err := w.svc.SendReminders(ctx)
	if err != nil || again.Notified != 0 {
		t.Fatalf("second run: %+v %v", again, err)
	}
	after, err := w.svc.Get(ctx, w.scope, task.UUID)
	if err != nil || after.RemindAt != nil {
		t.Fatalf("after reminder: %+v %v", after, err)
	}

	// Ofis ayarı kapalı: gönder 409, yeni talep eski davranışla açılır.
	exec(`UPDATE organizations SET settings = '{"doc_request_channels": false}' WHERE id = $1`, org)
	if _, err := w.svc.Send(ctx, w.scope, 0, task.UUID, SendInput{}); !errors.Is(err, ErrChannelsDisabled) {
		t.Fatalf("send with channels off: %v", err)
	}
	before := w.notif.count(TemplateRequested)
	old, err := w.svc.CreateRequest(ctx, w.scope, 0, RequestInput{Title: "Kasa defteri", DueDate: "2026-11-25", Channels: []string{"sms"}, RemindDaysBefore: &two})
	if err != nil {
		t.Fatal(err)
	}
	if len(old.Channels) != 0 || old.RemindAt != nil || old.NextReminderAt == nil || count("sms_messages", SourceRequest, old.UUID.String()) != 0 ||
		w.notif.count(TemplateRequested) != before+1 {
		t.Fatalf("channels off: %+v", old)
	}
}

func TestChannelsEnabled(t *testing.T) {
	for in, want := range map[string]bool{"": true, `{}`: true, `{"doc_request_channels":true}`: true,
		`{"doc_request_channels":"x"}`: true, `{"doc_request_channels":false}`: false} {
		if got := ChannelsEnabled([]byte(in)); got != want {
			t.Fatalf("%q = %v", in, got)
		}
	}
}
