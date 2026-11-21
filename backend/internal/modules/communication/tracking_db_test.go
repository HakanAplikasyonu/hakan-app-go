package communication

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mail"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type fakeMailer struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (f *fakeMailer) Send(_ context.Context, m mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

// liveSetup: a live office (test mode off) with a fake mailer and a fake
// NetGSM whose send answers J1, J2, … and whose report answers from
// reportFn. Nothing ever reaches the real NetGSM.
func liveSetup(t *testing.T, reportFn func(ids []string) string) (*Service, *testdb.DB, int64, int64, *fakeMailer, *atomic.Int32) {
	t.Helper()
	d := testdb.Open(t)
	org := d.Organization(t)
	eng := d.Engagement(t, org, d.Taxpayer(t, "legal", "1111111111"))
	kr, err := secrets.ParseKeyring("", "", "test-legacy-key-32-bytes-long!!!")
	if err != nil {
		t.Fatal(err)
	}
	var sends, reports atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sms/rest/v2/send":
			n := sends.Add(1)
			_, _ = io.WriteString(w, `{"code":"00","jobid":"J`+string(rune('0'+n))+`"}`)
		case "/sms/rest/v2/report":
			reports.Add(1)
			var body struct {
				BulkIDs []string `json:"bulkIds"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			_, _ = io.WriteString(w, reportFn(body.BulkIDs))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	m := &fakeMailer{}
	svc := NewService(d.Pool, secrets.New(kr, secrets.NewPGStore(db.New(d.Pool))),
		Options{BaseURL: srv.URL, Live: true, PerMinute: 6000, Mailer: m}).
		WithTracking(Tracking{FrontendURL: "https://app.test", ExtraOrigins: []string{"https://files.test/bucket"}})
	if _, err := svc.Save(context.Background(), org, 0, SettingsInput{Enabled: true, TestMode: false, Usercode: "8501234567", Password: ptr("p"), MsgHeader: "OFISADI"}); err != nil {
		t.Fatal(err)
	}
	return svc, d, org, eng, m, &reports
}

func trackMux(svc *Service) *http.ServeMux {
	h := NewHandler(svc)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/public/mail/{token}/open", h.MailOpen)
	mux.HandleFunc("GET /v1/public/mail/{token}/link", h.MailLink)
	return mux
}

func get(mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func mailTimes(t *testing.T, d *testdb.DB, id int64) (opened, downloaded *time.Time) {
	t.Helper()
	if err := d.Pool.QueryRow(context.Background(), `SELECT opened_at, downloaded_at FROM mail_messages WHERE id = $1`, id).Scan(&opened, &downloaded); err != nil {
		t.Fatal(err)
	}
	return opened, downloaded
}

// Kabul (#780 D): giden e-postanın HTML bölümünde piksel var; piksel ilk
// açılışta opened_at yazar, sonraki açılışlar değiştirmez; bozuk ya da
// bilinmeyen anahtar aynı resmi alır, hiçbir şey yazılmaz.
func TestMailOpenPixelSetsOpenedAtOnce(t *testing.T) {
	ctx := context.Background()
	svc, d, org, eng, mailer, _ := liveSetup(t, func([]string) string { return `{"code":"00","jobs":[]}` })
	msg, err := svc.SendMail(ctx, MailInput{OrganizationID: org, EngagementID: &eng, To: "mukellef@example.com", Subject: "KDV tahakkuku", Body: "Merhaba,\n\nKDV tahakkukunuz hazır."})
	if err != nil || msg.Status != StatusSent {
		t.Fatalf("send = %+v, %v", msg, err)
	}
	if len(mailer.sent) != 1 {
		t.Fatalf("sent %d", len(mailer.sent))
	}
	pixel := "https://app.test/api/v1/public/mail/" + msg.TrackToken.String() + "/open"
	if !strings.Contains(mailer.sent[0].HTML, pixel) || !strings.Contains(mailer.sent[0].Body, "KDV tahakkukunuz hazır.") {
		t.Fatalf("html part without pixel:\n%s", mailer.sent[0].HTML)
	}

	mux := trackMux(svc)
	path := "/v1/public/mail/" + msg.TrackToken.String() + "/open"
	rec := get(mux, path)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/gif" || rec.Body.Len() != len(transparentGIF) {
		t.Fatalf("pixel = %d %q %d", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
	}
	first, _ := mailTimes(t, d, msg.ID)
	if first == nil {
		t.Fatal("opened_at not written")
	}
	time.Sleep(20 * time.Millisecond)
	get(mux, path)
	again, _ := mailTimes(t, d, msg.ID)
	if again == nil || !again.Equal(*first) {
		t.Fatalf("opened_at changed: %v -> %v", first, again)
	}

	for _, bad := range []string{"/v1/public/mail/not-a-uuid/open", "/v1/public/mail/" + uuid.NewString() + "/open"} {
		rec := get(mux, bad)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/gif" {
			t.Fatalf("%s = %d", bad, rec.Code)
		}
	}
}

// Ek bağlantısı downloaded_at yazar ve yalnız uygulamanın kendi
// adreslerine yönlendirir (açık yönlendirme yok).
func TestMailLinkRedirectsOnlyToOwnOrigins(t *testing.T) {
	ctx := context.Background()
	svc, d, org, eng, _, _ := liveSetup(t, func([]string) string { return `{"code":"00"}` })
	msg, err := svc.SendMail(ctx, MailInput{OrganizationID: org, EngagementID: &eng, To: "mukellef@example.com", Subject: "Rapor", Body: "Raporunuz ekte."})
	if err != nil {
		t.Fatal(err)
	}
	mux := trackMux(svc)
	base := "/v1/public/mail/" + msg.TrackToken.String() + "/link?to="
	for _, evil := range []string{"https://evil.example/x", "//evil.example/x", "javascript:alert(1)", "https://app.test@evil.example/", "", "https://files.test.evil.example/x"} {
		if rec := get(mux, base+urlQuery(evil)); rec.Code != http.StatusNotFound {
			t.Fatalf("%q = %d %s", evil, rec.Code, rec.Header().Get("Location"))
		}
	}
	if o, dl := mailTimes(t, d, msg.ID); o != nil || dl != nil {
		t.Fatal("refused redirect wrote a time")
	}
	rec := get(mux, base+urlQuery("https://files.test/bucket/rapor.pdf"))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://files.test/bucket/rapor.pdf" {
		t.Fatalf("storage = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	o, dl := mailTimes(t, d, msg.ID)
	if o == nil || dl == nil {
		t.Fatalf("times = %v %v", o, dl)
	}
	rec = get(mux, base+urlQuery("/t/ofis/belgeler"))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://app.test/t/ofis/belgeler" {
		t.Fatalf("frontend path = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if got := svc.TrackedLink(msg.TrackToken, "https://evil.example/x"); got != "https://evil.example/x" {
		t.Fatalf("foreign link wrapped: %s", got)
	}
}

func urlQuery(s string) string { return url.QueryEscape(s) }

// İletişim geçmişi: mükellefin SMS ve e-postaları yeniden eskiye, kanal
// süzgeci; başka mükellefin kaydı görünmez.
func TestHistoryMergesChannelsNewestFirst(t *testing.T) {
	ctx := context.Background()
	svc, d, org, eng, _, _ := liveSetup(t, func([]string) string { return `{"code":"00"}` })
	other := d.Engagement(t, org, d.Taxpayer(t, "legal", "2222222222"))
	if _, err := svc.SendMail(ctx, MailInput{OrganizationID: org, EngagementID: &eng, To: "a@example.com", Subject: "Konu", Body: "Metin"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := svc.SendSMS(ctx, SMSInput{OrganizationID: org, EngagementID: &eng, Phone: "5321234567", Text: "Bilgi", SourceRule: "mail_fallback"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendSMS(ctx, SMSInput{OrganizationID: org, EngagementID: &other, Phone: "5329999999", Text: "Başka"}); err != nil {
		t.Fatal(err)
	}
	items, err := svc.History(ctx, org, eng, "", 50)
	if err != nil || len(items) != 2 {
		t.Fatalf("history = %+v, %v", items, err)
	}
	if items[0].Channel != ChannelSMS || items[0].ProviderJobID == "" || items[0].SourceRule != "mail_fallback" || items[1].Channel != ChannelEmail || items[1].To != "a@example.com" {
		t.Fatalf("order / fields = %+v", items)
	}
	only, err := svc.History(ctx, org, eng, ChannelEmail, 50)
	if err != nil || len(only) != 1 || only[0].Channel != ChannelEmail {
		t.Fatalf("email only = %+v, %v", only, err)
	}
}

// NetGSM iletim raporu: iletilen SMS'e delivered_at, iletilemeyene hata
// yazılır; deneme modundaki ofis ve canlı olmayan sunucu NetGSM'e sormaz.
func TestSweepDeliveryReports(t *testing.T) {
	ctx := context.Background()
	deliveredAt := time.Now().Add(-time.Minute).Truncate(time.Second)
	report := func(ids []string) string {
		jobs := []map[string]any{}
		for _, id := range ids {
			switch id {
			case "J1":
				jobs = append(jobs, map[string]any{"jobid": id, "status": 1, "deliveredDate": deliveredAt.In(time.FixedZone("TRT", 3*3600)).Format("02.01.2006 15:04:05")})
			case "J2":
				jobs = append(jobs, map[string]any{"jobid": id, "status": 3})
			default:
				jobs = append(jobs, map[string]any{"jobid": id, "status": 0})
			}
		}
		raw, _ := json.Marshal(map[string]any{"code": "00", "jobs": jobs})
		return string(raw)
	}
	svc, d, org, eng, _, reports := liveSetup(t, report)
	var ids []int64
	for range 3 {
		m, err := svc.SendSMS(ctx, SMSInput{OrganizationID: org, EngagementID: &eng, Phone: "5321234567", Text: "Bilgi"})
		if err != nil || m.Status != StatusSent {
			t.Fatalf("send = %+v, %v", m, err)
		}
		ids = append(ids, m.ID)
	}
	n, err := svc.SweepDeliveryReports(ctx, nil)
	if err != nil || n != 2 || reports.Load() != 1 {
		t.Fatalf("sweep = %d, %v, reports=%d", n, err, reports.Load())
	}
	var status, code string
	var delivered *time.Time
	row := func(id int64) {
		if err := d.Pool.QueryRow(ctx, `SELECT status, error_code, delivered_at FROM sms_messages WHERE id = $1`, id).Scan(&status, &code, &delivered); err != nil {
			t.Fatal(err)
		}
	}
	row(ids[0])
	if status != StatusSent || delivered == nil || !delivered.Equal(deliveredAt) {
		t.Fatalf("J1 = %s %v", status, delivered)
	}
	row(ids[1])
	if status != StatusFailed || code != "dlr_3" || delivered != nil {
		t.Fatalf("J2 = %s %s %v", status, code, delivered)
	}
	row(ids[2])
	if status != StatusSent || delivered != nil {
		t.Fatalf("J3 = %s %v", status, delivered)
	}

	// Deneme moduna geçen ofis için NetGSM'e istek gitmez.
	if _, err := svc.Save(ctx, org, 0, SettingsInput{Enabled: true, TestMode: true, Usercode: "8501234567", MsgHeader: "OFISADI"}); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.SweepDeliveryReports(ctx, nil); err != nil || n != 0 || reports.Load() != 1 {
		t.Fatalf("test mode sweep = %d, %v, reports=%d", n, err, reports.Load())
	}
	// Sunucu canlı değilse tarama hiç çalışmaz.
	svc.live = false
	if n, err := svc.SweepDeliveryReports(ctx, nil); err != nil || n != 0 || reports.Load() != 1 {
		t.Fatalf("not live sweep = %d, %v, reports=%d", n, err, reports.Load())
	}
}
