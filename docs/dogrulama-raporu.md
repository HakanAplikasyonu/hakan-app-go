# Uçtan uca doğrulama raporu

| | |
|---|---|
| Kapsam | Yol haritası M1–M8 tamamlandıktan sonra main'in tamamı |
| Doğrulanan commit | `789648d` (main, 2026-11-16) |
| Ortam | Bulut oturumu; `compose.local.yml` (Postgres 18, Redis, SeaweedFS, Meilisearch, Centrifugo, Mailhog), API ve worker aynı süreçte (`cmd/server`), Next.js dev sunucusu, Chromium (Playwright) |
| Hesaplar | `make demo-seed` hesapları, ikinci ofis için bir platform süper yöneticisi, onay akışında kayıt olan yeni bir mükellef kullanıcısı |

## Özet

- **Testler:**
  - Backend (gerçek Postgres ile, 110 paket), frontend (tip kontrolü ve lint) ve mobil (tip kontrolü, lint, 24 test) tamamen yeşil.
  - Migration'lar boş veritabanına 1'den 72'ye temiz uygulandı; demo seed yüklendi.
- **Backend akışı:** baştan sona çalışıyor. Ofis girişi, mükellef ekleme ve onay, UBL yükleme, inceleme önerisi, banka ekstresi ve eşleme, dönem kilidi.
- **Tenant izolasyonu:** ofisler arası, ofis içi (atama) ve mükellefler arası **sızıntı bulunmadı**.
- **Kırıklar:**
  - Ofis panelinin ana ekranları API'ye bağlı değil (#142).
  - Luca/Zirve çıktısı hiçbir uçtan alınamıyor (#143).
  - Bu ikisi duman testindeki "UBL yükleme → … → Luca çıktısı" zincirinin arayüzden ve sonunun API'den yapılamamasına yol açıyor.
- **Açılan issue'lar:** 7 bug (#141–#146, #148).
  - #141 düzeltildi ([#147](https://github.com/HakanAplikasyonu/hakan-app-go/pull/147) birleşti).
  - #144 için [#150](https://github.com/HakanAplikasyonu/hakan-app-go/pull/150) açık.
  - Diğerleri koordinatörün dağıttığı şeritlerde.

## 1. Testler ve kurulum

| Adım | Sonuç |
|---|---|
| Backend `TESTDB_REQUIRED=1 go test ./...` (Postgres 16) | ✅ geçti, 110 paket, atlanan yok |
| Frontend `tsc --noEmit`, `eslint .` | ✅ geçti (frontend'de `test` betiği yok) |
| Mobil `tsc --noEmit`, `node --test`, `expo lint` | ✅ geçti, 24/24 |
| `make check-i18n` | ✅ 0 hata, 30 uyarı (§4) |
| `docker compose up` (`compose.local.yml`) | ✅ ortam ayarıyla (aşağıda) |
| Migration 1 → 72, boş veritabanı | ✅ `72`, dirty değil |
| `make demo-seed` | ✅ ofis `demo-smmm`, 4 kullanıcı, 2 mükellef ve engagement |

Bu ortama özgü ayarlar (ürün hatası değil):

- Docker Hub istek sınırına (429) takıldı. Daemon `mirror.gcr.io` aynasıyla çalıştırıldı.
- Centrifugo'nun `ulimits.nofile` ayarına bu sandbox izin vermiyor. Yalnızca bu oturumda bir override dosyasıyla kaldırıldı.
- Host'ta 5432 dolu olduğu için compose Postgres'i 55432'de açıldı.
- Duman testi sırasında giriş hız sınırına (429) defalarca takıldım. Bu beklenen koruma; yerel Redis'teki sayaçlar temizlendi.

## 2. Duman testi

| Adım | Nasıl | Sonuç |
|---|---|---|
| Ofis girişi | Tarayıcı, `sahip@demo` | ✅ `/t/demo-smmm` açılıyor |
| Mükellef ekleme (onay akışı) | API `POST taxpayer-requests` → Mailhog'daki onay e-postası → tarayıcıda kayıt ve onay | ✅ talep `approved`, engagement açıldı, yeni kullanıcı `/m/<mükellef>` paneline düştü. ⚠️ Kayıt sayfasında yanlış uyarı (#146) |
| UBL yükleme | API `ubl-uploads` (Atlas, 7 XML'lik ZIP) | ✅ 7 eklendi, 0 hata |
| | Ofis panelindeki "UBL yükle" ekranı | ❌ dosya backend'e gitmiyor, tarayıcıda demo verisine ekleniyor (#142) |
| İnceleme kuyruğu | API `account-suggestions` → `review/suggestions` → `accept` | ✅ 5 öneri, kabul çalışıyor |
| | Ofis panelindeki "İnceleme kuyruğu" | ❌ demo verisi gösteriyor (#142) |
| Banka eşleme | API: MT940 içe aktarma → `bank-matching?period=2026-10` | ❌→✅ hesap tanımlama ucu olmadığı için içe aktarma reddedildi (#144). Hesap elle eklenince 2 satır, 1 öneri ve `BNK_UNINVOICED_COLLECTION` uyarısı. #150 ile uçtan doğrulandı |
| Dönem kilidi | API `periods/2026-10/lock` | ✅ `locked: true`, `ready: true` |
| Luca çıktısı | — | ❌ uç yok: `journals` modülü HTTP'ye ve worker'a bağlı değil (#143) |
| Mükellef web paneli | Tarayıcı, onay akışından gelen kullanıcı | ✅ Özet, Faturalar, Fişler, Belge yükle ve Ofis talepleri açılıyor, hata yok |
| | Tarayıcı, seed'deki `mukellef@demo` | ❌ şirkete üye değil; girişte boilerplate CMS ana sayfası (#145) |
| Realtime | Ekrandaki "RT" göstergesi | ❌→✅ bütün ofis ve mükellef kullanıcılarında 403 (#141); düzeltmeden sonra "RT connected" |

Ofis panelindeki ekranların veri kaynağı:

| Ekran | Kaynak |
|---|---|
| Genel bakış, komuta merkezi, mükellefler, faturalar ve detayı, inceleme kuyruğu ve stüdyosu, uyarılar, UBL yükle | ❌ tarayıcı demo verisi (`features/office/store/office-store.ts`) |
| Mükellef talepleri, üyeler, roller, ayarlar, engagement belgeleri, dilekçeler | ✅ API |

Ofis panelinin bütün sayfaları tarandı: sayfa hatası yok, realtime dışında 4xx/5xx yanıt yok.

## 3. Tenant izolasyonu (RLS ve uygulama denetimi)

| Deneme | Sonuç |
|---|---|
| İkinci ofisin sahibi, `X-Organization: ikinci-ofis` ile birinci ofisin engagement'ını, uyarılarını ve UBL işlerini okumaya ve dönemini kilitlemeye çalışıyor | ✅ `NO_ENGAGEMENT_ACCESS`; listeler (engagement, öneri, uyarı, dönem matrisi, mükellef talepleri) boş |
| Ofis sahibi üyesi olmadığı ofisin slug'ıyla | ✅ `NO_TENANT_MEMBERSHIP` |
| Atanmamış stajyer, Atlas engagement'ı | ✅ `NO_ENGAGEMENT_ACCESS`; listede yalnızca atandığı engagement var |
| Mükellef kullanıcısı başka mükellefin `X-Taxpayer` başlığıyla | ✅ `NO_TAXPAYER_MEMBERSHIP` |
| Ofis kullanıcısı mükellef API'si ile | ✅ `NO_TAXPAYER_MEMBERSHIP` |
| Mükellef kullanıcısı ofis API'si ile | ✅ `NO_TENANT_MEMBERSHIP` |
| Stajyer, ofis sahibinin kişisel realtime kanallarına abonelik | ✅ `FORBIDDEN` (#141 düzeltmesinden sonra da) |
| Yeni kasa uçları (#150): atanmamış stajyer ve başka ofis | ✅ `NO_ENGAGEMENT_ACCESS`; DB testinde başka mükellef hesabı görmüyor |

Ayrıca `TestRLSCoversScopedTables` ve tenant tablo lint'i backend test setinde geçti.

## 4. Türkçe metin

- **`check-i18n`:** 0 hata.
  - Uyarıların çoğu dinamik anahtar (`t(\`…${…}\`)`).
  - TR dosyasında İngilizce kalan 4 metin var: `integrations.facebook.form.section_title`, `integrations.google.form.client_id_hint`, `integrations.google.form.client_secret`, `settings.system.subsystem_web_push`. Hepsi platform ayarlarında, ofis ve mükellef ekranlarında değil.
- **Taranan ekranlar:** ofis, mükellef, onay ve kayıt ekranlarında görünen metin Türkçe.
- **Ofis panelindeki eski metin:** "Demo verisi: … Backend uçları birleşince ekranlar API'den okuyacak" uyarısı duruyor (#142 ile kalkar).
- **Rolü olmayan kullanıcı için `/` ana sayfası:** boilerplate metni gösteriyor: "İçerik yönetimi kabuğu — modülleri buraya ekleyin", kenar çubuğunda "CMS" (#145 notu).
- **API hata mesajları İngilizce** (ör. `Invalid request body`). Arayüz hata koduna göre Türkçe metin gösteriyor; mobilde de testi var.

## 5. Açılan issue'lar

| Issue | Konu | Önem | Durum |
|---|---|---|---|
| [#141](https://github.com/HakanAplikasyonu/hakan-app-go/issues/141) | Ofis ve mükellef kullanıcılarında `auth.session` yok: realtime, KVKK onay kaydı ve arama 403 | Yüksek | ✅ [#147](https://github.com/HakanAplikasyonu/hakan-app-go/pull/147) birleşti; `/me` tutarlılığı için [#149](https://github.com/HakanAplikasyonu/hakan-app-go/pull/149) (W5) |
| [#142](https://github.com/HakanAplikasyonu/hakan-app-go/issues/142) | Ofis panelinin ana ekranları tarayıcı demo verisinden okuyor, API'ye bağlı değil | Yüksek | W2 / W4 |
| [#143](https://github.com/HakanAplikasyonu/hakan-app-go/issues/143) | Yevmiye taslağı ve Luca/Zirve çıktısı HTTP'ye bağlı değil | Yüksek | W2 (#142'den sonra) |
| [#144](https://github.com/HakanAplikasyonu/hakan-app-go/issues/144) | Banka hesabı tanımlama ucu yok; dosya kanalıyla ekstre yüklenemiyor | Orta | [#150](https://github.com/HakanAplikasyonu/hakan-app-go/pull/150) açık, auto-merge açık |
| [#145](https://github.com/HakanAplikasyonu/hakan-app-go/issues/145) | demo-seed: mükellef kullanıcısı şirkete üye değil, çıktı metni eski | Düşük | W5 (yaklaşım issue'da) |
| [#146](https://github.com/HakanAplikasyonu/hakan-app-go/issues/146) | Onay bağlantısından kayıtta yanlış "Kayıt bulunamadı" uyarısı | Düşük | W5 |
| [#148](https://github.com/HakanAplikasyonu/hakan-app-go/issues/148) | `demo-seed -reset`, demoya fatura yüklendikten sonra çalışmıyor (FK RESTRICT) | Düşük | açık |

Doğrulamada açık kalan dış bağımlılık: #58 (pilot ofis ve örnek dosyalar).

## 6. Bu turda doğrulanmayanlar

- **Mobil uygulama cihazda ya da simülatörde çalıştırılmadı.** Yalnızca tip kontrolü, lint ve test yapıldı.
- **Masaüstü ajanı (#51) çalıştırılmadı.**
- **Dış servisler:** entegratör, banka web servisi, Paraşüt/Easytrade/KolayBi ve Meilisearch araması gerçek servisle denenmedi. Arama bu ortamda kapalı (`SEARCH_ENABLED=false`).
- **Kilitli döneme fatura gelmesi:** dönem kilitlendikten sonra değişen bir fatura gelince değişiklik talebi ve `LOCKED_PERIOD_CHANGE` üretimi uçtan uca denenmedi (backend testlerinde var).
- **Uyarı motoru:** demo UBL setiyle fatura uyarısı üretmedi. Veri temiz mi, yoksa yükleme sonrası kural çalışmıyor mu, ayrıca bakılmadı. BNK uyarıları çalışıyor.

---

## 2. tur

| | |
|---|---|
| Doğrulanan commit | `a514b93` (main, 2026-11-16). 1. turdaki bulguların kapandığı sürüm: #147, #149, #150, #152–#159 |
| Ortam | 1. turla aynı. Compose birimleri (`down -v`) silinerek temiz veritabanıyla başlandı |

### Özet

- **1. turdaki 7 bulgunun hepsi düzelmiş**:
  - Ofis panelinin bütün ekranları artık API'den okuyor; Luca ve Zirve dosyaları panelden indiriliyor.
  - Realtime ve KVKK uçları 403 vermiyor; `-reset` veriyle çalışıyor.
- **Testler:** backend (113 paket), frontend ve mobil yeşil.
- **Kurulum:** migration'lar 1 → 73 temiz uygulandı.
- **Tenant izolasyonu:** yeni uçlar (yevmiye, çıktı, kasa) dahil sızıntı yok.
- **Yeni bulgular (2):**
  - Hesap kodu önerileri arayüzden üretilemiyordu (#160 → [#162](https://github.com/HakanAplikasyonu/hakan-app-go/pull/162), birleşti).
  - Ofis panelinde ham İngilizce hata metinleri ve sürücü adları vardı (#161 → [#163](https://github.com/HakanAplikasyonu/hakan-app-go/pull/163)).

### 2.1 Testler ve kurulum

| Adım | Sonuç |
|---|---|
| Backend `TESTDB_REQUIRED=1 go test ./...` | ✅ 113 paket |
| Frontend `tsc --noEmit`, `eslint .` | ✅ |
| Mobil `tsc --noEmit`, `node --test`, `expo lint` | ✅ 24/24 |
| `make check-i18n` | ✅ 0 hata, 27 uyarı (1. turda 30) |
| `docker compose up`, birimler silinmiş | ✅ |
| Migration 1 → 73 (boş veritabanı) | ✅ `73`, dirty değil |
| `make demo-seed` | ✅ `mukellef@demo` artık Atlas Gıda'nın sahibi (`/m`) |
| `make demo-seed ARGS=-reset`: fatura (7), banka hareketi (2) ve yevmiye fişi (2) varken | ✅ ofis silindi ve yeniden kuruldu. Demo verisi sıfırlandı, mükellef üyeliği yeniden bağlandı (#148) |

### 2.2 Ofis paneli akışı (tarayıcı, `sahip@demo`)

| Adım | Sonuç |
|---|---|
| UBL yükle (Atlas, 7 XML) | ✅ sunucuda işlendi: 7 eklendi, rapor ve "Son yüklemeler" ekranda |
| Faturalar | ✅ 7 fatura API'den listelendi (matrah, KDV, ödenecek, durum) |
| Fatura detayı → "Hesap kodu öner" (#162) | ✅ 3 öneri, bildirim ve kuyruk bağlantısı. Tekrar tıklayınca yeni kayıt oluşmuyor |
| İnceleme kuyruğu → Onayla | ✅ 2 öneri arayüzden onaylandı. Sunucuda `accepted: 2`, kuyruk 19 → 17 |
| Uyarılar | ✅ banka eşlemenin ürettiği `BNK_UNINVOICED_COLLECTION` ekranda; "Gördüm" ve "Çözüldü" eylemleri var |
| Genel bakış ve komuta merkezi | ✅ dönem durumu (Hazır), engelleyici ve değişiklik talebi sayıları, KDV tahmini (Atlas ₺8.564,85, beyan son günü) API'den |
| Yevmiye taslağı | ✅ 3 satırlı fiş kaydedildi ("2026-11 / 1", Taslak) ve onaylandı (Onaylı) |
| Luca çıktısı | ✅ "1 fiş aktarıldı", `luca_fis_2026-11.xlsx` indirildi. Sütunlar: Fiş No, Fiş Tarihi, Hesap Kodu, Borç, Alacak |
| Zirve çıktısı | ✅ ikinci fişle "1 fiş aktarıldı", `zirve_fis_2026-11.xlsx` indirildi. Sütunlar: Fiş No, Fiş Tarihi, Fiş Türü, Fiş Açıklaması, Hesap Kodu, Açıklama, Evrak No, Evrak Tarihi, Borç, Alacak |
| Aktarılmış fiş | ✅ "Aktarıldı" ve "Geri al". Aynı fiş ikinci programa aktarılmıyor (`NOTHING_TO_EXPORT`); bu tasarım gereği |
| Mükellef ekleme (onay bağlantısıyla kayıt ve onay) | ✅ kayıt sayfasında yanlış uyarı ve 404 isteği yok (#146); talep `approved` |
| Mükellef paneli (`mukellef@demo`) | ✅ girişte `/m/<Atlas>` açılıyor; ofisin yüklediği 7 fatura görünüyor |
| Ayarlar → Veri İşleyen Sözleşmesi | ✅ "Sürüm 1 kabul edildi" |

Bütün ofis sayfaları tarandı (genel bakış, komuta merkezi, mükellefler, faturalar, inceleme kuyruğu ve stüdyosu, uyarılar, yevmiye, üyeler, ayarlar):

- "Demo verisi" uyarısı kalmamış.
- Sayfa hatası ve hatalı istek yok.
- Realtime göstergesi her sayfada "RT connected".

### 2.3 Realtime ve KVKK uçları

| Kullanıcı | `POST /v1/realtime/connection-token` | `GET /v1/me/legal-acceptances` | `/v1/auth/me` → `auth.session` |
|---|---|---|---|
| `sahip@demo` (ofis sahibi) | ✅ 200 | ✅ 200 | ✅ var |
| `stajyer@demo` (ofis personeli) | ✅ 200 | ✅ 200 | ✅ var |
| `mukellef@demo` (mükellef) | ✅ 200 | ✅ 200 | ✅ var |

### 2.4 Tenant izolasyonu

İkinci ofisin sahibi birinci ofisin Atlas engagement'ına şu uçlardan erişmeye çalıştı; hepsi `NO_ENGAGEMENT_ACCESS` döndü:
- engagement
- faturalar
- uyarılar
- kasa ve banka hesapları
- yevmiye taslakları
- çıktılar
- dönem kilidi

İkinci ofisin kendi listeleri (engagement, öneri, uyarı) boş. Diğer denemeler:
- Atanmamış stajyerin Atlas yevmiyesine erişimi `NO_ENGAGEMENT_ACCESS`.
- Atlas'ın mükellef kullanıcısının Selin'in faturalarına erişimi `NO_TAXPAYER_MEMBERSHIP`.

### 2.5 Türkçe metin

- `check-i18n`: 0 hata.
- #163 ile düzeltilen ham İngilizce metinler:
  - Yükleme raporundaki dosya hatası (`PARTY_MISMATCH: own VKN/TCKN…`) ve fatura üretmeyen dosyada görünen sıfır UUID.
  - Yevmiye çıktı hatası (`no approved entries to export`).
  - Program listesindeki ham sürücü kimlikleri (`excel_formula`, `luca`, `zirve`).
- Düzeltmeler tarayıcıda doğrulandı: "Fatura bu şirkete ait değil", "Excel (formüllü) / Luca / Zirve", "Bu dönemde aktarılacak onaylı fiş yok…".

### 2.6 2. turda açılan issue'lar

| Issue | Konu | Önem | Durum |
|---|---|---|---|
| [#160](https://github.com/HakanAplikasyonu/hakan-app-go/issues/160) | Hesap kodu önerileri hiç üretilmiyor; inceleme kuyruğu arayüz akışında hep boş | Yüksek | ✅ [#162](https://github.com/HakanAplikasyonu/hakan-app-go/pull/162) birleşti (fatura detayında düğme). Otomatik üretim ürün kararı, issue'da |
| [#161](https://github.com/HakanAplikasyonu/hakan-app-go/issues/161) | Ofis panelinde ham İngilizce hata metinleri ve sürücü adları | Düşük | [#163](https://github.com/HakanAplikasyonu/hakan-app-go/pull/163), auto-merge açık |

### 2.7 Karar bekleyenler ve doğrulanmayanlar

- **Hesap kodu önerileri:** fatura içe aktarılınca otomatik mi üretilsin, toplu tetikleme mi olsun (#160)?
- **Yevmiye ekranının varsayılan dönemi:** ekran içinde bulunulan ayı açıyor, faturalar çoğunlukla bir önceki aya ait (#161'deki not).
- **1. turun §6 listesi geçerli:** mobil uygulama cihazda, masaüstü ajanı, gerçek dış servisler ve kilitli döneme fatura gelmesi bu turda da denenmedi.
- **Fatura uyarıları:** demo UBL seti fatura uyarısı üretmedi; uyarı ekranı BNK uyarısıyla doğrulandı.

---

## 3. tur (#195)

| | |
|---|---|
| Doğrulanan commit | `474e65f` ile `422beee` arası main (imajlar `474e65f`'ten, testler `422beee`'den) |
| Yığın | `deploy/compose.prod.yml` (düz Docker sunucusu, Caddy vekil): Postgres 18, Redis, SeaweedFS, Meilisearch, Centrifugo, migrate, backend, worker, frontend, backup, proxy |
| İmajlar | `backend`, `frontend`, `backup` yerelde `deploy/images.json`'daki Dockerfile'larla derlendi (`local/hakan-*:main`) |
| Env | `scripts/prod-env.sh init` (APP_URL `http://localhost`, sırlar üretildi). Üretilen değerlere şunlar eklendi: yedek için `age` alıcısı, `METRICS_TOKEN`, SMTP'yi Mailhog'a yönlendiren ayar |
| Ek | Yalnızca bu tur için bir override dosyası: Mailhog, API ve metrik portlarını `127.0.0.1`'e açma. Depoya girmedi |

### Özet

- **Üretim yığını temiz kurulumda ayakta:**
  - Migration'lar 78. sürümde ve dirty değil.
  - Bütün servisler sağlıklı.
  - Vekil → frontend → BFF → API zinciri 200 dönüyor; güvenlik başlıkları var.
- **Ofis ana akışı üretim yığınında 6/6 geçti** (`e2e/tests/office-onboarding.spec.ts`): ofis kaydı → mükellef ekleme ve onay → UBL yükle → hesap önerisi onayı → yevmiye → Luca/Zirve indirme → mükellef paneli.
- **Yedekten geri yükleme çalışıyor:**
  - Yığın birimleriyle silindi, belgedeki adımlarla geri yüklendi.
  - Şema, tablolar, satırlar ve 6 nesne manifestle doğrulandı.
  - Uygulama sayıları birebir aynı; eski kullanıcı giriş yapabiliyor, nesne deposundaki dosya indiriliyor.
- **RLS açık, metrikler çalışıyor.** Loglarda kişisel veri ya da belirteç görülmedi.
- **Yeni bulgular (4):**
  - #219: 5xx nedeni loglanmıyordu → [#220](https://github.com/HakanAplikasyonu/hakan-app-go/pull/220).
  - #221: e2e'de yükleme yarışı → [#222](https://github.com/HakanAplikasyonu/hakan-app-go/pull/222), birleşti.
  - #246: `frontend/.dockerignore` yok → [#249](https://github.com/HakanAplikasyonu/hakan-app-go/pull/249).
  - #247: SeaweedFS kapasite ayarı; ürün kararı gerekiyor.

### 3.1 Kurulum

| Adım | Sonuç |
|---|---|
| İmaj derleme (backend, frontend, backup) | ✅. Frontend önce düştü: `.dockerignore` olmadığı için bağlama 4,8 GB'lık `.next` ve `node_modules` girdi (#246). Git'teki temiz dosyalarla derlendi. Bu sandbox'ın vekili için derlemede geçici bir CA katmanı kullanıldı (depoya girmedi) |
| `prod-env.sh init` | ✅ şema v5, 42 anahtar, sırlar üretildi. `APP_URL` https değil uyarısı beklendiği gibi |
| `compose up` | ❌→✅ İlk denemede `SMTP_TLS=off` yazdım. Backend ve worker açık bir hata mesajıyla durdu (`auto|starttls|implicit|none`). `none` ile düzeldi; ürün hatası değil |
| Migration | ✅ `migrate` 0 ile çıktı, `schema_migrations` = 78, dirty değil |
| Sağlık | ✅ backend, frontend, postgres, redis, seaweedfs, meilisearch, centrifugo, backup sağlıklı; proxy çalışıyor |
| Vekil | ✅ `/health` 200, `/api/v1/app/config` 200 |
| Güvenlik başlıkları | ✅ HSTS, CSP (`frame-ancestors 'self'`), `X-Frame-Options`, `X-Content-Type-Options`, `Referrer-Policy`, `Permissions-Policy` |

### 3.2 Uçtan uca akış (üretim yığını)

- Platform yöneticisi imajdaki `create-super-admin` ile oluşturuldu.
- `office-onboarding.spec.ts`, `E2E_BASE_URL=http://localhost` (Caddy) ve API'nin doğrudan adresiyle koştu: **6/6** (18 sn).

| Adım | Sonuç |
|---|---|
| Ofis kaydı: platform panelinden ofis + e-posta daveti + davetle kayıt | ✅ |
| Mükellef ekleme: onay e-postası, mükellefin kaydı ve onayı | ✅ |
| UBL yükle: yeni VKN ile türetilen 2 fatura | ✅ |
| Hesap önerisi ve kuyruktan onay | ✅ |
| Yevmiye: taslak, onay, Luca ve Zirve indirme | ✅ |
| Mükellef paneli ve ofisler arası erişim reddi | ✅ |

Duman testi, davet ve erişim reddi testleri demo verisine dayanıyor. Demo seed üretimde (`APP_ENV=production`) çalışmayı reddediyor, bu yüzden o testler bu yığında koşmadı. e2e ortamında yeşiller.

### 3.3 Yedek ve geri yükleme

| Adım | Sonuç |
|---|---|
| Yedek öncesi | `org=1 users=3 taxpayers=1 invoices=2 journal_drafts=2 upload_jobs=2 schema=78` |
| `backup-run` | ✅ şifreli `db-*.dump.age`, `db-*.roles.sql.age`, `objects-*.tar.age` (6 nesne, 92 KB) ve manifestler |
| Felaket | Yığın `down -v` ile birimleriyle silindi |
| Geri yükleme (`docs/yedekleme.md`) | ✅ yalnızca postgres, seaweedfs ve backup başlatıldı. `restore-db`: roller + döküm, manifest doğrulaması "schema version 78, 106 tables, 668 rows". `restore-objects`: 6 nesne. Toplam **3 sn** (küçük veri) |
| Tam yığın | ✅ `migrate` 0 ile çıktı, bütün servisler sağlıklı |
| Yedek sonrası | `org=1 users=3 taxpayers=1 invoices=2 journal_drafts=2 upload_jobs=2 schema=78`, **aynı** |
| Uygulama | ✅ ofis sahibi eski şifresiyle giriş yaptı; iki fatura listede; nesne deposundaki `zirve_fis_2026-11.xlsx` indirildi (200, geçerli xlsx) |

Yedek anahtarı ve `.env.server` işten sonra silindi.

### 3.4 RLS (#166)

| Denetim | Sonuç |
|---|---|
| Ayar | `DB_RLS_RUNTIME` ve `DB_RLS_REQUEST_SCOPE` üretim compose'unda boş, kod varsayılanı `true`: açık |
| Roller | `hakan_rls_app`, `hakan_rls_runtime`, `hakan_rls_worker` var |
| Zorlanmış tablolar | `petitions`, `vat_carry_forwards`, `export_settings`: `relrowsecurity` ve `relforcerowsecurity` açık (`rls_enforced.txt` ile aynı) |
| Kapsamsız okuma | `SET ROLE hakan_rls_runtime` ile kapsamsız: zorlanmış tablolarda 0 satır (bu veride zaten 0 satırdı, kesin kanıt değil). Henüz zorlanmayan `invoices` kapsamsız okunabiliyor; kademeli geçiş tasarımı gereği |
| Kiracı izolasyonu | ✅ yeni ofisin sahibi demo ofisine `NO_TENANT_MEMBERSHIP` aldı; yeni mükellef yalnızca kendi faturalarını görüyor |

Zorlanmış tablolarda satır bazlı sızıntı testi (`rls_enforcement_db_test.go`) CI'da çalışıyor. Bu turda üretim verisinde ayrıca denenmedi.

### 3.5 Log maskeleme (#179)

- **Biçim:** JSON, `LOG_LEVEL=info`.
- **Yapılan deneme:** akış ve kasıtlı hatalar (e-postayla hatalı giriş; sorgu dizesinde e-posta, TCKN ve IBAN) sonrası backend ve worker logları tarandı.
- **Aranan ve bulunmayanlar:** test e-postaları, VKN, JWT (`eyJ…`), şifre, `Bearer`, onay ve davet belirteçleri. Hiçbiri yok.
- **Ne loglanıyor:** erişim logu yalnızca rota, durum, süre ve istek kimliğini yazıyor; sorgu dizesi yazılmıyor.
- **Eksik:** 401 gibi 4xx'lerin gerekçesi ve 5xx'lerin nedeni loga düşmüyordu (#219). Düzeltme #220'de. #220 birleşince gerekçeler maskelenmiş (`ScrubString`) olarak yazılacak.

### 3.6 Metrikler (#179)

| Denetim | Sonuç |
|---|---|
| API `:9091/metrics` tokensız | ✅ 401 |
| API tokenla | ✅ `http_requests_total`, `http_request_duration_seconds` (rota kalıbı ve durumla) |
| Worker `:9092/metrics` | ✅ `queue_tasks`, `queue_latency_seconds`, `queue_inspect_up` |

### 3.7 3. turda açılan issue'lar

| Issue | Konu | Önem | Durum |
|---|---|---|---|
| [#219](https://github.com/HakanAplikasyonu/hakan-app-go/issues/219) | 5xx nedeni ve 4xx gerekçesi loglanmıyor: metrik (#187) ve RLS (#191) yazıcıları kaydediciyi gizliyor | Yüksek (gözlemlenebilirlik) | [#220](https://github.com/HakanAplikasyonu/hakan-app-go/pull/220), auto-merge açık |
| [#221](https://github.com/HakanAplikasyonu/hakan-app-go/issues/221) | e2e `office-onboarding` ikinci yükleme işini beklemiyor (CI'da kararsız) | Orta | ✅ [#222](https://github.com/HakanAplikasyonu/hakan-app-go/pull/222) birleşti |
| [#246](https://github.com/HakanAplikasyonu/hakan-app-go/issues/246) | `frontend/.dockerignore` yok: yerel derleme host `node_modules`/`.next`'i imaja kopyalıyor | Orta | [#249](https://github.com/HakanAplikasyonu/hakan-app-go/pull/249), auto-merge açık |
| [#247](https://github.com/HakanAplikasyonu/hakan-app-go/issues/247) | SeaweedFS `-volume.max=0`: disk azalınca nesne yazımı 500 dönüyor | Orta (kapasite) | Ürün kararı gerekiyor |

### 3.8 Karar bekleyenler ve doğrulanmayanlar

- **Nesne deposu kapasitesi ve izlenmesi (#247):** birim boyutu, `-volume.max`, ayrı disk ve boş birim uyarısı.
- **RLS zorlaması:** şu an 3 tablo zorlanmış. Geri kalan tabloların gruplar halinde zorlanma takvimi #166'da.
- **CI'daki `prod-smoke` kararsızlığı:** #234'ün koşusunda `prod-smoke` yedek adımında düştü. SeaweedFS henüz lider seçmemişti ("raft.Server: Not current leader"), değişiklikle ilgisi yok. Yedek adımından önce SeaweedFS'in yazmaya hazır olması beklenmeli; tekrar ederse ayrı issue açılacak.
- **Doğrulanmayanlar:**
  - HTTPS (gerçek alan adı ve Caddy otomatik sertifika); bu turda `http://localhost` kullanıldı.
  - Gerçek SMTP sağlayıcısı.
  - Worker'ın ayrı çıkış IP'li kopyası (`worker-egress`).
  - Sentry ve OpenTelemetry dışa aktarımı.
  - Demo verisine dayanan duman, davet ve erişim testlerinin üretim yığınında koşması.

## 4. tur (#267)

| | |
|---|---|
| Doğrulanan commit | main `07578ba`; #277 düzeltmesi (#282) yerelde uygulanarak |
| Yığın | `scripts/e2e-up.sh` (yerel compose: Postgres, Redis, SeaweedFS, Centrifugo, Mailhog) + yerelde derlenen backend, demo verisi |
| AI | Sahte, OpenAI uyumlu bir OCR sunucusu (`AI_PROVIDER=openai_compatible`, yerel). Gerçek sağlayıcı denenmedi |
| Mobil | Expo web derlemesi (`expo export -p web`), Playwright ile 390×844 görünümde. Backend'de CORS olmadığından derleme ve `/v1` aynı kökenden sunuldu |
| Masaüstü | Go sidecar başsız çalıştırıldı, satır tabanlı JSON protokolüyle sürüldü. Tauri kabuğu (pencere, tepsi) çalıştırılmadı |
| Hesaplar | Demo: mükellef (Atlas Gıda), müşavir (demo ofis) |

### Özet

- **Mükellef mobil akışları gerçek backend'e karşı çalışıyor:** giriş, şirket seçimi, fiş yükleme, çevrimdışı kuyruk, belge görevi ve hesap silme.
- **Turda bulunan iki engelleyici hata:**
  - #277: AI açıkken mükellefin fiş yüklemesi 500 dönüyordu. Düzeltme [#282](https://github.com/HakanAplikasyonu/hakan-app-go/pull/282), birleşti.
  - #281: mükellef kapsamında öneri yazılamadığı için inceleme kuyruğu boş kalıyor, mükellef düzeltmesi 500 dönüyor. Başka bir oturumda düzeltiliyor.
- **Masaüstü ajanı** klasördeki UBL'i yükledi; fatura ofiste görünüyor. Ajanın ofise giden durum bilgisi gecikiyor (#284). Ofis panelinde ajan ekranı yok (#285).
- **Deep link** zinciri sunucu tarafında ve eşleme fonksiyonunda doğrulandı. Gerçek cihazda bildirime dokunma denenmedi.

### 4.1 Giriş ve şirket seçimi

| Adım | Sonuç |
|---|---|
| E-posta ve parolayla giriş | ✅ |
| Tek şirketi olan mükellef | ✅ Seçim ekranı atlanıyor, doğrudan ana sayfa (şirket adı, VKN ve rol başlıkta) |

### 4.2 Fiş yükleme → ofis inceleme kuyruğu

| Adım | Sonuç |
|---|---|
| Galeriden fiş seçip gönderme | ✅ Fiş ofisin fiş listesine düşüyor |
| AI kapalıyken | ✅ Beklendiği gibi "Okunamadı" (`extract: AI is not configured`) |
| AI açıkken (düzeltmeden önce) | ❌ `POST /v1/taxpayer/receipts` 500: AI maliyet kaydı mükellef kapsamında RLS'ye takılıyor ve isteğin işlemini iptal ediyor (#277) |
| AI açıkken (#282 ile) | ✅ Fiş ofise taslak olarak geliyor; mobilde "Müşavir onayında · 10.10 ₺" |
| Düşük güvenli alanın inceleme kuyruğuna düşmesi | ❌ Kuyruk boş: öneri satırı mükellef kapsamında yazılamıyor (#281). Mükellefin düzeltme göndermesi de 500 dönüyor (#281) |

### 4.3 Çevrimdışı kuyruk

| Adım | Sonuç |
|---|---|
| Bağlantı yokken fiş çekip gönderme | ✅ "Bağlantı yok. 1 fiş telefonda saklanıyor"; ofiste sayı değişmiyor |
| Bağlantı gelince | ✅ Fiş kendiliğinden gönderiliyor; ofisteki fiş sayısı 7 → 8 |
| Aynı görüntü ikinci kez | ✅ Ofiste "Daha önce gönderilmiş" (yinelenen) olarak işaretleniyor |

Not: web derlemesinde rota parçaları tembel yüklendiği için ekranın çevrimdışına geçmeden önce bir kez açılmış olması gerekti. Bu yalnızca web'e özgü; native paket parçaları içinde taşıyor.

### 4.4 Ofisin açtığı belge görevi

| Adım | Sonuç |
|---|---|
| Ofis: banka hesabı tanımlama, CSV ekstre yükleme (3 hareket) | ✅ |
| Ofis: `document-tasks/generate` (dönem 2026-11, tespitli, notlu) | ✅ 1 görev ("Tahsilat için fatura", 4.000,00 TRY), mükellefe e-posta ve uygulama içi bildirim |
| Mobil ana sayfa ve "Tüm görevler" | ✅ Görev, tutar, karşı taraf, tarih, müşavirin notu ve son günle görünüyor |
| Görev ekranında "Galeriden seç" ile fotoğraf | ✅ "Belge alındı ve hareketle eşleşti. Teşekkürler."; durum "Tamamlandı" |
| Ofiste görev | ✅ `matched`, 1 yükleme |

Not: ofisin varsayılan görev listesi yalnızca açık görevleri gösteriyor. Eşleşen görev `?status=matched` ya da `all` ile görünüyor.

### 4.5 Bildirimden deep link

| Adım | Sonuç |
|---|---|
| Görev bildiriminin `action_url`'i | ✅ `/m/{mükellef}/tasks/{görev}` (e-posta ve uygulama içi kayıtta) |
| Push `data.url` | ✅ `BuildPushMessage` aynı yolu taşıyor (kodla doğrulandı) |
| Mobil eşleme (`resolveLink`) | ✅ Bu URL → `/task/{görev}` ve doğru mükellef. Web derlemesinde `/task/{görev}` doğrudan açılıyor |
| Gerçek cihazda bildirime dokunma | ⚠️ Denenmedi (cihaz ve Expo push anahtarı yok) |

### 4.6 Hesap silme

| Adım | Sonuç |
|---|---|
| Profil → "Hesabımı sil" | ✅ Onay kodu e-postası geliyor (Mailhog) |
| Kodla onay | ✅ Oturum kapanıyor, giriş ekranına dönülüyor; hesap kapatıldı e-postası gidiyor |
| Tekrar giriş | ✅ 403 `ACCOUNT_DEACTIVATED` |

Not: profildeki sürüm web derlemesinde "Sürüm ?" görünüyor. Sürüm native ikiliden okunduğu için bu web'e özgü, hata değil. Hesap test sonrasında yerel veritabanında yeniden açıldı.

### 4.7 Masaüstü ajanı: klasör izleme ve UBL

| Adım | Sonuç |
|---|---|
| Sidecar ile giriş ve ofis seçimi | ✅ Ajan kaydı oluşuyor; ofiste `GET /v1/tenant/agents` → `online` |
| Klasör izleme ekleme, klasöre UBL bırakma | ✅ Yaklaşık 30 sn sonra yükleniyor (iki taramada değişmemiş olmalı) |
| Ofiste | ✅ UBL yükleme işi `done` (1 eklendi); fatura listesinde `ATL2026000000091` |
| Yeniden başlatma | ✅ İzleme kalıcı; aynı dosya ikinci kez yüklenmiyor |
| Ofise giden durum bilgisi | ❌ İzlenen klasör ve yüklenen dosya sayısı sonraki heartbeat'e (5 dk) kadar 0 görünüyor; `last_scan` sıfır zaman (#284) |
| Ofis panelinde ajan listesi | ❌ Ekran yok; uç yalnızca API'de (#285) |
| Pencere kapanınca izlemenin sürmesi | ⚠️ Tauri kabuğu çalıştırılmadı. Kodda pencere kapanınca tepsi hazırsa kapanma engelleniyor ve pencere gizleniyor (`desktop/src-tauri/src/lib.rs`); sidecar kabukla birlikte yaşadığı için izleme sürüyor. Sidecar'ın ön yüz olmadan tek başına izleyip yüklediği yukarıda doğrulandı |

### 4.8 4. turda açılan issue'lar

| Issue | Konu | Önem | Durum |
|---|---|---|---|
| [#277](https://github.com/HakanAplikasyonu/hakan-app-go/issues/277) | AI açıkken mükellefin fiş yüklemesi 500 (maliyet kaydı RLS) | Yüksek | ✅ [#282](https://github.com/HakanAplikasyonu/hakan-app-go/pull/282) birleşti |
| [#281](https://github.com/HakanAplikasyonu/hakan-app-go/issues/281) | Mükellef kapsamında öneri yazılamıyor: inceleme kuyruğu boş, düzeltme 500 | Yüksek | Başka oturumda |
| [#283](https://github.com/HakanAplikasyonu/hakan-app-go/issues/283) | Mobil ana sayfa "Fişlerim" özeti sunucudaki fişleri saymıyor | Düşük | Açık |
| [#284](https://github.com/HakanAplikasyonu/hakan-app-go/issues/284) | Masaüstü ajanının durum bilgisi 5 dk gecikiyor, `last_scan` sıfır zaman | Düşük | Açık |
| [#285](https://github.com/HakanAplikasyonu/hakan-app-go/issues/285) | Ofis panelinde masaüstü ajanı ekranı yok | Orta | Ürün kararı gerekiyor |

Turla ilgili, ayrı açılmış iş: #279 (müşavirin elle açtığı belge isteğinin mobilde gösterilmesi).

### 4.9 Karar bekleyenler ve doğrulanmayanlar

- **Mükellef düzeltmesinin önceliği (#281):** mükellefin gönderdiği düzeltme güven 1 ile `auto_accepted` olarak yazılıyor ve modelin önerisini geçersiz kılıyor. Mükellef düzeltmesinin müşavir onayına mı düşeceği, yoksa doğrudan kabul mü edileceği ürün kararı.
- **Ofis ajan ekranının yeri ve eylemleri (#285).**
- **Doğrulanmayanlar:**
  - Gerçek iOS/Android cihazda kamera, arka planda yükleme ve push bildirimine dokunma.
  - Gerçek AI sağlayıcısı (sahte OCR sunucusu aynı kod yolunu kullanıyor).
  - Tauri kabuğu: pencere, tepsi, anahtar zinciri ve otomatik güncelleme.
  - Logo/Mikro yerel okuma.
