# 03 · QNB eFinans ve EDM: erişim yolu araştırması (#44)

| | |
|---|---|
| Soru | Mükellefin kendi kimlik bilgisiyle okuma mümkün mü; liste uçları var mı? Easytrade'in mevcut istemcisine dayanılmaz (D13). |
| Daralan soru (go-ubltr v0.1.0 notu) | `provider/qnb` modülünde giden e-Fatura listesi ve e-Arşiv listesi, `provider/edm` modülünde e-Arşiv yok. Bu uçlar sağlayıcının API'sinde var mı, yoksa yalnızca modülde mi eksik? |
| Tarih | 2026-11-15 |
| Kaynak düzeyi | Canlı WSDL/XSD (QNB test ortamı, 2026-11-15'te indirildi; EDM'nin üç uç noktasının `?singleWsdl` kopyaları go-ubltr `provider/edm/testdata/wsdl` içinde) ve QNB'nin herkese açık API dokümanı. **Hiçbir çağrı gerçek hesapla yapılmadı.** |

## Karar

| Entegratör | Karar | Özet |
|---|---|---|
| **QNB eFinans** | **Şartlı yapılabilir** | Mükellefin kendi web servis kullanıcısıyla okuma resmi olarak destekleniyor. Gelen e-Fatura akışı dokümanda tam. Giden e-Fatura ve e-Arşiv **liste metotları API'de var** (WSDL), yalnızca herkese açık dokümanda ve go-ubltr modülünde yok. Şart: QNB test ortamında (ERP1 + Portaltest) bu iki metodun parametre ve yanıtının doğrulanması. |
| **EDM** | **Şartlı yapılabilir** | `Login` ile mükellef kullanıcısı. `GetInvoice` arama anahtarında **tarih filtresi ve sayfalama API'de var** (WSDL), go-ubltr modülü kullanmıyor. e-Arşiv için ayrı liste metodu yok; fatura başlığındaki `EARCHIVE` alanı e-Arşiv faturalarının aynı `GetInvoice` ile döndüğünü düşündürüyor, doğrulanmadı. Şart: mükellef hesabıyla canlı/test denemesi ve EDM'den e-Arşiv cevabı. |

İkisi için de **"yapılamaz" sonucu yok**: eksikler sağlayıcıda değil, go-ubltr modüllerinde ve herkese açık dokümanda. Adaptör, go-ubltr'nin ilgili modülüne liste metotları eklenerek (ya da adaptörde ince bir SOAP çağrısıyla) yazılabilir. Hangi yolun seçileceği #40'taki (W2) entegratör adaptör arayüzüne bağlıdır.

## 1. QNB eFinans

### 1.1 Kimlik ve erişim (doğrulandı)

- Kimlik: her istekte WS-Security `UsernameToken` ya da oturum çerezi (doküman, "Genel Açıklamalar").
- Kullanıcı mükellefin kendisidir: *"Webservis bağlantısı, portalde tanımlı olan bir kullanıcı kodu ve şifresinin programınıza kayıt edilmesiyle gerçekleştirilir."* Portal kullanıcısıyla aynıysa 90 günde bir şifre yenilemesi istenir. QNB bu yüzden **yalnızca programa kaydedilen, portala girmeyen bir alt kullanıcı** önerir: *Yönetim > Genel > Kullanıcı Tanımları > Yeni Kullanıcı Ekle* (doküman, "Sık Sorulan Sorular"). Bağlantı ekranında mükellefe bu adım yazılmalı.
- API anahtarı yok; kullanıcı kodu + şifre yeterli.
- `erpKodu`: *"Inhouse entegrasyonlarda kullanımı zorunlu değildir."* Bir yazılım firması olarak kaynak kodu QNB proje ekibinden istenir (bkz. §1.5).
- IP kısıtı dokümanda **yok**. Sabit çıkış IP'si (#45, W5) QNB için zorunlu görünmüyor.
- **Hız sınırı:** *"Her bir çağırdığınız web servis adresimiz, gönderici IP'si bazında 1 dakikada 300 adet istek karşılamaktadır"*, aşılınca `HTTP 429` (Retry-After yok). Sınır **IP bazında**, kullanıcı ya da VKN bazında değil: bütün mükelleflerin çağrıları aynı çıkış IP'sinden gidiyorsa 300/dk **toplam** bütçedir. Senkron motoru QNB çağrılarını WSDL başına (connector ve e-Arşiv ayrı) global bir kova ile sınırlamalı.
- Test ve canlı uç noktaları başvuru sonrası paylaşılıyor; test ortamına yalnızca başvuruda verilen VKN'ler tanımlanıyor (mükellefin VKN'si test ortamına eklenmiyor).

### 1.2 Gelen e-Fatura (doğrulandı, go-ubltr'de var)

`gelenBelgeleriListeleExt` + `gelenBelgeleriIndirExt`. Doküman ("Gelen e-Fatura Akışı", "Gelen e-Fatura Listeleme"):

- İmleç: `sonAlinanBelgeSiraNumarasi` (yanıtta `belgeSiraNo`, küçükten büyüğe, **en fazla 100 belge**). Alternatif: `faturaTarihiBaslangic/Bitis`, `gelisTarihiBaslangic/Bitis` (`yyyyMMdd`).
- **`belgelerAlindiMi` durum değiştirmez, filtredir:** *"daha öncesinde belgelerAlindi metotu ile işaretlenen belgelerin filtrelemesi sağlamaktadır. İşaretlenen belgelerin gelmesini istenirse … "true" …, gelmesini istenmezse "false" değeri vererek filtreleme yapılabilir."* Paketteki ⚠️ (`34` §3, `30` §5) bu cümleyle kapanır: parametre **gönderilmez** (her iki küme de gelsin); `belgelerAlindi` metodu ise **asla çağrılmaz** (durum değiştiren odur). go-ubltr'nin `OnlyNew` seçeneği `belgelerAlindiMi=false` gönderdiği için portal adaptöründe kullanılmamalı.
- **Yeni bulgu, `onayDurum`:** *"Boş bırakılması durumunda sadece "ONAYLANAN" faturalar listelenecektir."* Onay bekleyen (ticari) faturalar varsayılan listede **gelmez**. Portal `onayDurum=HEPSI` göndermeli; aksi halde KDV indirimi için gelen faturalar eksik kalır. go-ubltr bu parametreyi göndermiyor.
- go-ubltr notu: servis imleç ile tarih filtresini birlikte kabul etmiyor; modül tarih filtresini istemcide uyguluyor.

### 1.3 Giden e-Fatura listesi (API'de var, dokümanda ve modülde yok)

`connectorService` WSDL'inde (test, `?xsd=1`) üç liste metodu var:

| Metot | Parametre tipi | Alanlar |
|---|---|---|
| `gidenBelgeleriListele` | `gidenBelgeleriListeleParametreleri` | `vkn`, `belgeTuru`, `baslangicGonderimTarihi`, `bitisGonderimTarihi`, `baslangicBelgeTarihi`, `bitisBelgeTarihi` |
| `gidenBelgeleriListeleNew` | aynı tip | aynı |
| `gidenBelgeleriListelePortal` | `gidenBelgeleriListelePortalParametreleri` | `vergiTcKimlikNo`, `belgeTuru`, `baslangicGonderimTarihi`, `bitisGonderimTarihi`, `faturaBaslangicTarihi`, `faturaBitisTarihi`, `belgeEttnListesi`, `belgeVersiyon`, `kaynak`, `pageCount`, `subeKodu` |

Yanıt satır tipleri (XSD):

- `gidenBelgeleriListeleData`: `belgeOid`, `ettn`, `belgeTarihi`, `belgeTuru`, `yerelBelgeNo`, `alimDurumu`, `alimZamani`, `hataMesaji`, `islemYapanVkn`, `kaynak`, `kullaniciKodu`
- `gidenBelgeleriListeleDataExt`: yukarıdakiler + `invoiceID`, `profileID`, `payableAmount`, `dovizCinsi`, `receiverName`, `senderName`, `gonderimDurumu`, `gonderimCevabiKodu`, `gonderimCevabiDetayi`
- `gidenBelgeleriListelePortalData`: `ettn`, `belgeNo`, `belgeTarihi`, `alanVergiTcKimlikNo`, `receiverName`, `tutar`, `currency`, `durum`, `gibDurumu`, `yanitDurumu`, `onayDurumu`, `gonderimZamani` …

Sonuç: paketteki ⚠️ "OID listede geliyor mu" sorusunun cevabı **evet** (`belgeOid` satırda). İçerik `gidenBelgeleriIndir` (OID ile) veya `gidenBelgeleriIndirEttn` (ETTN ile, WSDL'de var) ile indirilir. `Portal` varyantı tutar ve durumu da verdiği için liste + durum için tek çağrı olabilir. Doğrulanacaklar: tarih aralığı üst sınırı, `pageCount` anlamı, `New` ile eskisi arasındaki fark, hangi yanıt tipinin döndüğü.

### 1.4 e-Arşiv listesi (API'de var, dokümanda ve modülde yok)

`EarsivWebService` WSDL'inde (test, `?xsd=1`):

| Metot | Girdi | Yanıt |
|---|---|---|
| `faturaListeSorgula` | `input` (string; dokümandaki diğer e-Arşiv metotları gibi JSON) | `earsivServiceResult` + `output` (`faturaDTO` listesi) |
| `faturaListeSorgulaPortal` | `input` (string) | — |
| `faturaListeSorgulaCount` | `input` (string) | — |
| `faturaZipiAl`, `raporBolumListesiSorgula(donem)`, `rapordaFaturaSorgula` | | e-Arşiv rapor tarafı |

`faturaDTO` alanları: `uuid`, `invoiceId`, `faturaZamani`, `issueDate`, `profileId`, `invoiceTypeCode`, `receiverIdentifier`, `receiverName`, `payableAmount`, `toplamTutar`, `vergilerToplami`, `currency`, `durum`, `iptalTarihi`, `raporDurum`, `raporTarihi`, `kagitFatura`, `yerelFaturaNo`, `sube`, `kasa` … Liste e-Arşiv için gerekenleri (ETTN, numara, tarih, alıcı, tutar, iptal) taşıyor. UBL tek tek `faturaSorgulaExt` (`faturaUuid`, `donenBelgeFormati=0`) ile alınır (doküman, "e-Arşiv Fatura Sorgula").

Doğrulanacak: `input` JSON'unun alan adları (tarih aralığı, sayfalama). Herkese açık dokümanda yalnızca örnek kodda yorum satırı olarak geçiyor (`//FaturaListeSorgula(methods);`); tanımı yok.

e-Arşiv kullanıcısı e-Fatura kullanıcısından **ayrı** olabilir (QNB'de e-Arşiv "Portaltest" ürün ailesinde; go-ubltr `EArchiveUsername`). Bağlantı formunda iki kullanıcı alanı olmalı (ikincisi boşsa ilki kullanılır).

### 1.5 QNB'ye sorulacaklar (açık kalanlar)

- [ ] `faturaListeSorgula` `input` JSON şeması, tarih aralığı ve sayfa sınırı
- [ ] `gidenBelgeleriListele` / `New` / `Portal` farkı, tarih aralığı üst sınırı, `pageCount`
- [ ] Bir yazılım firmasının çok sayıda mükellef adına çağrı yapması için `erpKodu` (kaynak kodu) alınması gerekiyor mu; rate limit yükseltme başvurusu (doküman yükseltmenin günlük belge sayısına göre değerlendirildiğini söylüyor)
- [ ] Gelen faturanın kabul/red durumu: `gelenBelgeDurumSorgulaExt` (WSDL'de var, `gelenBelgeDurum`: `yanitDurumu`, `yanitDetayi`, `alimTarihi` …) okuma için yeterli mi

## 2. EDM

### 2.1 Kimlik (WSDL'den, doküman yok)

- `Login(USER_NAME, PASSWORD)` → `SESSION_ID`; sonraki her istekte `REQUEST_HEADER/SESSION_ID`. Mükellefin kendi EDM kullanıcısı.
- IP kısıtına dair herkese açık bilgi yok.
- **`MaliMusavirLogin`** (`USER_NAME`, `PASSWORD`, `COMPANYID`), **`MaliMusavirInfo`** (`USERNAME`, `PASS`, `MM_VKN`, `MM_GIBCODE`, `MM_GIBPASS`) ve **`MaliMusavirList`** (`COMPANYID`, `COMPANYNAME`, `VKN`, `GIBALIAS`, sipariş alanları) üç WSDL'de de (`portal2`, `interaktif`, `test`; `List` ve `Info` yalnızca `portal2` ve `test`'te) var. Adlarından, **müşavirin kendi kullanıcısıyla mükellef şirketlerine (`COMPANYID`) girebildiği** bir yol olduğu anlaşılıyor. Doğrulanırsa mükellefin şifre vermesi gerekmeyebilir. Hiçbir herkese açık kaynakta açıklaması bulunamadı (rebasesoftware/edm-efatura yalnızca `Login`, `SendInvoice`, `GetInvoice` kullanıyor). **EDM'ye sorulacak ilk soru budur.**

### 2.2 Liste: `GetInvoice` arama anahtarı (WSDL, `portal2`)

`INVOICE_SEARCH_KEY` alanları: `LIMIT`, `OFFSET`, `ID`, `UUID`, `FROM`, `TO`, `START_DATE`, `END_DATE` (`xs:date`), `CR_START_DATE`, `CR_END_DATE` (`xs:dateTime`), `READ_INCLUDED`, `DIRECTION`, `SENDER`, `RECEIVER`, `ISARCHIVED`, `CONNECTORSTATUSDESCRIPTION`, `ERP_STATUS`, `PAYABLE_AMOUNT`, `LASTPAYMENT_START_DATE/END_DATE`, `SUBSCRIBER`, `ERP_REFERENCE`, `ISOBJECTION`, `ISACTIVE`, `DRAFTCHOICE`, `OZELALAN`, `USERID`, `GROUPCODE_ID`, `HEADER_ONLY`, `INVOICE_CONTENT_TYPE`, `GENERATE_ENTITY`.

Sonuç: tarih penceresi (`START_DATE`/`END_DATE` fatura tarihi, `CR_*` oluşturma zamanı) ve sayfalama (`LIMIT` + `OFFSET`) **API'de var**. go-ubltr modülü bunları "davranışı belgelenmemiş" diye bilerek dışarıda bırakmış; liste yalnızca `OnlyNew` + `MarkInvoice` ile çalışıyor, bu da okurken durum değiştirir (`30` §5'e aykırı). Portal adaptörü:

- `DIRECTION=IN` ve `OUT` ayrı çağrılar, `HEADER_ONLY=Y`, `READ_INCLUDED=true`, `START_DATE`/`END_DATE` ile pencere, `LIMIT`/`OFFSET` ile sayfa.
- `ISARCHIVED`: EDM'de arşiv **klasörü** (`ArchiveInvoice` `ARCHIVE`/`DEARCHIVE` ile taşınan), e-Arşiv değil. Arşive kaldırılmış faturaları kaçırmamak için `ISARCHIVED` hem `true` hem `false` ile çağrılmalı (İşNet'teki `IsArchiveIncluded` gibi), doğrulanacak.
- `MarkInvoice`, `ArchiveInvoice`, `SetInvoiceStatus`, `SendInvoiceResponse*` **çağrılmaz**.

### 2.3 e-Arşiv

Ayrı bir e-Arşiv liste metodu yok. Ancak `INVOICE/HEADER` tipinde `EARCHIVE` (`xs:boolean`) ve `INTERNETSALES` alanları var. go-ubltr notu e-Arşiv'in `SendInvoice` ile (`EARSIVFATURA` profili) gönderildiğini söylüyor. Bu, e-Arşiv faturalarının da `GetInvoice DIRECTION=OUT` ile, başlıkta `EARCHIVE=true` olarak döndüğünü düşündürüyor. **Doğrulanmadı**; mükellef hesabıyla tek çağrıda görülebilir. e-Arşiv iptal durumu için `CancelInvoice` / `DeCanceArchivelInvoice` (yazım WSDL'deki gibi) var; iptalin listede nasıl göründüğü de aynı denemede bakılmalı. `GetReportStatus(UUID)` e-Arşiv raporu için.

### 2.4 Uç noktalar

| | Adres |
|---|---|
| Test | `https://test.edmbilisim.com.tr/EFaturaEDM21ea/EFaturaEDM.svc` |
| Canlı (portal2) | `https://portal2.edmbilisim.com.tr/EFaturaEDM/EFaturaEDM.svc` |
| Canlı (interaktif) | `https://interaktif.edmbilisim.com.tr/EFaturaEDM/EFaturaEDM.svc` (operasyon kümesi daha dar) |

Mükellefin hangi canlı adresi kullandığı sözleşmesine bağlı; bağlantı formunda seçim (varsayılan `portal2`) olmalı.

### 2.5 EDM'ye sorulacaklar

- [ ] `MaliMusavirLogin` / `MaliMusavirList`: müşavir kendi kullanıcısıyla mükellef şirketlerine okuma erişimi alabilir mi; `COMPANYID` nereden gelir; mükellef onayı nasıl verilir
- [ ] `GetInvoice` `START_DATE`/`END_DATE` ve `LIMIT`/`OFFSET`: üst sınırlar, sıralama
- [ ] e-Arşiv faturaları `GetInvoice` ile mi okunur (`EARCHIVE` alanı); iptal durumu
- [ ] `ISARCHIVED` varsayılanı (arşivdekiler gelir mi)
- [ ] Oturum süresi, hız sınırı, IP kısıtı

## 3. Ne yapılacak

| Sıra | İş | Not |
|---|---|---|
| 1 | QNB'ye §1.5, EDM'ye §2.5 soruları | Ürün sahibi / iş ortaklığı kanalından. Easytrade'in kanalına dayanılmaz (D13). |
| 2 | QNB test başvurusu (ERP1 + Portaltest) | Test ortamı yalnızca başvuruda verilen VKN'lerle çalışıyor. |
| 3 | go-ubltr'ye liste metotları | QNB: `ListOutbox` (`gidenBelgeleriListele*`), e-Arşiv `List` (`faturaListeSorgula`), `onayDurum=HEPSI`. EDM: tarih/sayfa alanları, e-Arşiv ayrımı. go-ubltr ayrı repo; değişiklik orada yapılır. |
| 4 | Adaptörler | #40'taki arayüze göre (W2). Okuma kuralları: QNB'de `belgelerAlindi` ve `OnlyNew` yok; EDM'de `MarkInvoice` yok, `READ_INCLUDED=true`. |
| 5 | Hız sınırı | QNB: WSDL başına global 300/dk kova (IP bazında, bütün mükellefler ortak). |

## Kaynaklar

- QNB eSolutions API dokümantasyonu, https://www.qnbesolutions.com.tr/api-entegrasyon.html ("Genel Açıklamalar", "Rate Limit Düzenlemeleri", "Sık Sorulan Sorular", "Gelen e-Fatura Akışı", "Gelen e-Fatura Listeleme", "e-Arşiv Fatura Sorgula"). Sayfa içeriği gömülü JSON olarak geliyor; herkese açık dokümanda giden e-Fatura listesi ve e-Arşiv listesi başlığı yok.
- QNB test WSDL/XSD: `https://erpefaturatest1.qnbesolutions.com.tr/efatura/ws/connectorService?wsdl` (`?xsd=1`), `https://earsivtest.qnbesolutions.com.tr/earsiv/ws/EarsivWebService?xsd=1`.
- EDM WSDL: go-ubltr `provider/edm/testdata/wsdl/EFaturaEDM-{test,portal2,interaktif}.wsdl` (üç adresin `?singleWsdl` kopyaları).
- go-ubltr v0.1.0 `provider/qnb/README.md`, `provider/edm/README.md`.
- Paket: `docs/kaynak/musavir-entegrasyon-paketi/30-entegrator-genel.md`, `34-qnb-efinans.md`, `35-edm.md`.
