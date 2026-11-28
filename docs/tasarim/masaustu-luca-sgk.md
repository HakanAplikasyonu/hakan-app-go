# Masaüstü yardımcısı: Luca, SGK e-Borcu Yoktur, ticaret odası (tasarım notu)

> **Durum (28.11.2026):** §6 Adım 1 (sunucu: görev tablosu, API, ayar, yeni cari tetiği) ile Adım 4–5'in sunucu tarafı ("Luca'ya aktar" iş sırası, dosya üretimi ve bölme, hatalı satır raporu, cari sonucunun hafızaya yazılması) ve ofis panelindeki "Luca yardımcısı" bölümü uygulandı. Görev API sözleşmesi: `backend/docs/openapi.yaml` (`/v1/tenant/agent-tasks`). Masaüstü tarafı ayrı PR'larda.
>
> **Açık sorularda seçilen (en basit / güvenli taraf):** (9) `agent_tasks` ayrı tablo; yevmiye çıktısında `accounting_transfers` satırı `method = 'luca_web'` ile bağlanır. (14) Yeni cariler tek onayla açılır: tetikle biriken liste ofis "Onayla" deyince ya da bir "Luca'ya aktar" sırasına girince çalışır. (3) "Luca'ya aktar" için tek onay (seçili mükelleflerin tamamı), kim / ne zaman kaydedilir. §5.8 ofis ayarı `desktop_helper_settings` varsayılan KAPALI. (10) Luca fiş numarası şimdilik yalnız görev sonucunda tutulur, `journal_entries`'e yazılmaz. (11) Yoklama 10–15 sn, yeni kuyruk teknolojisi yok. (15) Yevmiye çıktısı ile liste dosyaları aynı belgeyi kapsarsa çift kayıt koruması (`expense_export_documents` / `export_batch_documents`) ikinciyi dışarıda bırakır.

Tarih: 28.11.2026. Kod yazılmadı; yalnızca okundu. Okunanlar: `desktop/` (origin/main 078f7aaa: README, sidecar `agent/transfer.go`, `src-tauri/src/office.rs`, `capabilities/default.json`), `backend/internal/modules/journals/{transfers,doublebooking}.go`, `accounting_transfers.sql`, `modules/agents`, `modules/archive`, `accountingexport/luca`, `modules/lucalists`, `docs/arastirma/luca-zirve-aktarim.md`, `arastirma-luca-sgk.md`, `tasarim-tek-program.md`, `koza-devam.md`.

## 0. Karar ve eski kararla ilişkisi

**Oğuz'un kararı:** Luca ve SGK işleri, mali müşavirin bilgisayarındaki HAKAN masaüstü uygulamasında çalışan bir **yardımcı** ile yapılır.
- Luca'ya ve SGK İşveren Sistemi'ne kullanıcı **kendisi** giriş yapar.
- CAPTCHA'yı ve doğrulama kodunu da kullanıcı **kendisi** girer.
- Şifre HAKAN sunucusuna hiç gitmez, hiçbir yerde saklanmaz.
- Yardımcı yalnızca kullanıcının açık oturumunu kullanır.

**Eski kararla çatışma:** `docs/arastirma/luca-zirve-aktarim.md` §1.2 C ve §4.4 "Luca'ya tarayıcı otomasyonu: yapılmaz" diyor. O kararın gerekçesi iki şeydi:
- doğrulama kodunu otomatik çözmek,
- müşavirin parolasını saklamak.

Yeni tasarımda ikisi de yok: kodu insan girer, parola saklanmaz. Yani gerekçe ortadan kalkıyor, ama belge hâlâ "yapılmaz" diyor. Bu belge ve `desktop/README.md` içindeki "Ajan programın web sitesine dokunmaz" cümlesi, ilk PR'da Oğuz'un kararıyla güncellenmeli (bkz. §6, Adım 1).

**Korunan yol:** bugünkü "klasöre bırak" (`agent_folder`) ve "Luca'ya yükledim" (`manual`) yolları kalır. Yardımcı bozulursa kullanıcı bu yollara döner.

**Ek karar (koordinatör):** Luca Excel toplu indirme (ZIP) ayrı bir iş olarak yapılmayacak. Bunun yerine yardımcının işi şunları da kapsar:
- dosyanın Luca'nın sınırlarına göre bölünmesi,
- çok mükellef ve çok dönemin sırayla yüklenmesi,
- hatalı satır raporu.

Bu iş planda ayrı bir adım olarak yer alıyor (§6, Adım 5).

## 0.1 Ana senaryolar (öncelik: ilk PR'lardan hemen sonra çalışmalı)

### Senaryo 1: Yeni cariden fatura geldi → Luca'da cari sorgula, yoksa aç, kodu HAKAN'a yaz

**Tetik:** bir fatura sisteme düşer (e-fatura entegratörü İşNet/EDM, mobil, WhatsApp) ve karşı tarafın VKN/TCKN'si o mükellefin cari hafızasında yoktur. Cari hafızası `counterparty_accounts` tablosudur (#319): VKN → ana hesap (120/320/…) → hesap kodu.

**Sunucu:** o mükellef için bir `luca_cari_sync` görevi açar ya da açık görevine ekler. Aynı mükellefin yeni carileri tek görevde birikir (VKN başına tek satır).
- Görev hemen çalışmaz. Kullanıcı yardımcıyı açınca, Luca'ya giriş yapmışsa ya da "tek tık aktarım" başlatınca (Senaryo 2'nin ilk adımı) çalışır.
- Gerekçe: Luca'nın içe aktarma ekranı doğrulama kodu istiyor (bkz. §2.3 adım 6).

**Yardımcı:**
1. Mükellefin Luca firmasını seçer ve VKN'yi doğrular.
2. Firmanın hesap planındaki cari alt hesaplarını okur: 120 ve 320 altı, Excel çıktısı ya da liste ekranı.
3. Her VKN için eşleştirir:
   - **Varsa:** Luca'daki kod (ör. `320.01.E10`) ve ad alınır.
   - **Yoksa:** sıradaki kod üretilir. Kural: o ana hesap altındaki en büyük kodun bir fazlası, ofisin kod deseniyle (ör. `E10 → E11`, `001 → 002`). Desen ofis/mükellef ayarıdır ve keşifte Luca'daki gerçek kodlardan çıkarılır. Yeni cariler tek bir **hesap planı Excel dosyasıyla** Luca'nın kendi aktarım ekranına yüklenir (yöntem (a), §3). Kodu kullanıcı girer.
4. Çakışma koruması: yüklemeden hemen önce hesap planı yeniden okunur; o arada biri aynı kodu ya da aynı VKN'yi açtıysa yeni kod ya da eşleştirme yapılır. Yüklemeden sonra da yeniden okunur ve "açıldı" doğrulanır.
5. Sonuç sunucuya döner. Sunucu kodu **cari hafızasına yazar**: `counterparty_accounts`, kaynak = Luca, kim ve ne zaman.
   - Hafızada **farklı** bir karar zaten varsa üstüne yazılmaz. "Luca'da 320.01.E10, HAKAN'da 320.01.E07" diye incelemeye düşer.

**Sonuç:** o carinin faturaları ve fişleri bundan sonra Luca'daki gerçek kodla çıkar. Luca'nın "hesap kodu bulunamadı" hatası ortadan kalkar.

### Senaryo 2: Tek tıkla Luca'ya aktar

**Ekran:** ofis panelinde "Luca'ya aktar". Kullanıcı mükellefleri (bir ya da çok) ve dönemi seçer, hangi türlerin dahil olacağını (hepsi seçili gelir) işaretler ve "Onayla" der.

**Kapsam:**
- banka ekstresi/hareketleri, Z raporları, gelen ve giden faturalar (`lucalists` fiş dosyası),
- masraflar (#852 masraf fişi dosyası),
- onaylı yevmiye çıktıları (journals export).

**Mükellef başına sıra:**
1. **Cari eşitleme:** Senaryo 1, o mükellefin bekleyen carileri.
2. **Dosyaları üret:** sunucuda, çift kayıt korumasından geçerek (#466, `expense_export_documents` anahtarları). Daha önce gitmiş belge girmez.
3. **Böl ve yükle:** Luca sınırlarına göre bölünür ve yüklenir (§6 Adım 5).
4. **Hatalı satır raporu:** sonuç ekranı okunur ve rapor çıkar.
5. **Sıradaki mükellef.**

**Ekranda görünen:** "Mükellef 3/12 · Dosya 2/4". Kullanıcının yapacağı iki şey var: giriş yapmak ve kodu girip "Yükle"ye basmak.

**Sonuç:** her mükellef için "aktarıldı / şu fişler girmedi (neden)". Hatalılar düzeltilip yalnızca onlar yeniden gönderilir.

---

## 1. Mimari

### 1.1 Mevcut yığın
- **Tauri v2 kabuğu (Rust):** pencereler, OS anahtar zinciri, güncelleyici.
- **Go sidecar:** iş mantığı, stdin/stdout JSON-RPC.
- **Ön yüz:** `ui/`, derleme adımı olmayan düz HTML/JS.
- **Ofis paneli:** ayrı `office` webview'ı. Bu pencereye hiçbir yetenek verilmez ve gezinme yalnızca kendi origin'inde kalır.
- **Sunucuyla bağlantı:** ofis kullanıcısının oturumu. Yenileme belirteci anahtar zincirinde durur.
- **Bugünkü görev kuyruğu:** `journal-transfers`. Ajan dakikada bir yoklar ve görevi şu uçlarla işler: `claim` → `delivery` → kullanıcı `complete`.

### 1.2 Seçenekler

| Seçenek | Artı | Eksi |
|---|---|---|
| **A. Go sidecar'da `chromedp` ile bilgisayardaki Chromium tarayıcısını (Windows'ta Edge, yoksa Chrome) görünür pencerede, HAKAN'a ait ayrı bir profille sürmek** | Saf Go, sidecar'a doğrudan girer, Node gerekmez. CDP ile şunlar yapılabilir: dosya alanına dosya verme (`DOM.setFileInputFiles`), indirmeyi yakalama (`Browser.setDownloadBehavior` / `downloadWillBegin`), keşifte istek biçimlerini görme (`Network.*`). Windows'ta Edge her zaman kurulu. CI'da headless Chrome ile sahte Luca'ya karşı test edilebilir. | Bilgisayarda Chrome ya da Edge olmalı (macOS'ta Chrome yoksa sorun, bkz. açık soru 6). Hata ayıklama portu korunmalı (bkz. §5). |
| B. Tauri webview'ı (yeni bir `luca` penceresi) + `eval` ile enjekte edilen betik | Ek tarayıcı gerekmez, indirme olayı zaten `office.rs`'de yakalanıyor. | macOS WKWebView'da CDP yok: ağ isteği görülemez, keşif zayıf kalır. Sonucu Rust'a geri taşımak için uzak sayfaya IPC açmak gerekir; bu, `office` penceresi için kurulan "uzak sayfaya yetenek yok" kuralını bozar. CI'da Tauri çalışma ortamı olmadan test edilemez. |
| C. Playwright (Node veya playwright-go) | Olgun API. | Node sürücüsü ve tarayıcı indirmesi gerekir; paket büyür, sidecar yığınına uymaz. |

**Öneri: A (chromedp + sistemdeki Edge/Chrome, ayrı profil).**
- Ticaret odası sayfası da aynı yardımcı tarayıcıda açılır. Yardımcı orada yalnızca **indirmeyi** yakalar; sayfanın içine hiç dokunmaz.
- Rust kabuğu bu işe karışmaz. Yerel arayüz yardımcıyı yeni RPC yöntemleriyle yönetir (`helper.*` gibi; `ALLOWED_METHODS` listesine eklenir).

### 1.3 Oturum ve çerezler
- Profil klasörü yalnızca kullanıcının bilgisayarında durur: `<uygulama veri dizini>/helper-profile`. Klasör izni 0700.
  - Chrome ve Edge çerezleri zaten OS anahtarlığıyla şifreler (macOS "Safe Storage" anahtarı, Windows DPAPI).
- Profilde **parola kaydetme ve otomatik doldurma kapatılır** (`credentials_enable_service=false`, `profile.password_manager_enabled=false`, autofill kapalı). Böylece "şifre hiçbir yerde saklanmaz" kuralı tarayıcının kendi kasası için de geçerli olur.
- **Luca oturumu:** çerez, Luca kendisi bitirene kadar profilde kalır. Ekranda "Luca oturumunu kapat" düğmesi bulunur; bu düğme Luca'nın çıkışını yapar ve Luca alan adının çerezlerini siler.
- **SGK oturumu:** belge indirildikten sonra yardımcı Çıkış yapar ve `uyg.sgk.gov.tr` çerezlerini siler. Her işyerinin şifresi ayrı olduğu için SGK oturumu tutulmaz.
- Sunucuya oturum, çerez ya da sayfa içeriği gitmez. Yalnızca görevin sonucu gider (§2.4).

### 1.4 Ekran haritası kodda değil, veride
- Luca ve SGK ekranlarının seçicileri, adres kalıpları ve beklenen metinleri bir veri dosyasında tutulur: `desktop/sidecar/helper/maps/luca.json`, `sgk.json`. Bu, Luca şablonlarının YAML'da tutulmasıyla aynı yaklaşım.
- Luca ekranı değişince yalnızca bu dosya güncellenir.
- Harita **sürümlüdür** (`verified: true/false`, `checked_at`).
- Beklenen öğe bulunamazsa yardımcı **tahmin etmez, tıklamaz**: görevi "Luca ekranı değişmiş olabilir" diye durdurur.

---

## 2. Akış (Luca aktarımı)

### 2.1 Görev modeli (sunucu)

Yeni tablo `agent_tasks` (engagement kapsamlı, RLS FORCE + `rls_engagement`). Alanlar:

| Alan | Anlamı |
|---|---|
| `kind` | `luca_cari_sync` (Senaryo 1: sorgula + yoksa aç + geri yaz), `luca_bulk_export` (Senaryo 2: mükellef + dönem + türler; içinde cari eşitleme ve fiş yükleme), `luca_fis_import` (tek çıktı), `luca_invoice_import`, `luca_mizan_fetch`, `sgk_no_debt`, `chamber_document` |
| `transfer_id` | Fiş ve fatura işlerinde mevcut `accounting_transfers` satırı. Bu satır yeni `method = 'luca_web'` ile açılır. Böylece bugünkü "bir çıktının tek açık ya da aktarılmış görevi" kuralı (`ON CONFLICT (export_batch_id) WHERE status IN (...)`) aynen geçerli olur. |
| `payload` (jsonb) | Dönem, kayıt listesi ya da çıktı kimliği, Luca firma eşlemesi. Sır yok. |
| `status` | `queued` → `claimed` → `waiting_user` (giriş ya da kod bekleniyor) → `running` → `done` / `partial` / `failed` / `needs_check` / `cancelled` |
| `result` (jsonb) | Satır sonuçları (§2.4) |
| Diğer | `requested_by`, `approved_by`, `approved_at`, `agent_id`, `claimed_at`, `finished_at`, `idempotency_key` |

**`needs_check` durumu:** yardımcı "Yükle"ye basıldıktan sonra sonucu okuyamadıysa (çökme, ağ kopması) görev bu duruma düşer. Bu görev **asla otomatik tekrar denenmez**. Kullanıcı Luca'ya bakar ve görevi "aktarılmış" ya da "aktarılmamış" diye kapatır. Çift fişin asıl riski bu durumda.

### 2.2 Uçlar (taslak)

| Uç | İzin | İş |
|---|---|---|
| `POST /v1/tenant/engagements/{e}/agent-tasks` | export / ilgili izin | Görev açar. Fiş işinde önce `RequestTransfer(method=luca_web)` çalışır. |
| `GET /v1/tenant/agent-tasks?status=queued` (+ bu ajanın `claimed`/`waiting_user` görevleri) | agent | Ajan görevleri yoklar. |
| `POST .../agent-tasks/{t}/claim` | agent | Görevi üstlenir. |
| `POST .../agent-tasks/{t}/progress` | agent | `waiting_user` / `running` ve "3/7 dosya" gibi ilerleme. |
| `POST .../agent-tasks/{t}/result` | agent | Satır sonuçları. Fiş işinde transfer otomatik olarak `imported` ya da `failed` olur. |
| `POST .../agent-tasks/{t}/resolve` | export | `needs_check` görevi kullanıcı kapatır. |
| `POST .../agent-tasks/{t}/cancel` | export | Vazgeç. |

- Yoklama: bugünkü 1 dakikalık aralık etkileşimli iş için yavaş. Yardımcı penceresi açıkken 10–15 saniyede bir yoklanır.
- Kullanılmayan bir kuyruk teknolojisi eklenmez.

### 2.3 Adım adım
1. Müşavir ofis panelinde çıktının ya da listenin yanında **"Luca'ya aktar (yardımcı ile)"** der. Ekran özet gösterir: mükellef, dönem, fiş ve dosya sayısı, Luca firması. Kullanıcı "Onayla" der. Bu, **görev onayıdır**.
2. Masaüstü görevi alır (`claim`). Dosyalar bugünkü indirme ucundan alınır ve SHA-256 ile denetlenir (`transfer.go` kodu yeniden kullanılır).
3. Yardımcı tarayıcıyı açar ve Luca'da oturum var mı bakar (haritadaki "oturum açık" işareti).
   - Oturum yoksa görev `waiting_user` olur. Masaüstü bildirim verir: "Luca'ya giriş yapın". Kullanıcı kullanıcı adını, şifreyi ve CAPTCHA'yı **kendisi** yazar. Yardımcı bu alanları okumaz, doldurmaz.
4. **Firma doğrulama (zorunlu):** yardımcı Luca'da görevdeki firmayı seçer. Ekrandaki VKN veya unvanı mükellefin VKN'siyle karşılaştırır. Tutmazsa görev durur ve hiçbir şey yüklenmez.
5. Yardımcı "Excel Veri Aktarımı" ekranına gider ve dosyayı dosya alanına verir. Fiş Tipi, Fiş Kodu ve Belge Türü seçimlerini haritaya göre yapar.
6. Ekran doğrulama kodu ister (Luca bunu her yüklemede istiyor, bkz. `luca-zirve-aktarim.md` §1.1). Kodu **kullanıcı girer** ve **"Yükle"ye kullanıcı basar**. Bu, dosya başına ikinci bir onaydır.
7. Yardımcı sonuç ekranını okur: başarılı fiş sayısı, Luca'nın hata satırları ve mesajları, görünüyorsa Luca fiş numaraları. Sonra sıradaki dosyaya geçer.
8. Sonuç sunucuya döner (`result`). Ofis panelinde ve masaüstünün "Geçmiş" ekranında görünür.

### 2.4 Sonuç biçimi
- `files[]`: ad, sha256, `status` (`ok` / `failed` / `unknown`), `luca_message` (en çok 500 karakter, maskelenmiş).
- `entries[]`: HAKAN fiş no, `ok/failed`, varsa Luca fiş no, hata satırı ve neden.
- Cari işlerinde: VKN, Luca hesap kodu, `found` / `created` / `failed`.
- Sonuçta ekran görüntüsü ya da HTML yoktur.

### 2.5 Tekrar aktarımın engellenmesi
1. **Çıktı düzeyi:** `accounting_transfers` kuralı aynen geçerli. Bir çıktının tek açık ya da `imported` görevi olur (`ErrTransferExists`).
2. **Belge düzeyi:** `doublebooking.go` (#466) kuralları değişmez: `DOCUMENT_ALREADY_EXPORTED`, `PROGRAM_PULLS_DOCUMENT` (Luca'nın kendi çektiği entegratör faturaları). Liste dosyalarında `expense_export_documents` anahtarları (`z:`, `bank:`, ETTN) kullanılır. Yardımcı yalnızca bu denetimden geçmiş dosyaları yükler.
3. **Dosya düzeyi:** aynı dosyanın (sha256) aynı firmaya ikinci yüklemesini yardımcı yerel durumda da (`agent-state.json`) engeller.
4. **Belirsizlik:** `needs_check` görevi otomatik tekrar denenmez (§2.1).
5. **Luca'da ön denetim (keşfe bağlı):** Luca'da Evrak No veya ETTN ile fiş arama mümkünse, yüklemeden önce yardımcı arar. Bulursa o fişi atlar ve "Luca'da zaten var" yazar.

---

## 3. Luca'ya yazma yöntemi

| Yöntem | Artı | Eksi |
|---|---|---|
| **(a) Luca'nın kendi içe aktarma ekranına dosyayı yüklemek** (Excel Veri Aktarımı, Excel Fatura Aktarımı, Hesap Planı Excel) | Luca'nın kendi doğrulamasından geçer (hesap kodu yok, dengesiz fiş vb.). HAKAN zaten bu dosyaları üretiyor: `luca/driver.go` 50 fiş / 400 satır bölmesini yapıyor, `fis_verified-1`, `hesap_plani_unverified-1`. Kullanıcıların bildiği ekran. Doğrulama kodu adımında kullanıcı onayı doğal olarak var. Ekranda az öğeye dokunulur, kırılganlık düşük. | Her yüklemede kod gerekir: 50 fişten büyük işler birkaç kod demek. Sonuç ekranından fiş numarası okunamayabilir. |
| (b) Ekran formlarını alan alan doldurmak (fiş giriş ekranı, cari kart) | Fiş numarası anında görünür. | Çok sayıda öğe var, en kırılgan yol bu. Yavaş, Luca'yı daha çok yorar. Luca'nın toplu doğrulamasından geçmez. |
| (c) Luca web uygulamasının iç uçlarını oturumla çağırmak (Rahat Aktarım modeli) | Hızlı; fiş numarası yanıtta gelir. | Belgesiz uçlar haber vermeden değişir. Kullanım şartları açısından en riskli yol. Kod adımı atlanıyorsa koruma aşılmış sayılabilir. Hatalı istek denetimsiz kayıt üretebilir. |

**Önerilen sıra:**
- Fiş, fatura ve cari açma için **önce (a)**.
- (b) yalnızca (a)'nın karşılamadığı tekil işlerde düşünülür (ör. tek cari kartın bir alanını düzeltmek), o da keşiften sonra.
- **(c) önerilmez.** Yalnızca **okuma** işlerinde (cari arama, firma listesi) ve keşifte görülen uç, sayfanın kendisinin de yaptığı sıradan bir GET ise düşünülebilir. Yazma işlerinde (c) kullanılmaz.
- **Okuma işleri** mümkünse Luca'nın kendi "Excel'e aktar" çıktısıyla yapılır:
  - **Mizan:** rapor ekranında Excel indirilir. Yardımcı indirmeyi yakalar ve mevcut ajan belge ucuna (`POST /v1/tenant/agents/{a}/documents`, #780 e) yükler. Sunucu mizanı zaten tanıyor ve içe aktarıyor.
  - **Cari sorgulama:** hesap planının 120/320 altı Excel'e alınır ve sunucuda VKN ile eşlenir.
  - **Cari açma:** eksik cariler HAKAN'ın hesap planı şablonuyla tek dosyada Luca'ya yüklenir. Yöntem (a).

### 3.1 Keşif adımı (Oğuz, test hesabıyla)
- Yardımcıda gizli bir **"Keşif modu"** olur. Yalnızca geliştirici derlemesinde ya da ayarla açılır.
- Oğuz Luca'ya **kendisi** giriş yapar. Test firması sahte verili olmalıdır.
- Oğuz akışları elle yürütür: firma seçimi, Excel Veri Aktarımı, Excel Fatura Aktarımı, Hesap Planı aktarımı, cari listesi, mizan raporu, çıkış.

**Kaydedilenler:**
- Sayfa adresinin yolu. Sorgu parametrelerinin değerleri silinir, yalnızca adları kalır.
- Sayfa başlığı ve menü yolu.
- Form öğeleri: etiket, `name`, `id`, tür, zorunluluk, seçenek listesindeki **sabit** değerler (Fiş Tipi gibi).
- Düğme metinleri ve seçicileri.
- Başarı ve hata mesajlarının kalıbı. Rakamlar ve adlar yer tutucuyla değiştirilir.
- Ağ istekleri: yöntem, yol, içerik türü, parametre **adları** ve değer türü (`<sayı>`, `<tarih>`, `<metin>`); yanıtın JSON anahtar yapısı.
- Oturum açık ve oturum düştü işaretleri; CAPTCHA ve doğrulama kodu alanının yeri.
- Luca'nın verdiği şablon dosyalarının yalnızca başlık satırı.

**Kaydedilmeyenler:**
- Hiçbir alanın değeri. `type=password` alanları hiç okunmaz.
- Çerez, `Authorization` ya da oturum başlığı, CSRF belirteci.
- VKN, TCKN, IBAN, tutar, unvan ve kişi adı. Bunlar `logmask.rs` kalıplarıyla maskelenir; maskelenemeyen metin atılır.
- Ekran görüntüsü ve sayfa HTML'i. Gerekirse Oğuz test firmasında kendisi çeker.

**Çıktı:** yerel bir `kesif-<tarih>.json` dosyası. Oğuz okur, temizler ve onaylar. Sonra bu dosyadan `maps/luca.json` ve sahte Luca sayfaları (§6, Adım 8) üretilir.

**Keşifle cevaplanacak sorular:**
- 400 satır ve 50 fiş sınırı gerçekten var mı?
- Sonuç ekranında fiş numarası görünüyor mu?
- Evrak No ile fiş aranabiliyor mu?
- Firma seçimi nasıl yapılıyor?
- Doğrulama kodu her yüklemede mi isteniyor?

---

## 4. SGK e-Borcu Yoktur ve ticaret odası

### 4.1 SGK e-Borcu Yoktur
- **Ön koşullar:** işyerinde e-Borcu Yoktur aktivasyonu olmalı (işyeri başına bir kez, e-Devlet). Kullanıcının o işyerinin e-Bildirge bilgileri olmalı.
- **Görevin içeriği** (`sgk_no_debt`): mükellef, işyeri sicil numarasının son haneleri (ekranda doğru işyerini göstermek için) ve amaç (genel / ihale; keşifle netleşir).

**Akış:**
1. Yardımcı `https://uyg.sgk.gov.tr/IsverenSistemi` adresini açar. Görev `waiting_user` olur.
2. Kullanıcı kullanıcı adını, sistem şifresini, işyeri şifresini ve güvenlik kodunu **kendisi** girer. Yardımcı bu alanlara dokunmaz.
3. Yardımcı ekrandaki işyeri unvanını ve sicilini görevdekiyle karşılaştırır. Tutmazsa durur.
4. Yardımcı e-Borcu Yoktur menüsüne gider ve belgeyi oluşturur. Belge PDF'i indirilir ve yardımcı indirmeyi yakalar.
5. PDF mükellefin arşivine yüklenir (`modules/archive`, yeni tür: "SGK borcu yoktur"). Sunucu PDF'ten belge tarihini, doğrulama kodunu ve VKN/sicili okur, mükellefle karşılaştırır, son belge tarihini günceller ve hatırlatma kurar.
6. Yardımcı Çıkış yapar ve SGK çerezlerini siler.

**Toplu iş:** birden çok mükellef seçilir. Yardımcı her biri için sırayla durur ve girişi ister. Her işyeri ayrı giriş demektir; ekranda "5 işyerinden 2'si" gibi sayaç görünür.

**Borç varsa:** SGK belge vermez, ekranda mesaj gösterir. Yardımcı mesajı (maskelenmiş) sonuca yazar: "Borç görünüyor, belge alınamadı". Borç tutarı kaydedilmez.

### 4.2 Ticaret odası belgeleri
- Oda sayfaları iframe'e gömülemez (`X-Frame-Options`, araştırma §3). Ödeme 3D Secure ile yapılır, giriş e-imza ya da e-Devlet ile.

**Akış:**
1. Mükellef kartında oda seçilir. HAKAN'da "oda → online işlemler adresi" listesi tutulur.
2. "Belge al" denince oda sayfası yardımcı tarayıcıda **yeni pencerede** açılır. Yardımcı tarayıcı yoksa sistem tarayıcısında açılır.
3. Giriş, ödeme ve belge seçimini kullanıcı yapar. Yardımcı bu sayfada **hiçbir öğeyi okumaz ya da doldurmaz**; yalnızca indirme olayını dinler.
4. PDF indirilince masaüstü sorar: "Bu belge X mükellefinin faaliyet belgesi olarak yüklensin mi?" Kullanıcı onaylarsa belge arşive yüklenir.
5. Belge e-postayla gelirse kullanıcı PDF'i masaüstüne sürükleyip bırakır. Bugünkü sürükle-bırak yolu kullanılır; arşiv türü eklenir.
6. Kart verisi ve e-Devlet bilgisi HAKAN'dan hiç geçmez.

---

## 5. Güvenlik ve hukuk

### 5.1 Kimlik bilgisi
- Şifre yok: Luca, SGK, oda ve e-Devlet şifreleri ne sunucuda ne yerel dosyada ne de tarayıcı kasasında tutulur. Profilde parola kaydetme kapalıdır.
- Yardımcı `type=password` alanını ve CAPTCHA alanını okumaz, doldurmaz.

### 5.2 CAPTCHA ve doğrulama kodu
Her zaman kullanıcı girer. Görüntü tanıma, dış çözücü servis ya da kodu atlayan bir uç kullanılmaz. Luca'nın yükleme kodu da buna dahildir.

### 5.3 Onay
- **Görev onayı:** panelde özet ve "Onayla". Kim ve ne zaman onayladığı kaydedilir.
- **Toplu onay:** "Seçili 12 mükellefin Kasım fişlerini aktar". Tek onay, liste kayıtta durur.
- **Dosya onayı:** Luca'da kodu girip "Yükle"ye basmak her zaman kullanıcıda kalır.
- **Tam otomatik (kodsuz) aktarım yoktur.** Luca kod istemeyi bırakırsa bile önce yeniden karar verilir.

### 5.4 Kayıt
Her görev için şu bilgiler `activity` / denetim kaydına yazılır: kim açtı, kim onayladı, hangi ajan ve bilgisayar, hangi mükellef ve dönem, ne zaman başladı ve bitti, kaç dosya ve fiş, sonuç, Luca fiş numaraları. Sayfa içeriği ya da Luca mesajının ham hâli yazılmaz.

### 5.5 Alan listesi
- Yardımcı yalnızca haritadaki alan adlarında iş yapar: Luca alan adları (keşifle kesinleşir) ve `uyg.sgk.gov.tr`.
- Oda sayfalarında yalnızca indirme dinlenir.
- Başka bir adrese yönlenirse yardımcı durur.

### 5.6 Hata ayıklama portu
- CDP bağlantısı mümkünse pipe ile kurulur (`--remote-debugging-pipe`).
- Pipe olmazsa: rastgele port, yalnızca 127.0.0.1, tarayıcı kapanınca kapanır.
- Yardımcı tarayıcı kullanıcının günlük tarayıcı profilini kullanmaz; HAKAN'ın ayrı profilini kullanır.

### 5.7 Hız sınırı
Luca yorulmasın diye:
- Ajan başına aynı anda tek görev, tek sekme.
- İki eylem arasında en az 1–2 saniye.
- Dosyalar arasında en az 5 saniye.
- Saatte en çok N yükleme (ayar, varsayılan 60).
- Luca hata ya da "çok istek" sayfası gösterirse üstel bekleme uygulanır; üç kez üst üste olursa görev durur.
- Gece toplu çalışma yoktur: kullanıcı başında olmalı, çünkü kod gerekiyor.

### 5.8 Varsayılan kapalı
- Ofis ayarı `desktop_helper_settings` (Luca ve SGK ayrı anahtarlar). Varsayılan KAPALI; `luca_koza_settings` gibi çalışır.
- Önce pilot ofiste açılır.

### 5.9 Hukuk notu
Bu bölüm yorumdur, hukuk görüşü değildir.
- **Luca:** üyelik sözleşmesinin otomasyon maddesi görülemedi (araştırma §1.4). Kullanıcının kendi oturumunda, gözetiminde ve kodu kendisi girerek çalışan bir yardımcı, sunucudan toplu oturuma göre daha savunulabilir. Yine de Luca erişimi engelleyebilir. Genel kullanıma açmadan önce **avukat görüşü** alınmalı. Paralelde Luca'ya web servis başvurusu (L2) sürmeli.
- **SGK:** e-Sigorta sözleşmesi şifrenin işveren ya da yetkili dışındakilerce kullanılmasını yasaklıyor. Önerilen yol, mali müşavirin e-Bildirge kullanıcısı olarak tanımlanması ya da mükelleften yazılı yetki alması. HAKAN şifreyi görmez, ama mükellef onayı metni (sorumluluk ve onay) ürüne eklenmeli.
- **KVKK:** üçüncü tarafa yeni bir veri aktarımı yok. Mizan ve belgeler bugünkü yükleme uçlarından HAKAN'a gider. Aydınlatma metnine "yardımcı" eklenmeli.
- **Sorumluluk:** Luca'ya giden kayıtların sorumluluğu müşavirde kalır. Onay ekranında bu yazılır.

---

## 6. İş planı (her adım küçük bir PR)

Göç numarası her PR'da main max+1 olur; koordinatör yeniden numaralar. Gerçek Luca'ya ya da SGK'ya CI'da **asla** gidilmez.

| # | PR | Neye bağlı | Test yaklaşımı |
|---|---|---|---|
| **1** | **Sunucu: görev tablosu + API + ayar.** Şunlar eklenir: `agent_tasks` göçü (RLS), `accounting_transfers.method` için `luca_web`, uçlar (§2.2), `desktop_helper_settings` (varsayılan kapalı), denetim kaydı, OpenAPI ve api-types. Yeni cari tetiği de bu PR'da: fatura geldiğinde VKN cari hafızasında yoksa `luca_cari_sync` görevine eklenir (mükellef başına tek açık görev). `luca-zirve-aktarim.md` §1.2/§4.4 ve `desktop/README.md` Oğuz'un kararıyla güncellenir; bu not `docs/tasarim/` altına girer. | — | DB testleri: tek açık transfer kuralı `luca_web` için de geçerli; `needs_check` tekrar denenmez; ayar kapalıyken 409; agent izni olmayan claim edemez; aynı VKN'li iki fatura tek cari satırı açar. `authz_table` ve `write_sweep` güncellenir. |
| **2** | **Masaüstü: görev alma + yardımcı tarayıcı + oturum penceresi + firma doğrulama.** `sidecar/helper`: Edge/Chrome bulma, profil (parola kasası kapalı), chromedp başlatma (pipe), alan listesi, "oturum açık mı" denetimi, `waiting_user` bildirimi, `helper.*` RPC'leri, yerel arayüzde "Yardımcı" bölümü. Ayrıca Luca firma seçimi ve VKN doğrulaması. Luca'ya yazmaz. | 1 | Go testleri: `httptest` ile sahte giriş ve firma sayfası + headless Chrome (CI'da Chrome var), alan listesi dışına yönlenince durma, firma tutmazsa durma, profil tercihleri. UI testi: `mock-tauri` ile bekleme ve giriş ekranı. |
| **3** | **Luca keşfi (Oğuz, test hesabıyla).** Keşif modu (yalnız geliştirici ayarı), maskeleme (logmask kalıpları + değer silme), `kesif.json` çıktısı. Keşif oturumundan sonra ayrı bir commit'te `maps/luca.json` (verified=false) ve keşfin cevapları bu nota eklenir. Öncelikle şu ekranlar: hesap planı / cari listesi, hesap planı Excel aktarımı, Excel Veri Aktarımı ve sonuç ekranı, cari kod deseni. | 2 | Maskeleme birim testleri: şifre alanı, çerez, VKN, IBAN ve tutar çıktıya girmez. Sahte sayfada keşif çıktısı beklenen anahtarları içerir. |
| **4** | **Senaryo 1: cari sorgula / yoksa aç / kodu HAKAN'a yaz.** Hesap planı okunur (120/320 altı), VKN ile eşleştirilir. Kod deseni ayarı ve sıradaki kodun üretimi (`E10 → E11`). Yeni cariler tek hesap planı Excel'iyle yüklenir (`hesap_plani` şablonu keşifle doğrulanır, `verified`). Yüklemeden önce ve sonra yeniden okuma. `counterparty_accounts`'a geri yazma; çelişen karar varsa incelemeye düşer. Panelde mükellefin "Luca'da açılacak cariler" listesi ve sonucu (küçük ekran). | 1, 2, 3 | Sahte Luca: cari varsa eşleşir; yoksa sıradaki kodla açılır ve tekrar sorgulanınca bulunur; arada aynı kod başkasınca açılırsa bir sonraki kod alınır; çelişen hafıza kararı ezilmez; sunucu testi: geri yazma kim/ne zaman/kaynak ile. |
| **5** | **Senaryo 2: tek tık aktarım (bölme, çok mükellef/dönem sırası, hatalı satır raporu).** Bu adım iptal edilen ZIP toplu indirme işinin yerine geçer. (i) `luca_bulk_export` görevi: mükellef(ler) + dönem + türler (banka, Z, faturalar, masraflar, yevmiye çıktıları); dosyalar sunucuda `lucalists` / masraf / journals sürücüleriyle ve çift kayıt koruması içinde üretilir. (ii) Mükellef başına önce cari eşitleme (Adım 4), sonra yükleme. (iii) Luca sınırlarına göre bölme: 50 fiş/yükleme, mahsupta 400 satır (`luca/driver.go` bölmesi; sınır farklı çıkarsa yalnız şablon YAML'ı değişir). (iv) Çok mükellef ve dönem tek **iş sırasında** yürür (mükellef, sonra dönem); her mükellefte firma değiştirilip doğrulanır; sayaç "Mükellef 3/12 · Dosya 2/4". (v) Kod adımında bekleme; kullanıcı "Yükle"ye basınca sonuç okunur. (vi) **Hatalı satır raporu:** Luca'nın her hata satırı HAKAN fişine ve kaynak belgeye eşlenir (ETTN / Evrak No / `z:` / `bank:` anahtarı); panelde "şu fişler girmedi, nedeni" listesi ve yalnız hatalıları yeniden gönderme (yeni çıktı; çift kayıt koruması bozulmaz). (vii) Transfer otomatik `imported` / `failed`; yarım kalırsa `needs_check`. Paneldeki "Luca'ya aktar" düğmesi ve sonuç ekranı bu PR'da. | 1, 2, 3, 4 | Sahte Luca: 120 fişlik iş 3 dosyaya bölünür; iki mükellef sırayla ve firma değişimiyle; yeni carisi olan mükellefte önce cari açılır, sonra fiş girer; sahte sonuç ekranında 2 hatalı satır doğru fişe eşlenir; "Yükle"den sonra tarayıcı kapanırsa `needs_check` olur ve tekrar denenmez; aynı sha256 ikinci kez yüklenmez; daha önce aktarılmış belge dosyaya girmez. |
| **6** | **Mizan alma + Excel Fatura Aktarımı.** Mizan Excel'i indirilir ve ajan belge ucuna gider (#780 e). Fatura aktarımı ekranı için yeni şablon (keşfe bağlı); bölme, sıra ve hata raporu Adım 5'in altyapısıyla. | 5 | Sahte Luca: mizan dosyası yakalanır ve yüklenir (sahte backend); fatura dosyası Luca ekranının biçiminde. Şablon testleri `accountingexport` içinde. |
| **7** | **SGK e-Borcu Yoktur + ticaret odası + panelde görev geçmişi.** SGK haritası (keşif: Oğuz kendi işyeriyle), PDF yakalama, arşiv türü, sunucuda PDF'ten tarih/kod okuma. Oda adres listesi, yeni pencere, indirme onayı. Panelde tüm görevlerin geçmişi, `needs_check` kapatma, yeniden dene. | 1, 2 | Sahte SGK sayfası (sahte güvenlik kodu görseli + PDF). Oda için sahte sayfa: yardımcının DOM'a dokunmadığı test edilir (yalnız indirme). Frontend birim testleri ve locale (tr/en). |
| **8** | **Uçtan uca test: sahte Luca sunucusu.** `backend/internal/testfakes/lucaweb` (Koza sahtesi kalıbında): giriş + sahte CAPTCHA, firma seçimi, hesap planı (liste + Excel aktarımı), Excel Veri Aktarımı (dosyayı gerçekten ayrıştırır, 50 fiş sınırını ve hesap kodu yokluğunu uygular), sonuç ekranı, mizan indirme. Sahte sunucunun ilk sürümü Adım 2'de başlar ve her adımda büyür; bu PR tam e2e'yi bağlar: backend + sidecar + headless Chrome; Playwright ile panelde "fatura geldi → cari açıldı → tek tık aktarım → sonuç". Ayrıca elle çalıştırılan **"harita sağlık denetimi"**: Oğuz kendi oturumuyla açar, yalnızca seçicilerin var olup olmadığını raporlar, hiçbir şey yazmaz. | 1–7 | CI'da yalnız sahte sunucu. Sağlık denetimi CI'da çalışmaz. |

Not: Öncelik Senaryo 1 ve 2. Yol 1 → 2 → 3 → 4 → 5; 2 ve 3, 1 birleşmeden başlayabilir (sahte backend ile). 6 ve 7 ana senaryolardan sonra gelir; 7 (SGK), 4–5 ile paralel yürüyebilir (en çok 2 işçi kuralı).

---

## 7. Açık sorular

**Oğuz'a:**
1. Luca test hesabı ve sahte verili test firması var mı? Keşif oturumu ne zaman yapılabilir?
2. Luca'da doğrulama kodu gerçekten her yüklemede isteniyor mu? İsteniyorsa 50 fişlik dosya başına bir kod kullanıcı için kabul edilebilir mi?
3. Toplu onay yeterli mi, yoksa her mükellef için ayrı "Onayla" mı olsun?
4. Avukat görüşü kimden ve ne zaman alınacak? Görüş gelene kadar yalnız pilot ofiste mi açık kalsın?
5. Luca'ya web servis başvurusu (L2) ve "Akıllı Entegrasyon Noktası" sorusu gönderildi mi? Cevap olumlu gelirse yardımcı yerine o yol mu seçilecek?
6. macOS'ta Chrome kurulu değilse ne yapılsın: kullanıcıdan kurmasını istemek mi, yoksa Chromium'u paketle göndermek mi (kurulum yaklaşık 150 MB büyür)?
7. SGK'da mali müşavirlerin çoğu mükellefin şifresiyle mi giriyor, kendisi e-Bildirge kullanıcısı olarak mı tanımlı? Mükellef onay metni ürüne eklensin mi?
8. Hangi odalarla başlanacak? Pilot ofisin mükelleflerinin bağlı olduğu odaların listesi gerekiyor.

**Furkan'a:**
9. `agent_tasks` ayrı tablo mu olsun, yoksa `accounting_transfers` genişletilsin mi? Önerim ayrı tablo ve fiş işlerinde `transfer_id` bağı.
10. Sonuçtaki Luca fiş numaraları HAKAN fişine yazılsın mı (ör. `journal_entries.external_ref`)? Mizan karşılaştırması ve iade/ters kayıt için gerekebilir.
11. Etkileşimli görevler için 10–15 saniyelik yoklama yeterli mi, yoksa mevcut bildirim altyapısıyla bir uyandırma sinyali mi gönderilsin?
12. chromedp'nin pipe modu Windows'ta Edge ile güvenilir çalışıyor mu? Çalışmıyorsa 127.0.0.1 port modu kabul edilebilir mi?

**Ana senaryolar için (Oğuz'a):**
13. Cari kod deseni ofis düzeyinde mi, mükellef düzeyinde mi? Örnek: `320.01.E10`, `320.001`, `120.01.0010`. Satıcı 320 ve müşteri 120 dışında 159, 336 gibi ana hesaplar da açılacak mı?
14. Yeni cari "otomatik açılsın" derken kullanıcı listeye bakmadan mı açılsın, yoksa yardımcı her açılışta "Luca'da açılacak 4 cari" listesini gösterip tek onay mı alsın? Önerim tek onay.
15. Tek tık aktarımda yevmiye çıktıları ile liste dosyaları (banka, Z, faturalar) aynı belgeyi kapsayabilir. Çift kayıt koruması ikisini zaten ayırıyor; ama ofis hangisini tercih ediyor?
