package helper

// Kayıtlı Luca girişi (Rahat Aktarım gibi). Kullanıcı Luca müşteri no,
// kullanıcı adı ve şifresini masaüstünde BİR KEZ girer; kabuk (Rust) bunları
// işletim sistemi şifre kasasında (Windows Credential Manager / macOS
// Keychain) saklar ve sidecar her açılışta yalnızca belleğe verir
// (`helper.set_login`, ön yüze kapalı). Şifre sunucuya, günlüğe, dosyaya ya
// da hata metnine yazılmaz; değer tarayıcıya CDP çağrısının argümanı olarak
// gider, sayfa koduna gömülmez.
//
// Luca giriş ekranında CAPTCHA / güvenlik kodu çıkarsa yardımcı kodu ÇÖZMEZ:
// alanları doldurur, kodu kullanıcı yazıp girişe basar. Kayıtlı bilgi Luca'da
// reddedilirse yardımcı bir daha denemez (hesap kilitlenmesin); kullanıcı
// bilgileri yeniden kaydedene kadar girişi kendisi yapar.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// Credentials kayıtlı Luca girişi. Yalnızca bellekte tutulur.
type Credentials struct {
	Customer string `json:"customer"`
	User     string `json:"user"`
	Password string `json:"password"`
}

// String şifreyi asla göstermez (yanlışlıkla %v ile yazılırsa).
func (c Credentials) String() string { return "Credentials{***}" }

// GoString %#v için de aynı.
func (c Credentials) GoString() string { return c.String() }

// LoginInfo arayüz için kayıtlı girişin durumu (değer içermez).
type LoginInfo struct {
	// Saved bu bilgisayarda kayıtlı giriş var (kabuk belleğe verdi).
	Saved bool `json:"saved"`
	// Rejected Luca kayıtlı bilgiyi kabul etmedi; yeniden kaydedilene kadar
	// yardımcı otomatik giriş denemez.
	Rejected bool `json:"rejected"`
	// Supported ekran haritasında giriş formu tanımlı (keşiften sonra).
	Supported bool `json:"supported"`
}

// Giriş bilgisi sınırları.
const (
	maxCustomerLen = 64
	maxUserLen     = 128
	maxSecretLen   = 256
)

// errLoginForm giriş formu doldurulamadı. Alttaki hata bilerek taşınmaz:
// hata metnine hiçbir giriş değeri karışmasın.
var errLoginForm = errors.New("luca giriş formu doldurulamadı")

// SetCredentials kayıtlı girişi belleğe alır (kabuk çağırır).
func (h *Helper) SetCredentials(c Credentials) error {
	c.Customer = strings.TrimSpace(c.Customer)
	c.User = strings.TrimSpace(c.User)
	if c.User == "" || c.Password == "" {
		return errors.New("kullanıcı adı ve şifre gerekli")
	}
	if len(c.Customer) > maxCustomerLen || len(c.User) > maxUserLen || len(c.Password) > maxSecretLen {
		return errors.New("giriş bilgisi çok uzun")
	}
	h.mu.Lock()
	h.creds = &c
	h.credsRejected = false
	h.mu.Unlock()
	return nil
}

// ClearCredentials kayıtlı girişi bellekten siler ("kayıtlı girişi sil" ya
// da "her seferinde kendim gireyim").
func (h *Helper) ClearCredentials() {
	h.mu.Lock()
	h.creds = nil
	h.credsRejected = false
	h.mu.Unlock()
}

// LoginInfo kayıtlı girişin durumu.
func (h *Helper) LoginInfo() LoginInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	return LoginInfo{Saved: h.creds != nil, Rejected: h.credsRejected, Supported: h.Map.Session.Login.auto()}
}

func (h *Helper) credentials() (Credentials, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.creds == nil || h.credsRejected {
		return Credentials{}, false
	}
	return *h.creds, true
}

func (h *Helper) rejectCredentials() {
	h.mu.Lock()
	h.credsRejected = true
	h.mu.Unlock()
}

// Bekleme metinleri.
const (
	msgLoginManual   = "Luca'ya giriş yapın. Kullanıcı adı, şifre ve güvenlik kodunu siz yazarsınız; HAKAN bunları görmez ve saklamaz."
	msgLoginCaptcha  = "Luca giriş ekranında güvenlik kodu çıktı. Yardımcı pencerede kodu yazıp Giriş'e basın; HAKAN kodu çözmez."
	msgLoginRejected = "Kayıtlı Luca bilgileri kabul edilmedi. Yardımcı pencerede kendiniz giriş yapın ya da ayarlardan bilgileri yeniden kaydedin."
)

type loginOutcome int

const (
	loginNoForm loginOutcome = iota
	loginOK
	loginCaptcha
	loginRejected
)

// autoLogin kayıtlı bilgiyle giriş formunu doldurur ve (CAPTCHA yoksa)
// girişe basar. Bir EnsureSession çağrısında en çok bir kez çalışır.
func (h *Helper) autoLogin(ctx context.Context, c Credentials) loginOutcome {
	l := h.Map.Session.Login
	if !h.loginFormReady(ctx) {
		return loginNoForm
	}
	if err := h.pace(ctx); err != nil {
		return loginNoForm
	}
	if err := h.fillLogin(ctx, c); err != nil {
		return loginNoForm
	}
	if h.captchaShown(ctx) {
		return loginCaptcha
	}
	if err := h.pace(ctx); err != nil {
		return loginNoForm
	}
	clicked, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(function(){var b=document.querySelector(`+js(l.Submit)+
		`);if(!b)return false;b.click();return true})()`))
	if err != nil || !clicked {
		return loginNoForm
	}
	deadline := time.Now().Add(h.Opts.ReadyTimeout)
	for {
		if h.guardErr() != nil || ctx.Err() != nil {
			return loginNoForm
		}
		if ok, _ := exists(ctx, h.Map.Session.LoggedIn); ok {
			return loginOK
		}
		if ok, _ := exists(ctx, l.Error); ok {
			return loginRejected
		}
		if h.captchaShown(ctx) {
			// Luca ilk denemeden sonra kod isteyebilir: alanlar yeniden
			// doldurulur, kodu kullanıcı yazar.
			if h.loginFormReady(ctx) {
				_ = h.fillLogin(ctx, c)
			}
			return loginCaptcha
		}
		if time.Now().After(deadline) {
			// Süre doldu ve giriş formu hâlâ orada: bilgi kabul edilmemiş
			// sayılır, hesap kilitlenmesin diye yeniden denenmez.
			if h.loginFormReady(ctx) {
				return loginRejected
			}
			return loginNoForm
		}
		if err := sleep(ctx, h.Opts.PollEvery); err != nil {
			return loginNoForm
		}
	}
}

// loginFormReady şifre alanı var ve gerçekten şifre alanı mı (başka bir alana
// değer yazılmaz).
func (h *Helper) loginFormReady(ctx context.Context) bool {
	sel := h.Map.Session.Login.PasswordInput
	ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(function(){var e=document.querySelector(`+js(sel)+
		`);return !!e&&e.tagName==="INPUT"&&e.type==="password"})()`))
	return err == nil && ok
}

func (h *Helper) captchaShown(ctx context.Context) bool {
	sel := h.Map.Session.Login.Captcha
	if sel == "" {
		return false
	}
	ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(function(){var e=document.querySelector(`+js(sel)+
		`);if(!e)return false;if(e.offsetParent===null&&getComputedStyle(e).position!=="fixed")return false;`+
		`if(e.tagName==="INPUT"&&e.type!=="password")e.focus();return true})()`))
	return err == nil && ok
}

func (h *Helper) fillLogin(ctx context.Context, c Credentials) error {
	l := h.Map.Session.Login
	fields := []struct{ sel, val string }{
		{l.CustomerInput, c.Customer},
		{l.UserInput, c.User},
		{l.PasswordInput, c.Password},
	}
	for _, f := range fields {
		if f.sel == "" || f.val == "" {
			continue
		}
		if err := fillField(ctx, f.sel, f.val, h.Opts.ReadyTimeout); err != nil {
			return err
		}
	}
	return nil
}

// fillField değeri CDP argümanıyla alana yazar ve giriş olaylarını tetikler.
func fillField(ctx context.Context, sel, value string, timeout time.Duration) error {
	fctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := chromedp.Do(fctx, chromedp.SetValue(chromedp.CSS(sel), value)); err != nil {
		return errLoginForm
	}
	if _, err := chromedp.Run(fctx, chromedp.Evaluate[chromedp.Void](`(function(){var e=document.querySelector(`+js(sel)+
		`);if(e){e.dispatchEvent(new Event("input",{bubbles:true}));e.dispatchEvent(new Event("change",{bubbles:true}))}})()`)); err != nil {
		return errLoginForm
	}
	return nil
}
