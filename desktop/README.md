# desktop/

HAKAN masaüstü uygulaması (D5, docs/01 §4.2): **Tauri v2** kabuğu ve yanında
çalışan **Go sidecar**.

```
desktop/
├── sidecar/          Go sidecar (ayrı Go modülü; backend modülünü replace ile kullanır)
│   ├── rpc/          stdin/stdout satır tabanlı JSON protokolü
│   ├── agent/        yerel ajan: API istemcisi, oturum, ajan kaydı + heartbeat, klasör izleme
│   ├── sources/      Logo Tiger/GO ve Mikro salt okunur SQL okuyucu (defaults.json: doğrulanmamış adlar)
│   └── methods/      RPC yöntemleri
├── src-tauri/        İnce Rust kabuk: pencere, sidecar yaşam döngüsü, OS anahtar zinciri, güncelleyici
│   ├── src/bridge.rs protokolün Rust tarafı (Tauri'den bağımsız, test edilebilir)
│   ├── src/secrets.rs Keychain / Credential Manager / Secret Service
│   ├── src/office.rs  Ofis paneli penceresi (tek program): gezinme, indirme, bildirim
│   └── tests/        kabuk ↔ gerçek Go sidecar uçtan uca testi
├── ui/               Statik arayüz (derleme adımı yok)
└── scripts/build-sidecar.mjs   sidecar'ı binaries/hakan-sidecar-<target-triple> olarak derler
```

## Mimari

- İş mantığı Go sidecar'da. Rust kabuk sidecar'ı başlatır, yanıtları id ile
  eşler, olayları (`ready`) ön yüze `sidecar://event` olarak iletir.
- Ön yüz sidecar'a doğrudan erişemez: `sidecar_call` yalnızca izin
  listesindeki yöntemleri (`ALLOWED_METHODS`, `src/lib.rs`) iletir. Kabuk
  (shell) ve dosya izinleri ön yüze açılmadı.
- Sırlar (API anahtarı vb.) OS anahtar zincirinde, servis adı
  `app.hakan.desktop`. Diske düz yazılmaz.
- Sidecar backend ile aynı Go kodunu kullanır: go-ubltr (UBL okuma) ve
  `backend/pkg/response` (API zarfı). `backend/internal/...` Go kuralı
  gereği başka modülden içe aktarılamaz; paylaşılacak kod `backend/pkg/`
  altına taşınmalı.
- Backend'de yalnızca mevcut API'ler kullanılır (`/healthz`,
  `/v1/app/config`; yükleme uçları #51'de).

## Yerel ajan (#51)

- **Oturum:** Ofis kullanıcısı masaüstünde oturum açar (`/v1/auth/login`, iki
  adımlı doğrulama destekli). Yenileme belirteci sidecar'dan `auth.rotated`
  olayıyla kabuğa gelir ve anahtar zincirine yazılır (`refresh_token`); ön
  yüze hiç iletilmez, ön yüzün `secret_*` komutları bu anahtarı okuyamaz.
  Uygulama açılınca kabuk belirteçle `auth.resume` çağırır.
- **Kayıt ve sağlık:** Ofis seçilince makine `POST /v1/tenant/agents` ile
  kaydolur (kalıcı `machine_id`), 5 dakikada bir heartbeat gönderir. Sunucu
  30 dakika sessiz ajanı `offline` yapar ve kaydeden kullanıcıya
  `AGENT_OFFLINE` bildirimi gönderir (02 §6, 05 §4).
- **Klasör izleme:** Klasör + mükellef (engagement) eşlemesi. 15 saniyede bir
  yoklama; iki taramada boyutu ve zamanı değişmeyen `.xml` / `.zip`
  dosyaları mevcut UBL yükleme ucuna (`/ubl-uploads`) gider. Dosyalar
  taşınmaz ya da silinmez; aynı içerik (SHA-256) bir kez yüklenir. Kalıcı
  hata (415/400/413) tekrar denenmez, ağ ya da 5xx hatası sonraki taramada
  denenir. Durum `<uygulama veri dizini>/agent-state.json` (0600, sır yok).
- **Luca dökümleri ve yabancı fatura PDF'i (#780 e):** Luca'nın API'si yok;
  Luca'dan alınan bordro dökümü ve mizan (`.xlsx` / `.xls` / `.csv`) ile
  yabancı hizmet faturası `.pdf`'i izlenen klasöre bırakılır, ajan bunları
  `POST /v1/tenant/agents/{agent}/documents` ucuna yükler (klasör yolu ve
  klasörün mükellefiyle). Sunucu dosyayı içeriğinden tanır, mükellefi
  dosyadaki VKN'den ya da klasör adından bulur ve elle yüklemeyle aynı içe
  aktarmayı çalıştırır; belli olmayanlar ofisin Yerel ajanlar sayfasında
  "Eşleştir" listesine düşer. Luca kimlik bilgisi istenmez ve saklanmaz.
- **Fiş / fatura fotoğrafı:** `.jpg` / `.jpeg` / `.png` / `.webp` / `.heic`
  aynı belge ucuna gider; sunucu fotoğrafı mobil ve WhatsApp'la aynı fiş
  okumasına (`receipts` CaptureKeyed) verir. İzlenen klasörün bir düzey alt
  klasörleri de taranır (yalnız belge ucuna giden dosyalar); `folder` alanı
  alt klasör yoludur, sunucu mükellefi önce alt klasör adından (unvan ya da
  VKN) bulur. HEIC macOS'ta `sips` ile JPEG'e çevrilir, başka sistemde
  "JPEG olarak kaydedin" nedeniyle reddedilir. PDF fiş okumasına girmez.
- **Logo / Mikro:** `source.test` bağlantıyı ve beklenen tabloları denetler,
  `source.preview` bir tarih penceresini okuyup özetler. Yalnızca
  `sources/defaults.json`'daki doğrulanmış SELECT sorguları çalışır (tek ifade,
  her tabloda `WITH (NOLOCK)`, yalnızca `@from`/`@to`), READ UNCOMMITTED ve
  her zaman geri alınan işlem içinde. Tablo/kolon adları ve tür kodları ⚠️
  doğrulanmadı; pilot mükellefte geçersiz kılma dosyasıyla düzeltilir.
  Okunan veri şimdilik sunucuya gönderilmez.

## Tek program: ofis paneli, sürükle-bırak, Luca indirme

Masaüstü uygulaması ofis için tek program: web ofis paneli, belge yükleme,
Luca dosyası indirme ve yerel ajan aynı süreçte.

- **Ofis paneli** (`src/office.rs`): web ofis uygulaması ayrı bir Tauri
  penceresinde (`office` etiketi) açılır. Adres derleme değişkeni
  `HAKAN_WEB_URL`; yoksa kullanıcının sunucu adresinden, o da yoksa
  derlemedeki `HAKAN_API_BASE_URL`'den baştaki `api.` → `app.` ile türetilir
  (`api.` ile başlamıyorsa panel API ile aynı sunucudadır).
- **Güvenlik:** `office` penceresine hiçbir yetenek verilmez
  (`capabilities/default.json` yalnızca `main`); eklenti komutları (olay
  dinleme dahil) ACL'de reddedilir. Uygulama komutları ACL dışında olduğu
  için `main_window_only` (lib.rs) her komutu yalnızca ana pencereden kabul
  eder. `withGlobalTauri` açık olduğundan uzak sayfada `window.__TAURI__`
  nesnesi bulunur, ama hiçbir çağrısı karşılık bulmaz. Gezinme yalnızca web
  origin'inde kalır; başka http(s), `mailto:` / `tel:` bağlantıları ve yeni
  pencere istekleri sistemde açılır; `file:`, `javascript:`, `data:` ve
  yabancı `blob:` hiç açılmaz. Panelin kendi sürükle-bırakı web sayfasına
  bırakılır (`disable_drag_drop_handler`).
- **Giriş:** parola saklanmaz. Backend'deki QR girişi tarayıcı sekmesinin
  sırrını ve telefondan onayı gerektirdiği, ajan oturumunu web oturumuna
  çeviren bir uç da olmadığı için kullanıcı panelde bir kez oturum açar;
  çerez webview profilinde (uygulama veri klasörü) kalır.
- **Sürükle-bırak yükleme** (yerel arayüz, uzak sayfa değil): bırakılan
  dosya ya da klasörün kendi dosyaları için mükellef + dönem (ay) + tür
  (fatura / Z raporu / masraf / banka ekstresi / diğer) seçilir; sidecar
  `upload.drop` ile arka planda yükler, her dosyanın sonucu `upload`
  olayıyla (`source: "drop"`) Geçmiş'e düşer. Tür ve dönem ofisin evrak
  yükleme ucuna (`POST /v1/tenant/engagements/{e}/document-uploads`) gider;
  Z raporu ve masraf fotoğrafı orada fiş okumasına girer. Luca dökümü
  (`.xlsx` / `.xls` / `.csv`) ve ofisin türlü evrak yüklemeyi kapattığı durum
  (`DOCUMENT_UPLOADS_OFF`) ajan belgesi ucuna (`/agents/{a}/documents`,
  fotoğrafta #831 fiş okuma yolu) mükellefle gider. HEIC macOS'ta JPEG'e
  çevrilir; dosya başına 20 MB, bir seferde en çok 200 dosya.
- **Luca dosyasını indir:** `office` penceresinin indirme olayı yakalanır.
  Yalnızca panelin kendi origin'inden (sayfanın oluşturduğu `blob:` dahil)
  gelen indirme kabul edilir; Ayarlar'da seçilen klasöre (yoksa sistemin
  İndirilenler klasörüne) kaydedilir, aynı ad varsa `ad (2).uzantı`. Seçim
  `<uygulama yapılandırma klasörü>/office.json`. Biten indirme ana pencereye
  `office://download` olarak bildirilir.
- **Arka planda ajan:** klasör izleme ve fiş okuma aynı programda sürer.
  Yeni belge yüklenince tepsi bildirimi (`tauri-plugin-notification`,
  yalnız Rust'tan; ön yüze izin verilmez) en çok 2 dakikada bir özet olarak
  gider; ofis paneli açıksa ana pencereye `office://new-documents` gider ve
  Ofis paneli bölümünde **Paneli yenile** görünür (panel kendiliğinden
  yenilenmez; açık formdaki veri kaybolmasın).
- Komutlar: `office_status`, `office_open`, `office_reload`,
  `office_pick_download_dir`, `office_clear_download_dir`. Tepsideki
  **Ofis paneli** aynı pencereyi açar.

## Luca / Zirve aktarım klasörü (#391)

Araştırma ve karar: `docs/arastirma/luca-zirve-aktarim.md`. Luca'nın herkese açık fiş API'si yok ve yükleme ekranı doğrulama kodu istiyor; Zirve masaüstü programı ve şeması belgelenmemiş. Bu yüzden ajan programa ya da veritabanına yazmaz. Yalnızca dosyayı programın içe aktaracağı klasöre bırakır.

- **Hedef:** **Muhasebe programı** ekranında mükellef + program (Luca / Zirve) + klasör seçilir (`transfer.add_target`, `transfer.remove_target`). Kayıt `agent-state.json` içinde `transfer_targets` alanına yazılır. Aynı mükellef ve programın tek klasörü olur.
- **Akış:** ofis panelinde (journals) "Masaüstü ajanına gönder" denen çıktılar dakikada bir yoklanır (`GET …/journal-transfers?status=queued`, ayrıca bu ajanın üstlendiği `claimed`). Klasörü olan görev üstlenilir (`…/claim`). Dosyalar mevcut indirme ucundan alınır ve SHA-256'ları çıktıdakiyle karşılaştırılır. Klasöre önce `.ad.part` adıyla yazılır, sonra yeniden adlandırılır. Ad `<çıktı adı>_<çıktı kimliğinin ilk 8 hanesi>` biçimindedir: aynı dönemin iki çıktısı çakışmaz, aynı çıktının yeniden teslimi aynı adı kullanır.
- **Sonuç** `…/delivery` ile bildirilir: `delivered` (dosya adları, özetleri) ya da `failed` + Türkçe neden. Nedenler: `FOLDER_MISSING`, `WRITE_DENIED`, `WRITE_FAILED`, `FILE_EXISTS` (aynı adla farklı dosya var; üstüne yazılmaz), `CHECKSUM_MISMATCH`, `FILE_MISSING`. Ağ ya da sunucu hatası bildirilmez: görev üstlenilmiş kalır, sonraki turda aynı ajan sürdürür; 15 dakika sessiz kalırsa başka ajan alabilir.
- Sunucuya klasörün yalnızca adı gider, tam yolu gitmez. Luca / Zirve parolası istenmez ve saklanmaz.
- Son 20 sonuç `agent.status` içindeki `transfer.recent` alanında, ekranda "Son aktarımlar" listesinde görünür. Programda içe aktarmayı kullanıcı yapar ve ofis panelinde "Aktarıldı" ya da "Hata verdi" olarak kapatır.

## Luca yardımcısı (tasarım masaüstü Luca / SGK, adım 2 ve 4–5)

Oğuz'un kararıyla Luca işleri, kullanıcının kendi açtığı Luca oturumunda çalışan bir **yardımcı** ile yapılır. Klasöre bırakma (#391) ve "Luca'ya yükledim" yolları kalır; yardımcı bozulursa kullanıcı onlara döner. Görev API'si: sunucu dalının `luca-ajan-sozlesme.md` belgesi (sürüm 1.3: ofis izni, aktarıcı bilgisayar; 1.2'deki `409 EXPORT_OPEN` istemcide `ErrExportOpen`).

- **Tarayıcı:** `sidecar/helper` bilgisayardaki Edge'i (Windows) ya da Chrome'u (macOS / Linux; `HAKAN_HELPER_BROWSER` ile elle) `chromedp` ile, görünür pencerede ve HAKAN'a ait ayrı profille (`<veri dizini>/helper-profile`, izin 0700) açar. Profilde parola kaydetme ve otomatik doldurma kapalıdır. Bağlantı `--remote-debugging-pipe` ile kurulur; ağ portu açılmaz.
- **Ofis izni (Rahat Aktarım gibi; Luca ile ortaklık yok):** ilk kurulumda yardımcı kartında Türkçe açık onay ekranı çıkar: yardımcı ofis adına Luca'ya girip aktarım yapacak, şifre yalnızca bu bilgisayarda saklanır, istenince silinir. Onay (kim, ne zaman, metin sürümü) `POST /v1/tenant/desktop-helper-settings/luca-consent` ile sunucuya yazılır. İzin yoksa yardımcı görev yoklamaz, üstlenmez; Ayarlar'dan geri alınabilir (`DELETE …/luca-consent`).
- **Kayıtlı giriş (yalnızca bu bilgisayar):** Ayarlar → "Luca girişi" ekranında müşteri no, kullanıcı adı ve şifre bir kez yazılır. Kabuk (Rust) bunları OS şifre kasasına (Windows Credential Manager / macOS Keychain, `keyring`) `luca_login` adıyla yazar; bu anahtar `refresh_token` gibi ön yüze kapalıdır (`secret_get` okuyamaz, `luca_login_status` yalnızca kayıtlı mı / müşteri no / kullanıcı adını döner). Sidecar her açılışta değeri yalnızca belleğe alır (`helper.set_login`, ön yüzün çağırabileceği yöntemler listesinde yok). Şifre sunucuya, günlüğe, dosyaya ya da hata metnine gitmez; tarayıcıya CDP argümanı olarak verilir. Oturum düşünce yardımcı kayıtlı bilgiyle yeniden girer. Giriş ekranında CAPTCHA / güvenlik kodu varsa formu doldurur, kodu kullanıcı yazar (otomatik çözme yok). Luca bilgiyi reddederse bir daha denemez (hesap kilitlenmesin). "Kayıtlı girişi sil" ya da "Her seferinde kendim gireyim" seçilirse kasadaki kayıt silinir ve kullanıcı her seferinde kendisi girer.
- **Aktarıcı bilgisayar:** Ayarlar'daki "Bu bilgisayar aktarıcı olsun" açıksa (`helper-state.json` → `always_on`) uygulama açılınca, pencere gizliyken de görev yoklaması başlar; tarayıcı ilk görevde açılır. Açılınca "Bilgisayar açılınca başlat" da açılır; pencere kapanınca uygulama tepside çalışır. Yoklama `agent_uuid` taşır; sunucu bu bilgisayarın son yoklama zamanından panelde "Aktarıcı bilgisayar: çevrimiçi / çevrimdışı, son görülme" gösterir.
- **Kod istemeyen aktarım:** haritada `import.code_input` isteğe bağlıdır. Kod alanı yoksa ya da ekranda görünmüyorsa ve ofis izni varsa "Yükle"ye yardımcı basar ("basıldı" kaydı tıklamadan önce düşülür; sonuç okunamazsa `needs_check`, asla ikinci kez yüklenmez). Kod alanı görünürse kodu kullanıcı yazıp "Yükle"ye basar.
- **Alan listesi:** yardımcı yalnızca ekran haritasındaki alan adlarında iş yapar; başka adrese yönlenirse görev `DOMAIN_BLOCKED` ile durur.
- **Firma doğrulama:** her görevde Luca'da seçili firmanın VKN'si mükellefinkiyle karşılaştırılır; tutmazsa firma listesinden seçilir, yine tutmazsa hiçbir şey yüklenmez (`FIRM_MISMATCH` / `FIRM_NOT_FOUND`).
- **Görevler:** yardımcı açıkken 15 sn'de bir `GET /v1/tenant/agent-tasks?open=true&agent_uuid=…` yoklanır (önce ofis ayarı ve izni okunur); iş sırası `run_uuid` + `sequence`. `luca_cari_sync`: hesap planı okunur, VKN ile eşleşen cari `found`; yoksa ana hesabın sıradaki kodu (`E10 → E11`) üretilir, yeni cariler tek hesap planı dosyasıyla Luca'nın aktarım ekranına yüklenir, yüklemeden önce ve sonra hesap planı yeniden okunur. `luca_bulk_export`: dosyalar sırayla Excel Veri Aktarımı ekranına verilir, her dosyanın sonucu hemen bildirilir, hatalı satırlar `entries` olarak döner.
- **Onay:** doğrulama kodunu kullanıcı yazar, "Yükle"ye kullanıcı basar. "Yükle"den sonra sonuç okunamazsa (tarayıcı kapandı, uygulama çöktü) dosya `unknown`, görev `needs_check` olur ve asla otomatik yeniden yüklenmez. Aynı dosya (sha256) aynı firmaya ikinci kez yüklenmez (`helper-state.json`). Gönderilemeyen sonuçlar aynı dosyada sırayla bekletilir.
- **Hız sınırı:** tek görev, tek sekme; eylemler arası 1,5 sn, dosyalar arası 5 sn, saatte en çok 60 yükleme; Luca hata sayfasında üstel bekleme, üç kez üst üste olursa durur.
- **Ekran haritası:** `sidecar/helper/maps/luca.json`. Luca keşfi (adım 3) yapılmadığı için seçiciler **yer tutucudur** (`placeholder: true`); bu durumda yardımcı görev üstlenmez. Keşiften sonra dosya doldurulur, `placeholder: false`, `verified: true` yapılır; kod değişmez.
- **Testler:** `sidecar/helper/*_test.go` yerel sahte Luca sayfaları (httptest) ve headless Chrome ile; gerçek Luca'ya gidilmez. Chrome yoksa ya da `-short` verilirse tarayıcı testleri atlanır.

## Arka planda çalışma (#204)

- **Tepsi** (`src-tauri/src/tray.rs`, #325; tuval `docs/tasarim/tuval/AjanTepsi.dc.html`): simgenin 5 durumu (Bağlı, Gönderiyor, Duraklatıldı, Sorun var, Güncelleme hazır) kodla çizilen M3 taç türevidir (32 px RGBA; macOS'ta şablon simge, Windows/Linux'ta pencere temasına göre lacivert ya da açık). Menü: durum satırı, ofis · bilgisayar, 3 sayaç, son dosyalar ya da sorunlu dosyalar (Şimdi yeniden dene, Ayrıntılar), HAKAN'ı aç, Şimdi tara, İzlemeyi duraklat (30 dk / 2 sa / süresiz) ya da sürdür, Ofis paneli (uygulama içindeki pencere), Ayarlar, Oturum açılışında başlat, sürüm / Güncelle ve yeniden başlat, Çıkış. Durum yalnızca mevcut sidecar olaylarından, `agent.status`'tan (kabuk içinden, dakikada bir ve olaylarda) ve güncelleyici durumundan okunur; sidecar değişmez. "Şimdi tara" ayrı RPC olmadığından izlenen ilk klasörü aynı değerlerle `watch.add` ile yeniden ekler (kayıt değişmez, tarama hemen tetiklenir). Sekme isteği ön yüze `shell://navigate` olayıyla gider. Sol tık pencereyi açar.
- **Pencereyi kapatmak** uygulamayı kapatmaz: pencere gizlenir; sidecar ve klasör izleme sürer. Çıkış tepsi menüsünden. Tepsi kurulamazsa (masaüstü ortamı desteklemiyorsa) kapatmak uygulamadan çıkar.
- **Otomatik başlatma** (`tauri-plugin-autostart`): varsayılan kapalı. Tepsiden ya da `set_autostart_enabled` komutuyla açılır. Oturum açılışında `--background` ile pencere göstermeden başlar.
- **Tek örnek** (`tauri-plugin-single-instance`): ikinci açılış yeni süreç başlatmaz, açık pencereyi öne getirir.
- **Sidecar denetimi** (`src/supervisor.rs`): kapanan ya da çöken sidecar 1, 2, 4, 8, 16 sn (en çok 16 sn) beklemeyle yeniden başlatılır. 60 sn sorunsuz çalışınca sayaç sıfırlanır.
  - "İzlemeyi duraklat" sidecar'ı durdurur ve yeniden başlatmaz; "sürdür" başlatır.
  - Komutlar: `shell_settings`, `set_watch_paused`.
- **Kalıcı günlük** (`tauri-plugin-log`): uygulama günlük klasöründe `hakan.log` (Linux `~/.local/share/app.hakan.desktop/logs/`, macOS `~/Library/Logs/app.hakan.desktop/`, Windows `%LOCALAPPDATA%\app.hakan.desktop\logs\`).
  - Dönerek tutulur: 5 dosya × 2 MB.
  - Satırlar yazılmadan önce maskelenir (`src/logmask.rs`): e-posta, VKN/TCKN, IBAN, telefon, JWT, `Bearer` ve belirteç/şifre alanları.

## Arayüz (#206)

`ui/` derleme adımı olmayan düz HTML/JS. Son kullanıcıya göre kuruldu:

- **Akış (arayüz v1, #325):** 3 adımlı kurulum (giriş, iki adımlı doğrulama dahil → ofis ve bilgisayar adı → ilk klasör) → kenar menülü ana pencere: **Ofis paneli**, **Klasörler**, **Geçmiş**, **Sorunlar**, **Ayarlar** (Muhasebe programı ve Gelişmiş bunun içinde; tek program). Tuval: `docs/tasarim/tuval/AjanKurulum.dc.html`, `AjanPencere.dc.html`. Token'lar `ui/tokens.css`, ortak bileşenler `ui/hakan-ui.js`, bildirim kuralları `ui/bildirim.js`.
- **Klasör ekleme:** mükellef adıyla aranıp seçilir (`agent.engagements` yanıtındaki `taxpayer_title`). Klasör `pick_folder` kabuk komutuyla, işletim sisteminin seçicisinden (`tauri-plugin-dialog`) gelir. Ön yüze dosya sistemi ya da diyalog izni verilmez. Sürükle-bırak da çalışır.
- **Klasör başına durum:** sidecar `agent.status` içinde her klasör için `activity` döner: son dosya, son yükleme zamanı, bu oturumda yüklenen sayısı, klasör okuma hatası ve son 20 dosya hatası. Hatalar Türkçe `reason` / `message` alanıyla gelir; ham hata yalnızca günlükte kalır. `watch.clear_errors` listeyi temizler.
- **Kural:** hiçbir ekranda UUID, iç hata kodu ya da ham JSON gösterilmez. Logo/Mikro sonuçları özet ve tablo olarak gösterilir. Ping ve yerel ajanı yeniden başlatma **Ayarlar → Gelişmiş**'tedir.
- **Test:** `npm run test:ui` (`tests/ui/`). Arayüz Chromium'da sahte Tauri IPC'siyle (`tests/ui/mock-tauri.js`) çalışır. Kurulumun üç adımı, klasör ekleme ve kaldırma, geçmiş, geçici / kalıcı sorunlar, program sonuçları, ayarlar, görünüm, güncelleme bandı, klavye gezintisi ve dar pencere denenir; her ekranda UUID/JSON/iç kod olmadığı denetlenir. Tarayıcı: `npx playwright install chromium`; başka bir Chromium için `PW_CHROMIUM_PATH`.
- **Görsel test (#332):** `npm run test:visual` (`tests/ui/gorsel/`). Dört ekran (kurulum girişi, klasörler, sorunlar, ayarlar) dört temada (m3-light, m3-dark, n3-light, n3-dark) ve üç pencere genişliğinde (1140 / 720 / 480) çekilir ve `tests/ui/gorsel/temel/` altındaki 48 temel görüntüyle karşılaştırılır. Saat sabittir, hareket azaltılmıştır. Karşılaştırma ek bağımlılık olmadan tarayıcıda yapılır: kanal farkı 24'ü aşan piksellerin oranı %0,02'yi geçerse test düşer (m3 ↔ n3 farkı bazı ekranlarda yalnızca %0,04). Fark çıkarsa gerçek ve fark görüntüleri `tests/ui/gorsel/sonuc/` altına yazılır; CI bunu `desktop-gorsel` artefaktı olarak yükler, adım şimdilik zorunlu değil. Ekranı bilerek değiştiren iş temel görüntüleri `GORSEL_GUNCELLE=1 npm run test:visual` ile yeniler. Yeni ekran eklemek için `SCREENS` listesine ad, hazırlık ve gidiş adımı yazılır.

## Otomatik güncelleme (#205)

- **Yapılandırma derleme zamanında:** `TAURI_UPDATER_PUBKEY` (imza açık anahtarı) ve `TAURI_UPDATER_BASE_URL` (manifest kökü) `cargo`/`tauri build` sırasında ortamdan okunur (`src/updater.rs`, `option_env!`).
  - Biri yoksa güncelleyici bu derlemede **kapalıdır**; arayüzdeki "Güncellemeler" kartı nedenini yazar. Yerel ve CI derlemeleri böyledir.
  - Sürüm derlemesinde adres `https` olmalıdır; değilse güncelleyici kapalı kalır.
- **Manifest adresi:** `<TAURI_UPDATER_BASE_URL>/<kanal>/latest.json` (`scripts/updater-manifest.mjs` ile aynı).
- **Kanal:** Kararlı (`stable`) ya da Beta (`beta`); arayüzden seçilir, `<uygulama yapılandırma klasörü>/updater.json`'a yazılır. Seçim yoksa `-beta.N` sürümler beta, diğerleri kararlı kanaldadır.
- **Denetim:** açılıştan 15 sn sonra ve 6 saatte bir. Yeni sürüm varsa pencerenin üstünde bant ve kartta "Güncelle ve yeniden başlat" görünür; indirme ilerleme çubuğuyla izlenir, kurulumdan sonra uygulama yeniden başlar. Kullanıcı onayı olmadan kurulmaz.
- Komutlar: `update_status`, `update_check`, `update_set_channel`, `update_install`; olaylar: `updater://status`, `updater://progress`.
- **Uçtan uca test** (`tests/updater_e2e.rs`): her çalıştırmada geçici minisign anahtar çifti üretilir, yerel HTTP sunucusu test manifestini ve paketi verir. 0.1.0 → 0.2.0 geçişinde paket indirilir, imzası doğrulanır ve kurulu ikilinin yerine konur; yanlış anahtarla imzalı paket reddedilir, kurulu dosya değişmez; kanal seçimi doğru manifeste gider.

Yerelde güncelleyiciyi açık derlemek (geçici anahtarla):

```bash
npx tauri signer generate --ci -w /tmp/hakan-test.key -p ""
TAURI_UPDATER_PUBKEY="$(cat /tmp/hakan-test.key.pub)" \
TAURI_UPDATER_BASE_URL=http://127.0.0.1:8080 npm run dev
```

## Protokol

Her satır bir JSON nesnesi:

```
→ {"id":1,"method":"ping","params":{"echo":"x"}}
← {"id":1,"result":{"pong":true,"echo":"x","time":"…"}}
← {"id":2,"error":{"code":"NOT_UBL","message":"…"}}
← {"event":"ready","data":{"version":"0.1.0",…}}
```

stdout yalnızca protokol içindir; sidecar günlükleri stderr'e yazar.

## Geliştirme

Gerekenler: Go (backend ile aynı sürüm), Rust (stable), Node 22. Linux'ta
ek olarak `libwebkit2gtk-4.1-dev libayatana-appindicator3-dev librsvg2-dev patchelf libdbus-1-dev`.

```bash
cd desktop
npm ci
npm run dev          # sidecar'ı derler ve tauri dev
```

Testler:

```bash
(cd sidecar && go test ./...)
npm run sidecar && (cd src-tauri && cargo test)
# Gerçek anahtar zinciriyle (masaüstü oturumu gerekir):
(cd src-tauri && cargo test --lib -- --ignored keychain)
```

Duman testi: `hakan-desktop --self-test` uygulamayı açar, sidecar'a
`version` çağrısı yapar, sonucu tek satır JSON yazar ve çıkar (0 / 1).

## Sürüm ve imzalama

`.github/workflows/desktop-release.yml` (elle ya da `desktop-v*` etiketiyle):

- macOS universal: `APPLE_CERTIFICATE`, `APPLE_CERTIFICATE_PASSWORD`,
  `APPLE_SIGNING_IDENTITY` ile imza; `APPLE_ID`, `APPLE_PASSWORD`,
  `APPLE_TEAM_ID` ile notarization. Tauri sidecar ikilisini de imzalar.
- Windows x64: `WINDOWS_CERTIFICATE` (pfx, base64) ve
  `WINDOWS_CERTIFICATE_PASSWORD`; parmak izi derlemede yapılandırmaya verilir.
- Güncelleyici: `TAURI_SIGNING_PRIVATE_KEY` (+ parola) sırrı ve
  `TAURI_UPDATER_PUBKEY` değişkeni varsa `tauri.release.conf.json` ile
  güncelleme paketleri üretilir. Uygulamanın güncellemeyi denetlemesi için
  ayrıca `TAURI_UPDATER_BASE_URL` değişkeni gerekir; ikisi derleme adımına env
  olarak verilir. Biri yoksa güncelleyici derlemede kapalıdır.

Sırlar yoksa iş düşmez; imzasız paket üretir ve özetine yazar. Anahtarlar koda
ya da depoya girmez.

### Sürüm yayınlama

Ofis panelindeki **Masaüstü ajanları** sayfası "Mac için indir" / "Windows
için indir" düğmelerini en yeni `desktop-v*` GitHub Release'inden alır
(önce kararlı, yoksa beta; `.dmg` ve `.msi`, yoksa `-setup.exe`).

- **Etiketle:** `package.json` sürümünü güncelleyip birleştirin, sonra
  `git tag desktop-v0.2.0 && git push origin desktop-v0.2.0`. İş akışı
  paketleri derler ve aynı adlı Release'e yükler (yoksa oluşturur). `-beta.N`
  ekli etiket ön sürüm (prerelease) olur.
- **Elle:** Actions › desktop-release › Run workflow. `publish` açıksa
  `desktop-v<package.json sürümü>` Release'i bu commit'te oluşturulur ya da
  paketleri güncellenir; kapalıysa yalnız iş çıktısı yüklenir.
- **Sunucu adresi:** sidecar'a derlemede gömülür
  (`HAKAN_API_BASE_URL` → `-X main.defaultBaseURL`, `scripts/build-sidecar.mjs`).
  Elle çalıştırmada `api_base_url` girdisi, etikette depo değişkeni
  `HAKAN_API_BASE_URL`, ikisi de yoksa `https://api.hakan.crmeasytradetr.com`.
  Kayıtlı adresi olmayan kurulumda oturum açma ekranında hazır gelir;
  kullanıcı değiştirebilir.
- **Ofis paneli adresi:** Rust kabuğuna derlemede gömülür
  (`HAKAN_WEB_URL` → `option_env!`, `src/office.rs`;
  `scripts/build-sidecar.mjs` erken doğrular). Elle çalıştırmada `web_url`
  girdisi, etikette depo değişkeni `HAKAN_WEB_URL`; ikisi de yoksa API
  adresinden türetilir (`api.` → `app.`).
- İmza sırrı gerekmez: sırlar yoksa paketler imzasız yayınlanır. İlk açılışta
  macOS'ta sağ tık → **Aç** (ya da Sistem Ayarları › Gizlilik ve Güvenlik ›
  **Yine de aç**), Windows'ta SmartScreen **Ek bilgi → Yine de çalıştır**.

### Kanallar ve sürüm numarası (#183)

- Sürüm `package.json` `version`; etiket iş akışında yazar.
- Kanal etiketten seçilir: `desktop-vX.Y.Z` → **stable**, `desktop-vX.Y.Z-beta.N` → **beta**.
- Güncelleyici yalnızca kendi kanalının `latest.json`'ına bakar. `TAURI_UPDATER_BASE_URL` verilirse adres `<base>/<kanal>/latest.json` olur, verilmezse GitHub Release adresi kullanılır.
- `manifest` işi imzalı paketlerden kanal başına `latest.json` üretir ve iş çıktısı olarak yükler; **yayımlamaz**.

```bash
npm run updater-manifest -- --dir <paketler> --version 1.2.3 --channel stable --tag desktop-v1.2.3
```

```bash
npm test
```

Ayrıntı ve açık kararlar: [`docs/07-surum-hazirligi.md`](../docs/07-surum-hazirligi.md).
