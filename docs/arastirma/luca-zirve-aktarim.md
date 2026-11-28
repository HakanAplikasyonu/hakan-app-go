# Luca ve Zirve'ye otomatik fiş aktarımı: araştırma (#391)

| | |
|---|---|
| Soru | Onaylanan yevmiye fişleri Luca'ya (Luca Net / e-Luca) ve Zirve'ye elle dosya indirip yüklemeden aktarılabilir mi? Hangi yol güvenli ve uygulanabilir? |
| Tarih | 2026-11-17 |
| Kaynak düzeyi | Luca ve Zirve'nin herkese açık destek / ürün sayfaları, üçüncü taraf entegrasyon rehberleri. **Hiçbir deneme gerçek bir Luca ya da Zirve hesabıyla yapılmadı.** |
| Önceki çalışma | `docs/02-muhasebe-cikti-surucu-modeli.md` (sürücü modeli, Luca şablon kuralları), `backend/internal/accountingexport/{luca,zirve}` (doğrulanmamış şablonlar, #29, #53), `desktop/` yerel ajan (#51). |

İşaretler: **[D]** kaynakla doğrulandı (bağlantı verildi), **[Ç]** çıkarım ya da doğrulanamayan üçüncü taraf iddiası.

## Karar

| Program | Karar | Özet |
|---|---|---|
| **Luca** | **Dosya + masaüstü ajanı teslimi; doğrudan yükleme yok** | Herkese açık, belgelenmiş fiş API'si yok. "Excel Veri Aktarımı" ekranı her yüklemede **doğrulama kodu** istiyor: tarayıcı otomasyonu bu korumayı aşmak demek. Luca'nın gizlilik sözleşmesiyle verdiği ücretsiz bir **web servis** var, kapsamı (fiş alıyor mu) herkese açık değil: yazılı olarak sorulacak. |
| **Zirve** | **Dosya + masaüstü ajanı teslimi; veritabanına yazma yok** | Zirve masaüstü programı (SQL sürümleri MS SQL Server üzerinde). Herkese açık API yok; şema belgelenmemiş. Excel / XML aktarım modülleri (Excel fatura, Express Aktarım) var. |

Uygulanan yol (#391 sonraki PR'lar): **aktarım görevi**. Müşavir journals sayfasında bir çıktıyı "masaüstü ajanına gönder" der; ofisin masaüstü ajanı dosyaları o mükellef için seçilmiş **aktarım klasörüne** bırakır ve sonucu (teslim edildi / hata) sunucuya bildirir. Programın içinde içe aktarmayı kullanıcı yapar ve journals sayfasında "aktarıldı" ya da "hata verdi + mesaj" olarak kapatır. Bütün adımlar aktarım geçmişinde görünür.

Bu yol hiçbir koruma önlemini aşmaz, programın veritabanına dokunmaz, Luca/Zirve parolası istemez. Gerçek otomasyon için tek açık kapı Luca web servisidir; başvuru ve cevap gelene kadar kapalı tutulur (§5).

## 1. Luca (Luca Yazılım / TÜRMOB)

### 1.1 Ne var

- **[D] Excel Veri Aktarımı (muhasebe fişi).** Muhasebe bölümünde ayrı ekran: "Excel formatında kayıtlı olan muhasebe fişlerinin sisteme aktarılarak muhasebeleşmesi bu ekrandan sağlanır."
  - Şablon ekrandaki **"Şablon indir"** bağlantısından alınır. Zorunlu alanlar: Fiş No, Fiş Tarihi, Hesap Kodu, Borç, Alacak.
  - Aynı Fiş No + Fiş Tarihi satırları tek fiştir. **Bir yüklemede en fazla 50 fiş**, mahsup fişinde en fazla 400 satır (açılış/kapanışta sınır yok).
  - Akış: Dosya seç → **doğrulama kodu** → Yükle → "Fiş Kes" (Fiş Tipi, Fiş Kodu, Belge Türü seçilir).
  - Hesap kodu Luca'da yoksa "hesap kodu bulunamadı".
  - Kaynak: https://lucayazilim.freshdesk.com/support/solutions/articles/67000267580-01-02-09-excel-veri-aktar-m-
- **[D] Diğer Excel ekranları:** fatura aktarımı, hesap planı (XML/CSV/Excel), işletme / serbest meslek defteri.
  - https://lucayazilim.freshdesk.com/support/solutions/articles/67000271379-01-02-11-excel-fatura-aktar-m-
  - https://lucayazilim.freshdesk.com/support/solutions/articles/67000267449-01-01-04-hesap-plan-xml-csv-excel-aktar
- **[D] Luca Net ürün sayfası:** cari, stok, çek/senet, fatura, **yevmiye fişi** ve banka ekstresi için Excel aktarımı sayılıyor. https://www.luca.com.tr/Products/Index/6
- **[D] Web servis:** Luca e-Belge SSS: "Entegrasyon sağlamak isteyen programlar, web servis hizmeti başvurusunda bulunmalıdır. Yapılacak gizlilik sözleşmesi ile birlikte Web servis hizmeti ücretsiz olarak kullanıma sunulmaktadır."
  - Metin e-Belge bağlamında; **fiş kaydını kapsayıp kapsamadığı yazmıyor**. Uç nokta ya da şema herkese açık değil.
  - Başvuru yolu: destek talep formu https://internet.luca.com.tr/destek/destek_talep.php
  - Kaynak: https://lucayazilim.freshdesk.com/support/solutions/articles/67000549601-e-belge-sso
- **[D] Kayıt Aktarımı** hizmeti başka programdan Luca'ya **tek seferlik geçiş** içindir (Luca Asistan + başvuru formu, `aktarim@luca.com.tr`), günlük aktarım için değil. https://www.luca.com.tr/Sayfa/luca-kayit-aktarimi/64
- **[D, üçüncü taraf]** Bir entegrasyon rehberi Luca'nın resmi sitesinde herkese açık, belgelenmiş bir REST ya da SOAP API bulunmadığını yazıyor. https://www.keydal.tr/blog/luca-entegrasyonu-rehberi
- **[Ç]** "Luca'ya tek tuşla aktarım" diyen ticari ürünler (ön muhasebe, ERP) yöntemlerini açıklamıyor. Büyük olasılıkla ya gizlilik sözleşmeli web servisini ya da içe aktarım dosyası üretmeyi kullanıyorlar.
- **[Ç]** "Luca J-Platform API" adıyla bir şey bulunamadı; j-Platform bir Logo ürünü.

### 1.2 Luca seçenekleri

| Seçenek | Gereken | Teknik risk | Hukuki / sözleşme riski | Değerlendirme |
|---|---|---|---|---|
| A. Excel dosyası, kullanıcı yükler (bugün var) | Luca kullanıcısı, hesap planı | Düşük. Şablon doğrulanmadı (D18). | Yok | Temel yol, kalır |
| B. Dosyayı masaüstü ajanı klasöre bırakır, kullanıcı yükler, sonucu işaretler | Masaüstü ajanı + klasör seçimi | Düşük | Yok | **Uygulanan yol** |
| C. Tarayıcı otomasyonu (Luca'ya müşavir adına giriş + yükleme) | Müşavirin Luca kullanıcı adı/parolası, doğrulama kodunu çözmek | Yüksek: ekran değişince kırılır, doğrulama kodu | Yüksek: doğrulama kodu otomasyonu engellemek için; aşmak koşulları ihlal eder. Müşavir parolasını saklamak ayrı risk. | **Yapılmaz** (bu biçimiyle) |
| C2. Masaüstü yardımcısı, kullanıcının kendi oturumunda (28.11.2026, Oğuz'un kararı) | Masaüstü uygulaması + kullanıcının kendi Luca girişi | Orta: ekran haritası veride tutulur, beklenen öğe yoksa yardımcı durur | Düşük–orta: giriş, CAPTCHA ve doğrulama kodu insan tarafından girilir, "Yükle"ye kullanıcı basar; parola saklanmaz. Genel kullanımdan önce avukat görüşü alınır. | **Uygulanır**, ofis ayarıyla (varsayılan kapalı). Tasarım: `docs/tasarim/masaustu-luca-sgk.md` |
| D. Luca web servisi (gizlilik sözleşmeli) | Başvuru, sözleşme, test hesabı, uç nokta dokümanı | Orta (bilinmiyor) | Düşük (resmi yol), sözleşme şartlarına bağlı | Başvuru yapılır, cevap gelince adaptör |
| E. TÜRMOB tercihli entegratör kanalı | Mükellefin entegratörü | — | — | Yalnızca faturalar için; fiş değil. Çift kayıt riski (`00` E9), mevcut `invoices_reach_luca_directly` ayarı korunur. |

### 1.3 Luca'da bilinmeyenler

- Şablon dosyasının gerçek başlık yazımı, sırası, tarih ve ondalık biçimi (şablon doğrulanmadı; `luca/templates/fis_unverified-1.yaml`).
- Aynı dosyanın iki kez yüklenmesi Luca'da iki fiş üretir [D, üçüncü taraf: https://dcloudsoftware.com/tr/blog/luca-excel-fis-aktarimi-sablon-hazirligi-sik-hatalar]. Bu yüzden aktarım görevi tekrar teslimde aynı dosyayı yeni adla değil aynı adla bırakır ve "aktarıldı" işaretli çıktı için yeni görev açılmaz.
- Web servisinin kapsamı ve kullanım koşulları.
- Üyelik sözleşmesinin otomasyon/bot maddesi (metin bulunamadı; üyelik iletişimi `uyelik@luca.com.tr`).

## 2. Zirve (Zirve Yazılım: Zirve Müşavir, Zirve Muhasebe, Zirve Nova)

### 2.1 Ne var

- **[D] Excel fatura modülü:** Excel'de hazırlanan fatura listesi, "Tanım tabloları"ndaki hesap kodlarıyla fişe çevrilir. https://www.zirveyazilim.net/muhasebe/
- **[D] Express Aktarım:** faturaları (XML dahil, sürükle-bırak toplu) içe alır; Zirve Müşavir SQL güncelleme notlarında "XML Aktarım" eklendiği yazıyor.
  - https://www.zirveyazilim.net/guncelleme-dosyalari/zirve-paket-programlari-kurulum-guncelleme-dosyalari/zirve-musavir-paketi-sql/
  - https://dcloudsoftware.com/tr/cozumler/zirve-entegrasyon
- **[D] Ön muhasebe ürünleri** Zirve'ye Excel dosyası ya da Express Aktarım modülü üzerinden veri gönderiyor. https://atlasaktarim.com/ozellikler/veri-aktarimi
- **[D, üçüncü taraf] Veritabanı ve API:** SQL ürünleri MS SQL Server üzerinde, şema herkese açık belgelenmemiş. "Kayıt Aktar" aracı tek seferlik geçiş içindir. Herkese açık Zirve API'si yok; Nova'daki "API Key" Zirve'nin GİB'e bağlanması içindir. https://dcloudsoftware.com/tr/cozumler/zirve-entegrasyon

### 2.2 Zirve seçenekleri

| Seçenek | Gereken | Teknik risk | Hukuki / sözleşme riski | Değerlendirme |
|---|---|---|---|---|
| A. Excel dosyası, kullanıcı içe alır (bugün var) | Zirve, aktarım şablonu | Düşük. Şablon doğrulanmadı (#53). | Yok | Temel yol, kalır |
| B. Masaüstü ajanı, Zirve'nin bulunduğu bilgisayarda aktarım klasörüne bırakır | Masaüstü ajanı aynı bilgisayarda ya da ortak klasörde | Düşük | Yok | **Uygulanan yol** |
| C. Ajanın Zirve SQL veritabanına doğrudan yazması | SQL kullanıcısı, belgelenmemiş şema | Çok yüksek: fiş numarası, e-Defter zinciri, sürüm farkları; muhasebe bütünlüğü bozulabilir | Lisans/destek kaybı; hatalı kayıtta sorumluluk | **Yapılmaz** |
| D. Ajanın Zirve veritabanını salt okuması (fişin girdiğini doğrulamak için) | SQL okuma kullanıcısı, tablo adları | Orta: tablo adları bilinmiyor | Düşük (salt okuma, müşavirin kendi verisi) | Sonra; Logo/Mikro okuyucusu (`desktop/sidecar/sources`) gibi doğrulanmış SELECT ile. Şema öğrenilince eklenebilir. |
| E. Ekran otomasyonu (Zirve penceresini tıklatmak) | Windows UI otomasyonu | Yüksek, kırılgan | Orta | **Yapılmaz** |

## 3. Karşılaştırma: başka programlar nasıl bağlanıyor

- **Logo (Tiger/GO):** [D] resmi Logo Objects (COM/WCF) ve Logo REST Service; Logo Objects lisansı gerekir. j-Platform REST v2 için Swagger dokümanı var. https://docs.logo.com.tr/public/wua/windows-platform-dependent-customization-tool/logo-rest-services
- **Netsis:** [D] NetOpenX entegrasyon kütüphanesi. https://docs.logo.com.tr/x/GYB1Bw
- **Mikro:** [D, zayıf] apidocs.mikro.com.tr adresinde API dokümanı olduğu ve yazılım evlerinin Mikro API üzerinden bağlanması gerektiği söyleniyor; fiş ucu doğrulanmadı.
- **[Ç] Örüntü:** ERP üreticileri (Logo, Mikro) lisanslı entegrasyon arayüzü satıyor; müşavir paketleri (Luca, Zirve) çoğunlukla dosya içe aktarımına ve (Luca'da) başvuruyla verilen kapalı web servisine dayanıyor. Bu yüzden ajan tabanlı teslim, ileride Logo/Mikro yazma adaptörü için de aynı görev modelini kullanabilir.

## 4. Uygulanan tasarım

### 4.1 Veri modeli

`accounting_transfers` (engagement kapsamlı, RLS FORCE + `rls_engagement` politikası):

- Bir satır = bir çıktının (`export_batches`) bir programa bir kez gönderilmesi.
- `method`: bugün yalnızca `agent_folder`. İleride `luca_webservice` gibi yöntemler aynı tabloya gelir.
- Durumlar: `queued` (ajan bekleniyor) → `delivered` (dosyalar klasörde) → `imported` (kullanıcı programda aktardı) ya da `failed` (ajan ya da program hatası, mesajıyla). `cancelled` kullanıcı vazgeçti.
- Bir çıktı için aynı anda tek açık görev; `imported` olan çıktı tekrar gönderilmez (çift kayıt koruması). Hata alan görev yeniden denenebilir.

### 4.2 Akış

1. Müşavir journals sayfasında bir Luca / Zirve çıktısının yanında **"Masaüstü ajanına gönder"** der → görev `queued`.
2. Masaüstü ajanı (heartbeat döngüsünde) ofisin bekleyen görevlerini alır, yalnızca kendisinde **aktarım klasörü** tanımlı mükelleflerinkini üstlenir (`claim`), dosyaları mevcut indirme ucundan alır, SHA-256'yı denetler ve klasöre yazar (önce geçici ad, sonra yeniden adlandırma; var olan farklı dosyanın üstüne yazmaz).
3. Ajan sonucu bildirir: `delivered` ya da `failed` + Türkçe neden (klasör yok, yazma izni yok, özet tutmadı…).
4. Kullanıcı programda içe aktarır ve journals sayfasında **"Aktarıldı"** ya da **"Hata verdi"** (programın verdiği mesajla) seçer.
5. Aktarım geçmişi ve hata ekranı journals sayfasındadır: tarih, program, çıktı, ajan, durum, hata mesajı, yeniden dene / vazgeç.

### 4.3 Kimlik bilgileri

`agent_folder` yöntemi Luca / Zirve kimlik bilgisi **istemez ve saklamaz**. Ajan ofis kullanıcısının mevcut oturumuyla çalışır (yenileme belirteci OS anahtar zincirinde, #51). Luca web servisi açılırsa servis kullanıcı adı/parolası ya da anahtarı `secrets` kasasında (`internal/platform/secrets`, zarf şifreleme) tutulur; boşken yöntem kapalıdır.

### 4.4 Yapılmayacaklar

- Doğrulama kodunu ya da CAPTCHA'yı otomatik çözme, müşavir Luca parolası saklama, kullanıcı başında olmadan (kodsuz) Luca'ya yükleme.
- Not (28.11.2026, Oğuz'un kararı): kullanıcının kendi açtığı Luca oturumunda, kodu kendisi girerek çalışan masaüstü yardımcısı bu maddenin dışındadır; eski "tarayıcı otomasyonu yapılmaz" kararının iki gerekçesi (kodu otomatik çözmek, parolayı saklamak) yardımcıda yoktur. Bkz. §1.2 C2 ve `docs/tasarim/masaustu-luca-sgk.md`.
- Zirve (ya da başka program) veritabanına yazma.

## 5. Dışarıdan beklenenler

| # | Kimden | Ne | Neyi açar |
|---|---|---|---|
| L1 | Müşavir (pilot ofis) | Luca "Excel Veri Aktarımı → Şablon indir" dosyası ve bir test mükellefinde deneme yüklemesi | Luca şablonunun `verified: true` sürümü |
| L2 | Luca Yazılım | Web servis başvurusu: fiş kaydını kapsıyor mu, kimlik doğrulama, test ortamı, sözleşme | `luca_webservice` yöntemi (adaptör) |
| Z1 | Müşavir (pilot ofis) | Zirve fiş aktarım şablonu (Excel) ya da kabul ettiği XML örneği, Zirve sürümü | Zirve şablonunun doğrulanmış sürümü |
| Z2 | Zirve Yazılım | Resmi entegrasyon yolu var mı (fiş içe aktarım klasörü, komut satırı, servis) | Ajanın sonucu programdan okuması |

### 5.1 Luca'ya gönderilecek talep metni (taslak)

> Konu: Web servis hizmeti başvurusu, muhasebe fişi aktarımı
>
> Merhaba,
>
> Mali müşavirlik ofislerine yönelik bir belge işleme yazılımı geliştiriyoruz. Ofislerin onayladığı yevmiye fişlerini bugün Luca "Excel Veri Aktarımı" ekranından yüklemelerini sağlıyoruz. e-Belge SSS'nizde entegrasyon sağlamak isteyen programların gizlilik sözleşmesiyle web servis hizmetinden ücretsiz yararlanabildiği yazıyor. Şunları öğrenmek istiyoruz:
>
> 1. Web servis muhasebe fişi (mahsup/açılış/kapanış) kaydını destekliyor mu? Destekliyorsa hangi alanlar ve sınırlar geçerli?
> 2. Kimlik doğrulama nasıl yapılıyor (ofis kullanıcısı, uygulama anahtarı)? Mükellef/ofis bazında yetki nasıl veriliyor?
> 3. Test ortamı ve dokümantasyon için başvuru süreci ve gizlilik sözleşmesi taslağı nedir?
> 4. Web servis yoksa, Excel Veri Aktarımı şablonunun güncel sürümünü (başlıklar, tarih ve ondalık biçimi, fiş tipi değerleri) paylaşabilir misiniz?
>
> Teşekkürler.

### 5.2 Zirve'ye gönderilecek talep metni (taslak)

> Konu: Muhasebe fişi içe aktarım biçimi
>
> Merhaba,
>
> Mali müşavirlik ofisleri için onaylı yevmiye fişlerini Zirve'ye aktarılacak dosya olarak hazırlayan bir yazılım geliştiriyoruz. Programa doğrudan veritabanı üzerinden yazmak istemiyoruz. Şunları öğrenmek istiyoruz:
>
> 1. Zirve Müşavir / Zirve Muhasebe muhasebe fişini hangi dosya biçimiyle (Excel şablonu, XML) içe alıyor? Şablon dosyası ve alan açıklaması paylaşılabilir mi?
> 2. Bir klasörden otomatik içe alma ya da komut satırı ile içe aktarma imkânı var mı?
> 3. Yazılım firmaları için resmi bir entegrasyon programı ya da web servis var mı?
>
> Teşekkürler.

## 6. KVKK notu

Aktarım görevi yeni bir veri aktarımı yaratmaz: dosyalar bugün zaten indirilebilen çıktılardır ve ofisin kendi bilgisayarına gider. Luca web servisi açılırsa mükellef verisi (VKN, tutarlar) üçüncü bir tarafa sistemden sistem gönderilmiş olur; ofisle veri işleyen sözleşmesi ve Luca ile gizlilik sözleşmesi bu adımın ön şartıdır (`docs/kvkk.md`).
