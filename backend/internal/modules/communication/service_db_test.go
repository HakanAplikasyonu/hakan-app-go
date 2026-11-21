package communication

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

func setup(t *testing.T, live bool) (*Service, *testdb.DB, int64, *atomic.Int32) {
	t.Helper()
	d := testdb.Open(t)
	org := d.Organization(t)
	kr, err := secrets.ParseKeyring("", "", "test-legacy-key-32-bytes-long!!!")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/sms/rest/v2/msgheader" {
			_, _ = io.WriteString(w, `{"code":"00","msgheaders":["OFISADI"]}`)
			return
		}
		_, _ = io.WriteString(w, `{"code":"00","jobid":"J1"}`)
	}))
	t.Cleanup(srv.Close)
	svc := NewService(d.Pool, secrets.New(kr, secrets.NewPGStore(db.New(d.Pool))), Options{BaseURL: srv.URL, Live: live, PerMinute: 6000})
	return svc, d, org, &calls
}

func ptr(s string) *string { return &s }

// Kabul (#780 A): şifre kasaya gider, ekrana dönmez; boş şifre "değişmedi".
func TestSettingsPasswordNeverReturned(t *testing.T) {
	ctx := context.Background()
	svc, _, org, _ := setup(t, false)
	v, err := svc.Get(ctx, org)
	if err != nil || !v.TestMode || v.PasswordSet || v.Encoding != "TR" {
		t.Fatalf("defaults = %+v, %v", v, err)
	}
	if _, err := svc.Save(ctx, org, 0, SettingsInput{Enabled: true, Usercode: "8501234567", MsgHeader: "OFISADI"}); !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("enable without password: %v", err)
	}
	v, err = svc.Save(ctx, org, 0, SettingsInput{Enabled: true, TestMode: true, Usercode: "8501234567", Password: ptr("s3cret!"), MsgHeader: "OFISADI", Encoding: "TR"})
	if err != nil || !v.PasswordSet {
		t.Fatalf("save = %+v, %v", v, err)
	}
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "s3cret") {
		t.Fatalf("password leaked: %s", raw)
	}
	v, err = svc.Save(ctx, org, 0, SettingsInput{Enabled: true, TestMode: true, Usercode: "8501234567", Password: ptr(""), MsgHeader: "OFISADI2", Encoding: "TR"})
	if err != nil || !v.PasswordSet || v.MsgHeader != "OFISADI2" {
		t.Fatalf("keep password = %+v, %v", v, err)
	}
	if _, err := svc.Save(ctx, org, 0, SettingsInput{Usercode: "abc"}); !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("bad usercode: %v", err)
	}
	v, err = svc.Test(ctx, org)
	if err != nil || v.LastTest == nil || !v.LastTest.OK || len(v.LastTest.Headers) != 1 {
		t.Fatalf("test = %+v, %v", v.LastTest, err)
	}
}

// Deneme modunda kayıt yazılır, NetGSM'e gönderim isteği gitmez; bozuk
// numara baştan başarısız; kampanya metni reddedilir.
func TestSendSMSTestModeAndRules(t *testing.T) {
	ctx := context.Background()
	svc, _, org, calls := setup(t, true)
	if _, err := svc.SendSMS(ctx, SMSInput{OrganizationID: org, Phone: "5321234567", Text: "x"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("not configured: %v", err)
	}
	if _, err := svc.Save(ctx, org, 0, SettingsInput{Enabled: true, TestMode: true, Usercode: "8501234567", Password: ptr("p"), MsgHeader: "OFISADI"}); err != nil {
		t.Fatal(err)
	}
	m, err := svc.SendSMS(ctx, SMSInput{OrganizationID: org, Phone: "+90 532 123 45 67", Text: "KDV tahakkuku 1.000 ₺", SourceRule: "test"})
	if err != nil || m.Status != StatusDryRun || m.Phone != "5321234567" || m.Body != "KDV tahakkuku 1.000 TL\n" || calls.Load() != 0 {
		t.Fatalf("dry run = %+v, %v, calls=%d", m, err, calls.Load())
	}
	m, err = svc.SendSMS(ctx, SMSInput{OrganizationID: org, Phone: "0212 123 45 67", Text: "Bilgi"})
	if err != nil || m.Status != StatusFailed || m.ErrorCode != "phone" {
		t.Fatalf("landline = %+v, %v", m, err)
	}
	if _, err := svc.SendSMS(ctx, SMSInput{OrganizationID: org, Phone: "5321234567", Text: "%20 indirim"}); !errors.Is(err, ErrCampaignText) {
		t.Fatalf("campaign: %v", err)
	}
	// Canlı: deneme modu kapalı ve sunucu izinli → sahte sunucuya gider.
	if _, err := svc.Save(ctx, org, 0, SettingsInput{Enabled: true, TestMode: false, Usercode: "8501234567", MsgHeader: "OFISADI"}); err != nil {
		t.Fatal(err)
	}
	m, err = svc.SendSMS(ctx, SMSInput{OrganizationID: org, Phone: "5321234567", Text: "Bilgi"})
	if err != nil || m.Status != StatusSent || m.ProviderJobID != "J1" || !m.SentAt.Valid || calls.Load() != 1 {
		t.Fatalf("live = %+v, %v", m, err)
	}
	recent, err := svc.Recent(ctx, org, 10)
	if err != nil || len(recent) != 3 {
		t.Fatalf("recent = %d, %v", len(recent), err)
	}
}

// Sunucu canlı gönderime izin vermiyorsa (NETGSM_SEND_ENABLED) ofis deneme
// modunu kapatsa da gönderilmez.
func TestServerNotLiveKeepsDryRun(t *testing.T) {
	ctx := context.Background()
	svc, _, org, calls := setup(t, false)
	if _, err := svc.Save(ctx, org, 0, SettingsInput{Enabled: true, TestMode: false, Usercode: "8501234567", Password: ptr("p"), MsgHeader: "OFISADI"}); err != nil {
		t.Fatal(err)
	}
	m, err := svc.SendSMS(ctx, SMSInput{OrganizationID: org, Phone: "5321234567", Text: "Bilgi"})
	if err != nil || m.Status != StatusDryRun || calls.Load() != 0 {
		t.Fatalf("not live = %+v, %v", m, err)
	}
	mm, err := svc.SendMail(ctx, MailInput{OrganizationID: org, To: "a@b.co", Subject: "Konu", Body: "Metin"})
	if err != nil || mm.Status != StatusDryRun {
		t.Fatalf("mail = %+v, %v", mm, err)
	}
}
