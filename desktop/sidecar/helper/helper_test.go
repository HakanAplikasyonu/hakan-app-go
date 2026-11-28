package helper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
)

func TestDefaultLucaMapIsPlaceholder(t *testing.T) {
	m, err := DefaultLucaMap()
	if err != nil {
		t.Fatal(err)
	}
	if !m.Placeholder || m.Verified {
		t.Fatalf("gömülü harita keşif öncesi yer tutucu olmalı: placeholder=%v verified=%v", m.Placeholder, m.Verified)
	}
	if len(m.TODO) == 0 {
		t.Fatal("yer tutucu haritada keşif listesi (todo) olmalı")
	}
	if _, err := m.Resolve("https://example.com/x"); err == nil {
		t.Fatal("alan listesi dışı adres çözülmemeli")
	}
}

func TestTestMapParses(t *testing.T) {
	m := testMap(t, "http://127.0.0.1:1234/")
	if m.Placeholder {
		t.Fatal("test haritası yer tutucu değil")
	}
	got, err := m.Resolve("/aktarim")
	if err != nil || got != "http://127.0.0.1:1234/aktarim" {
		t.Fatalf("Resolve = %q, %v", got, err)
	}
}

func TestMapValidateMissing(t *testing.T) {
	if _, err := ParseMap([]byte(`{"start_url":"https://luca.example/"}`)); err == nil || !strings.Contains(err.Error(), "session.logged_in") {
		t.Fatalf("eksik alanlar bildirilmeli: %v", err)
	}
}

func TestHostAllowed(t *testing.T) {
	allowed := []string{"luca.com.tr", "*.luca.com.tr"}
	cases := map[string]bool{
		"https://luca.com.tr/":          true,
		"https://web.luca.com.tr/a?b=1": true,
		"https://LUCA.com.tr/":          true,
		"https://luca.com.tr.evil.com/": false,
		"https://evilluca.com.tr/":      false,
		"https://example.com/":          false,
		"about:blank":                   true,
		"chrome-error://chromewebdata/": true,
		"file:///etc/passwd":            false,
		"javascript:alert(1)":           false,
		"data:text/html,<b>x</b>":       false,
		"http://127.0.0.1:8080/luca":    false,
	}
	for u, want := range cases {
		if got := urlAllowed(allowed, u); got != want {
			t.Errorf("urlAllowed(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestPrepareProfileDisablesPasswordManager(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "helper-profile")
	if err := os.MkdirAll(filepath.Join(dir, "Default"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Tarayıcının yazdığı başka tercih korunmalı; açılmış parola kasası kapanmalı.
	old := `{"profile":{"password_manager_enabled":true,"name":"HAKAN"},"credentials_enable_service":true}`
	if err := os.WriteFile(filepath.Join(dir, "Default", "Preferences"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PrepareProfile(dir); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "Default", "Preferences"))
	if err != nil {
		t.Fatal(err)
	}
	var prefs map[string]any
	if err := json.Unmarshal(raw, &prefs); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"credentials_enable_service", "profile.password_manager_enabled", "autofill.profile_enabled", "autofill.credit_card_enabled"} {
		if v, ok := prefValue(prefs, k); !ok || v != false {
			t.Errorf("%s = %v, false olmalı", k, v)
		}
	}
	if v, _ := prefValue(prefs, "profile.name"); v != "HAKAN" {
		t.Errorf("profile.name korunmalı: %v", v)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Errorf("profil izni %o, 0700 olmalı", info.Mode().Perm())
		}
	}
}

func TestBrowserCandidatesWindowsPrefersEdge(t *testing.T) {
	env := map[string]string{"ProgramFiles": `C:\Program Files`, "ProgramFiles(x86)": `C:\Program Files (x86)`, "LOCALAPPDATA": `C:\Users\a\AppData\Local`}
	c := BrowserCandidates("windows", func(k string) string { return env[k] })
	if len(c) == 0 || !strings.Contains(c[0], "msedge.exe") {
		t.Fatalf("Windows'ta önce Edge aranmalı: %v", c)
	}
	if BrowserName(`C:\x\msedge.exe`) != "Microsoft Edge" || BrowserName("/usr/bin/google-chrome") != "Google Chrome" {
		t.Fatal("tarayıcı adı")
	}
}

func TestNextCode(t *testing.T) {
	taken := func(codes ...string) map[string]bool {
		m := map[string]bool{}
		for _, c := range codes {
			m[c] = true
		}
		return m
	}
	cases := []struct {
		name   string
		parent string
		codes  map[string]bool
		rule   CodeRule
		want   string
	}{
		{"harfli", "320", taken("320", "320.01", "320.01.E09", "320.01.E10", "320.01.E2"), CodeRule{}, "320.01.E11"},
		{"üç hane", "320", taken("320.001", "320.002"), CodeRule{}, "320.003"},
		{"dört hane", "120", taken("120.01", "120.01.0010", "120.01.0009"), CodeRule{}, "120.01.0011"},
		{"başka ana hesap sayılmaz", "120", taken("320.01.E99", "120.01.001"), CodeRule{}, "120.01.002"},
		{"alt hesap yok, kural yok", "120", taken("120"), CodeRule{}, "120.01.001"},
		{"kural öneki", "320", taken("320.01.E10", "320.02.001"), CodeRule{Prefix: "320.01.E"}, "320.01.E11"},
		{"kural ilk kod", "336", taken(), CodeRule{Prefix: "336.01.", Width: 3}, "336.01.001"},
		{"taşma", "320", taken("320.01.E99"), CodeRule{}, "320.01.E100"},
	}
	for _, c := range cases {
		got, err := NextCode(c.parent, c.codes, c.rule)
		if err != nil || got != c.want {
			t.Errorf("%s: NextCode = %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

func TestPlanCari(t *testing.T) {
	accounts := []Account{
		{Code: "320.01.E10", Name: "Var Olan Tedarikçi", VKN: "1111111111"},
		{Code: "120.01.001", Name: "Müşteri", VKN: "2222222222"},
	}
	items := []CariItem{
		{VKN: "1111111111", Parent: "320"},
		{VKN: "111 111 1111", Parent: "320"},                  // aynı VKN tek satır
		{VKN: "2222222222", Parent: "320", Name: "Satıcı da"}, // 120'de var, 320'de yok: açılır
		{VKN: "3333333333", Parent: "320", Name: "Yeni"},
		{VKN: "12345", Parent: "320"},
		{VKN: "4444444444", Parent: ""},
	}
	found, create, failed := PlanCari(items, accounts, nil)
	if len(found) != 1 || found[0].Code != "320.01.E10" || found[0].Parent != "320" {
		t.Fatalf("found = %+v", found)
	}
	if len(create) != 2 || create[0].Code != "320.01.E11" || create[1].Code != "320.01.E12" {
		t.Fatalf("create = %+v", create)
	}
	if len(failed) != 2 {
		t.Fatalf("failed = %+v", failed)
	}
}

func TestBuildPlanFileWindows1254(t *testing.T) {
	m := testMap(t, "http://127.0.0.1/")
	data, err := BuildPlanFile(m.PlanFile, []PlannedCari{
		{Code: "320.01.E11", Item: CariItem{VKN: "3333333333", Name: "Şirin Gıda Çay İşletmesi", TaxOffice: "Kadıköy"}},
		{Code: "120.01.002", Item: CariItem{VKN: "12345678901", Name: "Ayşe Öz"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text, err := charmap.Windows1254.NewDecoder().Bytes(data)
	if err != nil {
		t.Fatal(err)
	}
	want := "Hesap Kodu;Hesap Adı;Vergi No;Vergi Dairesi;T.C. Kimlik No;Adres\r\n" +
		"320.01.E11;Şirin Gıda Çay İşletmesi;3333333333;Kadıköy;;\r\n" +
		"120.01.002;Ayşe Öz;;;12345678901;\r\n"
	if string(text) != want {
		t.Fatalf("dosya:\n%q\nbeklenen:\n%q", text, want)
	}
}

func TestMaskAndTaxID(t *testing.T) {
	in := "VKN 1234567890 için hesap yok; IBAN TR12 0006 4000 0011 2345 6789 01, tutar 12.345,67 TL, e-posta a.b@ornek.com, tel 0532 123 45 67"
	got := mask(in, 500)
	for _, leak := range []string{"1234567890", "TR12", "12.345,67", "a.b@ornek.com", "0532"} {
		if strings.Contains(got, leak) {
			t.Errorf("maskelenmedi: %q içinde %q", got, leak)
		}
	}
	if firstTaxID("Seçili firma: VKN 9876543210 (Merkez)") != "9876543210" {
		t.Fatal("firstTaxID")
	}
	if firstTaxID("Tel 053212345678") != "" {
		t.Fatal("12 haneli sayı VKN değil")
	}
}

func TestResultStatus(t *testing.T) {
	cases := []struct {
		count    bool
		ok, errs int
		failText bool
		want     string
	}{
		{true, 50, 0, false, FileOK},
		{true, 48, 2, false, FilePartial},
		{false, 0, 3, false, FileFailed},
		{true, 0, 0, true, FileFailed},
		{true, 0, 0, false, FileUnknown},
		{false, 0, 0, false, FileUnknown},
	}
	for _, c := range cases {
		if got := resultStatus(c.count, c.ok, c.errs, c.failText); got != c.want {
			t.Errorf("resultStatus(%v,%d,%d,%v) = %s, want %s", c.count, c.ok, c.errs, c.failText, got, c.want)
		}
	}
}

func TestEligibleOrder(t *testing.T) {
	st, _ := LoadLocalState(t.TempDir())
	r := &Runner{State: st}
	run1, run2 := "run-1", "run-2"
	mine := &TaskAgent{UUID: "ajan"}
	tasks := []Task{
		{UUID: "b2", RunUUID: &run2, Sequence: 2, Kind: KindBulkExport, Status: StatusQueued, Approved: true},
		{UUID: "a2", RunUUID: &run1, Sequence: 2, Kind: KindBulkExport, Status: StatusQueued, Approved: true},
		{UUID: "a1", RunUUID: &run1, Sequence: 1, Kind: KindCariSync, Status: StatusQueued, Approved: true},
		{UUID: "onaysiz", Kind: KindCariSync, Status: StatusQueued, Approved: false},
		{UUID: "baskasi", Kind: KindCariSync, Status: StatusClaimed, Approved: true, Agent: &TaskAgent{UUID: "diger"}},
		{UUID: "benim", RunUUID: &run2, Sequence: 1, Kind: KindCariSync, Status: StatusWaitingUser, Approved: true, Agent: mine},
		{UUID: "sgk", Kind: "sgk_no_debt", Status: StatusQueued, Approved: true},
	}
	var got []string
	for _, tk := range r.eligible(tasks, "ajan") {
		got = append(got, tk.UUID)
	}
	if strings.Join(got, ",") != "benim,b2,a1,a2" {
		t.Fatalf("sıra = %v", got)
	}
}

// --- Sahte HAKAN görev API'si --------------------------------------------------

type post struct {
	Path string
	Body json.RawMessage
}

type fakeAPI struct {
	mu       sync.Mutex
	tasks    []Task
	posts    []post
	progress []ProgressReport
	files    map[string][]byte
	failPost bool
	// noConsent: ofis izni yok; disabled: luca_enabled kapalı.
	noConsent bool
	disabled  bool
	consents  []map[string]string
	openPaths []string
}

type fakeErr struct {
	status int
	code   string
}

func (e fakeErr) Error() string     { return e.code }
func (e fakeErr) HTTPStatus() int   { return e.status }
func (e fakeErr) ErrorCode() string { return e.code }

func roundTrip(in, out any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func (f *fakeAPI) DoJSON(_ context.Context, method, path string, in, out any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case method == http.MethodGet && path == settingsPath:
		return roundTrip(f.settings(), out)
	case method == http.MethodPost && path == settingsPath+"/luca-consent":
		var body map[string]string
		_ = roundTrip(in, &body)
		f.consents = append(f.consents, body)
		f.noConsent = false
		return roundTrip(f.settings(), out)
	case method == http.MethodGet && strings.HasPrefix(path, tasksPath+"?"):
		f.openPaths = append(f.openPaths, path)
		return roundTrip(map[string]any{"items": f.tasks}, out)
	case strings.HasSuffix(path, "/claim"):
		var body map[string]string
		_ = roundTrip(in, &body)
		for i := range f.tasks {
			if strings.HasSuffix(path, "/"+f.tasks[i].UUID+"/claim") {
				f.tasks[i].Status = StatusClaimed
				f.tasks[i].Agent = &TaskAgent{UUID: body["agent_uuid"]}
				return roundTrip(f.tasks[i], out)
			}
		}
		return fakeErr{404, "NOT_FOUND"}
	case strings.HasSuffix(path, "/progress"):
		var p ProgressReport
		_ = roundTrip(in, &p)
		f.progress = append(f.progress, p)
		return nil
	}
	if f.failPost {
		return errors.New("ağ yok")
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	f.posts = append(f.posts, post{Path: path, Body: raw})
	return nil
}

func (f *fakeAPI) settings() HelperSettings {
	s := HelperSettings{LucaEnabled: !f.disabled}
	if !f.noConsent {
		s.LucaConsent = &Consent{GivenAt: time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC), GivenByName: "Oğuz", TextVersion: ConsentTextVersion}
	}
	return s
}

func (f *fakeAPI) Download(_ context.Context, path string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b, ok := f.files[path]; ok {
		return b, nil
	}
	return nil, fakeErr{404, "NOT_FOUND"}
}

func (f *fakeAPI) postsTo(suffix string) []post {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []post
	for _, p := range f.posts {
		if strings.HasSuffix(p.Path, suffix) {
			out = append(out, p)
		}
	}
	return out
}

func testMap(t *testing.T, start string) *Map {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "luca-test.json"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseMap(raw)
	if err != nil {
		t.Fatal(err)
	}
	m.StartURL = start
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRunnerPlaceholderMapClaimsNothing(t *testing.T) {
	m, err := DefaultLucaMap()
	if err != nil {
		t.Fatal(err)
	}
	api := &fakeAPI{tasks: []Task{{UUID: "t1", Kind: KindCariSync, Status: StatusQueued, Approved: true}}}
	st, _ := LoadLocalState(t.TempDir())
	r := NewRunner(New(m, Options{}, nil), TaskClient{T: api}, st, func() string { return "ajan" }, t.TempDir(), nil)
	r.RunOnce(context.Background())
	if api.tasks[0].Status != StatusQueued || len(api.posts) != 0 {
		t.Fatal("yer tutucu haritayla görev üstlenilmemeli")
	}
	if !strings.Contains(r.Status().Note, "keşif") {
		t.Fatalf("not: %q", r.Status().Note)
	}
}

// Uygulama "Yükle" izlenirken kapandıysa: yeniden açılışta görev
// needs_check olur, dosya asla yeniden yüklenmez (tarayıcı açılmaz).
func TestRunnerArmedTaskBecomesNeedsCheck(t *testing.T) {
	m := testMap(t, "http://127.0.0.1:1/")
	api := &fakeAPI{tasks: []Task{{UUID: "t1", Kind: KindBulkExport, Status: StatusRunning, Approved: true, Agent: &TaskAgent{UUID: "ajan"},
		Files: []TaskFile{{UUID: "f1", Name: "banka.xlsx", Status: "pending"}}}}}
	dir := t.TempDir()
	st, _ := LoadLocalState(dir)
	if err := st.Arm(ArmedFile{TaskUUID: "t1", File: "f1"}); err != nil {
		t.Fatal(err)
	}
	// Kayıt diske yazıldı: yeniden yüklenen durum da görür.
	st, err := LoadLocalState(dir)
	if err != nil {
		t.Fatal(err)
	}
	h := New(m, Options{ExecPath: "/olmayan/tarayici"}, nil)
	r := NewRunner(h, TaskClient{T: api}, st, func() string { return "ajan" }, t.TempDir(), nil)
	r.RunOnce(context.Background())

	fr := api.postsTo("/files/f1/result")
	if len(fr) != 1 || !strings.Contains(string(fr[0].Body), `"status":"unknown"`) {
		t.Fatalf("dosya sonucu unknown olmalı: %+v", fr)
	}
	res := api.postsTo("/t1/result")
	if len(res) != 1 || !strings.Contains(string(res[0].Body), `"outcome":"needs_check"`) || !strings.Contains(string(res[0].Body), CodeUnread) {
		t.Fatalf("görev needs_check olmalı: %+v", res)
	}
	if _, ok := st.Armed("t1"); ok {
		t.Fatal("kayıt temizlenmeli")
	}
	if h.State().Open {
		t.Fatal("tarayıcı açılmamalı")
	}
}

// Sonuç gönderilemezse yerel kuyruğa yazılır, sırayla yeniden gönderilir;
// kuyrukta sonucu olan görev yeniden çalıştırılmaz.
func TestRunnerOutbox(t *testing.T) {
	m := testMap(t, "http://127.0.0.1:1/")
	api := &fakeAPI{failPost: true, tasks: []Task{{UUID: "t1", Kind: KindBulkExport, Status: StatusRunning, Approved: true, Agent: &TaskAgent{UUID: "ajan"},
		Files: []TaskFile{{UUID: "f1", Status: "pending"}}}}}
	st, _ := LoadLocalState(t.TempDir())
	_ = st.Arm(ArmedFile{TaskUUID: "t1", File: "f1"})
	r := NewRunner(New(m, Options{ExecPath: "/olmayan"}, nil), TaskClient{T: api}, st, func() string { return "ajan" }, t.TempDir(), nil)
	r.RunOnce(context.Background())
	if got := len(st.Outbox()); got != 2 {
		t.Fatalf("kuyrukta 2 istek olmalı, %d", got)
	}
	if len(r.eligible(api.tasks, "ajan")) != 0 {
		t.Fatal("kuyrukta sonucu olan görev yeniden alınmamalı")
	}
	api.failPost = false
	if err := r.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(st.Outbox()) != 0 || len(api.posts) != 2 || !strings.HasSuffix(api.posts[0].Path, "/files/f1/result") || !strings.HasSuffix(api.posts[1].Path, "/t1/result") {
		t.Fatalf("sıra korunmalı: %+v", api.posts)
	}
}

// --- Ofis izni, aktarıcı bilgisayar, kayıtlı giriş -----------------------------

func TestRunnerWithoutConsentClaimsNothing(t *testing.T) {
	m := testMap(t, "http://127.0.0.1:1/")
	api := &fakeAPI{noConsent: true, tasks: []Task{{UUID: "t1", Kind: KindCariSync, Status: StatusQueued, Approved: true}}}
	st, _ := LoadLocalState(t.TempDir())
	h := New(m, Options{ExecPath: "/olmayan"}, nil)
	r := NewRunner(h, TaskClient{T: api}, st, func() string { return "ajan" }, t.TempDir(), nil)
	r.RunOnce(context.Background())
	if api.tasks[0].Status != StatusQueued || len(api.posts) != 0 || len(api.openPaths) != 0 {
		t.Fatal("ofis izni yokken görev yoklanmamalı ve üstlenilmemeli")
	}
	if !strings.Contains(r.Status().Note, "izin") || r.Status().Office.Consent != nil {
		t.Fatalf("durum = %+v", r.Status())
	}
	if h.State().Open {
		t.Fatal("tarayıcı açılmamalı")
	}
}

func TestRunnerHelperDisabledClaimsNothing(t *testing.T) {
	m := testMap(t, "http://127.0.0.1:1/")
	api := &fakeAPI{disabled: true, tasks: []Task{{UUID: "t1", Kind: KindCariSync, Status: StatusQueued, Approved: true}}}
	st, _ := LoadLocalState(t.TempDir())
	r := NewRunner(New(m, Options{ExecPath: "/olmayan"}, nil), TaskClient{T: api}, st, func() string { return "ajan" }, t.TempDir(), nil)
	r.RunOnce(context.Background())
	if api.tasks[0].Status != StatusQueued || len(api.openPaths) != 0 || !strings.Contains(r.Status().Note, "kapalı") {
		t.Fatalf("ayar kapalıyken görev alınmamalı: %+v", r.Status())
	}
}

// Yoklama agent_uuid taşır: sunucu bu bilgisayarın son yoklama zamanını
// yazar ("Aktarıcı bilgisayar: çevrimiçi").
func TestRunnerPollCarriesAgent(t *testing.T) {
	m := testMap(t, "http://127.0.0.1:1/")
	api := &fakeAPI{}
	st, _ := LoadLocalState(t.TempDir())
	r := NewRunner(New(m, Options{ExecPath: "/olmayan"}, nil), TaskClient{T: api}, st, func() string { return "ajan-9" }, t.TempDir(), nil)
	r.RunOnce(context.Background())
	if len(api.openPaths) != 1 || !strings.Contains(api.openPaths[0], "agent_uuid=ajan-9") || !strings.Contains(api.openPaths[0], "open=true") {
		t.Fatalf("yoklama = %v", api.openPaths)
	}
	if !r.Status().Office.Known || r.Status().Office.Consent == nil {
		t.Fatalf("ofis bilgisi = %+v", r.Status().Office)
	}
}

func TestRunnerGiveConsent(t *testing.T) {
	m := testMap(t, "http://127.0.0.1:1/")
	api := &fakeAPI{noConsent: true}
	st, _ := LoadLocalState(t.TempDir())
	r := NewRunner(New(m, Options{ExecPath: "/olmayan"}, nil), TaskClient{T: api}, st, func() string { return "ajan" }, t.TempDir(), nil)
	info, err := r.GiveConsent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Consent == nil || info.Consent.GivenByName == "" {
		t.Fatalf("izin = %+v", info)
	}
	if len(api.consents) != 1 || api.consents[0]["agent_uuid"] != "ajan" || api.consents[0]["text_version"] != ConsentTextVersion {
		t.Fatalf("gönderilen = %+v", api.consents)
	}
	// Oturum yokken izin verilemez.
	r2 := NewRunner(New(m, Options{}, nil), TaskClient{T: api}, st, func() string { return "" }, t.TempDir(), nil)
	if _, err := r2.GiveConsent(context.Background()); err == nil {
		t.Fatal("oturum yokken izin verilmemeli")
	}
}

func TestAlwaysOnPersists(t *testing.T) {
	dir := t.TempDir()
	st, _ := LoadLocalState(dir)
	if st.AlwaysOn() {
		t.Fatal("varsayılan kapalı")
	}
	if err := st.SetAlwaysOn(true); err != nil {
		t.Fatal(err)
	}
	st2, err := LoadLocalState(dir)
	if err != nil || !st2.AlwaysOn() {
		t.Fatalf("ayar kalıcı olmalı: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "helper-state.json"))
	if strings.Contains(string(raw), "password") {
		t.Fatal("yerel kayıtta giriş bilgisi olmamalı")
	}
}

// Şifre hiçbir biçimde metne dökülmez; doğrulama hatası değeri taşımaz.
func TestCredentialsNeverPrinted(t *testing.T) {
	c := Credentials{Customer: "123", User: "ali", Password: "Gz-81"}
	for _, s := range []string{fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c), fmt.Sprintf("%v", &c), fmt.Sprint(c)} {
		if strings.Contains(s, "Gz-81") || strings.Contains(s, "ali") {
			t.Fatalf("giriş bilgisi metne döküldü: %q", s)
		}
	}
	h := New(testMap(t, "http://127.0.0.1:1/"), Options{}, nil)
	if err := h.SetCredentials(Credentials{User: " ", Password: "Gz-81"}); err == nil || strings.Contains(err.Error(), "Gz-81") {
		t.Fatalf("boş kullanıcı reddedilmeli: %v", err)
	}
	if err := h.SetCredentials(Credentials{User: "ali", Password: strings.Repeat("x", maxSecretLen+1)}); err == nil {
		t.Fatal("çok uzun şifre reddedilmeli")
	}
	if err := h.SetCredentials(c); err != nil {
		t.Fatal(err)
	}
	if li := h.LoginInfo(); !li.Saved || !li.Supported || li.Rejected {
		t.Fatalf("durum = %+v", li)
	}
	h.rejectCredentials()
	if _, ok := h.credentials(); ok {
		t.Fatal("reddedilen bilgi kullanılmamalı")
	}
	if err := h.SetCredentials(c); err != nil || !h.LoginInfo().Saved || h.LoginInfo().Rejected {
		t.Fatal("yeniden kaydedince ret kalkmalı")
	}
	h.ClearCredentials()
	if h.LoginInfo().Saved {
		t.Fatal("silinince kayıt kalmamalı")
	}
	raw, _ := json.Marshal(h.LoginInfo())
	if strings.Contains(string(raw), "Gz-81") {
		t.Fatal("durum değer taşımamalı")
	}
}

func TestDefaultMapHasNoAutoLogin(t *testing.T) {
	m, err := DefaultLucaMap()
	if err != nil {
		t.Fatal(err)
	}
	if New(m, Options{}, nil).LoginInfo().Supported {
		t.Fatal("keşif yapılmadan giriş formu tanımlı olmamalı")
	}
	if m.Import.CodeInput == "" {
		t.Fatal("yer tutucu haritada kod alanı (eski akış) kalmalı")
	}
}

type exportOpenAPI struct{}

func (exportOpenAPI) DoJSON(context.Context, string, string, any, any) error {
	return fakeErr{409, "EXPORT_OPEN"}
}
func (exportOpenAPI) Download(context.Context, string) ([]byte, error) { return nil, nil }

func TestStartExportOpen(t *testing.T) {
	_, err := TaskClient{T: exportOpenAPI{}}.StartExport(context.Background(), ExportRequest{EngagementUUIDs: []string{"e1"}, Period: "2026-10"})
	if !errors.Is(err, ErrExportOpen) {
		t.Fatalf("409 EXPORT_OPEN tanınmalı: %v", err)
	}
}
