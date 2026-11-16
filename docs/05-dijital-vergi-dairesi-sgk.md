# 05 · Dijital vergi dairesi ve SGK: erişim yolu ve kapsam (#55)

| | |
|---|---|
| Talep | T12 (`kaynak/musavir-entegrasyon-paketi/06-mali-musavir-talepleri.md`, `kaynak/mali-musavir-portali-talepler.md` §9): borcu yoktur yazısı, mükellefiyet yazısı, SGK borcu yoktur yazısı; "dijital vergi dairesindeki veriler mükellefin ekranında görünsün"; mükellef "şifreyle uğraşmak istemiyor". |
| Tarih | 2026-11-16 |
| Kaynak düzeyi | GİB'in **e-Beyan Entegrasyon Kılavuzu (2025)** resmi kaynaktan indirilip okundu (§2.1). e-Devlet ve SGK tarafı için resmi teknik doküman **bulunamadı**; o kısımlar haber ve meslek kuruluşu kaynaklarına dayanıyor ve öyle işaretlendi. Hukuki değerlendirme hukuk görüşü değildir. |

## Karar

**İlk dilimde dijital vergi dairesine (DVD), İnternet Vergi Dairesine (İVD) ya da SGK sistemlerine otomatik bağlantı kurulmuyor. Kod yazılmadı.**

Bunun nedeni, T12'nin istediği **okuma** işlemleri için bulunabilen resmi ve izinli bir yol olmaması:

- borç durumu,
- borcu yoktur yazısı,
- mükellefiyet yazısı,
- tahakkuk ve ödeme bilgisi,
- SGK borç durumu.

GİB'in tek resmi web servisi (e-Beyan Entegrasyonu) **beyanname gönderimi** içindir. Okuma servisleri kılavuzda yer almıyor.

Önerilen kapsam:

1. **Faz 0, hemen yapılabilir:** "Resmi belge kasası". Hukuki risk düşük.
   - Mükellef ya da müşavir belgeyi kendisi alıp portala yükler.
   - e-Devlet'teki ilgili hizmete doğrudan bağlantı verilir.
   - Barkodlu belgenin doğrulama bağlantısı belgenin yanında tutulur.
   - Belgenin tarihi ve geçerlilik süresi takip edilir, yenileme hatırlatması gider.
   - Ayrıntı §4'te.
2. **Ayrı iş kalemi: e-Beyan entegrasyonu (beyanname gönderimi).**
   - Resmi bir yol, ama T12 değil. Beyanname düzenleme ve gönderme ürün kapsamına alınırsa açılmalı.
   - Şirket adına GİB'e başvuru ve sabit IP gerektiriyor. #45 bunu sağlıyor.
3. **Otomatik okuma (DVD, İVD, SGK):** Yalnızca resmi bir okuma servisi açılırsa ya da yazılı izin alınırsa yapılmalı. **Mükellef ya da müşavir şifresiyle oturum açma ve ekran kazıma yapılmayacak** ("Karar gerekiyor").

## 1. Ne isteniyor, nereden alınıyor (bugünkü durum)

| İhtiyaç | Bugün nasıl alınıyor | Doğrulama düzeyi |
|---|---|---|
| Vergi borcu yoktur yazısı ("Borç Durum Yazısı") | 2021'den beri e-Devlet'te GİB'in "Borç Durum Yazısı Talep Girişi" hizmeti. Gerçek ve tüzel kişi seçeneği var. DVD ve İVD'den de alınabiliyor. | Haber ve rehber kaynakları ([Capital](https://www.capital.com.tr/haberler/tum-haberler/borcu-yoktur-yazisi-e-devlet-kapisindan-dogrudan-alinabilecek), [Takvim](https://www.takvim.com.tr/guncel/2021/12/08/borcu-yoktur-yazisi-artik-e-devlet-kapisindan-dogrudan-alinabiliyor)). GİB'in e-Devlet hizmet listesi: [turkiye.gov.tr/gelir-idaresi-baskanligi](https://www.turkiye.gov.tr/gelir-idaresi-baskanligi). |
| Mükellefiyet yazısı | DVD, İVD ve e-Devlet üzerinden. | Rehber kaynakları. Resmi hizmet sayfası bu ortamdan okunamadı. |
| SGK e-Borcu Yoktur | İşveren e-Devlet ya da e-Bildirge girişiyle "e-Borcu Yoktur" hizmetinden kendisi üretir. Borç varsa belge çıkmaz, borç gösterilir. İhale amaçlı belgede borç limiti uygulanıyor. | [TÜRMOB notu](https://www.turmob.org.tr/arsiv/mbs/resmigazete/-eborcu-yoktur-uygulamasi.pdf), [Alomaliye](https://www.alomaliye.com/2026/09/21/e-borcu-yoktur-uygulamasi/amp/), [Paraşüt rehberi](https://www.parasut.com/blog/sgk-e-borcu-yoktur). |
| Belgenin gerçekliği | e-Devlet "Barkodlu Belge Doğrulama". Barkod ve T.C. no ile, herkes doğrulayabilir. | [turkiye.gov.tr hizmet sayfası](https://turkiye.gov.tr/hizmetler?lang=tr_TR) (arama sonucu). Akış bu ortamdan denenmedi. |
| "DVD'deki veriler mükellefin ekranında" (beyanname, tahakkuk, borç, ödeme) | Yalnızca DVD ya da İVD'ye mükellef veya meslek mensubu girişiyle. | DVD 7.10.2023'te 552 sıra no.lu VUK Genel Tebliği ile açıldı. Giriş GİB şifresi, e-imza ya da mobil imza ile ([Alomaliye](https://www.alomaliye.com/2023/10/09/dijital-vergi-dairesi-hizmete-acildi/)). |

## 2. Erişim yolları

### 2.1 GİB e-Beyan Entegrasyonu (resmi web servisi), okundu

Kaynak: [e-Beyan Entegrasyon Kılavuzu](https://ebeyan.gib.gov.tr/entegrator/files/e-Beyan%20Entegrasyon%20K%C4%B1lavuzu.pdf). GİB, Uygulama ve Veri Yönetimi Daire Başkanlığı, 2025, 8 sayfa.

- **Amaç:** *"…meslek mensuplarının kullandığı uygulamalar aracılığıyla ve beyanname gönderme yetkisine sahip tüzel kişiliği bulunan mükelleflerin kendi sistemlerinden hızlı ve güvenilir bir şekilde beyanname gönderebilmesi için web servis yoluyla **beyanname kabulü** yapılacaktır."*
- **Kimler başvurabilir:**
  - *"e-Beyan sistemine entegrasyon yöntemiyle beyanname gönderilmesine aracılık etmek isteyen tüzel kişiliği bulunan mükellef"*
  - *"…beyanname gönderme yetkisine sahip tüzel kişiliği bulunan mükellef"*
  - e-beyanname yetkisi olmayan aracı tüzel kişinin başvurabilmesi için *"entegrasyon yöntemiyle ile beyanname gönderebilecek bir altyapısının bulunması ve Başkanlığımızca başvurunun uygun görülmesi"* gerekiyor.
- **Başvuru:** DVD → "Başvuru Formlarım" → "e-Beyan Entegrasyon Başvurusu". Onay GİB'de: *İnceleme Bekliyor / İnceleniyor / Reddedildi / Onaylandı*.
- **Kimlik doğrulama:**
  - *"Bearer authentication"*. Token, e-Beyan → "Entegrasyon Yönetimi" altında, her ortam için ayrı üretiliyor. Alt kullanıcı token üretemiyor.
  - *"Token bilgilerinin muhafaza yükümlülüğü ve … tüm hukuki sorumluluk tarafınıza aittir."*
- **IP kısıtı:** *"…onaylanarak tanımlaması yapılan 'IP Adresleri' ile çalışma ortamlarına erişim sağlanmalıdır."* Asıl, yedek ve test IP'lerinin farklı ve *"Başkanlığımızın başka uygulamalarında kullanılmayan bir Real IP"* olması bekleniyor. Bunu #45'teki sabit çıkışlı worker havuzu karşılıyor.
- **Ortamlar:**

  | Ortam | Adres | Not |
  |---|---|---|
  | Pre-Prod | `ebeyanpreprodapi.gib.gov.tr` | Test |
  | Pilot | `ebeyanpilotapi.gib.gov.tr` | Belirli beyannameler |
  | Prod | `ebeyanapi.gib.gov.tr` | |

  Hangi beyannamelerin açıldığı DVD duyurularında yayımlanıyor.
- **e-imza / mali mühür:** Kılavuz API çağrıları için e-imza ya da mali mühür istemiyor (token + IP). DVD'ye girişte GİB şifresi, e-imza ya da mobil imza kullanılabiliyor.
- **Okuma servisleri:** Kılavuzda borç sorgulama, tahakkuk sorgulama, borcu yoktur yazısı, mükellefiyet yazısı ve e-tebligat okuma **yok**. Ayrıntılı uç listesi giriş gerektiren "Entegratör Portalı"ndaki "API Geliştirici Kılavuzları"nda. Bu portala erişilemedi, dolayısıyla okuma ucu olup olmadığı **kesin değil**. Kılavuzun amaç cümlesi yalnızca beyanname kabulünü söylüyor.
- **Hukuki çerçeve:**
  - Meslek mensubunun beyanname göndermesi için VUK mükerrer 257 kapsamında mükellefle özel sözleşme ve "Elektronik Beyanname Aracılık ve Sorumluluk Sözleşmesi" gerekiyor ([Alomaliye](https://www.alomaliye.com/2021/01/20/elektronik-beyanname-aracilik-ve-sorumluluk-sozlesmesi/)).
  - Gönderimin sorumluluğu meslek mensubunda. Ürün, müşavirin adına ve onun yetkisiyle gönderir.

### 2.2 e-Devlet Kapısı

- **Kurum servisleri:** e-Devlet, kurum servislerini kullanıcıya sunan bir platform ([nasıl çalışıyor](https://turkiye.gov.tr/bilgilendirme?konu=nasilCalisiyor), [yönetmelik](https://www.lexpera.com.tr/mevzuat/yonetmelikler/e-devlet-hizmetlerinin-yurutulmesine-iliskin-usul-ve-esaslar-hakkinda-yonetmelik)). Özel bir yazılımın kullanıcı adına e-Devlet hizmetlerini çağırabileceği **genel bir API bulunamadı**.
- **"e-Devlet ile Giriş":** Kimlik doğrulama hizmetinin bir uygulamaya bağlanması ayrı bir kurum başvurusu ve protokol konusu. Yalnızca kimlik doğrular, vergi ya da SGK verisini uygulamaya vermez.
- **Yapılabilecek olan:** Kullanıcıyı ilgili hizmetin sayfasına göndermek (derin bağlantı) ve kullanıcının indirdiği barkodlu belgeyi almak. "Tek tık" isteğine en yakın, izinli yol bu.

### 2.3 SGK

- **e-Bildirge girişi:** Kullanıcı kodu, sistem şifresi ve işyeri şifresiyle yapılıyor ([Paraşüt](https://www.parasut.com/blog/sgk-e-bildirge-nedir-nasil-calisir)). Bu bilgiler işveren adına bildirim yapmaya ve borç görmeye açılır. Salt okuma ile sınırlanamaz.
- **Belgelenmiş web servisleri:** Bulunanlar işlem odaklı. Örnek: çalışmadı bildirimi (ws-vizite, [Alomaliye](https://www.alomaliye.com/2021/03/31/sgk-4-a-calisanlar-icin-calismadi-bildirim-web-servisi/)).
- **"EKAP borç sorgulama web servisi":** Kamu ihale sistemine ve bankalara yönelik kurumlar arası bir servis olarak geçiyor. Özel yazılımlara açık olduğuna dair bir kaynak **bulunamadı**.
- **Sonuç:** SGK borç durumu için resmi ve özel yazılıma açık bir okuma yolu yok.

### 2.4 Rakipler

- e-Mükellef ve benzeri ürünlerin "hızlı giriş" özelliklerinden, mükellef ya da müşavirin kamu sitesi şifrelerinin üründe saklandığı **anlaşılıyor**. Kaynak: `04-e-tebligat-takibi.md` §3.
- Bu ürünlerin resmi bir servis kullandığına dair kaynak bulunamadı.
- Bu yol teknik olarak mümkün, ama aşağıdaki sınırlarla çatışıyor.

## 3. Hukuki ve KVKK sınırları

Hukuk görüşü değildir. Karar öncesinde bir avukata sorulmalı.

| Konu | Neden önemli |
|---|---|
| **Şifre saklama ve kullanma** | DVD/İVD şifresi beyanname, borç, başvuru ve ödeme işlemlerine açılır. SGK sistem şifresi işveren adına bildirim yapmaya açılır. Salt okuma ile sınırlanamaz, sızması mükellef için ciddi zarar demek. GİB token'ı için *"tüm hukuki sorumluluk"* kullanıcıdadır. Paylaşılan şifrenin hukuki durumu daha da belirsiz. |
| **Bilişim sistemine erişim (TCK 243–244)** | Kurumun kullanım koşullarının ve yetkisinin dışında otomatik erişim ceza hukuku riski taşır. Mükellefin rızası, kurumun izni yerine geçmeyebilir. |
| **Vergi mahremiyeti (VUK 5)** | Vergi bilgisi mahremdir. Ürün bu veriyi yalnızca mükellefin kendisine ve yetkili müşavirine göstermeli. Ofisler arası görünürlük zaten yok (D1b). |
| **KVKK** | Borç, SGK ve kimlik bilgileri kişisel veri. Gerçek kişi mükelleflerde TCKN ve prim borcu da var. İşleme amacı ve hukuki sebep (sözleşmenin ifası), aydınlatma, yurt içi saklama (D4) ve saklama süresi belirlenmeli. Belge kasasındaki PDF'ler de kişisel veri. |
| **Sorumluluk sınırı** | `06` §4 ilke 7: ürün mükellef adına yükümlülük üstlenmez. Belge kasası hatırlatma yapar, belgeyi "almış" sayılmaz. |
| **e-imza / mali mühür** | Okuma için bulunmuş resmi bir yol olmadığından e-imza ya da mali mühürle otomatik giriş de değerlendirme dışı. e-Beyan API'si e-imza istemiyor (§2.1). |

## 4. Önerilen ilk dilim: resmi belge kasası (Faz 0)

Hiçbir dış sisteme bağlanmaz. Ayrı bir issue olarak açılması önerilir.

| Parça | Ayrıntı |
|---|---|
| **Belge türleri** | Vergi borç durum yazısı, mükellefiyet yazısı, SGK e-Borcu Yoktur, faaliyet belgesi (genişletilebilir liste). |
| **"Al" düğmesi** | Mükellef panelinde ilgili e-Devlet hizmetine derin bağlantı. Belgeyi kullanıcı kendi girişiyle alır. Ürün şifre görmez. |
| **Yükleme** | Mobil ve web üzerinden PDF yükleme. Belge mükellefe aittir (`taxpayer_id`, RLS taxpayer, #6). Yalnızca mükellefin aktif ofisi görür (#8 görünürlük kuralı). |
| **Doğrulama** | Barkod / doğrulama kodu alanı ve e-Devlet "Barkodlu Belge Doğrulama" sayfasına bağlantı. Otomatik doğrulama yok. |
| **Takip** | Düzenleme tarihi ve kullanım amacı tutulur. Belge yalnızca düzenlendiği günün durumunu gösterir. Müşavir istediğinde ya da ihale gibi bir amaç için yenileme hatırlatması gider (bildirim altyapısı). |
| **Müşavir akışı** | Müşavir mükelleften belge ister (doctasks / #36 görev altyapısı). Mükellef yükler, müşavir görür. |

T12'deki "tek tıkla" isteğinin karşılığı bu dilimde: tek tıkla doğru e-Devlet ekranına gitmek ve belgeyi tek yerde tutmak. Belgeyi ürünün kendisinin alması ancak §2'de resmi bir okuma yolu açılırsa mümkün.

## 5. Karar gerekiyor

1. **Şifreyle oturum açma ve ekran kazıma yapılmayacak.** Mükellefin ya da müşavirin GİB, İVD, DVD, e-Devlet veya SGK şifresini saklayıp onunla kamu sitelerine girmek ve sayfa kazımak bu PR'da önerilmiyor ve yazılmadı. Gerekçe: §3, şifrenin yetki genişliği, kurum izninin olmaması ve KVKK. Rakipler bu yolu kullanıyor olabilir. Ürün sahibi bunu yine de istiyorsa önce yazılı hukuk görüşü ve kurumdan izin alınmalı.
2. **e-Beyan entegrasyonu ürün kapsamında mı?** Resmi ve izinli bir yol, ama beyanname gönderimi için.
   - Gerekenler: şirketin tüzel kişi olarak başvurması, sabit Real IP (#45) ve meslek mensubu aracılık sözleşmeleri.
   - Ayrıca Entegratör Portalı'ndaki "API Geliştirici Kılavuzları"nda okuma ucu olup olmadığı başvuru sonrası görülebilir. Okuma ucu varsa T12'nin bir kısmı (beyanname ve tahakkuk durumu) bu yoldan karşılanabilir.
3. **GİB ve SGK'ya resmi soru:** Meslek mensubu yazılımları için borç ve tahakkuk okuma servisi var mı ya da planlanıyor mu?
   - GİB: `ebeyanentegrasyon@gelirler.gov.tr` (kılavuzdaki adres).
   - SGK: Sigorta Primleri Genel Müdürlüğü.
   - Cevaba göre Faz 1 yeniden değerlendirilir.
4. **Faz 0 belge kasası** ayrı bir issue olarak açılsın mı? Öneri: evet. Bağımlılıkları (#4 mükellef paneli, #6 RLS, #8 görünürlük, #36 belge görevleri) hazır.

## 6. Doğrulanamayanlar

- Entegratör Portalı'ndaki API uç listesi (giriş gerekiyor).
- e-Devlet "Barkodlu Belge Doğrulama" ve "Borç Durum Yazısı" hizmetlerinin güncel ekran akışı. turkiye.gov.tr ve gib.gov.tr sayfaları bu ortamdan açılamadı. `04` belgesinde de aynı durum yaşanmıştı.
- SGK'nın özel yazılımlara açık bir borç sorgulama servisi olup olmadığı.
- Rakiplerin teknik yöntemi. Yalnızca kendi tanıtım metinlerinden çıkarım yapıldı.
