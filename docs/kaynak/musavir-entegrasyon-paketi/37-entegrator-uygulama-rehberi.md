# 37 · Entegratör Adaptörleri: Uygulama Rehberi

İşNet (`32`), Uyumsoft (`33`) ve İzibiz (`36`) adaptörlerinin **ortak** parçaları burada. Entegratör dosyaları sadece kendi farklarını anlatır. QNB eFinans ve EDM için Easytrade'in mevcut istemcileri kullanılır (`34`, `35`); bu dosyadaki senkron akışı, hata modeli ve indeks tablosu onlar için de geçerlidir.

Sıra: önce bu dosya, sonra `31` (UBL ayrıştırıcı), sonra entegratör dosyası.

## 1. Kod yerleşimi (öneri)

```
integrations/entegrator/
  core/
    soap-transport.ts   # HTTP POST, zaman aşımı, sabit çıkış IP'si, log maskeleme (§2)
    xml.ts              # güvenli ayrıştırıcı, ad alanından bağımsız okuma (§3)
    payload.ts          # base64 → ZIP / XML / PDF ayırma (§3.4)
    errors.ts           # EntegratorError (§4)
    windows.ts          # tarih penceresi bölme (§5.3)
    sync-job.ts         # ortak senkron akışı (§5)
  isnet/  uyumsoft/  izibiz/
    client.ts           # SADECE okuma metotları
    envelopes.ts        # istek şablonları (entegratör dosyasındaki XML'ler)
    mapper.ts           # yanıt → EntegratorDocHeader (30 §6)
    status.ts           # ham durum → status_normalized
    adapter.ts          # EntegratorAdapter uygulaması
    fixtures/           # test ortamından alınmış, maskelenmiş yanıtlar
  qnb/  edm/
    adapter.ts          # Easytrade'in mevcut istemcisini saran ince katman
```

Kod örnekleri TypeScript (`30` §6 ile aynı). Backend başka dildeyse yapı aynen taşınır.

**WSDL'den kod üretmek mi, elle şablon mu?** İkisi de olur. Her entegratörde 3–6 okuma metodu kullanılıyor; entegratör dosyalarında her çağrının tam istek XML'i var. Elle şablon kullanılırsa **eleman sırası XSD'deki sırayla aynı olmalı** (İşNet'te ayrıca alfabetik, `32` §3). Kod üretilirse üretilen istemcinin sadece okuma metotları dışarı açılır (`30` §5).

## 2. SOAP taşıma katmanı

| Konu | Kural |
|---|---|
| Sürüm | Üçü de SOAP 1.1 (`http://schemas.xmlsoap.org/soap/envelope/`) |
| Başlıklar | `Content-Type: text/xml; charset=utf-8`, `SOAPAction` entegratör dosyasında, `Accept-Encoding: gzip` |
| Çıkış IP'si | Bütün entegratör çağrıları `fixed-egress` havuzundan (`20` §4, `30` §4.2). İşNet'te zorunlu, diğerlerinde tutarlılık için. |
| Bağlantı zaman aşımı | 10 sn (öneri) |
| Okuma zaman aşımı | XML içeren liste çağrısı 120 sn, diğerleri 30 sn (öneri) |
| Yanıt boyutu | Yanıt belleğe değil geçici dosyaya akıtılır. 50 MB üstü yanıtta çağrı iptal edilir ve sayfa boyutu yarıya indirilerek tekrar edilir. |
| TLS | Sertifika doğrulaması kapatılmaz |
| Log | İstek/yanıt gövdesi **maskelenerek** nesne deposuna, 7 gün. Maskelenecek: şifre, `UsernameToken`, `SESSION_ID`, `PASSWORD`. VKN loglarda son 4 hane dışında maskelenir. |
| Eşzamanlılık | Bağlantı başına aynı anda 1 liste çağrısı, 2 UBL indirme. İşNet'te bütün çağrılar aynı IP'den çıktığı için **entegratör geneli** üst sınır (öneri: 4 eşzamanlı çağrı) |

## 3. XML ayrıştırma

### 3.1 Güvenlik

DTD reddedilir, dış varlık (XXE) çözümü kapalı, varlık genişletme kapalı. UBL ayrıştırıcıda (`31`) da aynı ayar.

### 3.2 Ad alanları

Yanıtlardaki önekler (`a:`, `b:`, `s:`, `i:`) sunucuya göre değişir. Elemanlar **yerel ad + ad alanı URI** ile okunur, önekle değil. `i:nil="true"` olan eleman `null`.

### 3.3 Değer tipleri

| Tip | Kural |
|---|---|
| Tutar | `string` olarak al, ondalık kütüphanesiyle işle. `float` kullanılmaz. |
| Tarih-saat | Saat dilimi yoksa `Europe/Istanbul` kabul edilir; varsa olduğu gibi. Fatura tarihi yine UBL'den alınır (`31` §2). |
| WCF enum (İşNet, Uyumsoft) | XML'de **ad** olarak gelir (`Success`, `Onaylandi`, `Approved`). Ayrıştırıcı hem adı hem sayıyı kabul eder. |
| `byte[]` / `base64Binary` | Base64 çöz, sonra §3.4 |

### 3.4 İçerik türünü ayırma

Base64 çözüldükten sonra ilk baytlara bakılır:

| İlk baytlar | Tür | İşlem |
|---|---|---|
| `50 4B 03 04` (`PK`) | ZIP | Aç; içindeki her `.xml` ayrı UBL. Tek dosya bekleniyorsa ve birden fazla varsa uyarı. |
| `EF BB BF` | UTF-8 BOM | BOM'u at, tekrar bak |
| `<` veya `<?xml` | XML | `31`'e ver |
| `%PDF` | PDF | `attachment` (`kind=pdf`) |
| Diğer | Bilinmeyen | Hata `payload_unknown`, ham veri saklanır |

## 4. Hata modeli

```ts
type EntegratorErrorReason =
  | 'network'            // DNS, bağlantı, zaman aşımı
  | 'http_5xx'
  | 'soap_fault_server'  // faultcode s:Server
  | 'soap_fault_client'  // faultcode s:Client (istek hatalı)
  | 'auth_failed'        // kullanıcı/şifre yanlış, yetki yok
  | 'ip_rejected'        // İşNet: IP tanımlı değil (metin test ortamında belirlenir)
  | 'vkn_not_authorized' // İşNet: bu VKN için yetki yok (metin test ortamında belirlenir)
  | 'session_expired'    // İzibiz: oturum düşmüş
  | 'provider_error'     // Result=Failed / IsSucceded=false / ERROR_TYPE, sınıflanamayan
  | 'payload_unknown';

class EntegratorError extends Error {
  reason: EntegratorErrorReason;
  retryable: boolean;
  entegrator: EntegratorId;
  operation: string;          // ör. 'SearchAllInvoice'
  httpStatus?: number;
  faultCode?: string;
  providerCode?: string;      // İzibiz ERROR_CODE vb.
  providerMessage?: string;   // entegratörün metni, olduğu gibi
}
```

**Sınıflandırma sırası:** HTTP hatası → SOAP Fault → yanıt gövdesindeki iş hatası (İşNet `Result=Failed` + `ErrorMessage`, Uyumsoft `IsSucceded=false` + `Message`, İzibiz `ERROR_TYPE`).

İş hatalarının metinleri dokümanlarda listelenmemiş. Bu yüzden:

- Bilinmeyen metin `provider_error` olur, 1 kez tekrar denenir, ham yanıt saklanır.
- Test ortamında karşılaşılan metinler entegratör dosyasındaki **"Bilinen hata metinleri"** tablosuna eklenir ve doğru sebebe bağlanır. Metin uydurulmaz.

| Sebep | Tekrar | Bağlantıya etkisi |
|---|---|---|
| `network`, `http_5xx`, `soap_fault_server` | Üstel geri çekilme 1-2-4-8-16 sn, en fazla 5 (`02` §5) | Yok. 5 denemeden sonra pencere `failed`, bir sonraki çalışmada tekrar |
| `session_expired` | 1 kez yeniden `Login` + aynı çağrı | Yok |
| `auth_failed` | Yok | `source_connection.status = auth_failed`, mükellefe "bağlantıyı yenile" bildirimi |
| `ip_rejected`, `vkn_not_authorized` | Yok | `auth_failed` + iç uyarı (Easytrade operasyonu, mükellef değil) |
| `soap_fault_client` | Yok | Yok. Ham istek ve yanıt saklanır, geliştirici uyarısı |
| `provider_error` | 1 | Yok |
| `payload_unknown` | Yok | Yok. Fatura `ubl_status=failed` |

## 5. Senkron akışı

### 5.1 İndeks tablosu `entegrator_document` ⊕

Listeden görülen her belge önce buraya yazılır. UBL indirme, durum güncelleme ve "listede var ama UBL yok" kontrolü bu tablo üzerinden yürür.

| Alan | Tip | Açıklama |
|---|---|---|
| id | UUID | |
| company_id | UUID | |
| connection_id | UUID | `source_connection.id` |
| uuid | TEXT | ETTN, küçük harfe çevrilmiş |
| doc_kind | ENUM | `e_invoice`, `e_archive` |
| direction | ENUM | `incoming`, `outgoing` |
| invoice_number | TEXT | Listeden |
| issue_date | DATE | Listeden (kontrol amaçlı; ortak modele UBL'deki yazılır) |
| created_at_source | TIMESTAMPTZ | Entegratördeki oluşturma / geliş zamanı |
| status_raw | TEXT | Ham durum (ad ve kod birlikte, ör. `Onaylandi(1)`) |
| status_normalized | ENUM | `30` §8 |
| warnings | TEXT[] | `PENDING_RESPONSE`, `REJECTED` … |
| header_raw | JSONB | Liste yanıtındaki öğenin tamamı |
| ubl_status | ENUM | `pending`, `ok`, `failed`, `not_available` |
| ubl_attachment_id | UUID | `attachment.id` |
| invoice_id | UUID | Oluşan `invoice.id` |
| first_seen_at, last_seen_at | TIMESTAMPTZ | |
| missing_since | TIMESTAMPTZ | Tam taramada görülmediği ilk an (§5.5) |

Tekil anahtar: `(connection_id, uuid, direction)`.

### 5.2 Liste işi

```ts
async function syncWindow(conn, adapter, q /* docKind, direction, from, to, dateField */) {
  for await (const batch of adapter.listDocuments(conn, q)) {
    for (const h of batch.items) {
      const prev = await idx.find(conn.id, h.uuid, h.direction);
      const status = adapter.normalizeStatus(h.statusRaw, h);
      await idx.upsert({ ...h, status, last_seen_at: now(), missing_since: null });
      const needUbl = !prev || prev.ubl_status !== 'ok';
      const statusChanged = prev && prev.status_raw !== h.statusRaw;
      if (needUbl) queue.ubl.push({ conn, ref: h });
      if (statusChanged && prev.invoice_id) await invoices.applyStatus(prev.invoice_id, status);
    }
  }
  await windows.markDone(conn.id, q);   // sync_window tablosu (01 §4.17)
}
```

### 5.3 Pencere bölme

Entegratörler toplam kaydı farklı verir. Ortak kural:

1. Pencere ile liste çağrısı yapılır.
2. Toplam kayıt **eşiği** aşıyorsa (İşNet `TotalNumberOfRecords`, Uyumsoft `TotalCount`; İzibiz'de `GetInvoiceCount` veya dönen kayıt sayısı = `LIMIT`) pencere ikiye bölünür.
3. Bölme 1 güne kadar iner. 1 günlük pencere hâlâ büyükse sayfalama ile devam edilir.

Eşik öneri değerleri entegratör dosyasında. Amaç tek pencerenin dakikalar sürmemesi ve yarıda kalınca baştan alınabilmesi.

### 5.4 UBL işi

```ts
async function fetchUbl(conn, adapter, ref) {
  const buf = await adapter.downloadUbl(conn, ref);           // ya da liste yanıtında geldiyse oradan
  for (const xml of extractXmlDocuments(buf)) {               // §3.4
    const att = await attachments.save(xml, 'ubl_xml');       // SHA-256 ile tekil
    const inv = await ubl.parseAndUpsert(xml, conn.company_id, {
      source_system: `entegrator_${adapter.id}`, source_id: ref.uuid,
    });                                                        // 31 + 01 §6 mükerrer kontrolü
    await idx.update(conn.id, ref, { ubl_status: 'ok', ubl_attachment_id: att.id, invoice_id: inv.id });
  }
}
```

- Liste yanıtında XML geliyorsa (İşNet `IsXMLIncluded=true`, İzibiz `HEADER_ONLY=N`) ayrı indirme yapılmaz; aynı fonksiyon liste yanıtındaki içerikle çağrılır.
- UBL'deki `cbc:UUID` ile listedeki ETTN farklıysa kayıt yazılmaz, uyarı `UUID_MISMATCH`.
- UBL'deki taraf VKN'si bağlantının VKN'siyle uyuşmuyorsa `31` §4 `PARTY_MISMATCH`.

### 5.5 Zamanlama

`30` §7'deki tablo geçerli. Ek kurallar:

| İş | Kural |
|---|---|
| Artımlı (60 dk) | `dateField='created'`, son 3 gün. Pencere sonu = şimdi. |
| Gece tam tarama | İçinde bulunulan ay + önceki ay, `dateField='issue'`. Önceden görülüp bu taramada görülmeyen belgeye `missing_since` yazılır, 2 ardışık taramada yoksa uyarı `MISSING_AT_SOURCE`. **Silinmez.** |
| Durum güncelleme (günde 1) | Son 60 günün `active` + `PENDING_RESPONSE` belgeleri ve bütün giden belgeler. Toplu durum metodu varsa o (Uyumsoft `Query*InvoiceStatus`, İzibiz `GetInvoiceStatusAll`), yoksa ETTN ile liste sorgusu (İşNet). |
| UBL tekrar | `ubl_status=failed` olanlar 6 saatte 1, en fazla 5 kez |

Artımlı ve gece taramasının ikisi birden çalıştığı için, artımlı çekimde kullanılan tarih alanının anlamında bir sürpriz çıksa bile (ör. gelen faturada oluşturma tarihi beklenenden farklı) belge en geç gece taramasında yakalanır.

### 5.6 Kilitler

Aynı bağlantıda aynı anda tek senkron işi (`02` §5). UBL kuyruğu ayrı çalışır, liste işini beklemez.

## 6. Test yaklaşımı

| Katman | Nasıl |
|---|---|
| Birim | `fixtures/` altındaki gerçek yanıtlarla (test ortamından alınmış, VKN ve isimler maskelenmiş) eşleyici ve durum tablosu testleri |
| Sözleşme | Test ortamına gece 1 kez: giriş, 1 sayfa liste, 1 UBL indirme. Kırılırsa geliştirici uyarısı. |
| Kabul | `30` §9 listesi, önce test ortamında, sonra ilk gerçek mükellefte |

Test ortamı erişimi:

| Entegratör | Test erişimi |
|---|---|
| İşNet | İşNet'ten alınır. Test ortamında da IP tanımı gerekebilir ⚠️ (`32` §2) |
| Uyumsoft | Dokümandaki test kullanıcısı `Uyumsoft` / `Uyumsoft` (`33` §1) |
| İzibiz | İzibiz'in kendi test projesindeki kullanıcı `izibiz-test2` / `izi321` (`36` §1). Kendi test hesabımız için İzibiz'e başvurulur. |

İlk iş olarak her entegratörde **gerçek yanıtlar kaydedilir** ve fixture yapılır. Entegratör dosyalarındaki yanıt iskeletleri doküman tablolarından ve şemalardan çıkarıldı; gerçek yanıtla karşılaştırılmadan eşleyici bitmiş sayılmaz.

## 7. Adaptör bitti sayılmadan önce

- [ ] Sadece okuma metotları var, durum değiştiren metot istemcide yok (`30` §5)
- [ ] Fixture'lar test ortamından alındı, eşleyici testleri geçiyor
- [ ] Durum tablosundaki her değer en az bir fixture ile test edildi (test ortamında üretilebilenler)
- [ ] Pencere bölme ve sayfalama büyük bir ayda denendi
- [ ] Hata sınıflandırma: yanlış şifre / yetkisiz VKN / zaman aşımı senaryoları denendi, "Bilinen hata metinleri" tablosu dolduruldu
- [ ] Loglarda şifre, oturum anahtarı görünmüyor
- [ ] `30` §9 kabul testleri geçti

## 8. Yapılmayacaklar

- Okurken durum değiştiren parametre veya metot (Uyumsoft `SetTaken=true`, İzibiz `MarkInvoice`, İşNet `ArchiveInvoice` …).
- Liste yanıtındaki tutarları ortak modele yazmak. İçerik UBL'den gelir.
- Artımlı çekimi sadece fatura tarihine dayandırmak.
- İşNet'te `CompanyTaxCode`'u kullanıcı girdisinden almak (`32` §2).
- Tutarları `float` ile işlemek.
- XML'i önek adıyla okumak (`a:Result`).
- Bir sayfa hata verdi diye bütün dönemi baştan çekmek; pencere ve sayfa bazında kaldığı yerden devam edilir.
