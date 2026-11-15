# 01 · Çekirdek, Hiyerarşi, Eksikler ve Açık Kararlar

| | |
|---|---|
| Tarih | 15 Kasım 2026 |
| Önceki | `00-degerlendirme-ve-kararlar.md`. Bu belge oradaki K1 (kiracı modeli) ve K2 (yığın) kararlarını **genişletir ve değiştirir**. |
| İncelenen | `furkanmeclis/go-ubltr` (main, 16.08.2026), `furkanmeclis/nextjs-go-boilerplate` (main `fdd52e4`, 11.11.2026; `docs/design/tenant-core.md`), `otopoly-go` (mobil, AI), getbasis.ai ve rakipleri |
| Durum | Karar taslağı. Kod yok. **[?]** ile işaretli maddeler senin kararını bekliyor. |

## 0. Verilen kararlar (15 Kasım 2026, kullanıcı)

| # | Karar |
|---|---|
| D1 | **Eşzamanlılık yok.** Mükellef bir anda tek ofise bağlıdır; geçiş desteklenir. |
| D1b | **Ofisler arası eşleşme yok.** Bir mükellef bir faturada alıcı, başka faturada satıcı olabilir. Bu bilgi yalnızca o ofisin kendi cari verisidir. Sistem "karşı taraf da bizde kayıtlı" bilgisini **hiçbir yerde göstermez ve kullanmaz.** §2.2'deki ağ etkisi önerisi **iptal**. |
| D2 | Geçişte geçmiş veri **mükellefin onayıyla** yeni ofise açılır. Eski ofisin iş ürünü (kurallar, notlar) eski ofiste kalır. |
| D3 | İlk sürümde mükellef tarafı **mobil + web panel**. |
| D4 | Barındırma: **mevcut Dokploy sunucusu.** |
| D5 | Masaüstü: **Tauri + Go sidecar.** |
| D6 | Entegratör istemcilerinin yeri: **go-ubltr'nin yeni sürümü bekleniyor**; sürüm gelince kullanıcı haber verecek. O zamana kadar entegratör kodu yazılmaz. Arayüz (`EntegratorAdapter`) uygulamada tanımlanır. |
| D7 | Hiyerarşinin yeni parçaları (G1–G4) **boilerplate'e** yazılır (tenant-core faz 6). |
| D11 | Perşembe 23:59 kapsamı: **çekirdek + ilk dilim.** Hiyerarşi ve yetki çekirdeği + UBL yükleme → kontrol → inceleme kuyruğu. |

D1b'nin modele etkisi:

- Karşı taraflar her mükellefin kendi `customer` (cari) kaydıdır. Global `taxpayer` kimliğine **bağlanmaz.**
- `document_party` yalnızca mükellefin kendi tarafını gösterir.
- Global `taxpayer` (VKN) sadece mükellefin kendi kimliği ve ofis geçişi içindir.
- **Varlık sızıntısı yasak:** Bir ofis yeni mükellef eklerken VKN sistemde olsa da olmasa da arayüz ve API **aynı yanıtı** verir. Akış "mükellefe davet / onay gönderildi" şeklinde ilerler. Daha önce başka ofiste kayıtlı olduğu, ancak mükellef onay verince ortaya çıkar.

Sonradan verilen kararlar:

| # | Karar |
|---|---|
| D8 | İlk çıktılar **Luca** ve **formüllü Excel**, sürücü (driver) modeliyle. Zirve ve diğerleri sonradan tek paket olarak eklenir. Ayrıntı `02-muhasebe-cikti-surucu-modeli.md`. |
| D9 | Fiyat: **ofis taban ücreti + aktif mükellef başına**. Mükellef uygulaması ücretsiz. |
| D12 | GitHub organizasyonu: **`HakanAplikasyonu`** (15.11.2026'da açıldı, sen admin'sin, henüz repo yok). Repo `hakan-app-go`. Go modül yolu `github.com/HakanAplikasyonu/hakan-app-go/backend` (boilerplate düzeni). Ürün adı hâlâ geçici. |

| D13 | **Ürün senin; Easytrade bir kaynak.** Easytrade, Paraşüt ve KolayBi gibi entegrasyonlardan biri, öncelikli değil. `00` belgesindeki "Easytrade'e sorulacaklar" listesi artık ürün sahibine değil, entegrasyon ortağına yöneliktir. Paygo/GMP-3 ilişkisi ve Easytrade'in mevcut Luca entegrasyonu bu üründe **varsayılamaz.** Luca şablonu müşavirden ya da Luca'dan alınır. |
| D14 | İlk sürümde ofisleri **platform yöneticisi açar**, ödeme alınmaz. Fiyat modeli (D9) tanımlanır, tahsilat sonraki sürümde. `TENANCY_SELF_SERVE` kapalı. |
| D15 | Mükellef **yalnızca ofis davetiyle** gelir; ofissiz mükellef kaydı yok. |
| D16 | **Ofise özel rol tanımı ilk sürümde.** Şablon roller (sahip / yönetici / müşavir / personel / stajyer) her ofise kopyalanır, ofis bunları değiştirip yeni rol ekleyebilir. Boilerplate tasarımının açık sorusu 1 bu yönde kapanır (şema zaten `roles.organization_id` ile hazır). SMMM oda sicil no profil alanı olarak tutulur, doğrulanmaz. |
| D17 | Perşembe teslimi **iç kontrol** (yalnızca sen, demo verisi). |
| D18 | Pilot ofis **henüz yok.** Luca şablonu ve örnek dosyalar pilot belirlenince istenir; o zamana kadar Luca sürücüsü `Verified=false`. |

| D19 | **Marka kimliği** (yan oturumda hazırlandı):<br>• Logo işareti `docs/brand/logo-mark.svg`: iki dikey sütun (borç/alacak) ve onay çizgisi bir arada "H" harfini oluşturuyor.<br>• Palet boilerplate'in slate tokenları üzerine kurulu: marka mavisi `#1447e6`, onay rengi `#5b8cff`.<br>• Durum renkleri: onaylandı `#009689`, inceleme `#ffb900`, uyarı `#f54900`, engelleyici `#e7000b`.<br>• Fontlar: Geist; tutar, hesap kodu, VKN ve ETTN için Geist Mono.<br>• Kanvas: https://claude.ai/artifact/X3LAbL8LTy645XLYtAL16N<br>• Uygulama ikonları (iOS/Android, Tauri, favicon) kanvasta tanımlı; repoda şimdilik yalnızca logo SVG'si var. |

Bütün kararlar verildi. Açık kalan tek dış bağımlılık go-ubltr'nin yeni sürümü (D6).

"HAKAN" yalnızca geliştirme ortamının adı. Ürün adı, GitHub organizasyonu ve modül yolu ayrıca karara bağlanacak (§7, D12).

---

## 1. Elimizdeki çekirdekler: ne veriyor, ne vermiyor

### 1.1 `go-ubltr`

| Veriyor | Vermiyor |
|---|---|
| UBL-TR 1.2.1 Invoice, CreditNote, ApplicationResponse, DespatchAdvice, ReceiptAdvice tipleri (XSD'den üretilmiş) | **Entegratör istemcileri.** ROADMAP bunları "out of scope" sayıyor (D6). |
| e-Arşiv raporu tipleri | **UBL → muhasebe modeli eşlemesi.** Paketteki `31` eşlemesi bizim uygulamada yazılır; kütüphane bilerek "ERP adapter" taşımıyor. |
| `Detect`, `ValidateXSD`, `ValidateSchematron` (GİB e-Fatura / e-Arşiv kuralları) | Zarf (envelope) tipleri: ROADMAP'te "possible later" |
| Builder, ondalık para ve miktar, GİB kod listelerinin tamamı (`codelist`) | İmza (XAdES) |
| `Inspect`, `Diff`, `Clone`, ekler | — |
| HTML render (`render`) ve PDF render (`render/chromium`) | Varsayılan XSLT. Çağıran taraf verir; faturanın kendi gömülü XSLT'si kullanılabilir. |

**Sonuç:** Paketin önerdiği elle yazılmış ayrıştırıcı **gereksiz.** Ayrıştırma, doğrulama ve görüntü (T2: "fatura görseli kaydın yanında") go-ubltr'den gelir. Tutarlar `string` tutulduğu için kayıpsız, ondalık kuralıyla da uyumlu. Uygulamada yazılacak tek parça `ubl → invoice / invoice_item / tax_summary` eşleyicisi; paketteki `31` tabloları bu eşleyicinin şartnamesi olur.

Schematron doğrulaması paketin kontrol listesine yeni bir uyarı ekler: `UBL_SCHEMATRON_FAIL` (GİB kuralını ihlal eden fatura).

### 1.2 `nextjs-go-boilerplate` tenant çekirdeği

| Var | Bizim hiyerarşi için yeterli mi |
|---|---|
| Platform yöneticisi (`super_admin`, `platform.*`) | ✅ Sistem yöneticisi |
| `organizations` (durum, erişim penceresi, ayarlar) | ✅ Mali müşavirlik ofisi |
| `organization_members`: kullanıcı global, **organizasyon başına rol**, bir kullanıcı birden çok organizasyonda | ✅ "Birden fazla ofise bağlı müşavir" bu modelle çözülür |
| İstek başına `X-Organization`; JWT'de org yok; iki sekmede iki ofis aynı anda | ✅ Çok ofisli müşavir için doğru tasarım |
| Davet akışı, son sahip koruması, önbellek sürüm sayacı | ✅ |
| İşlerin (import/export/bulk/activity/bildirim) org damgası, `org:{uuid}` realtime kanalı | ✅ |
| `tenant_tables.txt` + sorgu lint testi | ✅ İzolasyonun güvencesi |

**Yetmeyen kısımlar** (çekirdeğe eklenmesi ya da uygulamada çözülmesi gerekenler):

| # | Eksik | Neden gerekli |
|---|---|---|
| G1 | **İkinci kapsam seviyesi.** Ofis içinde mükellef (engagement) bazında yetki. Örnek: "Ayşe sadece atanmış 40 mükellefi görür." | Boilerplate'te yetki organizasyon düzeyinde bitiyor |
| G2 | **Organizasyondan bağımsız global mükellef kimliği** (VKN/TCKN) | Mükerrer kayıt, ofis değiştirme ve satıcı–alıcı ilişkisi kiracı dışında bir kimlik gerektiriyor |
| G3 | **Kiracılar arası kontrollü okuma.** Aynı faturayı satıcının ofisi de alıcının ofisi de görür. | Lint kuralı "her sorguda `organization_id`" diyor. Belge tabloları için yeni bir kapsam türü ve lint istisnası tanımlanmalı. |
| G4 | **Mükellef tarafı kullanıcılar** (B2B2C): kendi şirketine bağlı, ofis üyesi olmayan kullanıcı | Mevcut "CMS shell" yalnızca yer tutucu |
| G5 | Kendi kendine ofis açma (`TENANCY_SELF_SERVE` ayrılmış ama yazılmamış) | Ürün herhangi bir ofisin kayıt olabileceği bir yapı (transkript s.143) |
| G6 | Ofise özel rol tanımı (tasarımda açık soru 1) | Ofisler "stajyer / kıdemli / denetçi" rolleri isteyecek |
| G7 | Faturalama ve plan (çekirdek bilerek dışarıda bırakmış; `AccessPolicy` kancası hazır) | Mükellef başına fiyatlandırma |
| G8 | Postgres RLS ("optional later") | Mali veride lint tek savunma hattı olmamalı. Öneri: belge ve muhasebe tablolarında **ilk günden RLS**. |
| G9 | Sır kasası (entegratör ve banka kimlik bilgileri) | Zarf şifreleme; `00` belgesi K4 |
| G10 | Mobil şablon | Boilerplate'te yok. `otopoly-go/mobile` (Expo 58, RN 0.88, expo-router) alınır. |
| G11 | AI modülü | Boilerplate'te yok. `otopoly-go/internal/modules/ai` (Anthropic ağırlıklı sağlayıcı katmanı, araçlar) alınır. |
| G12 | Kayıt düzeyinde köken ve gerekçe ("her rakam kaynağına bağlı") | `activity` olay günlüğü var, ama alan düzeyinde köken (hangi belge, hangi kural, hangi model, güven skoru) yok (§3) |

G1–G4 bu ürüne özgü değil, B2B2C'ye gidecek her uygulama için gerekli. Öneri: bunları **boilerplate'e "tenant-core faz 6: alt kapsam + dış katılımcılar" olarak** yazmak, uygulamaya değil (D11).

---

## 2. Hiyerarşi modeli (öneri)

```
Platform (super_admin)
│
├── organization  (tip: accounting_office)            ← boilerplate
│     ├── organization_member (user, ofis rolü)       ← boilerplate; bir müşavir N ofiste olabilir
│     └── engagement  (ofis ↔ mükellef hizmet ilişkisi)          ← YENİ (G1)
│           ├── engagement_assignment (member ↔ engagement)      ← YENİ: atanmış müşavir
│           └── ofisin iş ürünü: hesap kodu kuralları, onaylar,
│               Luca çıktıları, notlar, uyarı kararları          (organization_id + engagement_id)
│
├── taxpayer  (global; VKN/TCKN tekil; platform düzeyi)          ← YENİ (G2)
│     ├── taxpayer_member (user, mükellef rolü)                  ← YENİ (G4); bir kişi N mükellefte
│     ├── source_connection (entegratör, banka, program)         mükellefe ait, ofise değil
│     └── kaynak belgeler: fatura (ETTN), banka hareketi, fiş    mükellefe ait
│
└── document_party  (belge ↔ taxpayer, rol: supplier / customer) ← YENİ (G3)
```

### 2.1 Talep ettiğin durumların modelde karşılığı

| Durum | Karşılığı |
|---|---|
| Müşavir birden fazla ofise bağlı | N adet `organization_member`; her ofiste ayrı rol; sekme başına ayrı ofis (boilerplate) |
| Mükellefin mükerrer kaydı | `taxpayer` tektir (VKN). Mükerrerlik **ilişkide** olur: aynı mükellefin farklı ofislerde (zamanla ya da [?] aynı anda) `engagement`'ı olabilir. |
| Mükellef başka müşavire geçer | Eski `engagement` `ended_at` ile kapanır, yeni ofiste yeni `engagement` açılır. Kaynak belgeler mükellefte kaldığı için veri kaybolmaz. Geçmişe erişim mükellefin onayına bağlı [?] (D2). |
| Satıcı A (ofis X) – alıcı B (ofis Y) | Fatura **bir kez** saklanır (ETTN). `document_party` ile A'ya supplier, B'ye customer olarak bağlanır. X, A üzerinden görür; Y, B üzerinden. İki ofis birbirinin başka verisini **görmez**. |
| Mükellef kullanıcısının iki şirketi var, şirketler farklı ofislerde | Bir `user`, iki `taxpayer_member`; mobil uygulamada şirket seçici |
| Ofis kendi personelini mükellefe göre kısıtlar | `engagement_assignment` + ofis rolündeki `tenant.engagements.read_all` izni |

> **Not (D1, D1b):** Yukarıdaki tabloda "aynı anda" ve "satıcı A – alıcı B" satırları geçersizdir. Mükellef tek aktif ofise bağlıdır. Fatura, mükellefin kendi verisi olarak saklanır; karşı taraf cari kaydıdır ve ofisler arası görünürlük yoktur.

### 2.2 ~~Bu modelin getirdiği ağ etkisi~~ (İPTAL, D1b)

Satıcı ve alıcının ikisi de sistemdeyse:

- **Otomatik cari mutabakat ve BA/BS mutabakatı** ofisler arası yapılabilir. Paylaşılan tek şey, iki tarafın zaten bildiği fatura toplamlarıdır.
- "Faturan karşı tarafta reddedilmiş" ve "karşı taraf bu faturayı iade olarak işledi" uyarıları üretilebilir.

Paylaşım kuralı: yalnızca **ortak belgenin kendisi** ve iki tarafın da sahip olduğu alanlar paylaşılır. Hesap kodu, not ve iç değerlendirme asla paylaşılmaz.

### 2.3 İzolasyon kuralları (lint ve RLS)

| Tablo türü | Anahtar | Erişim kuralı |
|---|---|---|
| `org` (ofis iş ürünü) | `organization_id NOT NULL` (+ `engagement_id`) | Boilerplate lint aynen |
| `taxpayer` (kaynak veri) | `taxpayer_id NOT NULL` | Erişim: aktif `taxpayer_member` **veya** aktif ve atanmış `engagement`. Yeni lint etiketi: `-- tenant-lint: taxpayer <neden>` |
| `shared` (ortak belge) | `document_party` üzerinden | Yalnızca taraf olunan belge; alan beyaz listesi |
| `platform` | — | Yalnızca `platform.*` |

RLS politikası bu dört türün her biri için yazılır. Uygulama katmanı kontrolü RLS'in yerine geçmez, ikisi birden çalışır.

---

## 3. Basis'ten alınacak dersler (kopya değil, yöntem)

Kaynak: getbasis.ai ürün, çözüm ve güvenlik sayfaları ile üçüncü taraf karşılaştırmaları. Ayrıntılı rapor bu belgenin ekinde (§8).

| Basis'teki fikir | Bizdeki karşılığı | Neden önemli |
|---|---|---|
| Güven skoruna göre otomatik gönderim veya **inceleme kuyruğu** ("staged for review before anything posts") | Her öneri (hesap kodu, eşleşme, OCR alanı) `confidence` taşır. Eşik ofis ve mükellef bazında ayarlanır. Altındakiler kuyruğa düşer. | Türk rakiplerde yok |
| **Shows its work**: kaynak, hesap, gerekçe, güven tek ekranda | G12: her önerinin kökeni (belge, kural, model, girdi) ve gerekçesi saklanır, inceleme ekranında gösterilir | Müşavirin güveni ve yasal savunma |
| **Procedures**: firma ve müşteri bazlı yöntem kütüphanesi | "Kural seti" = ofis şablonu + mükellef istisnaları (hesap eşleme, eşikler, kontroller). Yeni mükellefe şablondan kopyalanır. | T1 kural hafızasının genelleştirilmiş hali |
| **Carries context forward**: onaylanan kararlar sonraki döneme taşınır | Müşavirin her düzeltmesi bir kural adayı olur ("bu satıcı → 770, 3 kez onaylandı, kural yapılsın mı?") | Öğrenen sistem |
| Ajanın eksik belgeyi müşteriden kendisinin istemesi | Dönem kapanışında eksik belge tespiti (bankada var, faturası yok; `BNK_UNDOCUMENTED_PAYMENT`) → **mükellef uygulamasına görev** → mükellef fotoğrafı yükler → eşleşir | B2B2C'nin çekirdeği. Basis'te mükellef uygulaması yok, bizde var. |
| "Command center": iş nerede | Mükellef × dönem durum matrisi (toplandı / kontrol edildi / onaylandı / Luca'ya gitti / beyan) | Ofis yöneticisinin ana ekranı |
| Behavior specs (ajan davranışının yazılı tanımı ve değerlendirmesi) | Her ajan iş akışı için yazılı davranış tanımı + üretim izlerinden örneklem değerlendirmesi | Ajan kalitesi ölçülebilir olur |
| Model eğitimine veri vermeme, tam denetim izi, MFA, SOC2/ISO | KVKK + yurt içi barındırma [?] + "verin model eğitiminde kullanılmaz" beyanı + boilerplate step-up/passkey | Türk rakiplerde açık beyan bulunamadı |

**Basis'te olmayan, bizde olacak:** mükellef uygulaması, e-fatura/e-arşiv toplama, Z raporu ve ÖKC, Türk banka ekstreleri, Luca/Zirve/Mikro çıktısı, ofisler arası ağ etkisi.

**Türkiye'de izlenecek en yakın rakip:** **Nop** (eski Monorobi). 60+ kaynaktan e-belge toplama, 25+ banka ekstresi, Z/ÖKC/gider fişi, AI hesap kodu önerisi, Luca/Zirve/Logo/Orka aktarımı ve bir masaüstü köprüsü (NopConnect) sunduğunu söylüyor. Bu iddialar yalnızca kendi sitesinden; bağımsız doğrulama yok. Kalabalık ve büyük ölçüde ücretsiz dağıtılan katman (Paraşüt, Faturaport, Bilsoft) veri toplama ve aktarım. Boş olan katman inceleme, kontrol ve ajan katmanı.

---

## 4. Teknik sorular

### 4.1 Algoritma motoru için Go yeterli mi? **Evet.**

| İş | Go'da nasıl | Not |
|---|---|---|
| Kural motoru (hesap eşleme, kontroller, eşikler) | Saf Go, kurallar veritabanında (JSON tanım + sürüm) | Deterministik, test edilebilir |
| Banka–fatura eşleme (skorlama, `21` §5) | Saf Go; aday üretme SQL'de (tutar ± tolerans, tarih penceresi, VKN), skorlama Go'da | Ayda binlerce hareket, milisaniyeler |
| Ondalık hesap | `shopspring/decimal`; go-ubltr'nin string tutarlarıyla uyumlu | |
| Metin benzerliği (açıklama → karşı taraf) | Normalize + trigram (`pg_trgm`) + gerekirse embedding (API) | |
| OCR, belge anlama, finansal yorum, kolon eşleme | LLM API (otopoly'deki sağlayıcı katmanı) | Model barındırmıyoruz |
| Toplu senkron, zamanlayıcı | Asynq; bağlantı başına kilit | |

Python yalnızca şu durumda gerekir: **kendi eğittiğimiz** bir sınıflandırma modeli (ör. hesap kodu tahmini, milyonlarca onaylı kayıtla). O durumda ayrı bir servis olur; çekirdek yine Go kalır. Bugün ihtiyaç yok.

### 4.2 Masaüstü: Tauri, Rust ve Go

Tauri'nin kabuğu Rust'tır, bu kaçınılmaz. Asıl soru, **iş mantığının** nerede yazılacağı.

**Öneri: Tauri v2 + ince Rust kabuk + Go "sidecar".**

| Parça | Dil | İş |
|---|---|---|
| Pencere, tepsi, güncelleme, OS anahtar zinciri, dosya izni | Rust (Tauri eklentileri; elle yazılan Rust minimum) | |
| Arayüz | Web (Next.js'in paylaşılan bileşenleri / statik çıktı) | Web ile aynı tasarım sistemi |
| **Yerel ajan** | Go sidecar | Backend'le **aynı Go paketleri** (go-ubltr, eşleyiciler, adaptörler) |

Masaüstünün gerçek değeri ajanda. Paketin önerdiği **.NET 8 bağlantı ajanının yerine geçer**:

- Logo / Mikro yerel SQL okuma
- Klasör izleme (mükellefin indirdiği ZIP'ler, Luca/Rahat Aktarım çıktıları)
- Toplu dosya bırakma
- İleride e-imza / mali mühür (USB token, yerel erişim)

Aynı kodun sunucuda ve masaüstünde çalışması, ikinci bir dilde ikinci bir ajan bakımını ortadan kaldırır.

Bedeli:

- İki ikili dosya (Rust kabuk + Go sidecar)
- macOS'ta notarization ve imzalama her ikisi için
- Windows'ta SmartScreen için kod imzalama sertifikası

Alternatif **Wails** (tamamen Go) daha az parça demek, ama sen Tauri dedin. Tauri'nin eklenti ekosistemi ve güncelleyicisi daha olgun (D5).

### 4.3 Mobil

`otopoly-go/mobile` (Expo 58, expo-router) temel alınır. Sıra:

1. **Mükellef uygulaması:** fiş/Z fotoğrafı, eksik belge görevleri, onaylar, bildirimler
2. **Müşavir özeti:** Uyumsoft örneği; mizan ve fatura listesine hızlı bakış

Mükellef uygulaması B2B2C'nin girişi olduğu için önce gelir [?] (D3).

---

## 5. Eksikler listesi (tam)

### 5.1 Bilgi eksikleri (dışarıdan)

`00` §6'daki sorular geçerli. Özellikle:

- Easytrade'in mevcut Luca entegrasyonu ve iç müşavir portalı (E1, E2)
- Y belgesi (M1)
- Luca aktarım örneği (M2)
- İşNet başvurusu

### 5.2 Ürün kararı eksikleri

Bkz. §7, D1–D12.

### 5.3 Altyapı eksikleri

| # | Eksik | Nerede çözülür |
|---|---|---|
| A1 | G1–G4 alt kapsam ve dış katılımcı modeli | boilerplate (faz 6) [?] |
| A2 | RLS | boilerplate veya uygulama |
| A3 | Sır kasası (zarf şifreleme) | boilerplate `platform/secrets` (yeni) |
| A4 | Sabit çıkış IP'li worker havuzu (`fixed-egress`) | deploy (Dokploy: ayrı worker servisi + çıkış IP'si) |
| A5 | Entegratör istemcileri | ayrı modül (D6) |
| A6 | UBL → muhasebe eşleyici | uygulama |
| A7 | Kural motoru + inceleme kuyruğu + köken (G12) | uygulama |
| A8 | Mobil (Expo) ve AI modülünün boilerplate soyuna taşınması | otopoly'den port |
| A9 | Masaüstü kabuk + Go sidecar iskeleti | yeni repo veya monorepo içi `desktop/` (D5) |
| A10 | Faturalama (mükellef başına) | uygulama, `AccessPolicy` |
| A11 | KVKK: aydınlatma, açık rıza (mükellef geçişinde geçmiş veri), veri işleyen sözleşmesi, saklama süreleri | hukuk + uygulama |
| A12 | GitHub organizasyonu, repo, ürün adı, alan adı | sen (D12) |

---

## 6. Kod yerleşimi (monorepo önerisi, boilerplate düzeninde)

```
hakan-app-go/
  backend/            Go API + worker (boilerplate'ten türetilmiş)
  frontend/           Next.js: /platform, /t/{slug} (ofis), /m/{taxpayer} (mükellef web)
  mobile/             Expo (otopoly'den)
  desktop/            Tauri v2 kabuk (src-tauri) + sidecar/ (Go, backend paketlerini import eder)
  docs/
```

Harici Go modülleri: `github.com/furkanmeclis/go-ubltr` ve D6'ya göre entegratör modülü.

---

## 7. Karar bekleyenler

| # | Karar | Önerim |
|---|---|---|
| D1 | Bir mükellef **aynı anda** birden fazla ofiste aktif olabilir mi? (ör. defter bir ofiste, bordro diğerinde) | Evet, ama hizmet türü bazında: `engagement.scope` (defter, bordro, denetim). Aynı hizmette tek aktif ofis. |
| D2 | Ofis değişince **geçmiş veri** ne olur? | Kaynak belgeler mükellefindir; yeni ofis, mükellefin onayıyla geçmişe erişir. Eski ofisin iş ürünü (kurallar, notlar) eski ofiste kalır; dışa aktarılabilir. |
| D3 | Mükellef kullanıcısı (B2B2C) **ilk sürümde** var mı? | Evet: mobil fiş + eksik belge görevi ilk sürümde. Mükellef web paneli sonra. |
| D4 | Barındırma ve KVKK: yurt içi mi? | Yurt içi veri merkezi (mali veri, açık rıza yükünü azaltır). LLM çağrıları maskeli. |
| D5 | Masaüstü: Tauri + Go sidecar mı? | Evet (§4.2) |
| D6 | Entegratör istemcileri nerede? (go-ubltr ROADMAP'i bunu kapsam dışı sayıyor) | Ayrı modül, ör. `go-efatura-connectors`; go-ubltr'yi import eder. go-ubltr belge kütüphanesi olarak saf kalır. |
| D7 | G1–G4 boilerplate'e mi, uygulamaya mı? | Boilerplate'e (tenant-core faz 6). Diğer uygulamalar da kazanır. |
| D8 | İlk muhasebe çıktısı yalnız Luca mı? | Luca ilk; arayüz Zirve, Mikro ve Orka'ya açık. |
| D9 | Fiyat modeli | Ofis başına taban + aktif mükellef başına. Mükellef uygulaması ücretsiz (dağıtım kanalı). |
| D10 | Ofisler arası mutabakat (§2.2) ilk sürümde mi? | Model ilk günden buna göre kurulur, özellik ikinci sürümde açılır. |
| D11 | Perşembe 23:59 teslimi korunuyor mu, yoksa kapsam "çekirdek + ilk dilim" olarak mı yeniden tanımlanıyor? | Perşembe: çekirdek genişletmesi (G1–G4) + UBL dilimi + inceleme kuyruğu iskeleti. Diğer modüller sonraki hafta. |
| D12 | Ürün adı, GitHub organizasyonu, Go modül yolu | Sen belirleyeceksin; geçici ad `hakan-app-go`. |

---

## 8. Ek: Basis ve rakip araştırması (özet)

Ayrıntılı rapor oturum içinde üretildi. Kaynak bağlantıları:

- **Basis:**
  - ürün ve çözümler: https://www.getbasis.ai/ , `/solutions/cas-and-advisory`, `/solutions/small-firms`, `/solutions/tax`, `/solutions/audit`, `/solutions/corporate-accounting`
  - güvenlik: `/security`
  - Series B: `/blogs/basis-raises-100m-series-b-led-by-accel-and-google-ventures`
  - Behavior specs: `/blogs/behavior-specs-an-open-standard-for-supervising-long-horizon-agents`
- **Küresel rakipler:** Truewind, Double (eski Keeper), Meridian (Pilot), Digits, Puzzle, Docyt, Karbon, Dext, Booke AI, Uplinq, Rillet, Vic.ai. Botkeeper Şubat 2026'da kapandı.
- **Türkiye:**
  - Luca MMP (2026: kurulum 8.500 TL, yıllık 8.400 TL / 0–50 mükellef)
  - e-Mükellef / MuhasebeApp (Zirve Bilgi Teknolojileri)
  - Mikro Müşavir
  - Paraşüt Atlas/ExpressAktarım (müşavir araçları ücretsiz)
  - Uyumsoft MüşavirPro / ekoSMMM
  - Logo Mali Müşavir 3
  - **Nop**
  - Fisle
  - Faturaport
  - Bilsoft / Kolay Mükellef
  - Robom
- **Doğrulanamayanlar:**
  - Basis'in desteklediği ERP listesi ve fiyatı
  - Rahat Aktarım, MKLF, Nilvera, ETA ve Orka'nın müşavir özellikleri
  - Nop iddialarının bağımsız teyidi
