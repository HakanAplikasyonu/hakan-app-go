package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type fakeNotifier struct {
	mu   sync.Mutex
	sent []notifmodel.EnqueueInput
}

func (f *fakeNotifier) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, in)
	return nil, nil
}

// Kabul (#51): ajan 30 dk sessiz kalınca AGENT_OFFLINE; heartbeat ile geri
// döner; bildirim geçiş başına bir kez gider.
func TestAgentOfflineAfter30Minutes(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org, other := d.Organization(t), d.Organization(t)
	user := d.User(t)
	notif := &fakeNotifier{}
	svc := New(db.New(d.Pool), notif, "https://app.example.com/", nil)

	in := RegisterInput{MachineID: "mach-0001-abcd", Name: "Ofis bilgisayarı", Platform: "windows", Version: "0.1.0"}
	a, err := svc.Register(ctx, org, user, in)
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != "online" || a.HeartbeatSeconds != 300 || a.LastError != nil {
		t.Fatalf("kayıt: %+v", a)
	}
	// Aynı makine: aynı satır.
	again, err := svc.Register(ctx, org, user, RegisterInput{MachineID: in.MachineID, Name: "Yeni ad", Platform: "windows"})
	if err != nil || again.UUID != a.UUID || again.Name != "Yeni ad" {
		t.Fatalf("yeniden kayıt: %+v %v", again, err)
	}
	// Başka ofis aynı makine kimliğiyle ayrı ajan; ilk ofisin ajanını göremez.
	b, err := svc.Register(ctx, other, user, in)
	if err != nil || b.UUID == a.UUID {
		t.Fatalf("diğer ofis: %+v %v", b, err)
	}
	if _, err := svc.Heartbeat(ctx, other, a.UUID, HeartbeatInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("başka ofisin ajanına heartbeat: %v", err)
	}
	if _, err := svc.Get(ctx, other, a.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("başka ofisin ajanı görünmemeli: %v", err)
	}

	// 29 dk: hâlâ çevrimiçi.
	svc.now = func() time.Time { return time.Now().Add(29 * time.Minute) }
	rep, err := svc.SweepOffline(ctx)
	if err != nil || rep.Offline != 0 {
		t.Fatalf("29 dk: %+v %v", rep, err)
	}

	// 31 dk: offline + bildirim (iki ofisin ajanı da sessiz).
	svc.now = func() time.Time { return time.Now().Add(31 * time.Minute) }
	rep, err = svc.SweepOffline(ctx)
	if err != nil || rep.Offline != 2 || rep.Notified != 2 {
		t.Fatalf("31 dk: %+v %v", rep, err)
	}
	got, err := svc.Get(ctx, org, a.UUID)
	if err != nil || got.Status != "offline" || got.LastError == nil || got.LastError.Code != CodeOffline {
		t.Fatalf("offline durumu: %+v %v", got, err)
	}
	n := notif.sent[0]
	if n.TemplateCode != "agents.offline" || *n.UserID != user || n.TemplateVars["minutes"] == "" ||
		n.Payload["code"] != CodeOffline {
		t.Fatalf("bildirim: %+v", n)
	}

	// Aynı sessizlik için ikinci bildirim yok.
	rep, err = svc.SweepOffline(ctx)
	if err != nil || rep.Offline != 0 || len(notif.sent) != 2 {
		t.Fatalf("tekrar tarama: %+v %v (%d bildirim)", rep, err, len(notif.sent))
	}

	// Heartbeat: geri çevrimiçi, olay temizlenir, istatistik saklanır.
	stats := json.RawMessage(`{"watched_folders":2,"uploaded":5}`)
	back, err := svc.Heartbeat(ctx, org, a.UUID, HeartbeatInput{Version: "0.1.1", Stats: stats})
	if err != nil || back.Status != "online" || back.LastError != nil || back.Version != "0.1.1" {
		t.Fatalf("geri dönüş: %+v %v", back, err)
	}
	var hb map[string]any
	if err := json.Unmarshal(back.Heartbeat, &hb); err != nil || hb["uploaded"] != float64(5) {
		t.Fatalf("istatistik: %s %v", back.Heartbeat, err)
	}

	// Geri alınan ajan listede yok, heartbeat'i 404.
	if err := svc.Revoke(ctx, org, a.UUID); err != nil {
		t.Fatal(err)
	}
	list, err := svc.List(ctx, org)
	if err != nil || len(list) != 0 {
		t.Fatalf("liste: %+v %v", list, err)
	}
	if _, err := svc.Heartbeat(ctx, org, a.UUID, HeartbeatInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("geri alınmış ajan: %v", err)
	}
	if err := svc.Revoke(ctx, org, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("olmayan ajan: %v", err)
	}
	// Aynı makine yeniden giriş yapınca yeniden etkinleşir.
	re, err := svc.Register(ctx, org, user, in)
	if err != nil || re.UUID != a.UUID || re.Status != "online" {
		t.Fatalf("yeniden etkinleşme: %+v %v", re, err)
	}
}

func TestRegisterValidation(t *testing.T) {
	svc := New(nil, nil, "", nil)
	bad := []RegisterInput{
		{MachineID: "short", Name: "a", Platform: "windows"},
		{MachineID: "mach-0001-abcd", Name: " ", Platform: "windows"},
		{MachineID: "mach-0001-abcd", Name: "a", Platform: "android"},
		{MachineID: "mach 0001 abcd", Name: "a", Platform: "linux"},
	}
	for _, in := range bad {
		if _, err := svc.Register(context.Background(), 1, 1, in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v kabul edilmemeliydi: %v", in, err)
		}
	}
	if _, err := svc.Heartbeat(context.Background(), 1, uuid.New(), HeartbeatInput{Stats: json.RawMessage(`[1]`)}); !errors.Is(err, ErrInvalid) {
		t.Errorf("dizi istatistik kabul edilmemeliydi: %v", err)
	}
}
