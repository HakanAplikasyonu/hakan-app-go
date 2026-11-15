// Package sources, mükellefin kendi sunucusunda kurulu ön muhasebe
// programlarını (Logo Tiger / GO, Mikro) salt okunur bir SQL kullanıcısıyla
// okuyan yerel ajan adaptörüdür (docs/kaynak/musavir-entegrasyon-paketi
// 02 §6, 14 §9, 15 §2).
//
// Kurallar:
//   - Yalnızca bu paketle gelen (veya yapılandırma dosyasıyla değiştirilen)
//     adlı, parametreli SELECT sorguları çalışır; serbest SQL çalışmaz.
//     Her sorgu metni çalıştırılmadan önce ValidateQuery'den geçer.
//   - ⚠️ işaretli (doğrulanmamış) tablo / kolon adları ve tür kodu eşlemeleri
//     tek yerde durur: defaults.json (gömülü) + isteğe bağlı geçersiz kılma
//     dosyası (Config.OverridePath). Kod içine dağıtılmaz.
//   - Bilinmeyen değer çökertmez: satır ham haliyle korunur, kodlu uyarı
//     (TYPE_UNMAPPED vb.) eklenir.
//   - Tutarlar ondalık dizgi olarak taşınır (shopspring/decimal); float yok.
package sources

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/rpc"
)

// Kind kaynak programdır. Değerler backend'deki
// source_connections.source_system değerleriyle aynıdır.
type Kind string

const (
	KindLogoTiger Kind = "logo_tiger"
	KindLogoGO    Kind = "logo_go"
	KindMikro     Kind = "mikro"
)

// Valid, bilinen bir kaynak mı.
func (k Kind) Valid() bool {
	switch k {
	case KindLogoTiger, KindLogoGO, KindMikro:
		return true
	}
	return false
}

// IsLogo, Logo tablo adlandırmasının (LG_{FFF}_{PP}_...) geçerli olduğu kaynak mı.
func (k Kind) IsLogo() bool { return k == KindLogoTiger || k == KindLogoGO }

// profileKey, defaults.json'daki profil anahtarı. Tiger ve GO aynı SQL
// şemasını paylaşır (14 §9).
func (k Kind) profileKey() string {
	if k.IsLogo() {
		return "logo"
	}
	return string(k)
}

// Hata kodları (rpc.Error.Code olarak döner).
const (
	// CodeQueryRejected: sorgu metni salt okuma güvenlik denetiminden geçmedi.
	CodeQueryRejected = "QUERY_REJECTED"
	// CodeQueryFailed: sorgu sunucuda hata verdi (tablo / kolon yok, yetki vb.).
	CodeQueryFailed = "QUERY_FAILED"
)

// Varsayılanlar.
const (
	DefaultPort    = 1433
	DefaultMaxRows = 50_000
	DefaultTimeout = 2 * time.Minute
	// AppName, SQL Server oturumlarında görünen uygulama adı.
	AppName = "hakan-agent"
)

// Config tek bir yerel kaynak bağlantısıdır. Parola ajan makinesinde OS
// anahtar zincirinde durur; buraya yalnızca çalışma anında konur, loglanmaz
// (String() maskeler).
type Config struct {
	Kind Kind `json:"kind"`
	// Host SQL Server adresi. Adlandırılmış örnek için Instance kullanılır.
	Host     string `json:"host"`
	Instance string `json:"instance,omitempty"`
	// Port 0 ise DefaultPort. Instance verilmişse ve Port 0 ise port
	// SQL Browser ile çözülür (DSN'ye port yazılmaz).
	Port     int    `json:"port,omitempty"`
	Database string `json:"database"`
	User     string `json:"user"`
	Password string `json:"password"`
	// FirmNo Logo firma no (FFF, tam 3 hane), PeriodNo dönem no (PP, tam 2 hane).
	// Yalnızca Logo kaynaklarında gerekir.
	FirmNo   string `json:"firm_no,omitempty"`
	PeriodNo string `json:"period_no,omitempty"`
	// OverridePath, gömülü sorgu / eşleme tablosunu geçersiz kılan JSON
	// dosyası (pilot mükellefte doğrulanan adlar için). Boşsa gömülü
	// varsayılanlar kullanılır.
	OverridePath string `json:"override_path,omitempty"`
	// TrustServerCertificate, yerel sunucunun kendinden imzalı sertifikasına
	// güvenilsin mi (çoğu kurulu Logo / Mikro sunucusunda gerekir).
	TrustServerCertificate bool `json:"trust_server_certificate,omitempty"`
	// MaxRows tek sorguda okunacak en çok satır (0: DefaultMaxRows).
	MaxRows int `json:"max_rows,omitempty"`
	// TimeoutSeconds tek sorgunun süre sınırı, saniye (0: DefaultTimeout).
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

var (
	firmNoRe   = regexp.MustCompile(`^[0-9]{3}$`)
	periodNoRe = regexp.MustCompile(`^[0-9]{2}$`)
	// instanceRe SQL Server örnek adı (harf, rakam, _, $, -).
	instanceRe = regexp.MustCompile(`^[A-Za-z0-9_$-]{1,64}$`)
)

// ValidateFirmPeriod, Logo firma / dönem numarasını denetler. Tablo adları
// SQL parametresi olamadığından bu değerler sorgu metnine yalnızca bu
// denetimden sonra yazılır.
func ValidateFirmPeriod(firm, period string) error {
	if !firmNoRe.MatchString(firm) {
		return rpc.Errorf(rpc.CodeInvalidParams, "firma no tam 3 haneli rakam olmalı (ör. 001), gelen: %q", firm)
	}
	if !periodNoRe.MatchString(period) {
		return rpc.Errorf(rpc.CodeInvalidParams, "dönem no tam 2 haneli rakam olmalı (ör. 01), gelen: %q", period)
	}
	return nil
}

// Validate yapılandırmayı denetler.
func (c Config) Validate() error {
	if !c.Kind.Valid() {
		return rpc.Errorf(rpc.CodeInvalidParams, "bilinmeyen kaynak türü: %q", c.Kind)
	}
	if strings.TrimSpace(c.Host) == "" {
		return rpc.Errorf(rpc.CodeInvalidParams, "sunucu adresi gerekli")
	}
	if strings.ContainsAny(c.Host, "/\\@?#") {
		return rpc.Errorf(rpc.CodeInvalidParams, "sunucu adresi geçersiz; adlandırılmış örnek için instance alanını kullanın")
	}
	if c.Instance != "" && !instanceRe.MatchString(c.Instance) {
		return rpc.Errorf(rpc.CodeInvalidParams, "SQL Server örnek adı geçersiz")
	}
	if c.Port < 0 || c.Port > 65535 {
		return rpc.Errorf(rpc.CodeInvalidParams, "port geçersiz: %d", c.Port)
	}
	if strings.TrimSpace(c.Database) == "" {
		return rpc.Errorf(rpc.CodeInvalidParams, "veritabanı adı gerekli")
	}
	if strings.TrimSpace(c.User) == "" {
		return rpc.Errorf(rpc.CodeInvalidParams, "SQL kullanıcısı gerekli")
	}
	if c.Kind.IsLogo() {
		if err := ValidateFirmPeriod(c.FirmNo, c.PeriodNo); err != nil {
			return err
		}
	}
	if c.MaxRows < 0 {
		return rpc.Errorf(rpc.CodeInvalidParams, "max_rows negatif olamaz")
	}
	if c.TimeoutSeconds < 0 {
		return rpc.Errorf(rpc.CodeInvalidParams, "timeout_seconds negatif olamaz")
	}
	return nil
}

func (c Config) maxRows() int {
	if c.MaxRows > 0 {
		return c.MaxRows
	}
	return DefaultMaxRows
}

func (c Config) timeout() time.Duration {
	if c.TimeoutSeconds > 0 {
		return time.Duration(c.TimeoutSeconds) * time.Second
	}
	return DefaultTimeout
}

// DSN, go-mssqldb bağlantı adresini üretir:
// sqlserver://user:pass@host:port[/instance]?database=...&app+name=hakan-agent
// Dönen değer parola içerir; loglanmaz, hata mesajına konmaz.
func (c Config) DSN() string {
	return c.dsnURL(url.UserPassword(c.User, c.Password)).String()
}

func (c Config) dsnURL(user *url.Userinfo) *url.URL {
	host := c.Host
	switch {
	case c.Port > 0:
		host = net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	case c.Instance == "":
		host = net.JoinHostPort(c.Host, strconv.Itoa(DefaultPort))
	}
	q := url.Values{}
	q.Set("database", c.Database)
	q.Set("app name", AppName)
	// Salt okuma niyeti (Always On'da ikincil kopyaya yönlendirir; tek
	// sunucuda etkisizdir).
	q.Set("ApplicationIntent", "ReadOnly")
	if c.TrustServerCertificate {
		q.Set("TrustServerCertificate", "true")
	}
	u := &url.URL{Scheme: "sqlserver", User: user, Host: host, RawQuery: q.Encode()}
	if c.Instance != "" {
		u.Path = "/" + c.Instance
	}
	return u
}

// RedactedDSN, parolası maskelenmiş bağlantı adresidir (log için).
func (c Config) RedactedDSN() string {
	user := url.User(c.User)
	if c.Password != "" {
		user = url.UserPassword(c.User, "REDACTED")
	}
	return c.dsnURL(user).String()
}

// String parolayı göstermez; %v / %+v ile loglamak güvenlidir.
func (c Config) String() string {
	s := fmt.Sprintf("%s %s", c.Kind, c.RedactedDSN())
	if c.Kind.IsLogo() {
		s += fmt.Sprintf(" firma=%s dönem=%s", c.FirmNo, c.PeriodNo)
	}
	return s
}

// GoString, %#v ile de parolanın görünmemesi için.
func (c Config) GoString() string { return "sources.Config{" + c.String() + "}" }

// Direction faturanın yönü.
type Direction string

const (
	DirectionSales    Direction = "sales"
	DirectionPurchase Direction = "purchase"
	DirectionUnknown  Direction = "unknown"
)

// DocKind belgenin türü.
type DocKind string

const (
	DocKindInvoice DocKind = "invoice"
	DocKindReturn  DocKind = "return"
	DocKindUnknown DocKind = "unknown"
)

// Uyarı kodları. Metin değil kod esastır (05). Bu kodlar 05-uyari-kodlari.md'ye
// henüz eklenmedi; panel tarafı metni koddan üretir.
const (
	// WarnTypeUnmapped: tür kodu (Logo TRCODE/TYPE, Mikro cha_evrak_tip /
	// cha_cinsi / cha_normal_Iade) eşleme tablosunda yok. Yön ve tür
	// "unknown" yazıldı, ham satır saklandı.
	WarnTypeUnmapped = "TYPE_UNMAPPED"
	// WarnTaxRateUnmapped: oran bazında KDV alanları (Mikro cha_vergi1..N)
	// var ama hangi sıranın hangi orana denk geldiği doğrulanmadı (15 §2.2);
	// KDV toplamı yazılmadı, değerler ham satırda.
	WarnTaxRateUnmapped = "TAX_RATE_UNMAPPED"
	// WarnFieldMissing: beklenen kolon satırda yok (⚠️ kolon adı doğrulanmalı).
	WarnFieldMissing = "FIELD_MISSING"
	// WarnValueInvalid: kolon değeri beklenen biçimde değil (tutar, tarih...).
	WarnValueInvalid = "VALUE_INVALID"
	// WarnRowLimitReached: satır sınırına ulaşıldı; pencere daraltılmalı.
	WarnRowLimitReached = "ROW_LIMIT_REACHED"
)

// Warning kodlu uyarıdır.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Totals fatura toplamları; her biri ondalık dizgidir (ör. "1180.50"),
// kaynakta yoksa boş. Anlamı kaynağa göre değişen alanlar ayrı tutulur,
// birbirine dönüştürülmez.
type Totals struct {
	Gross         string `json:"gross,omitempty"`          // Logo TOTAL_GROSS
	Discounts     string `json:"discounts,omitempty"`      // Logo TOTAL_DISCOUNTS
	VAT           string `json:"vat,omitempty"`            // Logo TOTAL_VAT
	AdditionalTax string `json:"additional_tax,omitempty"` // Logo TOTAL_ADD_TAX ⚠️
	Net           string `json:"net,omitempty"`            // Logo TOTAL_NET
	// Amount / Subtotal: Mikro cha_meblag ("Tutar") ve cha_aratoplam
	// ("Ara toplam"). Brüt / net karşılığı doğrulanmadığından ayrı alanlar.
	Amount   string `json:"amount,omitempty"`
	Subtotal string `json:"subtotal,omitempty"`
}

// Invoice kaynaktan okunan fatura başlığının tarafsız halidir. Ortak modele
// (01) dönüşüm bulut tarafında yapılır.
type Invoice struct {
	Source   Kind   `json:"source"`
	SourceID string `json:"source_id,omitempty"`
	GUID     string `json:"guid,omitempty"`
	// DocNo belge (fatura) no; InternalNo programın iç fiş no'su.
	DocNo      string `json:"doc_no,omitempty"`
	InternalNo string `json:"internal_no,omitempty"`
	// Date ve DocDate "2006-01-02" biçiminde (sunucu saatinin takvim günü).
	// Hangisinin esas alınacağı müşavirle netleşecek (14 §4.2, §11).
	Date          string    `json:"date,omitempty"`
	DocDate       string    `json:"doc_date,omitempty"`
	Direction     Direction `json:"direction"`
	Kind          DocKind   `json:"kind"`
	DocumentClass string    `json:"document_class,omitempty"`
	// TypeCode ham tür kodu ("8" ya da Mikro için "evrak_tip=..;cinsi=..").
	TypeCode          string `json:"type_code,omitempty"`
	Cancelled         bool   `json:"cancelled"`
	CounterpartyCode  string `json:"counterparty_code,omitempty"`
	CounterpartyTitle string `json:"counterparty_title,omitempty"`
	CounterpartyTaxID string `json:"counterparty_tax_id,omitempty"`
	// Currency kaynağın kendi döviz kodu, olduğu gibi (ISO 4217 olmayabilir ⚠️).
	Currency string         `json:"currency,omitempty"`
	ETTN     string         `json:"ettn,omitempty"`
	Totals   Totals         `json:"totals"`
	Raw      map[string]any `json:"raw"`
	Warnings []Warning      `json:"warnings,omitempty"`
}

// InvoiceBatch bir okuma penceresinin sonucudur.
type InvoiceBatch struct {
	Source Kind      `json:"source"`
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	// Invoices eşleme kuralında "skip" olmayan satırlar (bilinmeyenler dahil).
	Invoices []Invoice `json:"invoices"`
	// Skipped, kuralı gereği çekilmeyen satır sayısı (ör. Logo proforma).
	Skipped int `json:"skipped"`
	// Truncated, satır sınırına ulaşıldı mı (Warnings'te ROW_LIMIT_REACHED).
	Truncated bool      `json:"truncated"`
	Warnings  []Warning `json:"warnings,omitempty"`
}
