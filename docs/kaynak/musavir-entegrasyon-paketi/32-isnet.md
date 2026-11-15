# 32 · İşNet (NetteFatura)

| | |
|---|---|
| Öncelik | **1** (müşavirin mükelleflerinde yaklaşık %80) |
| Resmi kaynak | ✅ İşNet "E-Fatura", "E-Arşiv", "AddressBook" ve "Company" Web Servis Arayüz Dokümanları (E-Fatura ve E-Arşiv V2 12.08.2024, AddressBook V1 10.01.2023, Company V1 16.12.2024) + İşNet'in örnek SOAP istekleri. İşNet sitesinde herkese açık değiller; bir topluluk SDK deposunda (`EfeSorogluu/NetteFatura-API`, `docs/official-docs`, `docs/request-samples`) yayımlanmış kopyalardan okundu. |
| Uygulama kaynağı | ⚠️ İki topluluk entegrasyonu: `EfeSorogluu/NetteFatura-API` (adresler, SOAPAction) ve `mbrksntrk/dolibarr-isnet-nettefatura` (2026; gelen fatura çeken çalışan bir modül). Dokümanda olmayan ama bu kodda görülen davranışlar ayrıca işaretlendi. |
| Protokol | SOAP 1.1 (WCF) |
| Kimlik doğrulama | İstekte kullanıcı/şifre alanı **yok.** Erişim: İşNet'in tanımladığı **sabit IP** + istekteki firma VKN'si (`CompanyTaxCode`). |
| Canlı test | Yapılmadı |

Önce `37` (ortak altyapı) okunmalı.

## 0. Başlamadan önce bilinmesi gereken üç şey

1. **Şifre yok.** Yetki, Easytrade'in çıkış IP'sinin İşNet'te tanımlı olmasına ve o IP'nin hangi VKN'ler için yetkili olduğuna bağlı. Bu yetkinin Easytrade için nasıl verileceği belli değil (§2). Kod test ortamına göre yazılır, canlıya geçiş İşNet görüşmesine bağlı.
2. **Tanımsız IP hata mesajı almaz, cevapsız kalır.** Topluluk modülünün notu: "İşNet çağıranları ağ seviyesinde IP ile süzüyor; bilinmeyen IP'ye TCP cevabı hiç gelmiyor." Yani IP sorunu `ip_rejected` mesajı olarak değil **bağlantı zaman aşımı** olarak görünür (§9).
3. **Etiketler alfabetik sırayla gönderilir.** İşNet dokümanı: "Taglar alfabetik sıralamaya göre gönderilmelidir. Aksi durumda sıralamaya uymayan tag içinde göndermiş olduğunuz veri okunamayacaktır." Aşağıdaki bütün şablonlar bu sırada.

## 1. Adresler ve SOAPAction

| Servis | Test | Canlı |
|---|---|---|
| InvoiceService (e-fatura ve e-arşiv) | `https://einvoiceservicetest.isnet.net.tr/InvoiceService/ServiceContract/InvoiceService.svc` | `https://einvoiceservice.isnet.net.tr/InvoiceService/ServiceContract/InvoiceService.svc` |
| AddressBookService | `https://einvoiceservicetest.isnet.net.tr/AddressBookService/ServiceContract/AddressBookService.svc` | `https://einvoiceservice.isnet.net.tr/AddressBookService/ServiceContract/AddressBookService.svc` |

Adresler dokümanda yok; iki topluluk entegrasyonunda aynı ⚠️. WSDL: adres + `?singleWsdl` (veya `?wsdl`).

**WSDL'deki port adresi kullanılmaz.** Topluluk modülü: "WSDL önce bir http portu listeliyor; https uç noktasını zorla." İstek her zaman yukarıdaki `https://` adrese gönderilir.

| Başlık | Değer |
|---|---|
| `Content-Type` | `text/xml; charset=utf-8` |
| `SOAPAction` (InvoiceService) | `"http://tempuri.org/IInvoiceService/{Metot}"` |
| `SOAPAction` (AddressBookService) | `"http://tempuri.org/IAddressBookService/{Metot}"` |

SOAPAction değerleri topluluk SDK'sından ⚠️; WSDL'deki `soapAction` ile karşılaştırılır.

**Ad alanları:**

| Önek | URI |
|---|---|
| `soapenv` | `http://schemas.xmlsoap.org/soap/envelope/` |
| `tem` | `http://tempuri.org/` |
| `ein` | `http://schemas.datacontract.org/2004/07/EInvoice.Service.Model` |
| `arr` | `http://schemas.microsoft.com/2003/10/Serialization/Arrays` (string listeleri için) |

Her istek gövdesi `tem:{Metot}` > `tem:request` > `ein:` alanları biçimindedir.

## 2. Erişim modeli (en kritik açık iş)

İşNet dokümanı (bütün servis dokümanlarında aynı not):

> "Canlı ortam IP tanımlarınızın yapılabilmesi için; test ortamlarımız ile kurduğunuz entegrasyonlar üzerinden gönderdiğiniz requestlere istinaden oluşacak faturaların … 'Başarıyla Tamamlandı' durumunda olması gerekmektedir. Bu aşamayı başarılı olarak geçtikten sonra canlı ortam tanımınız yapılabilecektir."

Bu süreç **fatura gönderen** entegrasyon için yazılmış. Easytrade sadece okuyacak. İşNet'e sorulacaklar:

1. Sadece okuma yapan bir yazılım firması için canlıya geçiş şartı ne (test faturası gönderme şartı aranıyor mu)?
2. Easytrade'in IP'si tanımlandıktan sonra **hangi VKN'ler adına** sorgu yapabilir? Her mükellef için ayrı onay mı gerekiyor; mükellef bunu nasıl veriyor (İşNet portalı, imzalı form, e-posta)?
3. `CompanyVendorNumber` (dokümanda "bayi numarası", topluluk modülünde "firmanın birimi") bir çözüm ortağı modeline mi işaret ediyor? Easytrade bu modele girebilir mi?
4. TÜRMOB–İşNet işbirliği kapsamında mali müşavir yazılımları için ayrı bir erişim yolu var mı?
5. Hız sınırı ve eşzamanlı çağrı sınırı.

Canlı erişim için topluluk SDK'sının kılavuzunda geçen İşNet adresleri: `efaturadestek@nettefatura.com.tr`, `servisyazilim@is.net.tr`.

**Easytrade tarafındaki adımlar:**

| # | Adım | Sorumlu |
|---|---|---|
| 1 | İşNet ile görüşme, yukarıdaki 5 sorunun cevabı | Easytrade iş geliştirme |
| 2 | Test ortamı erişimi ve test IP tanımı (`fixed-egress` IP'leri, `20` §4) | Easytrade backend + İşNet |
| 3 | Adaptör test ortamında (§12) | Backend |
| 4 | Canlı IP tanımı | İşNet |
| 5 | Her mükellef için yetki (İşNet'in vereceği yönteme göre) | Mükellef + İşNet |
| 6 | Bağlantı `active` | Portal |

Portalda bağlantı durumu: `authorization_requested` (mükellef yetki sürecini başlattı) → `verifying` → `active` / `failed` (`30` §4.3).

**Güvenlik kuralı:** Kimlik doğrulama sadece IP + VKN olduğu için Easytrade'in IP'sinden çıkan her istek, yetkili olunan **her** VKN'nin verisine erişebilir.

- `CompanyTaxCode` her zaman `source_connection`'daki doğrulanmış VKN'den alınır. API isteğinden, form alanından, kuyruk mesajındaki serbest metinden alınmaz.
- İstemci fonksiyonu VKN'yi parametre olarak değil `connection` nesnesinden okur.
- Her çağrı `(connection_id, VKN, metot)` ile denetim loguna yazılır.

## 3. Yazım kuralları

| Konu | Kural |
|---|---|
| Sıra | Her seviyede alan adları alfabetik (§0). Büyük/küçük harf: İşNet örneklerindeki gibi. |
| Tarih | `YYYY-MM-DDTHH:mm:ss`, saat dilimi yok (ör. `2026-11-01T00:00:00`). Topluluk modülü de böyle gönderiyor. İstanbul saati kabul edilir. |
| Bitiş tarihi | `Max…Date` için gün sonu: `2026-11-30T23:59:59` |
| Mantıksal | `true` / `false` |
| Enum | Ad olarak: `Incoming`, `Outgoing`, `EInvoice`, `EArchiveInvoice` (İşNet örnekleri böyle) |
| Boş alan | Gönderilmez (eleman hiç yazılmaz) |

## 4. Kurulum kontrolü: `GetTaxPayer` (AddressBookService)

Amaç: mükellef e-fatura kullanıcısı mı? Değilse gelen e-fatura olmaz, sadece giden e-arşiv çekilir.

```xml
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"
                  xmlns:tem="http://tempuri.org/"
                  xmlns:ein="http://schemas.datacontract.org/2004/07/EInvoice.Service.Model">
  <soapenv:Header/>
  <soapenv:Body>
    <tem:GetTaxPayer>
      <tem:request>
        <ein:TaxPayerTaxCode>{VKN}</ein:TaxPayerTaxCode>
      </tem:request>
    </tem:GetTaxPayer>
  </soapenv:Body>
</soapenv:Envelope>
```

**Uyarı (dokümandan):** "Bu alanlar boş gönderilirse tüm kayıtlı mükellefler listelenir." VKN boşsa çağrı **yapılmaz.**

Yanıt (dokümandan): `Result`, `ErrorMessage`, `TaxPayers[]` → `TaxPayer`: `Taxpayertaxcode`, `Taxpayername`, `Type`, `Registrationdate`, `Inboxtaglist[]`, `Outboxtaglist[]`.

| Sonuç | Karar |
|---|---|
| `TaxPayers` dolu, `Inboxtaglist` dolu | E-fatura kullanıcısı: gelen + giden e-fatura + giden e-arşiv |
| `Result=Failed`, mesajda "bulunamad" | E-fatura kullanıcısı değil: sadece giden e-arşiv. Topluluk modülü bu durumu böyle ele alıyor ("kayıtlı değil" İşNet'te `Result=Failed` olarak dönüyor) ⚠️ |
| Diğer `Failed` | `provider_error` (`37` §4) |

Not: Yanıttaki alan adları dokümanda bu yazımla (`Taxpayertaxcode`) geçiyor; gerçek yanıtta büyük/küçük harf farklı olabilir. Ayrıştırıcı bu alanlarda **büyük/küçük harf duyarsız** okur.

## 5. E-fatura listesi: `SearchAllInvoice`

### 5.1 İstek şablonu

Portalın kullandığı alanlar, alfabetik:

```xml
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"
                  xmlns:tem="http://tempuri.org/"
                  xmlns:ein="http://schemas.datacontract.org/2004/07/EInvoice.Service.Model"
                  xmlns:arr="http://schemas.microsoft.com/2003/10/Serialization/Arrays">
  <soapenv:Header/>
  <soapenv:Body>
    <tem:SearchAllInvoice>
      <tem:request>
        <ein:CompanyTaxCode>{VKN}</ein:CompanyTaxCode>
        <!-- sadece tek fatura sorgusunda: <ein:Ettn>{ETTN}</ein:Ettn> -->
        <!-- birim varsa (§5.5): <ein:IncludeCompanyVendorNumberList><arr:string>{BIRIM}</arr:string></ein:IncludeCompanyVendorNumberList> -->
        <ein:IncludeMainCompany>true</ein:IncludeMainCompany>
        <ein:InvoiceDirection>{Incoming|Outgoing}</ein:InvoiceDirection>
        <!-- pencere: ya Creation ya Invoice tarihi çifti -->
        <ein:MaxInvoiceCreationDate>{bitiş}</ein:MaxInvoiceCreationDate>
        <ein:MinInvoiceCreationDate>{başlangıç}</ein:MinInvoiceCreationDate>
        <!-- veya -->
        <!-- <ein:MaxInvoiceDate>{bitiş}</ein:MaxInvoiceDate> -->
        <!-- <ein:MinInvoiceDate>{başlangıç}</ein:MinInvoiceDate> -->
        <ein:PagingRequest>
          <ein:PageNumber>{1..}</ein:PageNumber>
          <ein:RecordsPerPage>{sayfa boyutu}</ein:RecordsPerPage>
        </ein:PagingRequest>
        <ein:ResultSet>
          <ein:IsAdditionalTaxIncluded>false</ein:IsAdditionalTaxIncluded>
          <ein:IsArchiveIncluded>true</ein:IsArchiveIncluded>
          <ein:IsAttachmentIncluded>false</ein:IsAttachmentIncluded>
          <ein:IsExternalUrlIncluded>false</ein:IsExternalUrlIncluded>
          <ein:IsHtmlIncluded>false</ein:IsHtmlIncluded>
          <ein:IsInvoiceDetailIncluded>false</ein:IsInvoiceDetailIncluded>
          <ein:IsPDFIncluded>false</ein:IsPDFIncluded>
          <ein:IsXMLIncluded>true</ein:IsXMLIncluded>
        </ein:ResultSet>
      </tem:request>
    </tem:SearchAllInvoice>
  </soapenv:Body>
</soapenv:Envelope>
```

Sıra kontrolü: `CompanyTaxCode` < `Ettn` < `IncludeCompanyVendorNumberList` < `IncludeMainCompany` < `InvoiceDirection` < `MaxInvoiceCreationDate` < `MaxInvoiceDate` < `MinInvoiceCreationDate` < `MinInvoiceDate` < `PagingRequest` < `ResultSet`.

### 5.2 Alanlar

| Alan | Doküman | Portal değeri | Not |
|---|---|---|---|
| `CompanyTaxCode` | Zorunlu | Bağlantının VKN'si | §2 güvenlik kuralı |
| `InvoiceDirection` | Zorunlu | `Incoming` / `Outgoing` | |
| `IncludeMainCompany` | Zorunlu | `true` | Ana firma dahil |
| `PagingRequest` | Zorunlu | `PageNumber` 1'den başlar | İşNet örneği `1` ile başlıyor |
| `ResultSet` | Zorunlu | Yukarıdaki gibi | `IsArchiveIncluded=true`: mükellefin İşNet portalında arşive kaldırdığı faturalar da gelsin |
| `MinInvoiceCreationDate`, `MaxInvoiceCreationDate` | Opsiyonel | Artımlı çekim penceresi | "Fatura yaratılma tarihi" |
| `MinInvoiceDate`, `MaxInvoiceDate` | Opsiyonel | Dönem taraması | Fatura tarihi |
| `Ettn` | Opsiyonel | Tek fatura / durum güncelleme | |
| `IncludeCompanyVendorNumberList` | Opsiyonel | Birim listesi (§5.5) | |
| `MinDirectionDate`, `MaxDirectionDate` | Opsiyonel | **Kullanılmaz** | Dokümanda hem `string` hem `DateTime` olarak iki kez geçiyor; tip belirsiz |

### 5.3 Sayfa boyutu ve XML

- `IsXMLIncluded=true` ile her faturanın imzalı UBL'i (base64) yanıtta gelir. Ayrı indirme gerekmez.
- Başlangıç sayfa boyutu: **20** (öneri). Topluluk modülü 50 kullanıyor ama XML'siz. Yanıt 50 MB'ı geçerse `37` §2 kuralıyla yarıya indirilir.
- Pencere bölme eşiği (`37` §5.3): `TotalNumberOfRecords > 1000` ise pencere ikiye bölünür (öneri).

### 5.4 Yanıt

Doküman: `Result` (`Success` / `Failed`), `ErrorMessage`, `Invoices` (`List<Invoice>`), `PagingResponse` (`PageNumber`, `RecordsPerPage`, `TotalNumberOfPages`, `TotalNumberOfRecords`).

Beklenen biçim (WCF kuralı ve topluluk modülünün okuduğu yapı; ilk gerçek yanıtla doğrulanır ⚠️):

```xml
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body>
    <SearchAllInvoiceResponse xmlns="http://tempuri.org/">
      <SearchAllInvoiceResult xmlns:a="http://schemas.datacontract.org/2004/07/EInvoice.Service.Model"
                              xmlns:i="http://www.w3.org/2001/XMLSchema-instance">
        <a:ErrorMessage i:nil="true"/>
        <a:Invoices>
          <a:Invoice>
            <!-- fatura alanları, §5.6 -->
          </a:Invoice>
        </a:Invoices>
        <a:PagingResponse>
          <a:PageNumber>1</a:PageNumber>
          <a:RecordsPerPage>20</a:RecordsPerPage>
          <a:TotalNumberOfPages>3</a:TotalNumberOfPages>
          <a:TotalNumberOfRecords>57</a:TotalNumberOfRecords>
        </a:PagingResponse>
        <a:Result>Success</a:Result>
      </SearchAllInvoiceResult>
    </SearchAllInvoiceResponse>
  </s:Body>
</s:Envelope>
```

Sayfalama: `PageNumber` 1'den `TotalNumberOfPages`'e kadar. `TotalNumberOfPages` gelmezse dönen kayıt sayısı `RecordsPerPage`'den az olana kadar.

### 5.5 Birimler (`CompanyVendorNumber`)

Topluluk modülü, firmanın faturalarının İşNet'te **birimlere** (`CompanyVendorNumber`) ayrılabildiğini ve birimleri `GetCompanyVendor` ile listelediğini gösteriyor (Company dokümanında: "Firmanın sistemdeki sahip olduğu alt bayilerinin bilgisi"). Birime atanmış faturaların `IncludeMainCompany=true` ile gelip gelmediği belli değil ⚠️.

Güvenli yol:

1. Bağlantı doğrulamasında `GetCompanyVendor` çağrılır (topluluk modülü bunu InvoiceService üzerinden çağırıyor ⚠️; Company dokümanında servis adresi yok).
2. Birim varsa liste çağrısına `IncludeCompanyVendorNumberList` ile hepsi eklenir.
3. Test ortamında birim atanmış bir faturayla `IncludeMainCompany=true` tek başına denenir; sonuç bu dosyaya yazılır.

```xml
<tem:GetCompanyVendor>
  <tem:request>
    <ein:CompanyTaxCode>{VKN}</ein:CompanyTaxCode>
  </tem:request>
</tem:GetCompanyVendor>
```

Yanıt: `Result`, `ErrorMessage`, `Vendors` (`List<CompanyVendor>`; alanları dokümanda "Vendors" tablosunda).

### 5.6 `Invoice` öğesi → `EntegratorDocHeader`

Alan adları dokümandan. **Büyük/küçük harf:** doküman `Ettn` yazıyor, topluluk modülü yanıttan `ETTN` okuyor. Ayrıştırıcı bu öğede alan adlarını büyük/küçük harf duyarsız eşler.

| İşNet | Portal | Not |
|---|---|---|
| `Ettn` / `ETTN` | `uuid` | Küçük harfe çevrilir |
| `InvoiceNumber` | `invoiceNumber` | |
| `InvoiceDate` | `issueDate` | Sadece kontrol; esas tarih UBL'den |
| `InvoiceCreationDate` | `createdAt` | Topluluk modülü gelen faturada bunu "alınma tarihi" olarak kullanıyor |
| `ScenarioType` | `profileId` (ham) | Değerler: `TemelFatura`, `TicariFatura`, `Ihracat`, `YolcuBeraberFatura`, `HKS`, `Kamu` |
| `InvoiceType` | `invoiceTypeCode` (ham) | `Satis`, `Iade`, `Istisna`, `Tevkifat`, `OzelMatrah`, `IhracKayitli`, `Sgk`, `Komisyoncu`, `TevkifatIade` |
| `Status` | `statusRaw` | §8 |
| `DetailStatus` | `raw` | Zarf durumu, §8 |
| `ApplicationResponseStatus` | `raw` | Uygulama yanıtı gönderim durumu |
| `Receiver` (`AddressBookEntry`) | `counterparty*` (sadece liste ekranı) | Topluluk modülü gelen faturada karşı tarafı (gönderen) buradan okuyor (`ReceiverTaxCode`, `ReceiverName`) ⚠️. Ortak modele UBL'deki taraf yazılır. |
| `TotalPayableAmount`, `CurrencyCode` | `payableAmount`, `currency` | Sadece liste ve kontrol |
| `CompanyVendorNumber` | `raw` | Birim |
| `IsArchived`, `ArchiveDate`, `ApprovalDate`, `EnvelopeId` | `raw` | |
| `InvoiceXml` | UBL (base64) | `37` §3.4 → `31` |

`docKind = 'e_invoice'`, `direction` istekteki yönden.

## 6. E-arşiv listesi: `SearchArchiveInvoice`

Giden e-arşiv faturaları. Aynı InvoiceService adresi (iki topluluk entegrasyonunda da böyle).

### 6.1 İstek şablonu

```xml
<tem:SearchArchiveInvoice>
  <tem:request>
    <ein:CompanyTaxCode>{VKN}</ein:CompanyTaxCode>
    <!-- iptal taraması (§6.3): <ein:IsCancelled>true</ein:IsCancelled> -->
    <ein:MaxInvoiceCreationDate>{bitiş}</ein:MaxInvoiceCreationDate>
    <ein:MinInvoiceCreationDate>{başlangıç}</ein:MinInvoiceCreationDate>
    <!-- veya Max/MinInvoiceDate -->
    <ein:PagingRequest>
      <ein:PageNumber>{1..}</ein:PageNumber>
      <ein:RecordsPerPage>{sayfa boyutu}</ein:RecordsPerPage>
    </ein:PagingRequest>
    <ein:ResultSet>
      <ein:IsAdditionalTaxIncluded>false</ein:IsAdditionalTaxIncluded>
      <ein:IsArchiveIncluded>true</ein:IsArchiveIncluded>
      <ein:IsAttachmentIncluded>false</ein:IsAttachmentIncluded>
      <ein:IsExternalUrlIncluded>false</ein:IsExternalUrlIncluded>
      <ein:IsHtmlIncluded>false</ein:IsHtmlIncluded>
      <ein:IsInvoiceDetailIncluded>false</ein:IsInvoiceDetailIncluded>
      <ein:IsPDFIncluded>false</ein:IsPDFIncluded>
      <ein:IsXMLIncluded>true</ein:IsXMLIncluded>
    </ein:ResultSet>
  </tem:request>
</tem:SearchArchiveInvoice>
```

**Alan adı farkı:** Doküman tablosunda sonuç alanı `InvoiceResultSet`, İşNet'in örnek isteğinde ve topluluk modülünde `ResultSet`. Örnek istek ve çalışan kod `ResultSet` kullandığı için o esas alınır; WSDL ile teyit edilir. (`InvoiceResultSet` olsaydı alfabetik sırada `IsCancelled`'dan önce gelirdi.)

### 6.2 Yanıt

Doküman: `Result`, `ErrorMessage`, `ArchiveInvoices` (`List<ArchiveInvoice>`), `PagingResponse`. Öğe adı `ArchiveInvoice` (topluluk modülü böyle okuyor).

**Dokümandaki `ArchiveInvoice` tablosu gönderim için yazılmış**; ETTN, fatura numarası ve durum alanları tabloda yok. Bu yüzden:

- `uuid` ve `invoiceNumber` **UBL'den** alınır (`cbc:UUID`, `cbc:ID`). Liste öğesinde `ETTN` / `InvoiceNumber` varsa kontrol için karşılaştırılır.
- Durum alanının adı ve değerleri ilk gerçek yanıttan belirlenir ⚠️. Durum listeleri §8.2.

### 6.3 İptal edilen e-arşivler

İstek alanı `IsCancelled` ("Arşiv fatura iptal mi?") dokümanda var. Yanıttaki iptal göstergesi belirsiz olduğu için iptaller **ayrı sorguyla** bulunur:

1. Normal tarama: `IsCancelled` gönderilmez → penceredeki bütün e-arşivler.
2. İptal taraması: aynı pencere, `IsCancelled=true` → dönen her ETTN `cancelled`.

İptal taraması artımlı çekimde de yapılır (iptal, fatura oluşturulduktan sonra olur; pencere olarak **son 60 gün** fatura tarihi kullanılır).

## 7. Tek belge bağlantısı: `GetDocumentViewerLink` (yedek)

UBL liste yanıtında gelmezse kullanılır.

```xml
<tem:GetDocumentViewerLink>
  <tem:request>
    <ein:CompanyTaxCode>{VKN}</ein:CompanyTaxCode>
    <ein:Ettn>{ETTN}</ein:Ettn>
    <ein:InvoiceDirection>{Incoming|Outgoing}</ein:InvoiceDirection>
    <ein:InvoiceDocumentType>{EInvoice|EArchiveInvoice}</ein:InvoiceDocumentType>
  </tem:request>
</tem:GetDocumentViewerLink>
```

Yanıt: `Result`, `ErrorMessage`, `DocumentViewerLink` ("ilgili belgeye ait Pdf ya da XML indirilmesini sağlayan" adres).

Topluluk modülünün notu: servis bağlantıyı `http://host:80/…` olarak veriyor, görüntüleyici sadece `https` ile cevap veriyor; modül adresi `https://host/…` olarak düzeltiyor ⚠️. Bağlantıdan XML mi PDF mi indiği ilk testte belirlenir.

## 8. Durumlar

### 8.1 E-fatura `Status` (`InvoiceStatus`, dokümandan)

| Ad | Kod | `status_normalized` |
|---|---|---|
| `Onay_Bekliyor` | 0 | `active` + `PENDING_RESPONSE` |
| `Onaylandi` | 1 | `active` |
| `Reddedildi` | 2 | `cancelled` + `REJECTED` |
| `Onay_Akisinda` | 3 | `active` + `PENDING_RESPONSE` |
| `Iade_Edildi` | 4 | `cancelled` + `RETURNED` ⚠️ |
| `Gonderildi` | 8 | `active` ⚠️ |
| `Ziplendi` | 12 | `draft` |
| `Gibe_Iletildi` | 13 | `active` ⚠️ |
| `Imza_Bekliyor` | 15 | `draft` |
| `Gib_Tarafinda_Hata_Olustu` | 16 | `draft` + `DELIVERY_ERROR` |
| `Sistem_Hatasi` | 22 | `draft` + `DELIVERY_ERROR` |
| `Alici_Kabul_Etti` | 24 | `active` |
| `Alici_Reddetti` | 25 | `cancelled` + `REJECTED` |
| `Alici_Iade_Etti` | 26 | `cancelled` + `RETURNED` ⚠️ |
| `Otomatik_Onaylandi` | 29 | `active` |
| `Otomatik_Alici_Kabul_Etti` | 30 | `active` |
| `Uygulama_Yaniti_Yollaniyor` | 36 | önceki durum korunur |
| `Uygulama_Yaniti_Hata_Aldi` | 37 | önceki durum korunur + uyarı |
| `Irsaliye_Yaniti_Yollaniyor`, `Irsaliye_Yaniti_Hata_Aldi` | 41, 42 | önceki durum korunur |
| Listede olmayan değer | — | `active` + uyarı `STATUS_UNMAPPED` |

`Gonderildi` ve `Gibe_Iletildi` GİB'den kesin kabul anlamına gelmeyebilir. Giden faturada `DetailStatus` ile birlikte değerlendirilir:

| `DetailStatus` (`InvoiceDetailStatus`) | Etki |
|---|---|
| `Basariyla_Tamamlandi` (1300), `Zarf_Basariyla_Islendi` (1200) | Kesin `active` |
| 1110–1195 arası (şema, şematron, imza, yetki, sistem hataları) | `draft` + `DELIVERY_ERROR` |
| 1210, 1215, 1220, 1230 (hedefe gönderilemedi / sistem yanıtı gelmedi veya başarısız) | `draft` + `DELIVERY_ERROR` |
| `Fatura_Iptale_Konu_Edildi` (1235) | `cancelled` |
| 1, 1000, 1100 (zarflanmadı, kuyrukta, işleniyor) | `draft` |

### 8.2 E-arşiv (dokümandan)

| Liste | Değerler | Etki |
|---|---|---|
| `ArchiveInvoiceStatus` | `Fatura_Hazirlaniyor`=1, `Fatura_Imzalaniyor`=2, `Pdf_Hazirlaniyor`=3, `Pdf_Imzalaniyor`=4, `Fatura_Olusturuldu`=5, `Silindi`=6 | 5 → `active`; 1–4 → `draft`; 6 → `cancelled` |
| `CanceledArchiveInvoiceReportSendingStatus` | `Iptal_Edilmedi`=0, `Gibe_Gonderilmek_Icin_Bekliyor`=1 … `Gib_Raporu_Kabul_Etti`=6 | 0 dışındaki her değer → `cancelled` |
| `ArchiveInvoiceReportSendingStatus` | `Gibe_Gonderilmek_Icin_Bekliyor`=1, `Gibe_Gonderiliyor`=2, `Gibe_Gonderildi`=3, `Gibe_Gonderilirken_Sistem_Hatasi_Olustu`=4, `Gib_Tarafinda_Hata_Olustu`=5, `Gib_Raporu_Kabul_Etti`=6 | Bilgi; 4 ve 5 → uyarı `EARCHIVE_REPORT_ERROR` |
| `ArchiveInvoiceSendingStatus` | `Musteriye_Gonderilmedi`=1 … `Musteriye_Gonderilirken_Hata_Olustu`=4 | Beyanı etkilemez, `raw` |

Bu değerlerin yanıtta hangi alan adlarıyla geldiği §6.2'deki gibi ilk gerçek yanıttan belirlenir. O zamana kadar iptal bilgisi §6.3'teki ayrı sorgudan gelir.

## 9. Hatalar

| Belirti | Sebep (`37` §4) | Not |
|---|---|---|
| Bağlantı kurulamıyor / zaman aşımı, **her** çağrıda | `ip_rejected` olabilir | Topluluk modülü: tanımsız IP'ye TCP cevabı gelmiyor. Ağ hatasından ayırmak için: aynı anda İşNet dışı bir sağlık adresi çalışıyorsa ve İşNet'e hiçbir çağrı geçmiyorsa `ip_rejected` say, iç uyarı üret. Tek seferlik zaman aşımı `network`. |
| SOAP Fault | `soap_fault_server` / `soap_fault_client` | WCF fault biçimi: `faultcode` (ör. `a:InternalServiceFault`), `faultstring` |
| `Result=Failed` + `ErrorMessage` | Mesaja göre (aşağıdaki tablo), bilinmiyorsa `provider_error` | |

**Bilinen hata metinleri** (test ortamında doldurulacak; metin uydurulmaz):

| Metot | Metin (parça) | Sebep | Kaynak |
|---|---|---|---|
| `GetTaxPayer` | "bulunamad" | Hata değil: e-fatura kullanıcısı değil (§4) | Topluluk modülü ⚠️ |
| Herhangi | VKN için yetki yok (gerçek metin test ortamında) | `vkn_not_authorized` | — |

## 10. Adaptör iskeleti

```ts
// isnet/adapter.ts
export class IsnetAdapter implements EntegratorAdapter {
  readonly id = 'isnet' as const;
  readonly capabilities = ['incoming', 'outgoing', 'e_archive', 'ubl', 'status'] as const;

  async verify(conn: EntegratorConnection): Promise<HealthStatus> {
    const tp = await this.client.getTaxPayer(conn);                 // §4
    conn.capabilities = tp.isEInvoiceUser
      ? ['incoming', 'outgoing', 'e_archive'] : ['e_archive'];      // source_connection.capabilities
    const units = await this.client.getCompanyVendor(conn).catch(() => []);   // §5.5
    conn.meta.isnetUnits = units.map(u => u.number);                        // source_connection.meta
    const dir = tp.isEInvoiceUser ? 'Incoming' : null;
    if (dir) await this.client.searchAllInvoice(conn, {               // 1 kayıt, son 7 gün
      direction: dir, created: lastDays(7), page: 1, size: 1, xml: false });
    else await this.client.searchArchiveInvoice(conn, { created: lastDays(7), page: 1, size: 1, xml: false });
    return { ok: true };
  }

  async *listDocuments(conn, q) {
    const win = q.dateField === 'created' ? { created: [q.from, q.to] } : { issued: [q.from, q.to] };
    let page = 1;
    for (;;) {
      const res = q.docKind === 'e_invoice'
        ? await this.client.searchAllInvoice(conn, { direction: q.direction === 'incoming' ? 'Incoming' : 'Outgoing',
            ...win, page, size: 20, xml: true, units: conn.meta.isnetUnits })
        : await this.client.searchArchiveInvoice(conn, { ...win, page, size: 20, xml: true });
      yield { items: res.items.map(i => mapIsnet(i, q)), total: res.paging?.totalRecords };
      const last = res.paging?.totalPages ?? (res.items.length < 20 ? page : page + 1);
      if (page >= last) break;
      page++;
    }
  }

  async downloadUbl(conn, ref) {
    // Normalde UBL liste yanıtında gelir (IsXMLIncluded=true). Gelmediyse:
    const one = ref.docKind === 'e_invoice'
      ? await this.client.searchAllInvoice(conn, { ettn: ref.uuid, direction: dirOf(ref), page: 1, size: 1, xml: true })
      : await this.client.searchArchiveInvoice(conn, { ettn: ref.uuid, page: 1, size: 1, xml: true });
    const xml = one.items[0]?.InvoiceXml;
    if (xml) return Buffer.from(xml, 'base64');
    const link = await this.client.getDocumentViewerLink(conn, ref);   // §7
    return http.get(forceHttps(link));
  }

  async queryStatus(conn, uuids, direction) {
    // Toplu durum metodu yok: ETTN başına SearchAllInvoice (bağlantı başına saniyede 1, 37 §2)
  }
}
```

`client.ts` içinde sadece `GetTaxPayer`, `GetCompanyVendor`, `SearchAllInvoice`, `SearchArchiveInvoice`, `GetDocumentViewerLink` vardır.

**Kullanılmayacak (durum değiştirir):** `ArchiveInvoice`, `DeArchiveInvoice`, `UpdateInvoiceState`, `SendInvoiceReply`, `DirectInvoice` (faturayı başka birime taşır), bütün `Send*` ve `Cancel*` metotları (`30` §5).

## 11. Senkron planı

| İş | Çağrı | Pencere |
|---|---|---|
| Kurulum | `GetTaxPayer`, `GetCompanyVendor` | — |
| İlk yükleme | `SearchAllInvoice` (Incoming, Outgoing) + `SearchArchiveInvoice` | İçinde bulunulan yıl + önceki yıl, aylık, `MinInvoiceDate`/`MaxInvoiceDate` |
| Artımlı (60 dk) | Aynı üç çağrı | Son 3 gün, `MinInvoiceCreationDate`/`MaxInvoiceCreationDate` |
| E-arşiv iptal (günde 1) | `SearchArchiveInvoice` + `IsCancelled=true` | Son 60 gün fatura tarihi |
| Gece tam tarama | Üç çağrı | İçinde bulunulan ay + önceki ay, fatura tarihi (`37` §5.5) |
| Durum güncelleme (günde 1) | `SearchAllInvoice` + `Ettn` (XML'siz) | `37` §5.5 listesi |

## 12. Test planı (İşNet'e özel)

1. `GetTaxPayer`: e-fatura kullanıcısı bir VKN ve kullanıcı olmayan bir VKN; ikinci durumda dönen `Result` / `ErrorMessage` kaydedilir (§9 tablosu).
2. `SearchAllInvoice` Incoming ve Outgoing, XML'li ve XML'siz; yanıt fixture olarak saklanır. Alan adlarının büyük/küçük harfi (`Ettn` / `ETTN`) kaydedilir.
3. Alfabetik sıra bozulunca ne olduğu bir kez denenir (alan yok sayılıyor mu, hata mı); sonuç buraya yazılır.
4. Arşive kaldırılmış bir fatura `IsArchiveIncluded=true` ile geliyor mu.
5. Birime atanmış fatura `IncludeMainCompany=true` ile geliyor mu (§5.5).
6. `SearchArchiveInvoice` yanıtındaki ETTN, numara ve durum alanlarının adları; `IsCancelled=true` sorgusu iptal edilen faturayı getiriyor mu.
7. `InvoiceCreationDate` gelen faturada İşNet'e düşme anı mı, faturanın kendi oluşturma anı mı (artımlı çekimin anlamı).
8. Sayfa boyutu 20 ile XML'li yanıtın boyutu; makul üst sınır.
9. IP tanımı kaldırılınca (veya tanımsız IP'den) çağrının nasıl başarısız olduğu.
10. `30` §9 kabul testleri.

## 13. Doğrulanacaklar

- [ ] §2 erişim modeli ve mükellef yetkisinin şekli (İşNet)
- [ ] Adresler ve SOAPAction değerleri (WSDL)
- [ ] §12 maddeleri 2, 4, 5, 6, 7
- [ ] Hız sınırı

## Kaynaklar

- İşNet Web Servis Arayüz Dokümanları (E-Fatura, E-Arşiv, AddressBook, Company) ve örnek istekler: https://github.com/EfeSorogluu/NetteFatura-API (`docs/official-docs`, `docs/request-samples`)
- Aynı SDK'nın adres ve SOAPAction sabitleri, canlıya geçiş kılavuzu: aynı depo (`src/constants/endpoints.ts`, `src/services/*.ts`, `docs/production-guide.md`)
- Gelen fatura çeken topluluk modülü (IP davranışı, `ETTN` alan adı, `GetTaxPayer` "bulunamadı" davranışı, birimler, görüntüleyici adresi): https://github.com/mbrksntrk/dolibarr-isnet-nettefatura (`class/isnetclient.class.php`, `class/isnetincoming.class.php`)
