package helper

// Sahte Luca (yerel HTML) + headless Chrome ile uçtan uca testler. Gerçek
// Luca'ya asla gidilmez. Bilgisayarda Chrome / Edge yoksa ya da -short
// verildiyse atlanır (CI'da ubuntu imajında Chrome var).
//
// "Kullanıcı" adımlarını (giriş düğmesine basmak, doğrulama kodunu yazıp
// Yükle'ye basmak) test, yardımcının waiting_user olayını görünce sayfada
// kendisi yapar; yardımcı bu alanlara dokunmaz.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"golang.org/x/text/encoding/charmap"
)

type fakeUpload struct {
	Name    string
	FisTipi string
	Kod     string
	Size    int
}

type fakeLuca struct {
	srv *httptest.Server

	mu        sync.Mutex
	current   string
	firms     map[string]string
	switchTo  string
	accounts  []Account
	uploads   []fakeUpload
	passwords []string
	planCSV   string
	hang      chan struct{}
}

func newFakeLuca(t *testing.T) *fakeLuca {
	t.Helper()
	fl := &fakeLuca{
		current: "9999999999",
		firms:   map[string]string{"9999999999": "Başka Firma A.Ş.", "1234567890": "Zeytin Gıda Ltd. Şti."},
		accounts: []Account{
			{Code: "320.01.E09", Name: "Eski Tedarikçi", VKN: "5555555555"},
			{Code: "320.01.E10", Name: "Var Olan Tedarikçi", VKN: "1111111111"},
			{Code: "120.01.001", Name: "Müşteri Bir", VKN: "7777777777"},
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", fl.home)
	mux.HandleFunc("/giris", fl.login)
	mux.HandleFunc("/cikis", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "oturum", Value: "", Path: "/", MaxAge: -1})
		http.Redirect(w, r, "/", http.StatusFound)
	})
	mux.HandleFunc("/firmalar", fl.firmList)
	mux.HandleFunc("/firma-sec", fl.firmSelect)
	mux.HandleFunc("/hesap-plani", fl.plan)
	mux.HandleFunc("/aktarim", fl.importForm)
	mux.HandleFunc("/aktarim/sonuc", fl.importResult)
	mux.HandleFunc("/plan-aktarim", fl.planForm)
	mux.HandleFunc("/plan-aktarim/sonuc", fl.planResult)
	mux.HandleFunc("/disari", func(w http.ResponseWriter, _ *http.Request) { htmlPage(w, "<p>Luca dışı sayfa</p>") })
	fl.srv = httptest.NewServer(mux)
	t.Cleanup(func() {
		fl.release()
		fl.srv.Close()
	})
	return fl
}

func (fl *fakeLuca) release() {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	if fl.hang != nil {
		close(fl.hang)
		fl.hang = nil
	}
}

func htmlPage(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, "<!doctype html><html lang=\"tr\"><head><meta charset=\"utf-8\"><title>Sahte Luca</title></head><body>%s</body></html>", body)
}

func loggedIn(r *http.Request) bool {
	c, err := r.Cookie("oturum")
	return err == nil && c.Value == "1"
}

func (fl *fakeLuca) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !loggedIn(r) {
		htmlPage(w, `<form id="giris" action="/giris" method="get"><input type="text" name="kullanici"><input type="password" name="sifre"><input name="guvenlik_kodu"><button id="giris-yap" type="submit">Giriş</button></form>`)
		return
	}
	fl.mu.Lock()
	cur, title := fl.current, fl.firms[fl.current]
	fl.mu.Unlock()
	htmlPage(w, `<div id="oturum">Luca</div><div>Seçili firma: <span id="firma-vkn">VKN `+cur+`</span> <span id="firma-unvan">`+html.EscapeString(title)+`</span></div><a id="cikis" href="/cikis">Çıkış</a>`)
}

func (fl *fakeLuca) login(w http.ResponseWriter, r *http.Request) {
	fl.mu.Lock()
	fl.passwords = append(fl.passwords, r.URL.Query().Get("sifre")+r.URL.Query().Get("guvenlik_kodu"))
	fl.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "oturum", Value: "1", Path: "/"})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (fl *fakeLuca) firmList(w http.ResponseWriter, r *http.Request) {
	if !loggedIn(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	fl.mu.Lock()
	defer fl.mu.Unlock()
	var b strings.Builder
	b.WriteString(`<table id="firmalar">`)
	for vkn, title := range fl.firms {
		fmt.Fprintf(&b, `<tr class="firma"><td>%s</td><td class="vkn">%s</td><td><a class="sec" href="/firma-sec?vkn=%s">Seç</a></td></tr>`, html.EscapeString(title), vkn, vkn)
	}
	b.WriteString(`</table>`)
	htmlPage(w, b.String())
}

func (fl *fakeLuca) firmSelect(w http.ResponseWriter, r *http.Request) {
	fl.mu.Lock()
	fl.current = r.URL.Query().Get("vkn")
	if fl.switchTo != "" {
		fl.current = fl.switchTo
	}
	fl.mu.Unlock()
	http.Redirect(w, r, "/", http.StatusFound)
}

func (fl *fakeLuca) plan(w http.ResponseWriter, r *http.Request) {
	if !loggedIn(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	fl.mu.Lock()
	defer fl.mu.Unlock()
	var b strings.Builder
	b.WriteString(`<table id="plan"><tr><th>Kod</th><th>Ad</th><th>VKN</th></tr><tr class="hesap"><td class="kod">320</td><td class="ad">SATICILAR</td><td class="vkn"></td></tr>`)
	for _, a := range fl.accounts {
		fmt.Fprintf(&b, `<tr class="hesap"><td class="kod">%s</td><td class="ad">%s</td><td class="vkn">VKN: %s</td></tr>`, a.Code, html.EscapeString(a.Name), a.VKN)
	}
	b.WriteString(`</table>`)
	htmlPage(w, b.String())
}

func (fl *fakeLuca) importForm(w http.ResponseWriter, r *http.Request) {
	if !loggedIn(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	htmlPage(w, `<form id="aktarim" method="post" enctype="multipart/form-data" action="/aktarim/sonuc">
<input type="file" id="dosya" name="dosya">
<select id="fis-tipi" name="fis_tipi"><option value="">Seçiniz</option><option value="mahsup">Mahsup</option><option value="tahsil">Tahsil</option></select>
<img alt="kod"><input id="kod" name="kod" autocomplete="off">
<button id="yukle" type="submit">Yükle</button></form>`)
}

func (fl *fakeLuca) waitHang(r *http.Request) {
	fl.mu.Lock()
	hang := fl.hang
	fl.mu.Unlock()
	if hang != nil {
		select {
		case <-hang:
		case <-r.Context().Done():
		}
	}
}

func (fl *fakeLuca) importResult(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f, hdr, err := r.FormFile("dosya")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, _ := io.ReadAll(f)
	_ = f.Close()
	fl.mu.Lock()
	fl.uploads = append(fl.uploads, fakeUpload{Name: hdr.Filename, FisTipi: r.FormValue("fis_tipi"), Kod: r.FormValue("kod"), Size: len(data)})
	fl.mu.Unlock()
	fl.waitHang(r)
	if strings.Contains(hdr.Filename, "hata") {
		htmlPage(w, `<div id="sonuc"><span id="basarili">3</span> fiş aktarıldı, 1 fiş aktarılamadı.<table>
<tr class="hata"><td class="satir">12</td><td class="evrak">ABC2026000000012</td><td class="mesaj">Hesap kodu bulunamadı: 320.01.E99 (VKN 1234567890, tutar 1.250,00)</td></tr></table></div>`)
		return
	}
	htmlPage(w, `<div id="sonuc"><span id="basarili">50</span> fiş aktarıldı.</div>`)
}

func (fl *fakeLuca) planForm(w http.ResponseWriter, r *http.Request) {
	if !loggedIn(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	htmlPage(w, `<form id="plan-aktarim" method="post" enctype="multipart/form-data" action="/plan-aktarim/sonuc">
<input type="file" id="plan-dosya" name="dosya"><input id="plan-kod" name="kod"><button id="plan-yukle" type="submit">Yükle</button></form>`)
}

func (fl *fakeLuca) planResult(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f, _, err := r.FormFile("dosya")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	raw, _ := io.ReadAll(f)
	_ = f.Close()
	text, err := charmap.Windows1254.NewDecoder().Bytes(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cr := csv.NewReader(bytes.NewReader(text))
	cr.Comma = ';'
	rows, err := cr.ReadAll()
	if err != nil || len(rows) < 1 {
		http.Error(w, "csv", http.StatusBadRequest)
		return
	}
	fl.mu.Lock()
	fl.planCSV = string(text)
	for _, row := range rows[1:] {
		vkn := row[2]
		if vkn == "" {
			vkn = row[4]
		}
		fl.accounts = append(fl.accounts, Account{Code: row[0], Name: row[1], VKN: vkn})
	}
	fl.mu.Unlock()
	htmlPage(w, fmt.Sprintf(`<div id="plan-sonuc"><span id="plan-basarili">%d</span> hesap aktarıldı.</div>`, len(rows)-1))
}

// --- Test yardımcıları -------------------------------------------------------

func newTestHelper(t *testing.T, fl *fakeLuca, events chan Event) *Helper {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: tarayıcı testi atlandı")
	}
	path, err := FindBrowser()
	if err != nil {
		t.Skip("Chrome / Edge bulunamadı: tarayıcı testi atlandı")
	}
	dir, err := os.MkdirTemp("", "hakan-helper-test-")
	if err != nil {
		t.Fatal(err)
	}
	// Chrome kapandıktan sonra da profil klasörüne yazabildiği için silme
	// hatası testi düşürmez.
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	h := New(testMap(t, fl.srv.URL+"/"), Options{
		ProfileDir: dir, ExecPath: path, Headless: true, NoSandbox: true,
		ReadyTimeout: 5 * time.Second, LoginTimeout: 30 * time.Second, UserTimeout: 30 * time.Second,
		PollEvery: 100 * time.Millisecond,
	}, func(ev Event) {
		select {
		case events <- ev:
		default:
		}
	})
	t.Cleanup(h.Close)
	return h
}

// userJS "kullanıcının" sayfada yaptığı işi çalıştırır.
func userJS(t *testing.T, h *Helper, script string) {
	h.mu.Lock()
	tab := h.tab
	h.mu.Unlock()
	if tab == nil {
		t.Logf("tarayıcı açık değil")
		return
	}
	ctx, cancel := context.WithTimeout(tab, 5*time.Second)
	defer cancel()
	if _, err := chromedp.Run(ctx, chromedp.Evaluate[chromedp.Void](script)); err != nil {
		t.Logf("kullanıcı adımı: %v", err)
	}
}

const userLogin = `document.querySelector("#giris-yap").click()`
const userUpload = `(function(){var k=document.querySelector("#kod")||document.querySelector("#plan-kod");k.value="4821";` +
	`(document.querySelector("#yukle")||document.querySelector("#plan-yukle")).click()})()`

// autoUser waiting_user olaylarında kullanıcı gibi davranır. onUpload
// verilirse Yükle adımı ona bırakılır.
func autoUser(ctx context.Context, t *testing.T, h *Helper, events chan Event, onUpload func()) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-events:
				if ev.Kind != "waiting_user" {
					continue
				}
				switch ev.Reason {
				case "login":
					userJS(t, h, userLogin)
				case "upload_code":
					if onUpload != nil {
						onUpload()
						continue
					}
					userJS(t, h, userUpload)
				}
			}
		}
	}()
}

func testCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// --- Testler -----------------------------------------------------------------

func TestHelperLoginCompanyAccountsImport(t *testing.T) {
	fl := newFakeLuca(t)
	events := make(chan Event, 64)
	h := newTestHelper(t, fl, events)
	ctx := testCtx(t)
	autoUser(ctx, t, h, events, nil)

	if err := h.Open(); err != nil {
		t.Fatal(err)
	}
	if ok, err := h.SessionOpen(ctx); err != nil || ok {
		t.Fatalf("başta oturum kapalı olmalı: %v %v", ok, err)
	}
	if err := h.EnsureSession(ctx); err != nil {
		t.Fatal(err)
	}
	fl.mu.Lock()
	for _, p := range fl.passwords {
		if p != "" {
			t.Errorf("yardımcı şifre ya da güvenlik kodu alanına yazmamalı: %q", p)
		}
	}
	fl.mu.Unlock()

	info, err := h.VerifyCompany(ctx, "1234567890")
	if err != nil {
		t.Fatal(err)
	}
	if info.VKN != "1234567890" || !info.Switched || info.Title == "" {
		t.Fatalf("firma = %+v", info)
	}
	// Firma zaten seçiliyse değiştirilmez.
	if info, err := h.VerifyCompany(ctx, "1234567890"); err != nil || info.Switched {
		t.Fatalf("ikinci doğrulama: %+v %v", info, err)
	}

	accs, err := h.ReadAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(accs) != 4 || accs[2].Code != "320.01.E10" || accs[2].VKN != "1111111111" {
		t.Fatalf("hesaplar = %+v", accs)
	}

	res, err := h.Import(ctx, ImportRequest{Screen: ScreenFis, Path: writeTemp(t, "fis_2026-10_hata.xlsx", []byte("xlsx")), Params: map[string]string{"fis_tipi": "mahsup"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != FilePartial || !res.Clicked || res.SuccessCount != 3 || len(res.Errors) != 1 {
		t.Fatalf("sonuç = %+v", res)
	}
	e := res.Errors[0]
	if e.Line != "12" || e.Ref != "ABC2026000000012" || !strings.Contains(e.Message, "Hesap kodu bulunamadı") {
		t.Fatalf("hata satırı = %+v", e)
	}
	if strings.Contains(e.Message, "1234567890") || strings.Contains(res.Message, "1.250,00") {
		t.Fatalf("Luca mesajı maskelenmeli: %q / %q", e.Message, res.Message)
	}
	fl.mu.Lock()
	up := fl.uploads
	fl.mu.Unlock()
	if len(up) != 1 || up[0].Name != "fis_2026-10_hata.xlsx" || up[0].FisTipi != "mahsup" || up[0].Kod != "4821" || up[0].Size != 4 {
		t.Fatalf("Luca'ya giden = %+v", up)
	}

	// Luca oturumunu kapat: Luca çerezleri silinir, oturum düşer.
	if err := h.LogoutLuca(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.withOp(ctx, func(ctx context.Context) error {
		return chromedp.Do(ctx, chromedp.Navigate(fl.srv.URL+"/"))
	}); err != nil {
		t.Fatal(err)
	}
	if ok, err := h.SessionOpen(ctx); err != nil || ok {
		t.Fatalf("çıkıştan sonra oturum kapalı olmalı: %v %v", ok, err)
	}
}

func TestHelperCompanyNotFoundAndMismatch(t *testing.T) {
	fl := newFakeLuca(t)
	events := make(chan Event, 64)
	h := newTestHelper(t, fl, events)
	ctx := testCtx(t)
	autoUser(ctx, t, h, events, nil)
	if err := h.Open(); err != nil {
		t.Fatal(err)
	}
	if err := h.EnsureSession(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := h.VerifyCompany(ctx, "4444444444"); !errors.Is(err, ErrCompanyNotFound) {
		t.Fatalf("listede olmayan firma: %v", err)
	}
	fl.mu.Lock()
	fl.switchTo = "9999999999" // Luca başka firmayı açar
	fl.mu.Unlock()
	if _, err := h.VerifyCompany(ctx, "1234567890"); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("firma tutmazsa durmalı: %v", err)
	}
	fl.mu.Lock()
	n := len(fl.uploads)
	fl.mu.Unlock()
	if n != 0 {
		t.Fatal("hiçbir şey yüklenmemeli")
	}
}

func TestHelperStopsOutsideAllowedHosts(t *testing.T) {
	fl := newFakeLuca(t)
	events := make(chan Event, 64)
	h := newTestHelper(t, fl, events)
	ctx := testCtx(t)
	outside := strings.Replace(fl.srv.URL, "127.0.0.1", "localhost", 1) + "/disari"
	go func() {
		for ev := range events {
			if ev.Kind == "waiting_user" && ev.Reason == "login" {
				userJS(t, h, `location.href=`+js(outside))
				return
			}
		}
	}()
	if err := h.Open(); err != nil {
		t.Fatal(err)
	}
	if err := h.EnsureSession(ctx); !errors.Is(err, ErrOutsideHost) {
		t.Fatalf("alan dışına çıkınca durmalı: %v", err)
	}
	// Görev başında sıfırlanana kadar yeni iş yapılmaz.
	if _, err := h.ReadAccounts(ctx); !errors.Is(err, ErrOutsideHost) {
		t.Fatalf("koruyucu açık kalmalı: %v", err)
	}
	h.ResetGuard()
	if err := h.withOp(ctx, func(ctx context.Context) error { return h.visit(ctx, "/", "") }); err != nil {
		t.Fatalf("sıfırlandıktan sonra Luca'ya dönülür: %v", err)
	}
}

func TestImportBrowserClosedAfterClickIsUnknown(t *testing.T) {
	fl := newFakeLuca(t)
	fl.hang = make(chan struct{})
	events := make(chan Event, 64)
	h := newTestHelper(t, fl, events)
	ctx := testCtx(t)
	autoUser(ctx, t, h, events, func() {
		userJS(t, h, userUpload)
		// Luca dosyayı aldı, sonuç gelmeden tarayıcı kapandı.
		for i := 0; i < 100; i++ {
			fl.mu.Lock()
			n := len(fl.uploads)
			fl.mu.Unlock()
			if n > 0 {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		go h.Close()
	})
	if err := h.Open(); err != nil {
		t.Fatal(err)
	}
	if err := h.EnsureSession(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := h.Import(ctx, ImportRequest{Screen: ScreenFis, Path: writeTemp(t, "banka.xlsx", []byte("x"))})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Clicked || res.Status != FileUnknown {
		t.Fatalf("Yükle'den sonra sonuç okunamazsa unknown (needs_check) olmalı: %+v", res)
	}
}

func TestImportWithoutClickIsNotUploaded(t *testing.T) {
	fl := newFakeLuca(t)
	events := make(chan Event, 64)
	h := newTestHelper(t, fl, events)
	h.Opts.UserTimeout = 2 * time.Second
	ctx := testCtx(t)
	autoUser(ctx, t, h, events, func() {}) // kullanıcı Yükle'ye basmaz
	if err := h.Open(); err != nil {
		t.Fatal(err)
	}
	if err := h.EnsureSession(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := h.Import(ctx, ImportRequest{Screen: ScreenFis, Path: writeTemp(t, "banka.xlsx", []byte("x"))})
	if !errors.Is(err, ErrNotUploaded) || res.Clicked {
		t.Fatalf("basılmadıysa yüklenmedi sayılmalı: %+v %v", res, err)
	}
}

func newTestRunner(t *testing.T, h *Helper, api *fakeAPI, events chan Event) (*Runner, *LocalState) {
	t.Helper()
	st, err := LoadLocalState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner(h, TaskClient{T: api}, st, func() string { return "ajan-1" }, t.TempDir(), nil)
	r.FileGap = 0
	h.Notify = func(ev Event) {
		r.onHelperEvent(ev)
		select {
		case events <- ev:
		default:
		}
	}
	return r, st
}

func lastBody(t *testing.T, api *fakeAPI, suffix string, into any) {
	t.Helper()
	p := api.postsTo(suffix)
	if len(p) == 0 {
		t.Fatalf("%s gönderilmedi; gönderilenler: %+v", suffix, api.posts)
	}
	if err := json.Unmarshal(p[len(p)-1].Body, into); err != nil {
		t.Fatal(err)
	}
}

// Senaryo 1: cari varsa eşleşir; yoksa sıradaki kodla açılır ve yeniden
// okununca bulunur; sonuç sunucuya döner.
func TestRunnerCariSyncEndToEnd(t *testing.T) {
	fl := newFakeLuca(t)
	events := make(chan Event, 64)
	h := newTestHelper(t, fl, events)
	ctx := testCtx(t)
	api := &fakeAPI{tasks: []Task{{
		UUID: "task-cari", Kind: KindCariSync, Status: StatusQueued, Approved: true,
		Taxpayer: Taxpayer{Title: "Zeytin Gıda Ltd. Şti.", TaxID: "1234567890"},
		Counterparties: []TaskCounterparty{
			{TaxID: "1111111111", Title: "Var Olan Tedarikçi", MainCode: "320", Status: "pending"},
			{TaxID: "2222222222", Title: "Yeni Tedarikçi Çay Ltd. Şti.", MainCode: "320", Status: "pending"},
			{TaxID: "33333333333", Title: "Ayşe Öz", MainCode: "120", Status: "pending"},
		},
	}}}
	r, _ := newTestRunner(t, h, api, events)
	autoUser(ctx, t, h, events, nil)
	r.RunOnce(ctx)

	var res TaskResult
	lastBody(t, api, "/task-cari/result", &res)
	if res.Outcome != OutcomeDone || res.AgentUUID != "ajan-1" {
		t.Fatalf("sonuç = %+v", res)
	}
	got := map[string]CounterpartyResult{}
	for _, c := range res.Counterparties {
		got[c.TaxID] = c
	}
	want := map[string][2]string{
		"1111111111":  {CariFound, "320.01.E10"},
		"2222222222":  {CariCreated, "320.01.E11"},
		"33333333333": {CariCreated, "120.01.002"},
	}
	for vkn, w := range want {
		c := got[vkn]
		if c.Status != w[0] || c.AccountCode != w[1] || c.MainCode == "" {
			t.Errorf("%s = %+v, want %v", vkn, c, w)
		}
	}
	fl.mu.Lock()
	planCSV := fl.planCSV
	fl.mu.Unlock()
	if !strings.HasPrefix(planCSV, "Hesap Kodu;Hesap Adı;") || !strings.Contains(planCSV, "320.01.E11;Yeni Tedarikçi Çay Ltd. Şti.;2222222222") {
		t.Fatalf("hesap planı dosyası: %q", planCSV)
	}
	if strings.Contains(planCSV, "1111111111") {
		t.Fatal("Luca'da olan cari yeniden açılmamalı")
	}
	api.mu.Lock()
	var waited bool
	for _, p := range api.progress {
		if p.Status == StatusWaitingUser {
			waited = true
		}
	}
	api.mu.Unlock()
	if !waited {
		t.Fatal("giriş / kod beklenirken waiting_user bildirilmeli")
	}
}

// Senaryo 2 (masaüstü tarafı): dosyalar sırayla yüklenir, her dosyanın
// sonucu hemen bildirilir; hatalı satır EntryResult olarak döner.
func TestRunnerBulkExportEndToEnd(t *testing.T) {
	fl := newFakeLuca(t)
	fl.current = "1234567890"
	events := make(chan Event, 64)
	h := newTestHelper(t, fl, events)
	ctx := testCtx(t)
	f1, f2 := []byte("banka-dosyasi"), []byte("z-dosyasi")
	sum := func(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
	api := &fakeAPI{
		tasks: []Task{{
			UUID: "task-bulk", Kind: KindBulkExport, Status: StatusQueued, Approved: true,
			Taxpayer: Taxpayer{Title: "Zeytin Gıda Ltd. Şti.", TaxID: "1234567890"},
			Files: []TaskFile{
				{UUID: "f2", Sequence: 2, Name: "z_2026-10_1_hata.xlsx", SHA256: sum(f2), Luca: map[string]string{"fis_tipi": "mahsup"}, Status: "pending"},
				{UUID: "f1", Sequence: 1, Name: "banka_2026-10_1.xlsx", SHA256: sum(f1), Luca: map[string]string{"fis_tipi": "mahsup"}, Status: "pending"},
			},
		}},
		files: map[string][]byte{
			"/v1/tenant/agent-tasks/task-bulk/files/f1/content": f1,
			"/v1/tenant/agent-tasks/task-bulk/files/f2/content": f2,
		},
	}
	r, st := newTestRunner(t, h, api, events)
	autoUser(ctx, t, h, events, nil)
	r.RunOnce(ctx)

	var r1, r2 FileResult
	lastBody(t, api, "/files/f1/result", &r1)
	lastBody(t, api, "/files/f2/result", &r2)
	if r1.Status != FileOK || len(r1.Entries) != 0 {
		t.Fatalf("f1 = %+v", r1)
	}
	if r2.Status != FileFailed || len(r2.Entries) != 1 || r2.Entries[0].DocumentNo != "ABC2026000000012" || r2.Entries[0].Row != 12 || r2.Entries[0].Status != "failed" {
		t.Fatalf("f2 = %+v", r2)
	}
	var res TaskResult
	lastBody(t, api, "/task-bulk/result", &res)
	if res.Outcome != OutcomeDone {
		t.Fatalf("görev = %+v", res)
	}
	fl.mu.Lock()
	up := fl.uploads
	fl.mu.Unlock()
	if len(up) != 2 || up[0].Name != "banka_2026-10_1.xlsx" || up[1].Name != "z_2026-10_1_hata.xlsx" {
		t.Fatalf("sıra = %+v", up)
	}
	if !st.WasUploaded("1234567890", sum(f1)) || !st.WasUploaded("1234567890", sum(f2)) {
		t.Fatal("yüklenen dosyalar yerel kayda yazılmalı")
	}
	api.mu.Lock()
	var step2 bool
	for _, p := range api.progress {
		if strings.Contains(p.Message, "Dosya 2/2") {
			step2 = true
		}
	}
	api.mu.Unlock()
	if !step2 {
		t.Fatal("ilerleme 'Dosya 2/2' göstermeli")
	}

	// Aynı dosya aynı firmaya ikinci kez yüklenmez.
	api.mu.Lock()
	api.tasks[0].Status, api.tasks[0].UUID = StatusQueued, "task-bulk-2"
	api.tasks[0].Agent = nil
	api.files["/v1/tenant/agent-tasks/task-bulk-2/files/f1/content"] = f1
	api.files["/v1/tenant/agent-tasks/task-bulk-2/files/f2/content"] = f2
	api.mu.Unlock()
	r.RunOnce(ctx)
	fl.mu.Lock()
	n := len(fl.uploads)
	fl.mu.Unlock()
	if n != 2 {
		t.Fatalf("aynı dosya yeniden yüklenmemeli: %d yükleme", n)
	}
}
