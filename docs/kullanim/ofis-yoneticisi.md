---
roles: [owner, admin]
---

# Ofis yöneticisi kılavuzu

> Ofisi kurun, ekibi davet edin, mükellef ekleyin; inceleme, yevmiye ve Luca / Zirve çıktısını yönetin.

Bu kılavuz ofis **sahibi** ve **yönetici** rolündeki kullanıcılar içindir.
Günlük işler (UBL yükleme, inceleme, uyarılar) için
[Ofis çalışanı kılavuzu](ofis-calisani.md)na bakın.

## 1. İlk giriş ve genel bakış

1. Size gelen davet e-postasındaki bağlantıyla hesabınızı açın ya da
   `/login` sayfasından e-posta ve parolanızla girin. Hesabınızda iki adımlı
   doğrulama açıksa doğrulama uygulamanızdaki kodu da girin.
2. Giriş sonrası ofis panelinin **genel bakış** sayfası açılır. Üstte beyan
   dönemine kalan gün ve KDV son günü, altında dönem tacı (beyana hazır
   mükellef sayısı, istenen belgeler ve müşavir onayı) yazar.
3. Genel bakış, işi geride olan mükellefleri (açık engelleyici uyarısı ya da
   değişiklik talebi olanlar önce) ve ödenecek KDV tahmini en yüksek
   mükellefleri gösterir.

![Ofis paneli genel bakış](../../frontend/public/yardim/img/ofis-genel-bakis.jpg)

Ofis yeni açıldıysa genel bakışta, dönem durumu ve sayaçların altında tek
satırlık **Ofis kurulumunu tamamlayın** kartı görünür. **Küçült** kartı küçük
bir bağlantıya indirir, **Göster** geri açar. **Kuruluma devam et** altı
adımlık sihirbazı açar: ofis
bilgileri, ekip, mükellef listesi, muhasebe programı, fatura kaynakları ve
mükellef davetleri. Soldaki listeden istediğiniz adıma geçebilir, **Geri** ile
dönebilir ya da **Bu adımı atla** ile ilerleyebilirsiniz. Mükellef listesinde
hatalı satırların VKN ve unvanını tabloda düzeltip **Düzeltmeleri uygula** ile
listeyi yeniden kontrol ettirirsiniz. Yarıda bırakılan kurulum genel bakıştaki
karttan sürer.

Her ekrana kenar çubuğundan ya da **Ctrl+K** (macOS'ta ⌘K) komut paletinden
gidebilirsiniz: mükellef adı ya da ekran adı yazın.

### Rehber ve tanıtım turu

Kenar menünün **Ofis** bölümündeki **Rehber** sayfası ilk kurulumu (mükellef
ekleme, Excel'den liste aktarımı, entegratör ve banka ekstresi, KDV eşiği,
masaüstü ajanı, WhatsApp uzantısı), her ayın sırasını (belgeler gelir →
Sıradaki iş → mükellefte Fatura / Z / Masraf / Banka kontrolü → KDV ve eşik →
Luca'ya aktarım → Luca'dan bordro ve mizanı geri yükleme → kapanış), kilitli
aya gelen belgeyi ve beyanname takibini kısaca anlatır. Her adımdaki **Bu
ekrana git** ilgili ekranı açar; henüz hazır olmayan adımlarda **Yakında**
yazar. İlk girişte kenar menüdeki ana ekranları tek tek gösteren kısa bir
tur açılır (**İleri**, **Geri**, **Kapat**). Tur bu tarayıcıda bir kez
gösterilir; Rehber sayfasındaki **Turu başlat** ile yeniden açılır.

## 2. Ofis ayarları

**Ayarlar** sayfasından ofisin görünen adını değiştirirsiniz. Ofisin adresi
(kısa adı, ör. `/t/demo-smmm`) platform tarafından yönetilir; değişmesi
gerekiyorsa platform yöneticisine yazın.

Ofis adına veri işleme sözleşmesi (KVKK) onayı da bu sayfadadır. Sözleşme
metni `/legal/...` sayfalarında yayımlanır; bu kılavuz o metinleri özetlemez.

Ayarlar dört bölümdür (geniş ekranda soldaki menüden, telefonda üstten):

- **Ofis bilgileri:** ad ve veri işleme sözleşmesi.
- **Yapay zekâ ve onay:**
  - **Otomatik onay eşiği** (%80–99): güveni bu değerin üstündeki hesap kodu
    önerileri kuyruğa düşmeden onaylanır. Kaydırırken son 30 günde bu eşikle
    kaç önerinin kendiliğinden onaylanacağı, bunlardan kaçının sonradan
    düzeltildiği ve kaçının kuyruğa düşeceği gösterilir. Otomatik onayı
    tamamen kapatabilirsiniz.
  - **Kural adayı eşiği:** kaç tutarlı karardan sonra kural adayı
    çıkacağı (2–10) ve hangi kararların sayılacağı. Bu ayarı ofis sahibi ve
    yöneticisi değiştirir.
- **Belgeler:** mükellef sayfasındaki ve mükellef panelindeki **Önemli
  belgeler** bölümü (varsayılan açık). Kapatınca bölüm gizlenir; belgeler
  arşivde kalır.
- **Yabancı hizmet faturaları:** mükellef sayfasındaki **Yabancı faturalar /
  KDV2** sekmesi (varsayılan açık) ve işlem türlerinin varsayılanları: her
  tür için yalnız KDV2 ya da KDV2 + stopaj, stopaj oranı, brütleştirme ve
  KDV oranı. Hazır değerler: yazılım / lisans %15 stopaj ve brütleştirme;
  reklam, bulut ve diğer yalnız KDV2; danışmanlık %20 stopaj. Oran çifte
  vergilendirme anlaşmasına göre değişebilir; ofisin kullandığı oranı yazın.
  Kapatınca sekme gizlenir; mükellef panelindeki yabancı fatura yükleme,
  WhatsApp'taki **Yabancı fatura olarak kaydet** ve "Ayın 1'i yabancı
  faturalarınızı gönderin" kuralı da çalışmaz. Girilmiş faturalar silinmez.
- **Döviz kurları:** **Eksik kuru TCMB'den otomatik doldur** anahtarı
  (varsayılan açık) ve **Hangi kur kullanılsın?** seçimi (döviz alış
  önerilir; döviz satış, efektif alış, efektif satış da seçilebilir). TCMB
  kurları her iş günü 15:30'dan sonra kendiliğinden alınır. Kuru gelmeyen
  dövizli faturalar ve yabancı hizmet faturaları, faturanın tarihinden
  önceki son TCMB bülteninin kurunu alır (pazartesi faturası cuma kurunu,
  bayram sonrası fatura tatilden önceki son kuru). Kur türünü değiştirince
  otomatik gelen kurlar hemen yeni türe geçer; elle girilen kurlar ve onaylı
  yabancı faturalar değişmez. Kapatınca kur otomatik gelmez, eksik kurlar
  elle girilir.
- **Mükellef paneli:** **Mükellef panelinde KDV ve satış raporu göster**
  anahtarı (varsayılan açık). Açıkken mükellef kendi panelinde bu ayın
  tahmini KDV'sini, son ödeme gününü ve satış raporunu görür; kapatınca bu
  bölüm tüm mükelleflerin panelinden kalkar.
- **Belge istekleri:** **Belge isterken kanal, hatırlatma ve "Hatırlat"
  düğmesi** anahtarı (varsayılan açık). Açıkken belge isterken mükellefe
  uygulama bildirimi, SMS ve e-postadan hangileriyle haber verileceği ve
  son tarihten kaç gün önce hatırlatılacağı seçilir. Kapatınca istekler
  eskisi gibi mükellefin uygulama kullanıcılarına bildirim ve e-posta
  olarak gider.
- **Eksik ekstre takibi:** **Banka hesaplarında aylık ekstre durumu,
  "Eksik ekstreler" listesi ve "Listeye ekle" önerisi** anahtarı (varsayılan
  açık). Kapatınca mükellefin banka hesapları listesi kalır; ay şeridi, ana
  sayfadaki eksik ekstre listesi ve ekstre yüklerken hesap ekleme önerisi
  görünmez.
- **Masaüstü ajanları:** ofise bağlı ajanlar ve durumları.
- **Görünüm:** tema (kristal mavi ya da yeşil; açık, koyu ya da sistemi izle).
  Yalnızca sizin hesabınız için geçerlidir.

### KDV takibi

**Ayarlar → KDV takibi** bölümünde **ay sonu KDV raporu** (Raporlar
sayfasında bütün mükelleflerin bu ayki KDV durumu) açılıp kapatılır ve
raporun hangi gün "hazır" sayılacağı seçilir. Varsayılan açık ve ayın
29'udur; ay seçilen günden kısaysa ayın son günü geçerlidir. Kapatınca
mükellef sayfalarındaki KDV bilgisi değişmez.

**KDV raporu günleri** ile rapor tek güne bağlı kalmaz. Her satır bir
kuraldır: **ne zaman** (ayın belirli günü, örneğin 15'i ara rapor, ya da
"ay sonundan 3 gün önce"), **kime** (yalnız ofise, ofis onayıyla mükellefe
ya da doğrudan mükellefe) ve **hangi mükellefler** (tümü, seçili
mükellefler ya da yalnız ödemesi çıkanlar). En çok 10 gün eklenir. Kural
çalışınca mükellefin sorumlularına **KDV raporu hazır** bildirimi gelir;
"doğrudan mükellefe" seçiliyse mükellefe tek tıkla KDV bildirimi de gider.
**Şimdi çalıştır** kuralı hemen dener; aynı kural aynı gün bir mükellef için
bir kez çalışır. Hiç kural yoksa yukarıdaki gün, yalnız ofise ve tüm
mükellefler için geçerlidir.

Aynı bölümdeki **KDV nöbetçisi** her sabah bütün mükelleflerin bu ayki
KDV'sini hesaplar. Mükellefe "ödeme çıkmayacak" (ya da "çıkacak") denmişken
yeni bir fatura durumu değiştirirse mükellefin sorumlularına **KDV durumu
değişti** bildirimi gelir; ödenecek KDV eşiği geçerse **KDV eşiği aşıldı**.
Her olay aynı mükellef ve dönem için bir kez gelir. Varsayılan açıktır;
anahtarla kapatınca günlük kontrol yapılmaz.

Eşik aynı yerde ayarlanır: **Ödenecek KDV şu tutarı geçerse uyar** kutusuna
tutarı yazıp (örn. 10.000) **Eşiği kaydet**'e basın. Kutuyu boşaltıp
kaydederseniz eşik uyarısı kapanır, yalnız "durum değişti" uyarısı kalır.
Bu eşik Uyarılar sayfasındaki "Ödenecek KDV eşiği aşıldı" uyarısıyla
aynıdır; uyarı orada ve Komuta merkezinde de görünür.
**Uyarı nasıl gelsin** ile uyarının yalnız uygulama içinde (zil) mi, yoksa
mükellefin sorumlusuna e-postayla da mı gideceği seçilir. Altındaki **Son
KDV uyarıları** listesi ofis genelindeki son nöbetçi uyarılarını (mükellef,
uyarı türü, ödenecek KDV, tarih) gösterir; mükellefin adına tıklayınca KDV
kartı açılır.

## 3. Kullanıcılar ve roller

**Üyeler** sayfasında ofisteki kişiler ve rolleri listelenir.

![Üyeler ve davet](../../frontend/public/yardim/img/ofis-uyeler.jpg)

- **Davet:** "Birini davet et" alanına e-posta adresini yazın, rolü seçin ve
  **Davet gönder**'e basın. Kişi e-postadaki bağlantıyla katılır. Davet
  bağlantıları tek kullanımlıktır ve süreleri dolar; gerekirse yeniden davet
  edin.
- **Rol değiştirme ve çıkarma:** üyenin satırındaki işlemlerden yapılır.
  Çıkarılan kişi ofise erişimini hemen kaybeder.
- **Roller** sayfası her rolün izinlerini gösterir. Stajyer gibi kısıtlı
  rollerin yevmiye ya da ayar ekranlarını görmemesi normaldir.

Kötüye kullanıma karşı davet, giriş ve şifre sıfırlama uçlarında hız sınırı
vardır. Kısa sürede çok sayıda davet gönderirseniz bir süre beklemeniz
istenir.

## 4. Mükellef ekleme

**Mükellef ekle** sayfasından VKN ya da TCKN ile mükellef eklersiniz.

![Mükellef ekleme talepleri](../../frontend/public/yardim/img/ofis-mukellef-ekle.jpg)

1. Unvanı (ya da ad soyadı), VKN/TCKN'yi ve mükellefin iletişim e-postasını
   girin.
2. Mükellefe bir onay bağlantısı gider. Mükellef onaylayınca **hizmet
   ilişkisi** açılır ve mükellef **Mükellefler** listesinde görünür.
3. "Gönderilen talepler" listesinde her talebin durumu görünür. Bekleyen bir
   talebi iptal ederseniz bağlantı geçersiz olur.

**Vergi levhasından doldur:** formun üstündeki **Vergi levhası yükle**
düğmesiyle levhanın fotoğrafını (JPEG, PNG) ya da e-Vergi Levhası PDF'ini
yükleyin. Yapay zekâ VKN/TCKN, unvan, vergi dairesi, iş yeri adresi ve ana
faaliyet kodunu (NACE) forma yazar; kişisel veriler fişlerdeki gibi
maskelenir. Okunamayan, emin olunamayan ya da geçersiz olan alanın (ör. VKN
sağlama hanesi tutmuyor) yanında **Kontrol et** yazar. Hiçbir şey kendiliğinden
kaydedilmez: alanlara bakıp **Onay iste**'ye siz basarsınız. Taranmış PDF ve
iPhone HEIC fotoğrafı okunmaz; levhanın JPEG fotoğrafını yükleyin. Bu özellik
**Ayarlar → Yapay zekâ ve onay → Vergi levhası okuma** anahtarıyla kapatılır;
kapalıyken düğme görünmez ve dosya yapay zekâya gitmez.

Mükellef başka bir ofisle çalışıyorsa, onay verdiğinde önceki hizmet ilişkisi
sona erer. Mükellef bunu onay ekranında görür.

![Mükellefler](../../frontend/public/yardim/img/ofis-mukellefler.jpg)

Mükellef kullanıcılarına panel daveti (fiş gönderme, belge yükleme) ofis
tarafından gönderilir: mükellefin sayfasındaki **Mükellef uygulaması daveti**
bölümünde e-posta adresini yazın, yetkiyi (Sahip / Çalışan) seçin ve **Davet
gönder**'e basın. Kullanıcı e-postadaki bağlantıyla daveti kabul eder. E-posta
ulaşmazsa linki kendiniz iletin: **Davet linkini kopyala** linki panoya alır
("Kopyalandı" yazar), **WhatsApp ile paylaş** kısa bir mesajla WhatsApp'ı açar.
E-posta gönderilemezse ekranda "E-posta gönderilemedi, linki kopyalayıp
paylaşın" uyarısı çıkar; davet yine oluşur ve link çalışır. Güvenlik için link
saklanmaz: bekleyen bir davetin linkini yeniden görmek için **Yeniden gönder**'e
basın; yeni link üretilir, e-posta tekrar gider ve eski link çalışmaz. Mükellef
tarafı için [Mükellef kılavuzu](mukellef.md)na bakın.

## 5. Entegratör ve kaynak bağlantıları

Faturalar üç yoldan gelir:

| Yol | Nasıl |
|---|---|
| UBL yükleme | Ofis panelinde **e-Fatura yükle** ya da mükellef panelinde **Belge yükle** (XML / ZIP) |
| Masaüstü ajanı | Ofis bilgisayarındaki bir klasörü izler, düşen XML / ZIP'i yükler: [Masaüstü ajanı kılavuzu](masaustu.md). Ajanların durumu **Masaüstü ajanları** sayfasında: [ajanları aç](/t?to=agents) |
| Ön muhasebe programları | Paraşüt, KolayBi ve Easytrade bağlantıları, mükellef (hizmet ilişkisi) başına |

> Paraşüt, KolayBi ve Easytrade bağlantılarının ofis panelinde henüz ekranı yoktur. Bağlantı şimdilik API ile ya da destek ekibi aracılığıyla kurulur. Paraşüt'te mükellef uygulamaya yetki verir, sonra şirket seçilir. KolayBi ve Easytrade'de mükellefin API bilgileri girilir. Bağlantıdan sonra senkronizasyon elle ya da zamanlanmış olarak çalışır; gelen kayıtlar **Faturalar** ve **Uyarılar** ekranlarında görünür.

**Easytrade bağlantı formu.** Mükellef sayfasındaki **Easytrade** sekmesinde mükellefin pro.easytradetr.com'a giriş yaptığı e-posta ve şifreyi girin. **ID** alanını boş bırakın: Easytrade girişte hesabın bağlı olduğu firmayı söyler ve her zaman o firma kullanılır; **Bağlantıyı test et** sonrasında "Bağlanılan firma: … (ID …)" satırında görünür ve bağlantı bu ID ile kaydedilir. Farklı bir ID yazılmışsa hata verilmez, "Easytrade hesabınız … firmasına bağlı (ID …), bu firma kullanıldı" yazar. Giriş başarılı olduğu hâlde şube listesi alınamazsa (oturum yenilenemedi) form bunu ayrı bir hata olarak, Easytrade'in durum koduyla gösterir. Giriş reddedilirse form Easytrade'in kendi nedenini, yanıt veren sunucuyu ve durum kodunu gösterir; örneğin "Easytrade (pro.easytradetr.com) 401: E-posta veya şifre hatalı." (bilgiler yanlış) ya da "… 422: Şifre: Şifre en az 4 karakter olmalıdır." (bir alan geçersiz; ilgili alan kırmızı işaretlenir). Sunucu adı pro.easytradetr.com değilse ortam ayarı yanlış sunucuyu gösteriyordur; destek ekibine bildirin. E-posta boşlukları silinip küçük harfe çevrilerek gönderilir, şifre olduğu gibi gider. Lisans bitmişse "Easytrade lisansınızın süresi dolmuş", yetki yoksa Easytrade'in kendi yetki mesajı gösterilir. Easytrade Z raporu sunmuyorsa senkron durmaz; Easytrade sekmesinde "Easytrade Z raporu sunmuyor" notu görünür.

## 6. İnceleme kuyruğu

Faturaların satırlarına hesap kodu önerilir. Güven skoru ofis eşiğinin
altında kalan öneriler **İnceleme kuyruğu**na düşer; eşiğin üstündekiler
otomatik onaylanır.

![İnceleme kuyruğu](../../frontend/public/yardim/img/ofis-inceleme-kuyrugu.jpg)

Yönetici olarak takip etmeniz gerekenler:

- Kuyruk en düşük güvenden başlayarak sıralıdır. Kuyruk boşsa
  "Bekleyen öneri yok" yazar.
- Onaylanan kod fatura satırına yazılır. Yanlış bir onay ya da düzeltme
  **Geri al** ile geri çevrilir; reddedilen öneri geri alınamaz.
- Hesap planı yüklenmemişse kodlar elle yazılır. Hesap planı mükellef
  ayrıntısından yüklenir.
- Mükellefin mizanını yükleyip kararlarınızı kurala çevirmek için
  [Mizan hafızası](mizan-hafizasi.md) kılavuzuna bakın.
- Klavye kısayolları ve "Kural yap" için
  [Ofis çalışanı kılavuzu](ofis-calisani.md#inceleme).

## 7. Yevmiye ve Luca / Zirve çıktısı

**Fişler ve aktarım** sayfasında yevmiye fişi hazırlanır, onaylanır ve
muhasebe programının dosyası indirilir.

![Fişler ve aktarım](../../frontend/public/yardim/img/ofis-yevmiye.jpg)

1. Üstten mükellefi ve dönemi seçin.
2. **Yeni fiş:** açıklamayı yazın, satırları ekleyin. Her satırda hesap kodu
   ve yalnızca borç ya da alacak olur. **Taslağı kaydet** ile kaydedin; fiş
   numarası dönem içinde sırayla verilir.
3. Fişi **Onayla**. Borç ve alacak toplamı eşit değilse onay reddedilir
   ("Fiş dengede değil").
4. **Çıktı al:** programı seçin (**Luca**, **Zirve** ya da
   **Excel (formüllü)**) ve indirin. O dönemin onaylı fişleri dosyaya
   aktarılır ve durumları **Aktarıldı** olur.
5. Aktarılmış bir fişi düzeltmek için önce **Geri al** (gerekçe zorunludur,
   kayda geçer), sonra düzeltin, onaylayın ve yeniden çıktı alın.

"Şablon doğrulanmadı" uyarısı, o programın içe aktarma şablonunun henüz
pilot ofiste doğrulanmadığını gösterir. İlk dosyayı programa aktarmadan önce
bir deneme şirketinde kontrol edin.

**Önceki çıktılar** listesi hangi dosyada kaç fiş olduğunu ve geri alınanları
gösterir.

### Çift kayıt koruması (Luca)

Luca e-faturaları entegratörden kendisi de çekebilir (İşNet NetteFatura,
TÜRMOB e-Belge, GİB e-Belge). Aynı faturanın fişi portaldan da giderse fatura
Luca'da iki kez işlenir. Bunu önlemek için **Çıktı al** önce bir kontrol yapar:

- Fişin faturası daha önce bir dosyada dışa aktarıldıysa (aynı ETTN) ya da
  Luca bu faturayı kendisi çekiyorsa fiş **dosyaya alınmaz** ve nedeniyle
  listelenir: önceki dosya, fiş ve tarih ya da entegratör.
- Belgeyi Luca'da kontrol ettiyseniz yanındaki kutuyu işaretleyip
  **Onaylanan belgelerle dışa aktar** deyin. Onayınız kim ve ne zaman
  bilgisiyle kayda geçer. İşaretlemezseniz uyarılı fişler dışarıda kalır,
  diğerleri dosyaya girer.
- **Luca e-faturaları kendisi çekiyor** bölümünde ofis geneli entegratörleri
  ve gerekirse mükellefe özel seçimi kaydedin.
- **Belge izi**, dışa aktarılan her faturanın hangi dosyada ve hangi fişte
  gittiğini gösterir. ETTN ya da fatura numarasıyla arayabilirsiniz.

Luca dosyasında her satır Evrak No, Evrak Tarihi ve açıklamada faturanın
ETTN'sini taşır. Bu sütunların biçimi Luca'da doğrulanmadı; ekrandaki uyarı
bunu hatırlatır.

### Masaüstü ajanıyla programa aktarım

Dosyayı indirip elle taşımak yerine Luca ya da Zirve çıktısını ofis
bilgisayarındaki masaüstü ajanına gönderebilirsiniz. Ajan dosyayı programın
içe aktaracağı klasöre bırakır. Luca ya da Zirve parolası istenmez; içe
aktarmayı programda siz yaparsınız. Bu yolun nedeni:
`docs/arastirma/luca-zirve-aktarim.md`.

1. Masaüstü uygulamasında **Muhasebe programı → Luca / Zirve aktarım
   klasörleri** bölümünde mükellefi, programı ve klasörü seçip kaydedin.
2. **Önceki çıktılar** listesinde çıktının yanındaki **Masaüstü ajanına
   gönder** düğmesine basın. Durum **Ajan bekleniyor** olur. Ajan bir dakika
   içinde dosyayı klasöre bırakınca durum **Klasörde, içe aktarılmayı
   bekliyor** olur.
3. Dosyayı programın içe aktarma ekranından yükleyin. Sonra **Programa
   aktarım** tablosunda **Aktarıldı** ya da **Hata verdi** düğmesine basın.
   Hata verdi derseniz programın gösterdiği mesajı yazın.
4. Ajan dosyayı bırakamazsa hata mesajı tabloda görünür; ör. klasör
   bulunamadı ya da yazma izni yok. Tablonun üstünde de uyarı çıkar. Sorunu
   giderip **Yeniden dene** ile tekrar gönderin.

Aktarıldı olarak işaretlenen bir çıktı bir daha gönderilemez. Luca aynı
dosya iki kez yüklenince fişleri iki kez keser; bu kural bunu önler.

### Ofis genelinde aktarımlar

Kenar çubuğundaki **Programa aktarımlar** sayfası bütün mükellefleri tek
yerde gösterir:

- **Aktarılmayı bekleyenler:** onaylanmış ama programa ulaşmamış fişi olan
  mükellef ve dönemler; kaç fişin çıktı beklediği, aktarımda olduğu ya da
  aktarımının hata verdiği. Yalnız indirilen çıktılar sayılmaz.
- **Son aktarımlar:** her çıktının son aktarımı ve programa aktarılan
  dönemin mizan durumu: **Uyumlu**, **N hesapta fark**, **Karşılaştırılmadı**
  ya da **Aktarımdan önce karşılaştırılmış**.

Satıra tıklayınca mükellefin Fişler ekranı o dönemle açılır. Atanmadığınız
mükellefler listede yer almaz (bütün mükellefleri görme izni yoksa).

## 8. Dönem durumu

**Komuta merkezi** mükellef × dönem matrisidir: iş nerede, kimde, ne bekliyor?
En geride olan mükellef üsttedir. Engelleyici uyarısı ve bekleyen önerisi
olmayan mükellef beyana hazır sayılır; kilitli dönem onaylanmış sayılır.

![Komuta merkezi](../../frontend/public/yardim/img/ofis-komuta-merkezi.jpg)

### Kilitli döneme gelen değişiklik talepleri

Kilitli bir döneme yeni ya da değişmiş fatura gelince kayıt kendiliğinden
değişmez; mükellef ekranında dönem şeridinin altında talep bekler. **İncele**
ile farkı görüp dört seçenekten birini seçin: **Uygula** (eski davranış;
fatura kendi kilitli dönemine yazılır), **Reddet** (kayıt değişmez),
**Geçmiş döneme düzeltme olarak işle** (fatura kapalı döneme yazılır ve
düzeltme olarak işaretlenir; o ayın beyanı verildiyse düzeltme beyannamesi
gerekebilir, ekran bunu uyarır) ya da **Seçilen aya uygula** (ay seçiciden
açık bir ay seçin, örneğin Kasım; fatura o aya yazılır, kilitli ay
seçilemez). Verilen kararlar, hangi seçenekle ve hangi aya işlendiğiyle
birlikte mükellefin **Geçmiş dönem takibi** sekmesinde "Kilitli dönem
talepleri: verilen kararlar" tablosunda görünür.

## 9. Güvenlik notları

- Hesabınızda iki adımlı doğrulamayı açın: **Profil** sayfasındaki doğrulama uygulaması bölümü.
- Art arda yanlış parola girilen hesap kısa bir süre kilitlenir ve bekleme
  her denemede uzar (en çok 15 dakika). Bu bir saldırı belirtisi olabilir;
  ofis etkinlik kaydında `Başarısız denemelerden sonra giriş kilitlendi`
  olayı görünür.
- Ayrılan çalışanı **Üyeler**'den hemen çıkarın ve kullandığı bilgisayarın
  ajanını **Masaüstü ajanları** sayfasından **Kaldır**'ın.
- Genel bakışta "masaüstü ajanı çevrimdışı" kartı görünürse o bilgisayarda
  klasör izleme durmuştur; [Masaüstü ajanı kılavuzu](masaustu.md#sorun-giderme)ndaki
  adımlara bakın.
