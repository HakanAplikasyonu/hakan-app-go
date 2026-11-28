package helper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

// Options yardımcı tarayıcının ayarları.
type Options struct {
	// ProfileDir HAKAN profil klasörü (<uygulama veri dizini>/helper-profile).
	ProfileDir string
	// ExecPath boşsa FindBrowser.
	ExecPath string
	// Headless ve NoSandbox yalnızca testlerde (CI'da görünür pencere yok).
	Headless  bool
	NoSandbox bool
	// ActionGap iki eylem arası en az bekleme (§5.7: 1–2 sn).
	ActionGap time.Duration
	// ReadyTimeout bir ekranın yüklenmesi için üst süre.
	ReadyTimeout time.Duration
	// LoginTimeout kullanıcının Luca'ya girişi için üst süre.
	LoginTimeout time.Duration
	// UserTimeout kullanıcının kodu girip "Yükle"ye basması için üst süre.
	UserTimeout time.Duration
	// MaxUploadsPerHour saatte en çok yükleme (§5.7, varsayılan 60).
	MaxUploadsPerHour int
	// PollEvery ekran yoklama aralığı.
	PollEvery time.Duration
}

func (o Options) withDefaults() Options {
	if o.ActionGap < 0 {
		o.ActionGap = 0
	}
	if o.ReadyTimeout == 0 {
		o.ReadyTimeout = 30 * time.Second
	}
	if o.LoginTimeout == 0 {
		o.LoginTimeout = 15 * time.Minute
	}
	if o.UserTimeout == 0 {
		o.UserTimeout = 15 * time.Minute
	}
	if o.MaxUploadsPerHour == 0 {
		o.MaxUploadsPerHour = 60
	}
	if o.PollEvery == 0 {
		o.PollEvery = 500 * time.Millisecond
	}
	return o
}

// Event yardımcının arayüze bildirdiği durum değişikliği.
type Event struct {
	// Kind: waiting_user | running | closed
	Kind string `json:"kind"`
	// Reason: login | upload_code (waiting_user için)
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message"`
}

var (
	// ErrNotOpen yardımcı tarayıcı açık değil.
	ErrNotOpen = errors.New("yardımcı tarayıcı açık değil")
	// ErrOutsideHost tarayıcı alan listesi dışındaki bir adrese gitti (§5.5).
	ErrOutsideHost = errors.New("tarayıcı izin verilen alan adlarının dışına çıktı; yardımcı durdu")
	// ErrLoginTimeout kullanıcı süre içinde giriş yapmadı.
	ErrLoginTimeout = errors.New("luca girişi süre içinde yapılmadı")
	// ErrCompanyMismatch ekrandaki firma görevdeki mükellef değil (§2.3 adım 4).
	ErrCompanyMismatch = errors.New("luca'da seçili firma mükellefin VKN'siyle tutmuyor; hiçbir şey yüklenmedi")
	// ErrCompanyNotFound firma listesinde mükellefin VKN'si yok.
	ErrCompanyNotFound = errors.New("luca firma listesinde mükellefin VKN'si bulunamadı")
	// ErrThrottled Luca üst üste "çok istek / hata" sayfası gösterdi.
	ErrThrottled = errors.New("luca üst üste hata sayfası gösterdi; görev durduruldu")
	// ErrRateLimit saatlik yükleme sınırı doldu.
	ErrRateLimit = errors.New("saatlik yükleme sınırı doldu")
	// ErrNotUploaded kullanıcı "Yükle"ye basmadan pencere kapandı ya da süre
	// doldu: dosya Luca'ya gitmedi.
	ErrNotUploaded = errors.New("dosya yüklenmedi: doğrulama kodu girilip Yükle'ye basılmadı")
	// ErrBusy yardımcı başka bir iş yapıyor.
	ErrBusy = errors.New("yardımcı şu an başka bir iş yapıyor")
)

// exposeName sayfada "Yükle"ye basıldığını Go'ya bildiren işlevin adı.
const exposeName = "hakanYukleme"

// Helper yardımcı tarayıcıyı yönetir. Aynı anda tek iş, tek sekme (§5.7).
type Helper struct {
	Map    *Map
	Opts   Options
	Notify func(Event)

	life sync.Mutex // Open / Close sırası
	ops  sync.Mutex // sayfa işleri sırası

	mu          sync.Mutex // aşağıdaki alanlar; komut çalışırken tutulmaz
	tab         context.Context
	tabCancel   context.CancelFunc
	allocCancel context.CancelFunc
	browserPath string
	waiting     string
	lastAction  time.Time
	uploads     []time.Time

	gmu       sync.Mutex // koruyucu (olay döngüsünden çağrılır)
	violation string
	opCancel  context.CancelFunc
	armed     bool
	clicked   bool
}

// New yardımcıyı kurar (tarayıcıyı açmaz).
func New(m *Map, opts Options, notify func(Event)) *Helper {
	if notify == nil {
		notify = func(Event) {}
	}
	return &Helper{Map: m, Opts: opts.withDefaults(), Notify: notify}
}

// State arayüz için özet.
type State struct {
	Open    bool   `json:"open"`
	Browser string `json:"browser,omitempty"`
	Waiting string `json:"waiting,omitempty"`
}

// State açık mı, hangi tarayıcı, ne bekleniyor.
func (h *Helper) State() State {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := State{Open: h.tab != nil, Waiting: h.waiting}
	if h.browserPath != "" {
		s.Browser = BrowserName(h.browserPath)
	}
	return s
}

// Open tarayıcıyı HAKAN profiliyle görünür pencerede açar ve Luca giriş
// sayfasına gider. Açıksa bir şey yapmaz.
func (h *Helper) Open() error {
	h.life.Lock()
	defer h.life.Unlock()
	h.mu.Lock()
	open := h.tab != nil
	h.mu.Unlock()
	if open {
		return nil
	}
	path := h.Opts.ExecPath
	if path == "" {
		p, err := FindBrowser()
		if err != nil {
			return err
		}
		path = p
	}
	if h.Opts.ProfileDir == "" {
		return errors.New("yardımcı profil klasörü verilmedi")
	}
	if err := PrepareProfile(h.Opts.ProfileDir); err != nil {
		return fmt.Errorf("yardımcı profili hazırlanamadı: %w", err)
	}
	// Kendi seçenek listemiz: chromedp'nin varsayılanlarındaki
	// password-store=basic / use-mock-keychain çerezlerin OS anahtarlığıyla
	// şifrelenmesini kapatır; burada yok. Bağlantı pipe ile kurulur
	// (--remote-debugging-pipe, §5.6), ağ portu açılmaz.
	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(path),
		chromedp.UserDataDir(h.Opts.ProfileDir),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-features", "Translate,PasswordLeakDetection,AutofillServerCommunication"),
	}
	if h.Opts.Headless {
		opts = append(opts, chromedp.Headless)
	} else {
		opts = append(opts, chromedp.WindowSize(1280, 900))
	}
	if h.Opts.NoSandbox {
		opts = append(opts, chromedp.NoSandbox)
	}
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	tab, tabCancel := chromedp.NewContext(allocCtx)
	fail := func(err error) error {
		tabCancel()
		allocCancel()
		return fmt.Errorf("yardımcı tarayıcı açılamadı: %w", err)
	}
	if err := chromedp.Do(tab); err != nil {
		return fail(err)
	}
	if err := chromedp.Listen(tab, h.onEvent); err != nil {
		return fail(err)
	}
	if err := chromedp.Do(tab, chromedp.ExposeFunc(exposeName, func(context.Context, []any) (chromedp.Void, error) {
		h.markClicked()
		return chromedp.Void{}, nil
	})); err != nil {
		return fail(err)
	}
	detached, err := chromedp.Detached(tab)
	if err != nil {
		return fail(err)
	}
	h.mu.Lock()
	h.tab, h.tabCancel, h.allocCancel, h.browserPath = tab, tabCancel, allocCancel, path
	h.mu.Unlock()
	go func() {
		// Kullanıcı pencereyi kapattı ya da tarayıcı çöktü.
		select {
		case <-detached:
		case <-tab.Done():
		}
		if h.closeTab(tab) {
			h.Notify(Event{Kind: "closed", Message: "Yardımcı tarayıcı kapandı."})
		}
	}()
	start, err := h.Map.Resolve(h.Map.StartURL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(tab, h.Opts.ReadyTimeout)
	defer cancel()
	if err := chromedp.Do(ctx, chromedp.Navigate(start)); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("luca giriş sayfası açılamadı: %w", err)
	}
	return nil
}

// Close tarayıcıyı kapatır. Profil (Luca oturum çerezi dahil) kalır.
func (h *Helper) Close() {
	h.life.Lock()
	defer h.life.Unlock()
	h.mu.Lock()
	tab := h.tab
	h.mu.Unlock()
	if tab != nil {
		h.closeTab(tab)
	}
}

// closeTab yalnızca hâlâ açık olan aynı sekmeyi kapatır; kapattıysa true.
func (h *Helper) closeTab(tab context.Context) bool {
	h.mu.Lock()
	if h.tab != tab {
		h.mu.Unlock()
		return false
	}
	tabCancel, allocCancel := h.tabCancel, h.allocCancel
	h.tab, h.tabCancel, h.allocCancel, h.waiting = nil, nil, nil, ""
	h.mu.Unlock()
	h.gmu.Lock()
	if h.opCancel != nil {
		h.opCancel()
	}
	h.gmu.Unlock()
	tabCancel()
	allocCancel()
	return true
}

// onEvent sekmenin olay döngüsünde çalışır: hızlı döner, komut çalıştırmaz.
func (h *Helper) onEvent(ev any) {
	e, ok := ev.(*page.EventFrameNavigated)
	if !ok || e.Frame == nil || e.Frame.ParentID != "" {
		return
	}
	if urlAllowed(h.Map.AllowedHosts, e.Frame.URL) {
		return
	}
	host := e.Frame.URL
	if u, err := url.Parse(e.Frame.URL); err == nil && u.Host != "" {
		host = u.Hostname()
	}
	h.gmu.Lock()
	h.violation = host
	cancel := h.opCancel
	h.gmu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// ResetGuard yeni görevin başında önceki alan dışı gezinme kaydını siler.
func (h *Helper) ResetGuard() {
	h.gmu.Lock()
	h.violation = ""
	h.gmu.Unlock()
}

func (h *Helper) guardErr() error {
	h.gmu.Lock()
	defer h.gmu.Unlock()
	if h.violation != "" {
		return fmt.Errorf("%w (%s)", ErrOutsideHost, h.violation)
	}
	return nil
}

func (h *Helper) markClicked() {
	h.gmu.Lock()
	if h.armed {
		h.clicked = true
	}
	h.gmu.Unlock()
}

func (h *Helper) arm(on bool) {
	h.gmu.Lock()
	h.armed, h.clicked = on, false
	h.gmu.Unlock()
}

func (h *Helper) wasClicked() bool {
	h.gmu.Lock()
	defer h.gmu.Unlock()
	return h.clicked
}

// op bir sayfa işi için bağlam: sekmeye bağlı, çağıranın bağlamı bitince ya
// da alan dışı gezinmede iptal olur.
func (h *Helper) op(parent context.Context) (context.Context, func(), error) {
	h.mu.Lock()
	tab := h.tab
	h.mu.Unlock()
	if tab == nil {
		return nil, nil, ErrNotOpen
	}
	if err := h.guardErr(); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(tab)
	stop := context.AfterFunc(parent, cancel)
	h.gmu.Lock()
	h.opCancel = cancel
	h.gmu.Unlock()
	return ctx, func() {
		stop()
		cancel()
		h.gmu.Lock()
		h.opCancel = nil
		h.gmu.Unlock()
	}, nil
}

// withOp sayfa işlerini sıraya koyar.
func (h *Helper) withOp(parent context.Context, fn func(ctx context.Context) error) error {
	h.ops.Lock()
	defer h.ops.Unlock()
	return h.runOp(parent, fn)
}

// runOp ops kilidi tutulurken çalışır.
func (h *Helper) runOp(parent context.Context, fn func(ctx context.Context) error) error {
	ctx, done, err := h.op(parent)
	if err != nil {
		return err
	}
	defer done()
	err = fn(ctx)
	if gerr := h.guardErr(); gerr != nil {
		return gerr
	}
	return err
}

// tryOp meşgulse beklemez (arayüzden gelen denetimler için).
func (h *Helper) tryOp(parent context.Context, fn func(ctx context.Context) error) error {
	if !h.ops.TryLock() {
		return ErrBusy
	}
	defer h.ops.Unlock()
	return h.runOp(parent, fn)
}

func (h *Helper) setWaiting(reason, msg string) {
	h.mu.Lock()
	changed := h.waiting != reason
	h.waiting = reason
	h.mu.Unlock()
	if changed && reason != "" {
		h.Notify(Event{Kind: "waiting_user", Reason: reason, Message: msg})
	}
	if changed && reason == "" {
		h.Notify(Event{Kind: "running", Message: msg})
	}
}

// pace iki eylem arasında en az ActionGap bekler (§5.7).
func (h *Helper) pace(ctx context.Context) error {
	h.mu.Lock()
	wait := time.Until(h.lastAction.Add(h.Opts.ActionGap))
	h.mu.Unlock()
	if wait > 0 {
		if err := sleep(ctx, wait); err != nil {
			return err
		}
	}
	h.mu.Lock()
	h.lastAction = time.Now()
	h.mu.Unlock()
	return nil
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// --- Sayfa yardımcıları (seçiciler JSON ile kaçışlanır) ---------------------

func js(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func exists(ctx context.Context, sel string) (bool, error) {
	if sel == "" {
		return false, nil
	}
	return chromedp.Run(ctx, chromedp.Evaluate[bool](`!!document.querySelector(`+js(sel)+`)`))
}

func textOf(ctx context.Context, sel string) (*string, error) {
	return chromedp.Run(ctx, chromedp.Evaluate[*string](`(function(){var e=document.querySelector(`+js(sel)+
		`);return e?(e.innerText||e.textContent||"").trim():null})()`))
}

// waitFor seçici görünene kadar yoklar; süre dolarsa ErrScreenChanged.
func (h *Helper) waitFor(ctx context.Context, sel string) error {
	deadline := time.Now().Add(h.Opts.ReadyTimeout)
	for {
		ok, err := exists(ctx, sel)
		if err == nil && ok {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w (%s)", ErrScreenChanged, sel)
		}
		if err := sleep(ctx, h.Opts.PollEvery); err != nil {
			return err
		}
	}
}

// visit adrese gider, Luca'nın hata sayfasında üstel bekler (en çok üç kez)
// ve hazır işaretini bekler.
func (h *Helper) visit(ctx context.Context, path, ready string) error {
	target, err := h.Map.Resolve(path)
	if err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		if err := h.pace(ctx); err != nil {
			return err
		}
		nctx, cancel := context.WithTimeout(ctx, h.Opts.ReadyTimeout)
		err := chromedp.Do(nctx, chromedp.Navigate(target))
		cancel()
		if gerr := h.guardErr(); gerr != nil {
			return gerr
		}
		if err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if throttled, _ := exists(ctx, h.Map.Session.Throttled); throttled {
			if attempt >= 2 {
				return ErrThrottled
			}
			if err := sleep(ctx, time.Duration(10<<attempt)*time.Second); err != nil {
				return err
			}
			continue
		}
		if ready == "" {
			return err
		}
		return h.waitFor(ctx, ready)
	}
}

// --- Oturum ----------------------------------------------------------------

// SessionOpen Luca'da oturum açık mı ("oturum açık" işaretine bakar).
func (h *Helper) SessionOpen(ctx context.Context) (bool, error) {
	var ok bool
	err := h.tryOp(ctx, func(ctx context.Context) error {
		var err error
		ok, err = exists(ctx, h.Map.Session.LoggedIn)
		return err
	})
	return ok, err
}

// EnsureSession oturum yoksa kullanıcıdan girişi bekler. Yardımcı giriş
// formuna dokunmaz; yalnızca "oturum açık" işaretinin gelmesini yoklar.
func (h *Helper) EnsureSession(ctx context.Context) error {
	return h.withOp(ctx, func(ctx context.Context) error {
		loc, _ := chromedp.Run(ctx, chromedp.Location())
		if u, err := url.Parse(loc); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			start, err := h.Map.Resolve(h.Map.StartURL)
			if err != nil {
				return err
			}
			nctx, cancel := context.WithTimeout(ctx, h.Opts.ReadyTimeout)
			_ = chromedp.Do(nctx, chromedp.Navigate(start))
			cancel()
		}
		deadline := time.Now().Add(h.Opts.LoginTimeout)
		for {
			if err := h.guardErr(); err != nil {
				return err
			}
			ok, err := exists(ctx, h.Map.Session.LoggedIn)
			if err == nil && ok {
				h.setWaiting("", "Luca oturumu açık.")
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			h.setWaiting("login", "Luca'ya giriş yapın. Kullanıcı adı, şifre ve güvenlik kodunu siz yazarsınız; HAKAN bunları görmez ve saklamaz.")
			if time.Now().After(deadline) {
				h.setWaiting("", "")
				return ErrLoginTimeout
			}
			if err := sleep(ctx, h.Opts.PollEvery*2); err != nil {
				return err
			}
		}
	})
}

// --- Firma doğrulama --------------------------------------------------------

// CompanyInfo ekranda doğrulanan firma.
type CompanyInfo struct {
	VKN      string `json:"vkn"`
	Title    string `json:"title,omitempty"`
	Switched bool   `json:"switched"`
}

// VerifyCompany seçili firmayı mükellefin VKN'siyle karşılaştırır; farklıysa
// firma listesinden VKN'si tutan satırı seçer ve yeniden doğrular. Tutmazsa
// durur (hiçbir şey yüklenmez).
func (h *Helper) VerifyCompany(ctx context.Context, vkn string) (CompanyInfo, error) {
	vkn = digits(vkn)
	if len(vkn) != 10 && len(vkn) != 11 {
		return CompanyInfo{}, errors.New("mükellefin VKN/TCKN'si geçersiz")
	}
	var info CompanyInfo
	err := h.withOp(ctx, func(ctx context.Context) error {
		cur, err := h.currentCompany(ctx)
		if err == nil && cur.VKN == vkn {
			info = cur
			return nil
		}
		c := h.Map.Company
		if c.SwitchURL == "" || c.Row == "" || c.RowVKN == "" || c.RowOpen == "" {
			return ErrCompanyMismatch
		}
		if err := h.visit(ctx, c.SwitchURL, c.Row); err != nil {
			return err
		}
		if err := h.pace(ctx); err != nil {
			return err
		}
		res, err := chromedp.Run(ctx, chromedp.Evaluate[string](`(function(){var rows=document.querySelectorAll(`+js(c.Row)+
			`);for(var i=0;i<rows.length;i++){var v=rows[i].querySelector(`+js(c.RowVKN)+
			`);var t=v?(v.innerText||v.textContent||""):"";if(t.replace(/\D/g,"")===`+js(vkn)+
			`){var o=rows[i].querySelector(`+js(c.RowOpen)+`);if(!o)return "noopen";o.click();return "ok"}}return "none"})()`))
		if err != nil {
			return err
		}
		switch res {
		case "none":
			return ErrCompanyNotFound
		case "noopen":
			return fmt.Errorf("%w (%s)", ErrScreenChanged, c.RowOpen)
		}
		deadline := time.Now().Add(h.Opts.ReadyTimeout)
		for {
			if err := h.guardErr(); err != nil {
				return err
			}
			// Sayfa başlığında eski firma bir süre görünebilir: süre dolana
			// kadar beklenir, sonra durulur.
			cur, err := h.currentCompany(ctx)
			if err == nil && cur.VKN == vkn {
				cur.Switched = true
				info = cur
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if time.Now().After(deadline) {
				return ErrCompanyMismatch
			}
			if err := sleep(ctx, h.Opts.PollEvery); err != nil {
				return err
			}
		}
	})
	return info, err
}

func (h *Helper) currentCompany(ctx context.Context) (CompanyInfo, error) {
	t, err := textOf(ctx, h.Map.Company.CurrentVKN)
	if err != nil {
		return CompanyInfo{}, err
	}
	if t == nil {
		return CompanyInfo{}, fmt.Errorf("%w (%s)", ErrScreenChanged, h.Map.Company.CurrentVKN)
	}
	info := CompanyInfo{VKN: firstTaxID(*t)}
	if h.Map.Company.CurrentTitle != "" {
		if tt, err := textOf(ctx, h.Map.Company.CurrentTitle); err == nil && tt != nil {
			info.Title = mask(*tt, 200)
		}
	}
	return info, nil
}

// --- Hesap planı -------------------------------------------------------------

// Account Luca hesap planındaki bir hesap.
type Account struct {
	Code string `json:"code"`
	Name string `json:"name"`
	VKN  string `json:"vkn"`
}

// ReadAccounts hesap planı / cari listesi ekranını okur.
//
// TODO(keşif): Luca listesi sayfalıysa ya da yalnızca Excel çıktısıyla
// okunabiliyorsa (tasarım §3 "Cari sorgulama") burası o yola göre
// genişletilir.
func (h *Helper) ReadAccounts(ctx context.Context) ([]Account, error) {
	var out []Account
	err := h.withOp(ctx, func(ctx context.Context) error {
		a := h.Map.Accounts
		ready := a.Ready
		if ready == "" {
			ready = a.Row
		}
		if err := h.visit(ctx, a.URL, ready); err != nil {
			return err
		}
		rows, err := chromedp.Run(ctx, chromedp.Evaluate[[]Account](`(function(){function t(r,s){if(!s)return "";var e=r.querySelector(s);return e?(e.innerText||e.textContent||"").trim():""}`+
			`return Array.from(document.querySelectorAll(`+js(a.Row)+`)).map(function(r){return {code:t(r,`+js(a.Code)+`),name:t(r,`+js(a.Name)+`),vkn:t(r,`+js(a.VKN)+`)}})})()`))
		if err != nil {
			return err
		}
		for _, r := range rows {
			r.Code = strings.TrimSpace(r.Code)
			r.VKN = firstTaxID(r.VKN)
			if r.Code != "" {
				out = append(out, r)
			}
		}
		return nil
	})
	return out, err
}

// --- Luca oturumunu kapat -----------------------------------------------------

// LogoutLuca Luca'dan çıkar ve Luca alan adlarının çerezlerini siler (§1.3).
func (h *Helper) LogoutLuca(ctx context.Context) error {
	if err := h.Open(); err != nil {
		return err
	}
	return h.withOp(ctx, func(ctx context.Context) error {
		if sel := h.Map.Logout.Button; sel != "" {
			if ok, _ := exists(ctx, sel); ok {
				_, _ = chromedp.Run(ctx, chromedp.Evaluate[chromedp.Void](`document.querySelector(`+js(sel)+`).click()`))
				_ = sleep(ctx, 2*time.Second)
			}
		}
		res, err := chromedp.CallBrowser(ctx, storage.GetCookies, storage.GetCookiesParams{})
		if err != nil {
			return err
		}
		for _, c := range res.Cookies {
			if c == nil || !hostAllowed(h.Map.AllowedHosts, strings.TrimPrefix(c.Domain, ".")) {
				continue
			}
			if _, err := chromedp.Call(ctx, network.DeleteCookies, network.DeleteCookiesParams{Name: c.Name, Domain: c.Domain, Path: c.Path}); err != nil {
				return err
			}
		}
		return nil
	})
}
