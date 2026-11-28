package helper

// Ekran haritası (tasarım notu §1.4): Luca ekranlarının seçicileri, adresleri
// ve beklenen metinleri kodda değil, maps/luca.json veri dosyasındadır. Luca
// ekranı değişince yalnızca bu dosya güncellenir. Beklenen öğe bulunamazsa
// yardımcı tahmin etmez, tıklamaz: görevi ErrScreenChanged ile durdurur.

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

//go:embed maps/luca.json
var lucaMapJSON []byte

// Map bir web uygulamasının (Luca) ekran haritasıdır.
type Map struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// Verified keşifle doğrulandı mı (tasarım §3.1). Placeholder doğruysa
	// seçiciler yer tutucudur: yardımcı görev üstlenmez.
	Verified    bool   `json:"verified"`
	Placeholder bool   `json:"placeholder"`
	CheckedAt   string `json:"checked_at,omitempty"`
	// AllowedHosts yardımcının iş yapabildiği alan adları (§5.5). "*.ornek"
	// alt alan adlarını da kapsar.
	AllowedHosts []string `json:"allowed_hosts"`
	// StartURL Luca giriş / ana sayfası. Diğer adresler buna göre çözülür.
	StartURL string `json:"start_url"`

	Session    SessionMap  `json:"session"`
	Company    CompanyMap  `json:"company"`
	Accounts   AccountsMap `json:"accounts"`
	Import     ImportMap   `json:"import"`
	PlanImport ImportMap   `json:"plan_import"`
	PlanFile   PlanFileMap `json:"plan_file"`
	Logout     LogoutMap   `json:"logout"`
	TODO       []string    `json:"todo,omitempty"`
}

// SessionMap oturum işaretleri. Yardımcı giriş formunu okumaz, doldurmaz;
// yalnızca "oturum açık" işaretinin varlığına bakar.
type SessionMap struct {
	LoggedIn  string `json:"logged_in"`
	LoginForm string `json:"login_form"`
	// Throttled Luca'nın "çok istek / hata" sayfasının işareti (§5.7).
	Throttled string `json:"throttled,omitempty"`
}

// CompanyMap firma seçimi ve doğrulaması (§2.3 adım 4).
type CompanyMap struct {
	// CurrentVKN seçili firmanın VKN'sini içeren öğe (metninde aranır).
	CurrentVKN   string `json:"current_vkn"`
	CurrentTitle string `json:"current_title,omitempty"`
	SwitchURL    string `json:"switch_url"`
	Row          string `json:"row"`
	RowVKN       string `json:"row_vkn"`
	RowOpen      string `json:"row_open"`
}

// AccountsMap hesap planı / cari listesi ekranı (Senaryo 1, adım 2).
type AccountsMap struct {
	URL   string `json:"url"`
	Ready string `json:"ready"`
	Row   string `json:"row"`
	Code  string `json:"code"`
	Name  string `json:"name"`
	VKN   string `json:"vkn"`
}

// SelectStep içe aktarma ekranındaki bir seçim (Fiş Tipi, Belge Türü …).
// Değer sabittir (Value) ya da görevin parametresinden gelir (Param).
type SelectStep struct {
	Selector string `json:"selector"`
	Value    string `json:"value,omitempty"`
	Param    string `json:"param,omitempty"`
	Optional bool   `json:"optional,omitempty"`
}

// ImportMap Luca'nın kendi içe aktarma ekranı (yöntem (a), §3).
type ImportMap struct {
	URL       string       `json:"url"`
	Ready     string       `json:"ready"`
	FileInput string       `json:"file_input"`
	Selects   []SelectStep `json:"selects,omitempty"`
	// CodeInput doğrulama kodu alanı: yardımcı yalnızca odaklar, kodu
	// kullanıcı yazar (§5.2).
	CodeInput    string `json:"code_input"`
	UploadButton string `json:"upload_button"`
	// Result sonuç alanı; içinde başarı sayısı ve hata satırları okunur.
	Result         string `json:"result"`
	SuccessCount   string `json:"success_count,omitempty"`
	SuccessPattern string `json:"success_pattern,omitempty"`
	FailurePattern string `json:"failure_pattern,omitempty"`
	ErrorRow       string `json:"error_row,omitempty"`
	ErrorLine      string `json:"error_line,omitempty"`
	ErrorRef       string `json:"error_ref,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
}

// PlanColumn hesap planı aktarım dosyasının bir sütunu.
type PlanColumn struct {
	Title string `json:"title"`
	// Key: code | name | tax_no | tckn | tax_office | address
	Key string `json:"key"`
}

// PlanFileMap yeni carilerin yüklendiği hesap planı dosyasının biçimi.
type PlanFileMap struct {
	Extension string `json:"extension"`
	Separator string `json:"separator"`
	// Encoding windows-1254 (varsayılan; backend hesap_plani şablonu gibi) ya
	// da utf-8. LineEnding crlf (varsayılan) ya da lf.
	Encoding   string       `json:"encoding,omitempty"`
	LineEnding string       `json:"line_ending,omitempty"`
	Header     bool         `json:"header"`
	Columns    []PlanColumn `json:"columns"`
}

// LogoutMap "Luca oturumunu kapat" (§1.3).
type LogoutMap struct {
	Button string `json:"button,omitempty"`
	URL    string `json:"url,omitempty"`
}

// ErrScreenChanged beklenen öğe bulunamadı: yardımcı tahmin etmez.
var ErrScreenChanged = errors.New("ekran değişmiş olabilir: Luca'da beklenen öğe bulunamadı")

// DefaultLucaMap gömülü Luca haritası.
func DefaultLucaMap() (*Map, error) { return ParseMap(lucaMapJSON) }

// ParseMap haritayı çözer ve denetler.
func ParseMap(data []byte) (*Map, error) {
	var m Map
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("ekran haritası okunamadı: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate zorunlu alanları denetler.
func (m *Map) Validate() error {
	var missing []string
	need := func(name, v string) {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, name)
		}
	}
	need("start_url", m.StartURL)
	need("session.logged_in", m.Session.LoggedIn)
	need("company.current_vkn", m.Company.CurrentVKN)
	need("accounts.url", m.Accounts.URL)
	need("accounts.row", m.Accounts.Row)
	need("accounts.code", m.Accounts.Code)
	for name, im := range map[string]ImportMap{"import": m.Import, "plan_import": m.PlanImport} {
		need(name+".url", im.URL)
		need(name+".file_input", im.FileInput)
		need(name+".upload_button", im.UploadButton)
		need(name+".result", im.Result)
	}
	if len(m.AllowedHosts) == 0 {
		missing = append(missing, "allowed_hosts")
	}
	if len(m.PlanFile.Columns) == 0 {
		missing = append(missing, "plan_file.columns")
	}
	if len(missing) > 0 {
		return fmt.Errorf("ekran haritası eksik: %s", strings.Join(missing, ", "))
	}
	u, err := url.Parse(m.StartURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("ekran haritası: start_url geçerli bir adres değil")
	}
	if !hostAllowed(m.AllowedHosts, u.Hostname()) {
		return errors.New("ekran haritası: start_url alan listesinde değil")
	}
	return nil
}

// Resolve haritadaki göreli adresi start_url'e göre mutlak yapar.
func (m *Map) Resolve(p string) (string, error) {
	base, err := url.Parse(m.StartURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(p)
	if err != nil {
		return "", err
	}
	out := base.ResolveReference(ref)
	if !hostAllowed(m.AllowedHosts, out.Hostname()) {
		return "", fmt.Errorf("adres alan listesinde değil: %s", out.Hostname())
	}
	return out.String(), nil
}

// hostAllowed alan listesi denetimi (§5.5).
func hostAllowed(allowed []string, host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return false
	}
	for _, a := range allowed {
		a = strings.ToLower(strings.TrimSpace(a))
		if suffix, ok := strings.CutPrefix(a, "*."); ok {
			if strings.HasSuffix(host, "."+suffix) {
				return true
			}
			continue
		}
		if host == a {
			return true
		}
	}
	return false
}

// urlAllowed bir gezinme adresi alan listesinde mi. http(s) dışı iç
// sayfalar (about:blank, tarayıcı hata sayfası) gezinme sayılmaz.
func urlAllowed(allowed []string, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "http", "https":
		return hostAllowed(allowed, u.Hostname())
	case "about", "chrome-error":
		return true
	}
	return false
}
