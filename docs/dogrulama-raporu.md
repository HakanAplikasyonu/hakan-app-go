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
