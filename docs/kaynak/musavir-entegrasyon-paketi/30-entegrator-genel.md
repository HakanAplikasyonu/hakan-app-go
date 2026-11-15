# 30 · E-Fatura Entegratörleri: Genel Mimari

Mükellefin e-fatura ve e-arşiv faturalarını doğrudan **özel entegratörden** (İşNet, Uyumsoft, QNB eFinans, EDM, İzibiz …) çekmek için yapı.

| Dosya | İçerik |
|---|---|
| `30-entegrator-genel.md` | Bu dosya: neden, kapsam, bağlantı modeli, adaptör arayüzü, senkron, durum kuralları |
| `31-ubl-tr-esleme.md` | **UBL-TR XML → ortak model** (`invoice`, `invoice_item`, `invoice_tax_summary`). Bütün entegratörler için tek ayrıştırıcı. |
| `32-isnet.md` | İşNet (NetteFatura) |
| `33-uyumsoft.md` | Uyumsoft |
| `34-qnb-efinans.md` | QNB eFinans (Easytrade'de mevcut entegrasyon; portalın ihtiyaç listesi) |
| `35-edm.md` | EDM Bilişim (Easytrade'de mevcut entegrasyon; portalın ihtiyaç listesi) |
| `36-izibiz.md` | İzibiz |
| `37-entegrator-uygulama-rehberi.md` | **Uygulama rehberi:** SOAP katmanı, XML ayrıştırma, hata modeli, indeks tablosu, senkron akışı, test yaklaşımı. Adaptör yazmaya buradan başlanır. |

## 1. Neden ve öncelik

Müşavir görüşmesinden (Kasım 2026):

- Müşavir ofisleri faturayı ön muhasebe programından değil, **mükellefin e-fatura sisteminden** alıp Luca'ya aktarıyor.
- Bir ofisin mükelleflerinde dağılım: **yaklaşık %80 İşNet**, kalanı Uyumsoft, QNB (Finansbank) eFinans, EDM ve benzerleri.

Bu yüzden entegratör bağlantısı, ön muhasebe programı bağlantısından **önce** gelir. Mükellef hangi programı kullanırsa kullansın (veya hiç kullanmasın) e-fatura ve e-arşiv faturaları buradan gelir.

**Öncelik:** İşNet → Uyumsoft → İzibiz → diğerleri. QNB eFinans ve EDM için Easytrade'de mevcut entegrasyon var; portal onları mevcut istemci üzerinden kullanır (`34`, `35`).

**Not (Luca):** Luca Yazılım'ın kendi duyurusuna göre, e-fatura entegratörü olarak **TÜRMOB tercihini** kullanan firmaların muhasebe kayıtları Luca'ya otomatik aktarılıyor. TÜRMOB ile İşNet arasında e-fatura işbirliği anlaşması var (Dünya gazetesi haberi); İşNet payının yüksek olması bununla ilişkili olabilir, doğrulanmadı. Yani bu mükelleflerde faturalar Luca'ya zaten gidiyor olabilir. Portalın Luca'ya aktarımında **çift kayıt riskini** doğurur. Luca çıkış dokümanı yazılırken ele alınmalı (§10).

## 2. Entegratör ne verir, ne vermez

| Veri | Entegratörden gelir mi |
|---|---|
| Giden e-fatura (satış, iade, tevkifat, istisna …) | **Evet** |
| Giden e-arşiv faturası | **Evet** (e-arşiv servisi ayrı olabilir, entegratör dosyasında) |
| Gelen e-fatura | **Evet** (mükellef e-fatura kullanıcısıysa) |
| Faturanın UBL-TR XML'i (satırlar, oran bazında KDV, tevkifat, istisna kodu) | **Evet.** Beyan için en güvenilir kaynak. |
| Fatura durumu (kabul, red, iade, iptal) | **Evet** |
| Gelen kâğıt fatura | **Hayır** → OCR / Excel / ön muhasebe programı |
| Mükellef e-fatura kullanıcısı değilse ona kesilen e-arşiv faturalar | **Hayır** (alıcı tarafında entegratör gelen kutusuna düşmez) → OCR / yükleme |
| Masraf fişi, ÖKC fişi, Z raporu | **Hayır** |
| Kasa, banka, tahsilat, ödeme | **Hayır** → banka (`20`), ön muhasebe programı |
| Gelir / gider kategorisi | **Hayır** → portal kuralları, ön muhasebe programı |

Sonuç: Entegratör **faturaların** ana kaynağıdır. Diğer kalemler için ön muhasebe programı, banka ve yükleme kanalları devam eder.

## 3. Entegratörler: ne var, ne yok

| Entegratör | Öncelik | Kaynak seviyesi | Gelen e-fatura listesi | Giden e-fatura listesi | E-arşiv listesi | UBL XML indirme | Kimlik doğrulama |
|---|---|---|---|---|---|---|---|
| **İşNet** (`32`) | 1 | ✅ İşNet Web Servis Arayüz Dokümanları (V2, 12.08.2024) · adresler topluluk SDK'sından ⚠️ | Var (`SearchAllInvoice`, yön=Gelen) | Var (yön=Giden) | Var (`SearchArchiveInvoice`) | Var (`IsXMLIncluded`) | **Kullanıcı/şifre yok**: firma VKN'si + İşNet'in tanımladığı **sabit IP** ⚠️ (§4.1) |
| **Uyumsoft** (`33`) | 2 | ✅ Uyumsoft Müşteri Web Servisi Dokümanı (v2.0.6, 2018) | Var (`GetInboxInvoiceList`) | Var (`GetOutboxInvoiceList`) | Giden kutusunda (aynı servis) | Var (`GetInboxInvoiceData` / `GetOutboxInvoiceData`) | Mükellefin kullanıcı adı/şifresi (WS-Security) |
| **QNB eFinans** (`34`) | Mevcut | **Easytrade'de mevcut entegrasyon.** Portalın ihtiyaç listesi `34` §2'de; eksik metot için topluluk notları ⚠️ | Mevcut istemciye göre | Mevcut istemciye göre | Bulunan kaynaklarda liste ucu yok | Var | Kullanıcı adı/şifre + VKN |
| **EDM** (`35`) | Mevcut | **Easytrade'de mevcut entegrasyon.** Portalın ihtiyaç listesi `35` §2'de; eksik metot için topluluk notları ⚠️ | Mevcut istemciye göre | Mevcut istemciye göre | ⚠️ | Var | Kullanıcı adı/şifre → `Login` → `SESSION_ID` |
| **İzibiz** (`36`) | 3 | ✅ İzibiz'in kendi GitHub istemcileri (WSDL/XSD dahil, test ortamı) | Var (`GetInvoice`, `DIRECTION=IN`, tarih aralığı) | Var (`DIRECTION=OUT`) | Var (`GetEArchiveInvoiceList`) | Var (`CONTENT` UBL) | Kullanıcı adı/şifre → `Login` → `SESSION_ID` |

**Hiçbir entegratör Easytrade tarafından canlı hesapla test edilmedi.** Her adaptör §8 kabul testlerini test ortamında ve ilk gerçek mükellefte geçmeden üretime alınmaz.

## 4. Bağlantı modeli

### 4.1 İki farklı model var

| Model | Entegratörler | Mükellefin portala girdiği | Easytrade'in yapacağı |
|---|---|---|---|
| **Mükellef kimlik bilgisi** | Uyumsoft, İzibiz; QNB eFinans ve EDM (mevcut istemci, mükellefin kendi bilgileriyle) | Entegratör portal / web servis kullanıcı adı ve şifresi (QNB'de + VKN) | Bilgileri kasada sakla (`02` §4). Mükellefe **sadece entegrasyon için ayrı kullanıcı** açtırılması önerilir. |
| **IP + VKN yetkilendirmesi** | İşNet | Hiçbir şifre yok. Mükellef İşNet'e "Easytrade'in IP'sinden benim VKN'im adına sorgu yapılabilsin" onayı verir. | Sabit çıkış IP'sinin İşNet'e tanımlatılması. İşNet'in VKN bazında yetki süreci netleştirilmeli ⚠️ (`32` §2). |

İşNet servislerinde istek gövdesinde sadece `CompanyTaxCode` (VKN) ve isteğe bağlı `CompanyVendorNumber` (bayi numarası) var; kimlik doğrulama alanı yok. İşNet dokümanı canlı ortam erişiminin **IP tanımıyla** açıldığını söylüyor. Bu yüzden İşNet tarafında Easytrade'in hangi statüde (çözüm ortağı / bayi / mükellef adına yetkili) erişeceği İşNet ile görüşülmeden adaptör canlıya alınamaz. Bu, İşNet'in %80 payı nedeniyle **en kritik açık iş.**

### 4.2 Sabit IP

Entegratörlerin IP kısıtı olabilir (İşNet'te kesin). Banka için kurulan **bank-egress** sabit çıkış IP'leri (`20` §4) entegratör çağrıları için de kullanılır. Ayrı IP gerekmez; worker havuzu adı `fixed-egress` olarak genelleştirilir.

### 4.3 Bağlantı durumları

`draft` → `credentials_entered` (veya İşNet'te `authorization_requested`) → `verifying` → `active` / `failed`.

Doğrulama: entegratör dosyasındaki `verify` adımı (İşNet `GetTaxPayer` + tek liste çağrısı, Uyumsoft `WhoAmI` ile VKN kontrolü, İzibiz `Login` + tek liste çağrısı). Başarılıysa `active`.

## 5. Okuma kuralları (çok önemli)

Entegratör servislerinin bir kısmı **okurken durum değiştiren** parametreler içeriyor. Mükellefin kendi ERP'si veya muhasebe programı bu durumlara güveniyor olabilir. Portal **asla** mükellefin entegratördeki durumunu değiştirmez:

| Entegratör | Kullanılmayacak / sabit değer |
|---|---|
| Uyumsoft | `SetTaken=false`, `OnlyNewestInvoices=false`. `SetInvoicesTaken`, `ChangeInvoiceArchiveStatus`, `SendDocumentResponse` **çağrılmaz.** |
| İzibiz | `READ_INCLUDED=true` (okunmuşlar dahil gelsin). `MarkInvoice`, `SendInvoiceResponse*` **çağrılmaz.** |
| İşNet | `ArchiveInvoice`, `DeArchiveInvoice`, `UpdateInvoiceState`, `SendInvoiceReply` **çağrılmaz.** Arşivlenmiş faturalar için `IsArchiveIncluded=true`. |
| QNB eFinans | `belgelerAlindiMi` parametresinin etkisi doğrulanmadan gönderilmez ⚠️. Mevcut istemci gönderiyorsa portal çağrılarında kapatılır. |
| EDM | Sadece `Login`, `Logout`, `GetInvoice`, `GetInvoiceStatus`. |
| Hepsi | Fatura gönderme, taslak, iptal, kabul/red gönderme uçları **kullanılmaz.** |

Adaptör kodunda bu uçlar için istemci metodu üretilmez (WSDL'den kod üretildiyse sarmalayıcı sadece okuma metotlarını açar).

## 6. Adaptör arayüzü

```ts
type EntegratorId = 'isnet' | 'uyumsoft' | 'qnb_efinans' | 'edm' | 'izibiz';
type DocKind = 'e_invoice' | 'e_archive';
type Direction = 'incoming' | 'outgoing';

interface EntegratorDocHeader {
  uuid: string;                 // ETTN
  invoiceNumber: string;
  docKind: DocKind;
  direction: Direction;
  issueDate: string;            // YYYY-MM-DD
  createdAt?: string;           // entegratöre düşme / oluşturma zamanı (artımlı çekim için)
  counterpartyIdentifier?: string; // VKN/TCKN
  counterpartyName?: string;
  payableAmount?: string;
  currency?: string;
  profileId?: string;           // ham
  invoiceTypeCode?: string;     // ham
  statusRaw: string;            // entegratörün ham durum kodu/adı
  raw: unknown;
}

type EntegratorBatch = { items: EntegratorDocHeader[]; total?: number };   // total: kaynağın verdiği toplam kayıt (pencere bölme için, 37 §5.3)

interface EntegratorAdapter {
  readonly id: EntegratorId;
  readonly capabilities: Array<'incoming' | 'outgoing' | 'e_archive' | 'ubl' | 'pdf' | 'status'>;

  // Bağlantıyı doğrular; bağlantıya özel bilgiyi (e-fatura kullanıcısı mı, birimler…) source_connection.meta'ya,
  // verebildiği kalemleri source_connection.capabilities'e yazar.
  verify(conn: EntegratorConnection): Promise<HealthStatus>;
  listDocuments(conn: EntegratorConnection, q: {
    docKind: DocKind | 'any';                // 'any': kaynak e-fatura ve e-arşivi tek listede veriyorsa (Uyumsoft giden kutusu)
    direction: Direction;
    from: string; to: string;               // tarih penceresi (ISO)
    dateField: 'issue' | 'created';          // entegratör destekliyorsa 'created' tercih
    page?: number;
  }): AsyncIterable<EntegratorBatch>;
  normalizeStatus(statusRaw: string, h: EntegratorDocHeader): { status: 'draft' | 'active' | 'cancelled'; warnings: string[] };  // entegratör dosyasındaki durum tablosu
  downloadUbl(conn: EntegratorConnection, ref: { uuid: string; docKind: DocKind; direction: Direction }): Promise<Buffer>;
  downloadPdf?(conn: EntegratorConnection, ref: { uuid: string; docKind: DocKind; direction: Direction }): Promise<Buffer>;
  queryStatus?(conn: EntegratorConnection, uuids: string[], direction: Direction): Promise<Array<{ uuid: string; statusRaw: string }>>;
}
```

Kurallar:

- **Fatura içeriği her zaman UBL XML'den** üretilir (`31`). Entegratörün liste yanıtındaki başlık alanları (tutar, karşı taraf) sadece liste ekranı ve kontrol için kullanılır, ortak modele yazılmaz.
- UBL indirilemezse fatura `has_items=false`, `tax_summary_source=header_estimate` olarak yazılır ve uyarı üretilir (`01` §4.6).
- Ham UBL XML `attachment` (`kind=ubl_xml`) olarak saklanır. PDF varsa ayrıca indirilir (ayrı kuyrukta).
- `source_system` = `entegrator_<id>` (ör. `entegrator_isnet`), `source_id` = ETTN.

## 7. Senkron

| İş | Pencere / sıklık |
|---|---|
| İlk yükleme | İçinde bulunulan yıl + önceki yıl, **aylık** pencereler (entegratör daha dar istiyorsa haftalık). Her yön ve belge türü ayrı. |
| Artımlı | 60 dakikada 1, son 3 günün **oluşturma / geliş tarihi** penceresi (entegratör destekliyorsa). Desteklemiyorsa fatura tarihi penceresi. |
| Durum güncelleme | Son 60 günün faturalarının durumları günde 1 (kabul/red/iade/iptal sonradan değişir). |
| Açık dönem tam tarama | Gece 1, içinde bulunulan ay + önceki ay. Silinen / iptal edilen yakalanır. |
| Müşavir "şimdi çek" | Anlık, 5 dakikada 1. |

- **Neden fatura tarihi değil oluşturma tarihi:** Geçmiş tarihli kesilen veya geç düşen faturalar fatura tarihi penceresinde kaçırılır. İşNet (`MinInvoiceCreationDate`), Uyumsoft (`CreateStartDate`), İzibiz (`DATE_TYPE=CREATE`) bunu destekliyor. Gelen faturada İşNet'in ayrıca `MinDirectionDate` (son yönlendirme tarihi) alanı var.
- Dönem kilidi `02` §3.4 ile aynı: kilitli döneme sonradan düşen fatura `change_request` olur.
- Hız sınırı entegratör dokümanlarında belirtilmemiş. Güvenli varsayılan: bağlantı başına saniyede 1 istek, UBL indirme paralelliği 2.

## 8. Durum kuralları (beyana girer mi)

Her entegratörün ham durumu ortak `status_normalized` alanına çevrilir (`01` §4.4). Entegratör bazında tablo ilgili dosyada.

| Durum ailesi | `status_normalized` | Not |
|---|---|---|
| Başarıyla iletildi, onaylandı, kabul edildi, otomatik onay | `active` | |
| Ticari faturada onay bekliyor | `active` + uyarı `PENDING_RESPONSE` | Kabul/red süresi dolmadan karar değişebilir |
| Alıcı reddetti (ticari fatura) | `cancelled` + uyarı `REJECTED` | Müşavir görür, beyana girmez |
| İade edildi (uygulama yanıtı) | `cancelled` + uyarı `RETURNED` ⚠️ | Mevzuat yorumuna göre müşavir karar verir |
| E-arşiv iptal | `cancelled` | |
| Taslak, kuyrukta, imza bekliyor | `draft` | Giden faturada henüz kesinleşmemiş |
| GİB tarafında hata, sistem hatası | `draft` + uyarı `DELIVERY_ERROR` | Fatura geçerli olmayabilir, mükellef düzeltmeli |

## 9. Kabul testleri (her entegratör adaptörü için)

1. Test ortamında bağlantı ve tek gelen fatura listeleme.
2. 1 aylık gelen ve giden e-fatura: adet ve `amounts.payable_amount` toplamı entegratör portalıyla birebir.
3. 1 aylık e-arşiv: aynı kontrol.
4. Oran bazında KDV (`%1`, `%10`, `%20`) UBL'den doğru `invoice_tax_summary` üretiyor.
5. Tevkifatlı faturada tevkifat kodu ve tutarı doğru (`31` §5).
6. İade faturasında `type=IADE` ve asıl fatura numarası doğru.
7. Reddedilmiş ticari fatura `cancelled`; iptal e-arşiv `cancelled`.
8. Aynı fatura hem ön muhasebe programından hem entegratörden gelince **tek kayıt** (ETTN ile, `01` §6).
9. Okuma sonrası entegratör portalında hiçbir faturanın durumu değişmemiş (§5).
10. Geçmiş tarihli kesilmiş bir fatura artımlı çekimde yakalanıyor (§7).

## 10. Açık işler

| Konu | Kimle |
|---|---|
| İşNet'e Easytrade'in hangi statüde ve hangi yetkiyle (IP + VKN) erişeceği; mükellef onayının şekli | İşNet |
| QNB ve EDM: mevcut Easytrade istemcisinin portal ihtiyaç listesini (`34` §2, `35` §2) karşılayıp karşılamadığı | Easytrade backend |
| QNB e-arşiv liste ucu; EDM tarih aralığıyla listeleme (mevcut dokümanda yoksa) | QNB eSolutions, EDM (mevcut kanal) |
| İzibiz canlı ortam adresleri, durum ve hata kodları | İzibiz |
| TÜRMOB tercihli mükelleflerde faturaların Luca'ya zaten otomatik gidip gitmediği; portalın Luca aktarımında çift kaydın nasıl önleneceği | Müşavir, Luca |
| Diğer entegratörler (Nilvera, Sovos/Foriba, Digital Planet, Logo, Turkcell e-Şirket, Mysoft …): müşavirin mükellef listesinde varsa sıraya alınır | — |

Not: Nilvera'nın herkese açık REST geliştirici dokümanı var (gelen fatura, satış faturası, e-arşiv listeleme ve XML/PDF uçları), ayrıntıları bu çalışmada alınmadı.

## Kaynaklar

- Luca Yazılım duyurusu (TÜRMOB tercihli entegratör → Luca otomatik aktarım): https://x.com/Lucayazilim/status/1534484589854347264
- İşNet resmi arayüz dokümanlarının bulunduğu topluluk SDK'sı: https://github.com/EfeSorogluu/NetteFatura-API
- Uyumsoft resmi dokümanın bulunduğu topluluk paketi: https://github.com/gkvdt/uyumsoft-einvoice
- QNB eFinans topluluk kütüphaneleri: https://github.com/Hasokeyk/qnb-finansbank-invoice-php · https://github.com/muhittingulap/qnbfinansbank
- EDM topluluk kütüphanesi: https://github.com/rebasesoftware/edm-efatura
- İzibiz resmi istemciler: https://github.com/izibiz/ws-client-dotnet · https://github.com/izibiz/ws-client-php
- Nilvera geliştirici dokümanı: https://developer.nilvera.com
