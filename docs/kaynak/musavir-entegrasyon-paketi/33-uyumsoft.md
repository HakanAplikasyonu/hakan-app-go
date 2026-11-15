# 33 · Uyumsoft

| | |
|---|---|
| Öncelik | 2 |
| Resmi kaynak | ✅ Uyumsoft "e-Fatura Müşteri Web Servisi Dokümanı" v2.0.6 (son revizyon 26.04.2018) + Uyumsoft test ortamının **SoapUI projesi** (WSDL ve XSD şemaları içinde). İkisi bir topluluk paketinde (`gkvdt/uyumsoft-einvoice`, `src/files`) yayımlanmış kopyalardan okundu. |
| Doküman ile şema farkı | Doküman 2018 tarihli. Alan adları ve enum değerleri için **SoapUI projesindeki şema esas alındı**; farklar aşağıda işaretli. |
| Protokol | SOAP 1.1 (WCF), ad alanı `http://tempuri.org/` |
| Kimlik doğrulama | Mükellefin Uyumsoft kullanıcı adı ve şifresi, WS-Security `UsernameToken` + `Timestamp` |
| Canlı test | Yapılmadı |

Önce `37` (ortak altyapı) okunmalı.

**Not:** Uyumsoft hem e-fatura entegratörü hem ön muhasebe / ERP üreticisi. Bu dosya entegratör servislerini kapsar.

## 1. Adresler

| Ortam | Integration servisi (kullanılacak) | Kaynak |
|---|---|---|
| Canlı | `https://efatura.uyumsoft.com.tr/services/Integration` | Doküman |
| Test | `https://efatura-test.uyumsoft.com.tr/services/Integration` | Doküman |

Test kullanıcısı (dokümandan): `Uyumsoft` / `Uyumsoft`. Test portalı: `http://portal-test.uyumsoft.com.tr/`.

Dokümanda bir de **BasicIntegration** servisi var (`http://…/services/BasicIntegration`): SSL'siz, kullanıcı bilgisi her çağrıda gövdede (`<userInfo Username=".." Password=".."/>`). **Kullanılmaz** (şifre şifrelenmeden gider). SoapUI projesi BasicIntegration'a göre hazırlanmış; gövde yapısı aynı, sadece `userInfo` elemanı yok ve kimlik başlıkta.

| Başlık | Değer |
|---|---|
| `Content-Type` | `text/xml; charset=utf-8` |
| `SOAPAction` | Integration WSDL'inden alınır. SoapUI projesinde BasicIntegration için `http://tempuri.org/IBasicIntegration/{Metot}`; Integration için `http://tempuri.org/IIntegration/{Metot}` olması beklenir ama **görülmedi** ⚠️. WSDL: servis adresi + `?wsdl` (WCF standardı) ⚠️. |

## 2. Mükellefin girdiği

- Uyumsoft kullanıcı adı ve şifresi. Mükellefe Uyumsoft portalında **sadece entegrasyon için ayrı kullanıcı** açması önerilir (yetkisi sadece görüntüleme olacak şekilde, Uyumsoft destekliyorsa ⚠️).
- VKN girilmez; `WhoAmI` ile Uyumsoft'tan okunur ve portaldaki mükellef VKN'siyle karşılaştırılır (§4).

## 3. Kimlik doğrulama (dokümandaki örnek)

```xml
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"
            xmlns:u="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
  <s:Header>
    <o:Security s:mustUnderstand="1"
                xmlns:o="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
      <u:Timestamp u:Id="_0">
        <u:Created>{şimdi, UTC, ör. 2026-11-08T13:34:48.817Z}</u:Created>
        <u:Expires>{şimdi + 5 dk, UTC}</u:Expires>
      </u:Timestamp>
      <o:UsernameToken u:Id="uuid-{yeni GUID}-1">
        <o:Username>{kullanıcı}</o:Username>
        <o:Password>{şifre}</o:Password>
      </o:UsernameToken>
    </o:Security>
  </s:Header>
  <s:Body xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
          xmlns:xsd="http://www.w3.org/2001/XMLSchema">
    <!-- metot gövdesi -->
  </s:Body>
</s:Envelope>
```

- Şifre düz metin (`PasswordText`) gidiyor; bu yüzden sadece HTTPS.
- `Timestamp` her istekte yeniden üretilir. Sunucu saatiyle fark olursa istek reddedilebilir; sunucularımızda NTP açık olmalı.
- Dokümandaki örnekte ayrıca WCF tanılama başlığı (`ActivityId`) var; gönderilmesi gerekmez.

## 4. Kurulum kontrolü: `WhoAmI` ve `TestConnection`

İkisi de SoapUI projesinde var; dokümanda açıklaması yok.

```xml
<WhoAmI xmlns="http://tempuri.org/"/>
```

Yanıt (şemadan): `WhoAmIResult` (`IsSucceded`, `Message` öznitelikleri) > `Value` (`WhoAmIInfo`):

| Alan | Kullanım |
|---|---|
| `Value/Customer/VkTckNo` | **Bağlantının VKN'si.** Portaldaki mükellef VKN'siyle aynı değilse bağlantı `failed` + uyarı `VKN_MISMATCH` (yanlış firmanın şifresi girilmiş) |
| `Value/Customer/Name` | Unvan (müşavire gösterilir) |
| `Value/Services/@HasEInvoice` | `false` ise gelen/giden e-fatura çekilmez |
| `Value/Services/@HasEarchive` | `false` ise e-arşiv beklenmez |
| `Value/User/Username` | Log |

`WhoAmI` beklenen yapıyı vermezse yedek: `TestConnection` (`TestConnectionResult`, `FlagResponse`) + son 7 günün `GetInboxInvoiceList` çağrısı.

## 5. Ortak yanıt yapısı

Bütün metotlar `Response` türünde döner (doküman §2.1 + şema):

| Alan | Şemada | Anlamı |
|---|---|---|
| `IsSucceded` | Öznitelik, zorunlu | `false` ise iş hatası |
| `Message` | Öznitelik | Hata metni |
| `Value` | Eleman | Metodun sonucu |

Sonuç elemanının adı `{Metot}Result` (ör. `GetInboxInvoiceListResult`).

```xml
<GetInboxInvoiceListResponse xmlns="http://tempuri.org/">
  <GetInboxInvoiceListResult IsSucceded="true" Message="">
    <Value PageIndex="0" PageSize="50" TotalCount="57" TotalPages="2">
      <Items> ... </Items>
      <Items> ... </Items>
    </Value>
  </GetInboxInvoiceListResult>
</GetInboxInvoiceListResponse>
```

(Şemadaki yapıya göre beklenen biçim; ilk gerçek yanıtla doğrulanır ⚠️.)

## 6. Kullanılacak metotlar

| Metot | İş | Sonuç tipi (şema) |
|---|---|---|
| `WhoAmI` | Kurulum kontrolü | `ResponseOfWhoAmIInfo` |
| `GetInboxInvoiceList(query)` | Gelen fatura listesi | `InboxInvoiceListResponse` → `Value` (`PagedResponseOfInboxInvoiceListItem`) |
| `GetOutboxInvoiceList(query)` | Giden fatura listesi (e-fatura **ve** e-arşiv) | `OutboxInvoiceListResponse` |
| `GetInboxInvoiceData(invoiceId)` | Gelen faturanın imzalı UBL'i | `InvoiceDataResponse` → `Value/Data` (base64) |
| `GetOutboxInvoiceData(invoiceId)` | Giden faturanın imzalı UBL'i | aynı |
| `QueryInboxInvoiceStatus(invoiceIds)` | Gelen durum, toplu | `InvoiceStatusResponse` → `Value[]` (`InvoiceStatusInfo`) |
| `QueryOutboxInvoiceStatus(invoiceIds)` | Giden durum, toplu | aynı |
| `GetInboxInvoicePdf` / `GetOutboxInvoicePdf` | PDF (isteğe bağlı) | `Value/Data` |

`invoiceId` = ETTN (UBL `UUID`).

**Kullanılmayacak (durum değiştirir):** `SetInvoicesTaken`, `ChangeInvoiceArchiveStatus`, `SendDocumentResponse`, `SendInvoice`, `SaveAsDraft`, `SendDraft`, `CancelDraft`, `MoveToDraftStatus`, `RetrySendInvoices`, `TransferInboxInvoiceToAnotherBranch`, `CancelEArchiveInvoice` (`30` §5).

**Kullanılmayacak (risk):** `GetInboxInvoices` ve `GetInboxInvoicesData`. İkisinde de `SetTaken` parametresi var; `true` gönderilirse mükellefin "yeni fatura" işareti kalkar. Portal liste için `GetInboxInvoiceList` (bu parametre yok), içerik için tek tek `GetInboxInvoiceData` kullanır. Toplu XML gerekirse ve kullanılırsa `SetTaken="false"`, `OnlyNewestInvoices="false"` **sabit** yazılır ve kod incelemesinde kontrol edilir.

## 7. Liste: `GetInboxInvoiceList` / `GetOutboxInvoiceList`

### 7.1 İstek şablonu

Eleman sırası şemadaki sırayla aynı olmalı (`InvoiceListQueryModel`):

```xml
<GetInboxInvoiceList xmlns="http://tempuri.org/">
  <query PageIndex="{0..}" PageSize="{sayfa boyutu}" OnlyNewestInvoices="false">
    <!-- dönem taraması: fatura tarihi -->
    <ExecutionStartDate>{2026-10-01T00:00:00+03:00}</ExecutionStartDate>
    <ExecutionEndDate>{2026-10-31T23:59:59+03:00}</ExecutionEndDate>
    <!-- artımlı: oluşturma tarihi (Execution* yerine) -->
    <!-- <CreateStartDate>…</CreateStartDate> <CreateEndDate>…</CreateEndDate> -->
    <!-- belirli faturalar: <InvoiceIds>{ETTN}</InvoiceIds> (tekrarlanabilir) -->
    <SortColumn>CreateDate</SortColumn>
    <SortMode>Ascending</SortMode>
  </query>
</GetInboxInvoiceList>
```

`GetOutboxInvoiceList` aynı; `OnlyNewestInvoices` özniteliği yok, sonda isteğe bağlı `<Scenario>` (şemada var, açıklaması yok; gönderilmez).

Şemadaki tam sıra: `ExecutionStartDate`, `ExecutionEndDate`, `CreateStartDate`, `CreateEndDate`, `Status`, `InvoiceIds`*, `InvoiceNumbers`*, `StatusInList`*, `StatusNotInList`*, `SortColumn`, `SortMode`, `IsArchived` (`*` tekrarlanabilir).

### 7.2 Alanlar

| Alan | Doküman / şema | Portal değeri |
|---|---|---|
| `PageIndex` (öznitelik) | Zorunlu | Dokümandaki örnek `0` ile başlıyor; **0 tabanlı olduğu test ortamında doğrulanır** ⚠️ |
| `PageSize` (öznitelik) | Zorunlu | 50 (öneri) |
| `OnlyNewestInvoices` (öznitelik, sadece gelen) | Zorunlu | `false` |
| `ExecutionStartDate` / `ExecutionEndDate` | Doküman (liste metotları): "Faturanın tarihine göre" | Dönem taraması |
| `CreateStartDate` / `CreateEndDate` | Doküman: "Faturanın oluşturulma tarihine göre" | Artımlı çekim. Test ortamında doğrulandı (2026-11-15): `CreateStartDate` saatiyle uygulanıyor, `CreateEndDate` yalnızca **tarih** olarak alınıyor (gece yarısına kesiliyor). 19:25'te oluşan belge, bitişi aynı gün 19:26 olan pencerede gelmiyor, bitiş ertesi gün 00:00 olunca geliyor. Adaptör bitişi pencerenin son anının ertesi günü 00:00 olarak gönderir. |
| `InvoiceIds` | UUID listesi | Durum / tek fatura |
| `SortColumn` | Şema: `Default`, `Id`, `CreateDate`, `ExecutionDate` | `CreateDate` (sayfalar arasında kayma olmasın) |
| `SortMode` | Şema: `Default`, `Ascending`, `Descending` | `Ascending` |
| `IsArchived` | Şemada var, açıklama yok | Gönderilmez; arşive kaldırılmış faturaların gelip gelmediği test edilir (§11). Gelmiyorsa ikinci geçiş `IsArchived=true` ile yapılır. |
| `Status`, `StatusInList`, `StatusNotInList` | Şemada var | Gönderilmez (bütün durumlar gelsin) |

**Tarih alanlarının anlamı metoda göre değişiyor (doküman):**

| Metot | `ExecutionStartDate` |
|---|---|
| `GetInboxInvoiceList`, `GetOutboxInvoiceList` | **Fatura tarihi** |
| `GetInboxInvoices`, `GetInboxInvoicesData`, `GetOutboxInvoicesData` | **Oluşturulma tarihi** |

Portal sadece liste metotlarını kullandığı için: `Execution*` = fatura tarihi, `Create*` = oluşturma tarihi. Tarihler dokümandaki örnek gibi saat dilimiyle gönderilir (`+03:00`).

Pencere bölme eşiği (`37` §5.3): `TotalCount > 1000` (öneri).

### 7.3 Liste öğesi → `EntegratorDocHeader`

Şemadaki `InvoiceListItemBase` (gelen için `+ IsNew`, giden için `+ Scenario, ScenarioCode, LocalDocumentId, ExtraInformation`):

| Uyumsoft | Portal | Not |
|---|---|---|
| `InvoiceId` | `uuid` | |
| `DocumentId` | `invoiceNumber` | Alanın anlamı dokümanda yok; UBL `cbc:ID` ile karşılaştırılır ⚠️ |
| `ExecutionDate` | `issueDate` | Şemada `nillable`; boşsa UBL'den |
| `CreateDateUtc` | `createdAt` | UTC |
| `TargetTcknVkn`, `TargetTitle` | `counterpartyIdentifier`, `counterpartyName` | Sadece liste ekranı |
| `Type`, `TypeCode` | `profileId` (ham) + **e-arşiv ayrımı** | Şema enum'u `InvoiceTypes`: `BaseInvoice`, `ComercialInvoice`, `InvoiceWithPassanger`, `Export`, `eArchive`, `Hks`, `PublicAdministration`. Bunlar senaryo (profil) karşılıkları; `eArchive` → `docKind='e_archive'` ⚠️ |
| `InvoiceTipType`, `InvoiceTipTypeCode` | `invoiceTypeCode` (ham) | Şema enum'u `InvoiceTipType`: `Sales`, `Return`, `Tax`, `Exception`, `TaxBase`, `ExportSaved`, `Sgk`, `Broker`, `HksSales`, `HksBroker`, `WithholdingReturn` (fatura tipi karşılıkları) |
| `Scenario` (sadece giden) | e-arşiv ayrımı (ikinci gösterge) | Şema enum'u `InvoiceScenarioType`: `eInvoice`, `eArchive` |
| `Status`, `StatusCode` | `statusRaw` | §9; eşleme **koda** göre yapılır |
| `EnvelopeStatus`, `EnvelopeStatusCode`, `Message` | `raw` | Zarf durumu |
| `PayableAmount`, `TaxTotal`, `TaxExclusiveAmount`, `DocumentCurrencyCode`, `ExchangeRate` | Sadece liste ve kontrol | |
| `Vat1`, `Vat8`, `Vat18`, `Vat0TaxableAmount` … | **Kullanılmaz** | Eski KDV oranlarına göre sabit alanlar; KDV kırılımı UBL'den |
| `IsArchived`, `IsNew`, `EnvelopeIdentifier`, `OrderDocumentId` | `raw` | Değiştirilmez |

**E-arşiv ayrımı, kesin kural:** `docKind` önce listedeki `Type=eArchive` / `Scenario=eArchive` ile belirlenir, UBL indirilince `cbc:ProfileID = EARSIVFATURA` ile kesinleşir (doküman: "Earşiv Faturalarında ProfileID alanı 'EARSIVFATURA' olmalıdır… sistem bu alanı otomatik olarak 'EARSIVFATURA' yazacaktır"). İkisi çelişirse UBL esas, uyarı `DOC_KIND_MISMATCH`.

## 8. İçerik: `GetInboxInvoiceData` / `GetOutboxInvoiceData`

```xml
<GetInboxInvoiceData xmlns="http://tempuri.org/">
  <invoiceId>{ETTN}</invoiceId>
</GetInboxInvoiceData>
```

Yanıt: `GetInboxInvoiceDataResult` (`IsSucceded`, `Message`) > `Value` (`InvoiceData`; öznitelikler `InvoiceId`, `LocalDocumentId`, `ExtraInformation`) > `Data` (base64). Doküman: "Dönen değer faturanın imzasını da içeren resmi geçerliliği olan faturadır." → `37` §3.4 → `31`.

## 9. Durumlar

### 9.1 Kod → `status_normalized`

Doküman kodları (`InvoiceQueryStatus`); eşleme `StatusCode` sayısına göre yapılır, çünkü ad yazımı doküman ile şema arasında farklı:

| Kod | Doküman adı | Şemadaki ad | `status_normalized` |
|---|---|---|---|
| 0 | `Draft` (Taslak) | `Draft` | `draft` |
| 10 | `Canceled` (İptal Edildi) | `Canceled` | `cancelled` |
| 100 | `Queued` (Kuyrukta) | `Queued` | `draft` |
| 200 | `Processing` (İşlemde) | `Processing` | `draft` |
| 300 | `SentToGib` (GİB'e gönderildi) | `SentToGib` | `active` ⚠️ |
| 1000 | `Approved` (Onaylandı) | `Approved` | `active` |
| 1100 | `WaitingForApprovement` (Onay Bekliyor) | `WaitingForAprovement` | `active` + `PENDING_RESPONSE` |
| 1200 | `Declined` (Reddedildi) | `Declined` | `cancelled` + `REJECTED` |
| 1300 | `Return` (İade Edildi) | `Return` | `cancelled` + `RETURNED` ⚠️ |
| 1400 | `eArchiveCanceled` (e-Arşiv İptal) | `EArchivedCanceled` | `cancelled` |
| 2000 | `Error` (Hata) | `Error` | `draft` + `DELIVERY_ERROR` |
| ? | — | `NotPrepared`, `NotSend` | Kodları dokümanda yok ⚠️. `draft` + uyarı `STATUS_UNMAPPED` |
| Diğer | — | — | `active` + uyarı `STATUS_UNMAPPED` |

Doküman: Uyumsoft'ta gelen faturaya "İade" yanıtı verilemiyor; giden faturaya iade yanıtı gelebiliyor.

`EnvelopeStatus` (şemada 40 değer: `SchematronCheckFailed`, `XmlSchemaCheckFailed`, `CompletedSuccessfully` …) giden faturada `DELIVERY_ERROR` uyarısının açıklaması olarak saklanır; ayrı eşleme yapılmaz.

### 9.2 Toplu durum: `Query*InvoiceStatus`

```xml
<QueryOutboxInvoiceStatus xmlns="http://tempuri.org/">
  <invoiceIds>
    <string>{ETTN-1}</string>
    <string>{ETTN-2}</string>
  </invoiceIds>
</QueryOutboxInvoiceStatus>
```

Yanıt: `Value[]` (`InvoiceStatusInfo`; öznitelikler `InvoiceId`, `Status`, `StatusCode`, `Message`). Tek çağrıda kaç ETTN gönderilebileceği dokümanda yok; 100'lük gruplar (öneri), hata alınırsa küçültülür.

## 10. Hatalar

| Belirti | Sebep (`37` §4) |
|---|---|
| HTTP 500 + SOAP Fault, WS-Security / kimlik metni | `auth_failed` (gerçek metin test ortamında yanlış şifreyle belirlenir) |
| `IsSucceded="false"` + `Message` | Mesaja göre, bilinmiyorsa `provider_error` |
| `WhoAmI` VKN'si farklı | Bağlantı `failed`, `VKN_MISMATCH` (§4) |

**Bilinen hata metinleri** (test ortamında doldurulacak):

| Metot | Metin (parça) | Sebep |
|---|---|---|
| Herhangi | Yanlış şifre metni | `auth_failed` |

## 11. Adaptör iskeleti

```ts
export class UyumsoftAdapter implements EntegratorAdapter {
  readonly id = 'uyumsoft' as const;
  readonly capabilities = ['incoming', 'outgoing', 'e_archive', 'ubl', 'pdf', 'status'] as const;

  async verify(conn) {
    const me = await this.client.whoAmI(conn);                       // §4
    if (digits(me.customer.vkTckNo) !== digits(conn.company.taxNumber))
      throw new EntegratorError({ reason: 'auth_failed', providerMessage: 'VKN_MISMATCH' });
    conn.meta.hasEInvoice = me.services.hasEInvoice;
    conn.meta.hasEArchive = me.services.hasEarchive;
    return { ok: true };
  }

  async *listDocuments(conn, q) {
    // Uyumsoft'ta e-arşiv ayrı liste değil: docKind='e_archive' istendiğinde giden liste çekilir ve Type/Scenario ile süzülür.
    const outbox = q.direction === 'outgoing';
    let page = 0;                                                    // 0 tabanlı (§7.2, doğrulanacak)
    for (;;) {
      const res = await this.client.list(conn, outbox ? 'Outbox' : 'Inbox', {
        page, size: 50,
        ...(q.dateField === 'created' ? { createStart: q.from, createEnd: q.to } : { execStart: q.from, execEnd: q.to }),
        sortColumn: 'CreateDate', sortMode: 'Ascending',
      });
      const items = res.items.map(i => mapUyumsoft(i, q.direction))
        .filter(h => q.docKind === 'any' || h.docKind === q.docKind);
      yield { items, total: res.totalCount };
      if (page + 1 >= res.totalPages) break;
      page++;
    }
  }

  downloadUbl(conn, ref) {
    return ref.direction === 'incoming'
      ? this.client.getInboxInvoiceData(conn, ref.uuid)
      : this.client.getOutboxInvoiceData(conn, ref.uuid);
  }

  async queryStatus(conn, uuids, direction) {
    const out = [];
    for (const c of chunk(uuids, 100)) {                             // sırayla, bağlantı limiti 37 §2
      const r = direction === 'incoming'
        ? await this.client.queryInboxInvoiceStatus(conn, c)
        : await this.client.queryOutboxInvoiceStatus(conn, c);
      out.push(...r.map(s => ({ uuid: s.invoiceId, statusRaw: `${s.status}(${s.statusCode})` })));
    }
    return out;
  }
}
```

Not: Giden liste e-fatura ve e-arşivi birlikte döndüğü için senkron işi giden yön için listeyi **bir kez** çekip iki `docKind`'a ayırmalıdır (`listDocuments` giden yönde `docKind: 'any'` ile çağrılır, `30` §6).

## 12. Senkron planı

| İş | Çağrı | Pencere |
|---|---|---|
| Kurulum | `WhoAmI` | — |
| İlk yükleme | `GetInboxInvoiceList`, `GetOutboxInvoiceList` + her yeni ETTN için `Get*InvoiceData` | İçinde bulunulan yıl + önceki yıl, aylık, `ExecutionStartDate`/`ExecutionEndDate` |
| Artımlı (60 dk) | Aynı | Son 3 gün, `CreateStartDate`/`CreateEndDate` |
| Gece tam tarama | Aynı | İçinde bulunulan ay + önceki ay, `Execution*` |
| Durum (günde 1) | `QueryInboxInvoiceStatus`, `QueryOutboxInvoiceStatus` | `37` §5.5 listesi |

## 13. Test planı (Uyumsoft'a özel)

1. `WhoAmI` yanıtı; `VkTckNo` ve `Services` alanları dolu mu.
2. Liste çağrısı `PageIndex=0` ve `PageIndex=1` ile; sayfalamanın 0 tabanlı olduğu doğrulanır.
3. Arşive kaldırılmış bir fatura `IsArchived` gönderilmeden listede geliyor mu (§7.2).
4. Giden listede e-arşiv faturası: `Type` ve `Scenario` değerleri; UBL `ProfileID` ile tutarlı mı.
5. `DocumentId` = UBL `cbc:ID` mi.
6. Gelen listede `IsNew=true` bir fatura listelenip UBL'i indirildikten sonra `IsNew` hâlâ `true` mu (portal hiçbir işaret değiştirmemeli, `30` §5).
7. Yanlış şifre ve süresi geçmiş `Timestamp` ile alınan hata metinleri (§10 tablosu).
8. `Query*InvoiceStatus` tek çağrıda 100 ETTN.
9. `30` §9 kabul testleri.

## 14. Doğrulanacaklar

- [x] Integration WSDL adresi ve SOAPAction değerleri (go-ubltr `provider/uyumsoft` test ortamında liste ve UBL indirme çalıştı, 2026-11-15)
- [x] `CreateEndDate` hassasiyeti: yalnızca tarih (§7.2)
- [ ] §13 maddeleri 2, 3, 4, 5, 6
- [ ] `NotPrepared`, `NotSend` durum kodları
- [ ] Sadece görüntüleme yetkili kullanıcı açılabiliyor mu
- [ ] Hız sınırı (dokümanda yok)
- [ ] 2018 sonrası servis değişiklikleri (Uyumsoft'tan güncel doküman istenmeli)

## Kaynaklar

- Uyumsoft e-Fatura Müşteri Web Servisi Dokümanı v2.0.6 ve test ortamı SoapUI projesi (`efatura-test-uyumsoft-com-soapui-project.xml`): https://github.com/gkvdt/uyumsoft-einvoice (`src/files`)
