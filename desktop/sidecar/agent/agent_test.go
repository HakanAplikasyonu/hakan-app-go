package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAPI, HAKAN API'nin ajanın kullandığı uçlarını taklit eder; zarf
// biçimi backend'deki gibi.
type fakeAPI struct {
	mu         sync.Mutex
	access     string
	refresh    string
	refreshes  int
	uploads    []string
	reject     map[string]int // dosya adı → HTTP durum
	heartbeats int
	orgHeader  string
}

func ok(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data, "meta": map[string]string{"request_id": "r"}})
}

func fail(w http.ResponseWriter, status int, code string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]string{"code": code, "message": code}, "meta": map[string]string{}})
}

func (f *fakeAPI) handler() http.Handler {
	mux := http.NewServeMux()
	tokens := func() map[string]any {
		f.access = "acc-" + time.Now().Format("150405.000000000")
		f.refresh = "ref-" + f.access
		return map[string]any{"access_token": f.access, "refresh_token": f.refresh, "token_type": "Bearer", "expires_in": 900}
	}
	authed := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			good := r.Header.Get("Authorization") == "Bearer "+f.access && f.access != ""
			f.orgHeader = r.Header.Get("X-Organization")
			f.mu.Unlock()
			if !good {
				fail(w, 401, "UNAUTHENTICATED")
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("POST /v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["password"] != "dogru" {
			fail(w, 401, "INVALID_CREDENTIALS")
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		ok(w, 200, tokens())
	})
	mux.HandleFunc("POST /v1/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		defer f.mu.Unlock()
		if in["refresh_token"] != f.refresh {
			fail(w, 401, "UNAUTHENTICATED")
			return
		}
		f.refreshes++
		ok(w, 200, tokens())
	})
	mux.HandleFunc("GET /v1/auth/me", authed(func(w http.ResponseWriter, _ *http.Request) {
		ok(w, 200, map[string]any{"organizations": []map[string]any{{"uuid": "o-1", "slug": "atlas-ymm", "name": "Atlas YMM", "status": "active"}}})
	}))
	mux.HandleFunc("POST /v1/tenant/agents", authed(func(w http.ResponseWriter, _ *http.Request) {
		ok(w, 200, map[string]any{"uuid": "a-1", "name": "PC", "status": "online", "heartbeat_seconds": 300})
	}))
	mux.HandleFunc("POST /v1/tenant/agents/{id}/heartbeat", authed(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.heartbeats++
		f.mu.Unlock()
		if r.PathValue("id") != "a-1" {
			fail(w, 404, "NOT_FOUND")
			return
		}
		ok(w, 200, map[string]any{"uuid": "a-1", "status": "online", "heartbeat_seconds": 300})
	}))
	mux.HandleFunc("POST /v1/tenant/engagements/{eng}/ubl-uploads", authed(func(w http.ResponseWriter, r *http.Request) {
		file, hdr, err := r.FormFile("file")
		if err != nil {
			fail(w, 400, "VALIDATION_ERROR")
			return
		}
		_, _ = io.ReadAll(file)
		f.mu.Lock()
		st, bad := f.reject[hdr.Filename]
		if !bad {
			f.uploads = append(f.uploads, r.PathValue("eng")+"/"+hdr.Filename)
		}
		f.mu.Unlock()
		if bad {
			fail(w, st, "REJECTED")
			return
		}
		ok(w, 202, map[string]any{"uuid": "job-" + hdr.Filename, "file_name": hdr.Filename, "status": "queued"})
	}))
	return mux
}

func setup(t *testing.T) (*Runtime, *fakeAPI, *httptest.Server, *[]string) {
	t.Helper()
	api := &fakeAPI{reject: map[string]int{}}
	srv := httptest.NewServer(api.handler())
	t.Cleanup(srv.Close)
	var mu sync.Mutex
	var events []string
	rt, err := NewRuntime(t.TempDir(), "0.1.0", func(name string, data any) {
		mu.Lock()
		defer mu.Unlock()
		b, _ := json.Marshal(data)
		events = append(events, name+" "+string(b))
	})
	if err != nil {
		t.Fatal(err)
	}
	return rt, api, srv, &events
}

func TestLoginRegisterHeartbeat(t *testing.T) {
	rt, api, srv, events := setup(t)
	ctx := context.Background()

	if _, err := rt.Login(ctx, srv.URL, "a@b.c", "yanlis", ""); err == nil || !strings.Contains(err.Error(), "INVALID_CREDENTIALS") {
		t.Fatalf("yanlış parola: %v", err)
	}
	orgs, err := rt.Login(ctx, srv.URL+"/v1/", "a@b.c", "dogru", "")
	if err != nil || len(orgs) != 1 || orgs[0].Slug != "atlas-ymm" {
		t.Fatalf("giriş: %v %v", orgs, err)
	}
	st, err := rt.UseOrganization(ctx, "atlas-ymm", "Ofis PC")
	if err != nil || st.AgentUUID != "a-1" || !st.Running {
		t.Fatalf("ofis seçimi: %+v %v", st, err)
	}
	defer rt.Stop()
	if _, err := rt.UseOrganization(ctx, "baska-ofis", ""); err == nil {
		t.Fatal("üye olunmayan ofis kabul edilmemeli")
	}
	// Başlangıç heartbeat'i X-Organization ile gitmeli.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && rt.Status().LastBeat == nil {
		time.Sleep(10 * time.Millisecond)
	}
	if rt.Status().LastBeat == nil {
		t.Fatalf("heartbeat gönderilmedi: %+v", rt.Status())
	}
	api.mu.Lock()
	if api.orgHeader != "atlas-ymm" {
		t.Errorf("X-Organization: %q", api.orgHeader)
	}
	// Erişim belirteci geçersizleşince istemci bir kez yenileyip tekrar dener.
	api.access = "eskimis"
	api.mu.Unlock()
	rt.Beat(ctx)
	api.mu.Lock()
	refreshes := api.refreshes
	api.mu.Unlock()
	if refreshes != 1 || rt.Status().BeatError != "" {
		t.Fatalf("yenileme: %d, hata %q", refreshes, rt.Status().BeatError)
	}
	// Yenileme belirteci kabuğa olay olarak gider (anahtar zinciri).
	rotated := 0
	for _, e := range *events {
		if strings.HasPrefix(e, "auth.rotated ") && strings.Contains(e, "ref-") {
			rotated++
		}
	}
	if rotated < 2 {
		t.Errorf("auth.rotated olayları: %v", *events)
	}

	// Yeni süreç: anahtar zincirindeki belirteçle oturum sürer.
	api.mu.Lock()
	rtok := api.refresh
	api.mu.Unlock()
	rt.Stop()
	rt2, err := NewRuntime(filepath.Dir(rt.State.path), "0.1.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt2.Resume(ctx, rtok); err != nil {
		t.Fatalf("sürdürme: %v", err)
	}
	defer rt2.Stop()
	if !rt2.Status().Running || rt2.Status().Organization.Slug != "atlas-ymm" {
		t.Fatalf("sürdürülen durum: %+v", rt2.Status())
	}
	if err := rt2.Resume(ctx, "gecersiz"); err == nil {
		t.Fatal("geçersiz belirteç kabul edilmemeli")
	}
}

func TestWatcherUploadsSettledFilesOnce(t *testing.T) {
	rt, api, srv, events := setup(t)
	ctx := context.Background()
	if _, err := rt.Login(ctx, srv.URL, "a@b.c", "dogru", ""); err != nil {
		t.Fatal(err)
	}
	rt.Client.SetOrganization("atlas-ymm")
	dir := t.TempDir()
	if _, err := rt.AddWatch("goreli/yol", "eng-1", ""); err == nil {
		t.Fatal("göreli yol kabul edilmemeli")
	}
	if _, err := rt.AddWatch(dir, "eng-1", "İndirilenler"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	write := func(name, body string) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	write("fatura1.xml", "<Invoice>1</Invoice>")
	write("paket.zip", "PK-zip")
	write("not.txt", "yok")
	write(".gizli.xml", "yok")
	write("bozuk.xml", "<x/>")
	api.reject["bozuk.xml"] = 415

	now := time.Now()
	rt.Watcher.Scan(ctx, now) // ilk görüş: yerleşmesi beklenir
	if len(api.uploads) != 0 {
		t.Fatalf("ilk taramada yükleme olmamalı: %v", api.uploads)
	}
	rt.Watcher.Scan(ctx, now.Add(ScanInterval))
	if len(api.uploads) != 2 {
		t.Fatalf("yüklemeler: %v", api.uploads)
	}
	// Aynı içerik başka adla: yüklenmez. Reddedilen tekrar denenmez.
	write("fatura1-kopya.xml", "<Invoice>1</Invoice>")
	for i := 2; i < 5; i++ {
		rt.Watcher.Scan(ctx, now.Add(time.Duration(i)*ScanInterval))
	}
	if len(api.uploads) != 2 {
		t.Fatalf("mükerrer yüklendi: %v", api.uploads)
	}
	rejected := 0
	for _, e := range *events {
		if strings.HasPrefix(e, "upload ") && strings.Contains(e, `"outcome":"rejected"`) {
			rejected++
		}
	}
	if rejected != 1 {
		t.Errorf("ret olayı bir kez olmalı: %v", *events)
	}

	// Henüz yazılmakta olan dosya (boyut değişiyor) beklenir.
	p := filepath.Join(dir, "yeni.xml")
	_ = os.WriteFile(p, []byte("<a>"), 0o600)
	rt.Watcher.Scan(ctx, time.Now())
	_ = os.WriteFile(p, []byte("<a>tam</a>"), 0o600)
	rt.Watcher.Scan(ctx, time.Now())
	if len(api.uploads) != 2 {
		t.Fatalf("yazılmakta olan dosya yüklendi: %v", api.uploads)
	}
	rt.Watcher.Scan(ctx, time.Now().Add(settleFor+time.Second))
	if len(api.uploads) != 3 {
		t.Fatalf("yerleşen dosya yüklenmedi: %v", api.uploads)
	}

	// Geçici hata (sunucu kapalı): sonraki taramada yeniden denenir.
	srv.Close()
	write("sonra.xml", "<Invoice>2</Invoice>")
	rt.Watcher.Scan(ctx, time.Now())
	rt.Watcher.Scan(ctx, time.Now())
	if rt.Watcher.Stats().Failed == 0 {
		t.Fatalf("geçici hata sayılmadı: %+v", rt.Watcher.Stats())
	}
	if _, done := rt.State.Seen(sha("<Invoice>2</Invoice>")); done {
		t.Fatal("başarısız yükleme kaydedilmemeli")
	}

	// Durum dosyası yeniden yüklenince klasör ve geçmiş korunur.
	st2, err := LoadState(filepath.Dir(rt.State.path))
	if err != nil || len(st2.Snapshot().Watches) != 1 || st2.UploadedCount() != 4 {
		t.Fatalf("kalıcı durum: %+v %d %v", st2.Snapshot(), st2.UploadedCount(), err)
	}
	info, _ := os.Stat(rt.State.path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("durum dosyası izni: %v", info.Mode().Perm())
	}
	if err := rt.RemoveWatch(dir); err != nil || len(rt.State.Snapshot().Watches) != 0 {
		t.Fatalf("izleme kaldırma: %v", err)
	}
}

func TestNormalizeBase(t *testing.T) {
	for in, want := range map[string]string{
		"https://api.ornek.com/v1/": "https://api.ornek.com",
		" http://localhost:8080 ":   "http://localhost:8080",
	} {
		if got, err := NormalizeBase(in); err != nil || got != want {
			t.Errorf("%q → %q %v", in, got, err)
		}
	}
	for _, in := range []string{"", "ftp://x", "http://api.ornek.com", "adres"} {
		if _, err := NormalizeBase(in); err == nil {
			t.Errorf("%q kabul edilmemeli", in)
		}
	}
}

func sha(s string) string {
	h := sha256Sum([]byte(s))
	return h
}
