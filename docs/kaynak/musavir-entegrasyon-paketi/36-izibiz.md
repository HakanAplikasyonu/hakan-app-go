# 36 · İzibiz

| | |
|---|---|
| Öncelik | 5 |
| Kaynak | ✅ İzibiz'in **kendi GitHub hesabındaki** istemci projeleri: `izibiz/ws-client-dotnet` (servislerin WSDL ve XSD dosyaları) ve `izibiz/ws-client-php` (test çağrıları). Yazılı bir servis dokümanı bulunamadı; alanlar şemadan, kullanım İzibiz'in test kodundan. |
| Adresler | Test adresleri istemci projelerinden ✅ · **canlı adres bulunamadı** ⚠️ |
| Protokol | SOAP 1.1, `soapAction=""` |
| Kimlik doğrulama | `Login` (kullanıcı adı + şifre) → `SESSION_ID`; sonraki her istekte `REQUEST_HEADER/SESSION_ID` |
| Canlı test | Yapılmadı |

Önce `37` (ortak altyapı) okunmalı.

## 1. Adresler

| Servis | Test adresi | Kaynak |
|---|---|---|
| Kimlik (`Login`, `Logout`) | `https://efaturatest.izibiz.com.tr/AuthenticationWS` | PHP test projesi + .NET WSDL |
| E-fatura | `https://efaturatest.izibiz.com.tr/EFaturaOIB` | .NET WSDL (PHP projesi `…/EInvoiceWS` kullanıyor; ikisi de İzibiz'in kendi kodunda, hangisinin canlıda kullanılacağı İzibiz'e sorulur) |
| E-arşiv | `https://efaturatest.izibiz.com.tr/EIArchiveWS/EFaturaArchive` | .NET WSDL |

WSDL: adres + `?wsdl`.

**Canlı adres** istemci projelerinde yok. İzibiz'den alınır; test adresindeki `efaturatest` yerine ne geleceği tahmin edilmez.

**Test kullanıcısı:** İzibiz'in PHP test projesinde `izibiz-test2` / `izi321`. Kendi test hesabımız için İzibiz'e başvurulur.

Oturum, PHP test projesinde kimlik servisinden alınıp e-fatura ve e-arşiv servislerinde kullanılıyor; yani tek `SESSION_ID` üç serviste geçerli.

## 2. Mükellefin girdiği

İzibiz web servis kullanıcı adı ve şifresi. VKN girilmez; yanıtlardaki `SENDER` / `RECEIVER` ile ve UBL'deki taraflarla kontrol edilir (`31` §4 `PARTY_MISMATCH`).

## 3. Ad alanları (önemli)

| Servis | Kök eleman ad alanı | Alt elemanlar |
|---|---|---|
| Kimlik, e-fatura | `http://schemas.i2i.com/ei/wsdl` | **Ad alanısız** |
| E-arşiv | `http://schemas.i2i.com/ei/wsdl/archive` | **Ad alanısız** |

Şemalarda `elementFormDefault` tanımlı değil (varsayılan `unqualified`). Bu yüzden sadece istek kök elemanı (`GetInvoiceRequest` vb.) ad alanlı yazılır; `REQUEST_HEADER`, `INVOICE_SEARCH_KEY` ve diğer alt elemanlar ad alanısız. WSDL'den üretilen istemci bunu kendisi yapar; elle XML yazılıyorsa aşağıdaki şablonlar gibi **önekli kök + öneksiz alt eleman** kullanılır ve kökte varsayılan ad alanı (`xmlns="…"`) **tanımlanmaz**. Şemadan çıkarıldı; ilk test çağrısında doğrulanır ⚠️.

Elemanlar şemadaki **sırayla** yazılır.

## 4. Oturum

### 4.1 `Login`

```xml
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"
                  xmlns:w="http://schemas.i2i.com/ei/wsdl">
  <soapenv:Body>
    <w:LoginRequest>
      <REQUEST_HEADER>
        <SESSION_ID></SESSION_ID>
        <APPLICATION_NAME>EASYTRADE-PORTAL</APPLICATION_NAME>
      </REQUEST_HEADER>
      <USER_NAME>{kullanıcı}</USER_NAME>
      <PASSWORD>{şifre}</PASSWORD>
    </w:LoginRequest>
  </soapenv:Body>
</soapenv:Envelope>
```

Yanıt (`LoginResponse`): `SESSION_ID`, hata varsa `ERROR_TYPE`.

### 4.2 `REQUEST_HEADER` (şemadaki sıra)

| # | Alan | Zorunlu | Portal değeri |
|---|---|---|---|
| 1 | `SESSION_ID` | Evet | `Login` yanıtı (`Login`'de boş) |
| 2 | `CLIENT_TXN_ID` | Hayır | Her istek için yeni GUID (log eşleştirmesi için öneri) |
| 3–6 | `INTL_TXN_ID`, `INTL_PARENT_TXN_ID`, `ACTION_DATE`, `CHANGE_INFO` | Hayır | Gönderilmez |
| 7 | `REASON` | Hayır | Gönderilmez |
| 8 | `APPLICATION_NAME` | Hayır | `EASYTRADE-PORTAL` (İzibiz'in test kodu kendi uygulama adını yazıyor) |
| 9–11 | `HOSTNAME`, `CHANNEL_NAME`, `SIMULATION_FLAG` | Hayır | Gönderilmez |
| 12 | `COMPRESSED` | Hayır | `N` (e-fatura), `Y` (e-arşiv toplu içerik; içerik ZIP gelir) |
| 13 | `ATTRIBUTES` | Hayır | Gönderilmez |

### 4.3 `Logout`

```xml
<w:LogoutRequest>
  <REQUEST_HEADER><SESSION_ID>{SESSION_ID}</SESSION_ID></REQUEST_HEADER>
</w:LogoutRequest>
```

Başarılı: `REQUEST_RETURN/RETURN_CODE = 0`.

**Oturum yönetimi:** Senkron işi başında `Login`, sonunda `Logout`. Oturum süresi bilinmiyor ⚠️; çağrı oturum hatası verirse 1 kez yeniden `Login` (`37` §4 `session_expired`). Oturum anahtarı loglanmaz.

## 5. E-fatura listesi: `GetInvoice` (EFaturaOIB)

### 5.1 İstek

```xml
<w:GetInvoiceRequest>
  <REQUEST_HEADER>
    <SESSION_ID>{SESSION_ID}</SESSION_ID>
    <APPLICATION_NAME>EASYTRADE-PORTAL</APPLICATION_NAME>
    <COMPRESSED>N</COMPRESSED>
  </REQUEST_HEADER>
  <INVOICE_SEARCH_KEY>
    <LIMIT>{üst sınır}</LIMIT>
    <!-- tek fatura: <UUID>{ETTN}</UUID> -->
    <DATE_TYPE>{ISSUE|CREATE}</DATE_TYPE>
    <START_DATE>{YYYY-MM-DD}</START_DATE>
    <END_DATE>{YYYY-MM-DD}</END_DATE>
    <READ_INCLUDED>true</READ_INCLUDED>
    <DIRECTION>{IN|OUT}</DIRECTION>
  </INVOICE_SEARCH_KEY>
  <HEADER_ONLY>{Y|N}</HEADER_ONLY>
</w:GetInvoiceRequest>
```

`INVOICE_SEARCH_KEY` şemadaki tam sıra: `LIMIT`, `ID`, `UUID`, `FROM`, `TO`, `DATE_TYPE`, `START_DATE`, `END_DATE`, `READ_INCLUDED`, `DIRECTION`, `SENDER`, `RECEIVER`, `DRAFT_FLAG`.

| Alan | Şema | Portal değeri | Not |
|---|---|---|---|
| `LIMIT` | `int` | Liste: 1000 · içerik: 1 | Sayfalama alanı **yok** (§5.3) |
| `UUID` | | Tek fatura | |
| `DATE_TYPE` | `ISSUE` / `CREATE` | Dönem taraması `ISSUE`, artımlı `CREATE` | |
| `START_DATE`, `END_DATE` | `xsd:date` | `YYYY-MM-DD` | |
| `READ_INCLUDED` | `boolean` | **`true`** | Okunmuş faturalar da gelsin. `false` / gönderilmezse sadece okunmamışlar gelebilir. |
| `DIRECTION` | `token` | `IN` / `OUT` | İzibiz test kodunda `IN`, `OUT` |
| `HEADER_ONLY` | `token`, varsayılan `N` | Liste `Y`, içerik `N` | `N` → `CONTENT` (UBL) gelir |

### 5.2 Yanıt

`GetInvoiceResponse`: `INVOICE[]` (0..n), hata varsa `ERROR_TYPE`.

`INVOICE` (şema): öznitelikler `ID`, `UUID`, `LIST_ID`; elemanlar `HEADER`, `CONTENT`.

| İzibiz | Portal | Not |
|---|---|---|
| `@UUID` | `uuid` | |
| `@ID` | `invoiceNumber` | |
| `HEADER/ISSUE_DATE` | `issueDate` | |
| `HEADER/CDATE` | `createdAt` | |
| `HEADER/SENDER`, `HEADER/RECEIVER` | VKN'ler; yöne göre karşı taraf → `counterpartyIdentifier` | `IN`: karşı taraf `SENDER`; `OUT`: `RECEIVER` |
| `HEADER/SUPPLIER`, `HEADER/CUSTOMER` | Unvanlar → `counterpartyName` | |
| `HEADER/FROM`, `HEADER/TO` | Posta kutusu etiketleri | `raw` |
| `HEADER/PROFILEID` | `profileId` | |
| `HEADER/INVOICE_TYPE_CODE` | `invoiceTypeCode` | |
| `HEADER/STATUS`, `STATUS_DESCRIPTION`, `STATUS_CODE` | `statusRaw` | §7 |
| `HEADER/GIB_STATUS_CODE`, `GIB_STATUS_DESCRIPTION` | `raw` | |
| `HEADER/RESPONSE_CODE`, `RESPONSE_DESCRIPTION` | `raw` | Kabul / red yanıtı |
| `HEADER/PAYABLE_AMOUNT`, `TAX_EXCLUSIVE_TOTAL_AMOUNT`, `TAX_INCLUSIVE_TOTAL_AMOUNT`, `LINE_EXTENSION_AMOUNT`, `ALLOWANCE_TOTAL_AMOUNT`, `CHARGE_TOTAL_AMOUNT` | Sadece liste ve kontrol | |
| `HEADER/ENVELOPE_IDENTIFIER`, `FILENAME`, `HASH`, `DIRECTION` | `raw` | |
| `CONTENT` | UBL (base64; `COMPRESSED=Y` ise ZIP) | `37` §3.4 → `31` |

`docKind = 'e_invoice'`.

### 5.3 Pencere ve sayı

`GetInvoice`'da sayfalama yok; `LIMIT` sadece üst sınır. Kural:

1. Önce `GetInvoiceCount` ile pencerenin sayısı alınır:

   ```xml
   <w:GetInvoiceCountRequest>
     <REQUEST_HEADER><SESSION_ID>{SESSION_ID}</SESSION_ID></REQUEST_HEADER>
     <ISSUE_DATE_START>{YYYY-MM-DD}</ISSUE_DATE_START>   <!-- veya CDATE_START -->
     <ISSUE_DATE_END>{YYYY-MM-DD}</ISSUE_DATE_END>       <!-- veya CDATE_END -->
     <DIRECTION>{IN|OUT}</DIRECTION>
   </w:GetInvoiceCountRequest>
   ```

   Yanıt: `PORTAL` ve `WS` (iki sayı). Anlamları şemada yok ⚠️; test ortamında İzibiz portalındaki sayıyla karşılaştırılır. Doğrulanana kadar toplamları üst tahmin olarak kullanılır.

2. Sayı (veya dönen kayıt adedi) `LIMIT`'e eşit ya da büyükse pencere ikiye bölünür (`37` §5.3), 1 güne kadar.
3. 1 günlük pencerede bile `LIMIT` doluyorsa `LIMIT` büyütülür ve uyarı üretilir (bu durumda kayıt kaçırılmış olabilir).

`GetInvoiceCount` çalışmazsa sadece 2. kural uygulanır.

### 5.4 İçerik indirme

Liste `HEADER_ONLY=Y` ile alınır, sonra yeni / UBL'i eksik her ETTN için:

```xml
<w:GetInvoiceRequest>
  <REQUEST_HEADER>…</REQUEST_HEADER>
  <INVOICE_SEARCH_KEY>
    <LIMIT>1</LIMIT>
    <UUID>{ETTN}</UUID>
    <READ_INCLUDED>true</READ_INCLUDED>
    <DIRECTION>{IN|OUT}</DIRECTION>
  </INVOICE_SEARCH_KEY>
  <HEADER_ONLY>N</HEADER_ONLY>
</w:GetInvoiceRequest>
```

Alternatif: `GetInvoiceWithType` (`INVOICE_SEARCH_KEY/TYPE` = `XML` / `PDF` / `HTML`). PDF gerekirse bu metot kullanılır.

## 6. E-arşiv listesi: `GetEArchiveInvoiceList` (EFaturaArchive)

Giden e-arşiv faturaları.

### 6.1 İstek

```xml
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"
                  xmlns:a="http://schemas.i2i.com/ei/wsdl/archive">
  <soapenv:Body>
    <a:GetEArchiveInvoiceListRequest>
      <REQUEST_HEADER>
        <SESSION_ID>{SESSION_ID}</SESSION_ID>
        <APPLICATION_NAME>EASYTRADE-PORTAL</APPLICATION_NAME>
        <COMPRESSED>Y</COMPRESSED>
      </REQUEST_HEADER>
      <LIMIT>{üst sınır}</LIMIT>
      <START_DATE>{YYYY-MM-DDTHH:mm:ss}</START_DATE>
      <END_DATE>{YYYY-MM-DDTHH:mm:ss}</END_DATE>
      <HEADER_ONLY>{Y|N}</HEADER_ONLY>
      <CONTENT_TYPE>XML</CONTENT_TYPE>
      <READ_INCLUDED>Y</READ_INCLUDED>
    </a:GetEArchiveInvoiceListRequest>
  </soapenv:Body>
</soapenv:Envelope>
```

Şemadaki tam sıra: `REQUEST_HEADER`, `LIMIT`, `ID`, `UUID`, `START_DATE`, `END_DATE`, `PERIOD`, `PREFIX`, `REPORT_INCLUDED`, `REPORT_FLAG`, `HEADER_ONLY`, `CONTENT_TYPE`, `READ_INCLUDED`.

| Alan | Şema | Portal değeri | Not |
|---|---|---|---|
| `LIMIT` | `int` | Liste 1000, içerikli 100 | Yanıtta en fazla 10.000 `INVOICE` (şema) |
| `START_DATE`, `END_DATE` | `xsd:dateTime` | Pencere | Fatura tarihi mi oluşturma tarihi mi şemada yok ⚠️ (§10) |
| `UUID` | | Tek fatura | |
| `HEADER_ONLY` | `token` | `Y` / `N` | |
| `CONTENT_TYPE` | `string` | `XML` | İzibiz test kodu |
| `READ_INCLUDED` | `string` | `Y` | Burada `boolean` değil `Y`/`N` (e-fatura servisinden farklı) |
| `PERIOD`, `PREFIX`, `REPORT_*` | | Gönderilmez | |

İzibiz test kodu bu çağrıyı `COMPRESSED=Y` ile yapıp her `CONTENT`'i ZIP olarak açıyor.

### 6.2 Yanıt

`GetEArchiveInvoiceListResponse`: `REQUEST_RETURN`, `ERROR_TYPE`, `INVOICE[]` (`EARCHIVEINV`, en fazla 10.000) → `HEADER` + `CONTENT`.

| İzibiz (`HEADER/…`) | Portal |
|---|---|
| `UUID` | `uuid` |
| `INVOICE_ID` | `invoiceNumber` |
| `ISSUE_DATE` | `issueDate` (şemada `string`) |
| `CUSTOMER_IDENTIFIER`, `CUSTOMER_NAME` | `counterpartyIdentifier`, `counterpartyName` |
| `SENDER_IDENTIFIER`, `SENDER_NAME` | Mükellefin kendisi (kontrol) |
| `PROFILE_ID` | `profileId` |
| `INVOICE_TYPE` | `invoiceTypeCode` |
| `EARCHIVE_TYPE`, `SENDING_TYPE` | `raw` |
| `STATUS`, `STATUS_CODE` | `statusRaw` (§7) |
| `PAYABLE_AMOUNT`, `TAXABLE_AMOUNT`, `CURRENCY_CODE` | Sadece liste ve kontrol (şemada `string`) |
| `REPORTED` | `raw` (GİB'e raporlandı mı) |
| `CONTENT` | UBL (ZIP) → `31` |

`docKind = 'e_archive'`, `direction = 'outgoing'`.

### 6.3 Dönem raporu kontrolü (isteğe bağlı)

`GetEArchiveReport` (`REPORT_PERIOD=YYYYMM`) → rapordaki faturalar. GİB'e raporlanan e-arşiv listesiyle portal listesi karşılaştırılabilir. İlk sürümde gerekmez.

## 7. Durumlar

`STATUS`, `STATUS_CODE`, `GIB_STATUS_CODE` değer listeleri şemada ve test kodunda **yok** ⚠️. İzibiz'den alınıp `30` §8 kurallarına göre tablo yazılır.

O zamana kadar geçici kural:

| Durum | `status_normalized` |
|---|---|
| `RESPONSE_CODE` / `STATUS` metni red bildiriyorsa (ör. "RED", "REJECT") | `cancelled` + `REJECTED` |
| E-arşivde `STATUS` iptal bildiriyorsa | `cancelled` |
| Diğer | `active` + uyarı `STATUS_UNVERIFIED` |

EDM ile aynı servis ailesinden göründüğü için EDM'deki `İŞLEM-SONUÇ` değerleri (`35` §4) başlangıç noktası olabilir; doğrulanmadan kullanılmaz.

**Toplu durum:** `GetInvoiceStatusAll` (`UUID` tekrarlanabilir) → `INVOICE_STATUS[]`: `STATUS`, `STATUS_DESCRIPTION`, `GIB_STATUS_CODE`, `GIB_STATUS_DESCRIPTION`, `RESPONSE_CODE`, `RESPONSE_DESCRIPTION`, `CDATE`, `ENVELOPE_IDENTIFIER`, `STATUS_CODE`, `DIRECTION` …

```xml
<w:GetInvoiceStatusAllRequest>
  <REQUEST_HEADER><SESSION_ID>{SESSION_ID}</SESSION_ID></REQUEST_HEADER>
  <UUID>{ETTN-1}</UUID>
  <UUID>{ETTN-2}</UUID>
</w:GetInvoiceStatusAllRequest>
```

## 8. Hatalar

Hata iki yerde gelebilir (şema):

- SOAP Fault, detayında `RequestFault` (`REQUEST_ERRORType`)
- Yanıt gövdesinde `ERROR_TYPE` (`REQUEST_ERRORType`)

`REQUEST_ERRORType`: `INTL_TXN_ID`, `CLIENT_TXN_ID`, `ERROR_CODE` (int), `ERROR_SHORT_DES`, `ERROR_LONG_DES`, `STACKTRACE`, `ERROR_ELEMENT_INDEX`.

`providerCode = ERROR_CODE`, `providerMessage = ERROR_SHORT_DES`. Kod listesi yok; test ortamında doldurulur:

| Durum | `ERROR_CODE` / metin | Sebep |
|---|---|---|
| Yanlış kullanıcı/şifre | (test ortamında) | `auth_failed` |
| Oturum geçersiz / süresi dolmuş | (test ortamında) | `session_expired` |

Başarılı yanıtlarda `REQUEST_RETURN/RETURN_CODE = 0` (İzibiz test kodu bunu kontrol ediyor).

## 9. Adaptör iskeleti

```ts
const LIMIT = 1000;   // §5.1, üst sınır test ortamında doğrulanır

export class IzibizAdapter implements EntegratorAdapter {
  readonly id = 'izibiz' as const;
  readonly capabilities = ['incoming', 'outgoing', 'e_archive', 'ubl', 'pdf', 'status'] as const;

  async verify(conn) {
    await this.withSession(conn, async s => {
      await this.client.getInvoice(s, { direction: 'IN', dateType: 'CREATE', ...lastDays(7), limit: 1, headerOnly: true });
    });
    return { ok: true };
  }

  async *listDocuments(conn, q) {
    const s = await this.client.login(conn);
    try {
      const pending = [{ from: q.from, to: q.to }];
      while (pending.length) {
        const w = pending.shift()!;
        const res = q.docKind === 'e_invoice'
          ? await this.client.getInvoice(s, { direction: q.direction === 'incoming' ? 'IN' : 'OUT',
              dateType: q.dateField === 'created' ? 'CREATE' : 'ISSUE', start: w.from, end: w.to,
              limit: LIMIT, headerOnly: true })
          : await this.client.getEArchiveInvoiceList(s, { start: w.from, end: w.to, limit: LIMIT, headerOnly: true });
        if (res.items.length >= LIMIT && daysBetween(w.from, w.to) > 1) {
          pending.unshift(...splitInHalf(w));                          // 37 §5.3; sonuç atılır, iki yarı tekrar çekilir
          continue;
        }
        if (res.items.length >= LIMIT) warn('IZIBIZ_LIMIT_REACHED', w);
        yield { items: res.items.map(i => mapIzibiz(i, q)) };
      }
    } finally {
      await this.client.logout(s);
    }
  }

  downloadUbl(conn, ref) {
    return this.withSession(conn, s => ref.docKind === 'e_invoice'
      ? this.client.getInvoiceContent(s, ref.uuid, ref.direction === 'incoming' ? 'IN' : 'OUT')  // §5.4
      : this.client.getEArchiveInvoiceContent(s, ref.uuid));                                        // §6.1, UUID ile
  }

  queryStatus(conn, uuids) {
    return this.withSession(conn, s => this.client.getInvoiceStatusAll(s, uuids));               // §7
  }
}
```

`client.ts` içinde sadece `Login`, `Logout`, `GetInvoice`, `GetInvoiceCount`, `GetInvoiceWithType`, `GetInvoiceStatusAll`, `GetEArchiveInvoiceList` (ve isteğe bağlı `GetEArchiveReport`) vardır.

**Kullanılmayacak (durum değiştirir):** `MarkInvoice` (okundu / okunmadı işareti koyar), `MarkEnvelope`, `SendInvoice`, `LoadInvoice`, `SendInvoiceResponse`, `SendInvoiceResponseWithServerSign`, `PrepareInvoiceResponse`, `CancelDraftInvoice`, `ArchiveInvoice`; e-arşivde `WriteToArchive*`, `CancelEArchiveInvoice`, `MarkEArchiveInvoice`, `CopyToArchive`, `SendSmsEarchiveInvoice`, `GetEmailEarchiveInvoice` (`30` §5).

## 10. Senkron planı

| İş | Çağrı | Pencere |
|---|---|---|
| Kurulum | `Login` + `GetInvoice` (1 kayıt) + `Logout` | — |
| İlk yükleme | `GetInvoice` (`IN`, `OUT`, `DATE_TYPE=ISSUE`) + `GetEArchiveInvoiceList` | İçinde bulunulan yıl + önceki yıl, aylık |
| Artımlı (60 dk) | `GetInvoice` (`DATE_TYPE=CREATE`) + `GetEArchiveInvoiceList` | Son 3 gün |
| Gece tam tarama | Aynı, `DATE_TYPE=ISSUE` | İçinde bulunulan ay + önceki ay |
| Durum (günde 1) | `GetInvoiceStatusAll` | `37` §5.5 listesi |

E-arşiv listesinin tarih alanı (fatura / oluşturma) bilinmediği için artımlı çekimde son 3 gün yerine **son 7 gün** kullanılır; gece taraması zaten eksikleri yakalar.

## 11. Test planı (İzibiz'e özel)

1. Elle yazılmış XML ile `Login` (ad alanı kuralı §3); başarısızsa alt elemanlar ad alanlı denenir, sonuç buraya yazılır.
2. Kimlik servisinden alınan `SESSION_ID` e-fatura ve e-arşiv servisinde çalışıyor mu.
3. `GetInvoice` `READ_INCLUDED=false` ve `true` farkı; listeleme ve içerik indirme sonrası faturanın "okunmamış" durumu **değişmiyor** mu (`30` §5).
4. `GetInvoiceCount` `PORTAL` ve `WS` sayılarının anlamı.
5. `LIMIT` üst sınırı (1000 kabul ediliyor mu).
6. `GetEArchiveInvoiceList` `START_DATE` / `END_DATE` hangi tarihe göre süzüyor.
7. `STATUS` / `STATUS_CODE` değerleri: kabul edilmiş, reddedilmiş, onay bekleyen ticari fatura; iptal e-arşiv.
8. Yanlış şifre ve geçersiz oturumla `ERROR_CODE` değerleri (§8 tablosu).
9. `30` §9 kabul testleri.

## 12. İzibiz'den istenecekler

- [ ] Canlı servis adresleri; e-fatura için `EFaturaOIB` mi `EInvoiceWS` mi
- [ ] Durum kodları listesi (`STATUS`, `STATUS_CODE`, `GIB_STATUS_CODE`)
- [ ] Hata kodları listesi (`ERROR_CODE`)
- [ ] `LIMIT` üst sınırı, oturum süresi, hız sınırı
- [ ] `GetInvoiceCount` yanıtındaki `PORTAL` / `WS` anlamı
- [ ] Entegratör yazılım firması için erişim modeli

## Kaynaklar

- https://github.com/izibiz/ws-client-dotnet (`izibiz.Application/izibiz.SERVICES/Connected Services/serviceAuth`, `serviceOib`, `serviceArchive`: WSDL ve XSD)
- https://github.com/izibiz/ws-client-php (`Izibiz_php_soap_client/tests/AuthenticationTest.php`, `EInvoiceTest.php`, `EAchiveInvoiceTest.php`)
