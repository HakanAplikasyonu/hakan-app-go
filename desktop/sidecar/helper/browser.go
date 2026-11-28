// Package helper, masaüstü yardımcısıdır (tasarım notu
// docs/tasarim/masaustu-luca-sgk.md): bilgisayardaki Edge ya da Chrome'u,
// HAKAN'a ait ayrı bir profille, görünür bir pencerede chromedp ile sürer.
//
// Kurallar (§5):
//   - Luca'ya kullanıcı kendisi giriş yapar; şifre, CAPTCHA ve doğrulama kodu
//     alanları okunmaz, doldurulmaz. Şifre ve çerez sunucuya gitmez.
//   - Profilde parola kaydetme ve otomatik doldurma kapalıdır.
//   - Yardımcı yalnızca ekran haritasındaki alan adlarında iş yapar; başka
//     adrese yönlenirse durur.
//   - "Yükle"ye kullanıcı basar; sonuç okunamazsa görev needs_check olur.
package helper

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// BrowserEnv tarayıcı yolunu elle vermek için ortam değişkeni.
const BrowserEnv = "HAKAN_HELPER_BROWSER"

// ErrNoBrowser bilgisayarda Edge ya da Chrome bulunamadı.
var ErrNoBrowser = errors.New("bilgisayarda Microsoft Edge ya da Google Chrome bulunamadı")

// BrowserCandidates işletim sistemine göre aranan tarayıcı yolları. Windows'ta
// önce Edge (her zaman kurulu), sonra Chrome; macOS ve Linux'ta önce Chrome.
func BrowserCandidates(goos string, env func(string) string) []string {
	switch goos {
	case "windows":
		var out []string
		for _, base := range []string{env("ProgramFiles(x86)"), env("ProgramFiles"), env("LOCALAPPDATA")} {
			if base != "" {
				out = append(out, filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"))
			}
		}
		for _, base := range []string{env("ProgramFiles"), env("ProgramFiles(x86)"), env("LOCALAPPDATA")} {
			if base != "" {
				out = append(out, filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"))
			}
		}
		return out
	case "darwin":
		apps := []string{
			"Google Chrome.app/Contents/MacOS/Google Chrome",
			"Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"Chromium.app/Contents/MacOS/Chromium",
		}
		var out []string
		for _, a := range apps {
			out = append(out, filepath.Join("/Applications", a))
			if home := env("HOME"); home != "" {
				out = append(out, filepath.Join(home, "Applications", a))
			}
		}
		return out
	}
	return []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "microsoft-edge-stable"}
}

// FindBrowser kullanılacak tarayıcının yolunu bulur.
func FindBrowser() (string, error) {
	if p := strings.TrimSpace(os.Getenv(BrowserEnv)); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", ErrNoBrowser
	}
	for _, c := range BrowserCandidates(runtime.GOOS, os.Getenv) {
		if filepath.IsAbs(c) {
			if info, err := os.Stat(c); err == nil && !info.IsDir() {
				return c, nil
			}
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", ErrNoBrowser
}

// BrowserName kullanıcıya gösterilen ad.
func BrowserName(path string) string {
	l := strings.ToLower(filepath.Base(path))
	switch {
	case strings.Contains(l, "edge"):
		return "Microsoft Edge"
	case strings.Contains(l, "chromium"):
		return "Chromium"
	case strings.Contains(l, "chrome"):
		return "Google Chrome"
	}
	return filepath.Base(path)
}

// profilePrefs profilin parola ve otomatik doldurma tercihleri (§1.3):
// tarayıcının kendi kasası da şifre saklamaz.
var profilePrefs = map[string]any{
	"credentials_enable_service":              false,
	"credentials_enable_autosignin":           false,
	"profile.password_manager_enabled":        false,
	"profile.password_manager_leak_detection": false,
	"autofill.enabled":                        false,
	"autofill.profile_enabled":                false,
	"autofill.credit_card_enabled":            false,
	"browser.show_home_button":                false,
	"signin.allowed":                          false,
	"sync.requested":                          false,
}

// PrepareProfile HAKAN profil klasörünü hazırlar: klasör izni 0700 ve
// Default/Preferences içinde parola kaydetme kapalı. Tarayıcının yazdığı
// diğer tercihler korunur.
func PrepareProfile(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "Default"), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "Default", "Preferences")
	prefs := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil { //nolint:gosec // HAKAN'ın kendi profil klasörü
		if json.Unmarshal(raw, &prefs) != nil {
			prefs = map[string]any{}
		}
	}
	for k, v := range profilePrefs {
		setPref(prefs, strings.Split(k, "."), v)
	}
	out, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}

// setPref noktalı anahtarı iç içe nesneye yazar.
func setPref(m map[string]any, keys []string, v any) {
	if len(keys) == 1 {
		m[keys[0]] = v
		return
	}
	child, ok := m[keys[0]].(map[string]any)
	if !ok {
		child = map[string]any{}
		m[keys[0]] = child
	}
	setPref(child, keys[1:], v)
}

// prefValue testler ve tanılama için noktalı anahtarın değeri.
func prefValue(m map[string]any, key string) (any, bool) {
	keys := strings.Split(key, ".")
	cur := any(m)
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = mm[k]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}
