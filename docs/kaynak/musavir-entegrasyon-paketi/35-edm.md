# 35 · EDM Bilişim: Easytrade'de mevcut entegrasyon

| | |
|---|---|
| Durum | **Easytrade backend'inde EDM entegrasyonu mevcut.** Portal yeni bir istemci yazmaz; mevcut istemciyi kullanır. |
| Bu dosyanın işi | Portalın mevcut istemciden neye ihtiyaç duyduğunu listelemek (§2) ve eksik çıkan metotlar için önceki araştırmadan teknik notları vermek (§4) |
| Ortak kurallar | `30` (okuma kuralları, durum ailesi, kabul testleri), `37` (senkron akışı, indeks tablosu, hata modeli) |

## 1. İki durum

| Mükellef | Veri nereden gelir |
|---|---|
| Easytrade kullanıcısı (faturasını Easytrade üzerinden EDM ile kesiyor) | **Easytrade adaptörü** (`09`): satış, alış, gelen kutusu (`invoices/inbox`), UBL (`download-ubl`). Entegratörden ayrıca çekilmez. |
| Easytrade kullanıcısı değil, kendi EDM hesabı var | Mevcut EDM istemcisi, **mükellefin kendi** EDM kullanıcı bilgileriyle (`Login` → `SESSION_ID`) çağrılır |

## 2. Portalın ihtiyaç listesi (backend dolduracak)

| # | İhtiyaç | Mevcut istemcide var mı | Yoksa (§4'teki not) |
|---|---|---|---|
| 1 | Mükellefin kendi kullanıcı adı / şifresiyle oturum (çok kiracılı) | ☐ | `Login` / `Logout` |
| 2 | Gelen e-fatura listesi, tarih penceresiyle | ☐ | `GetInvoice`, `DIRECTION=IN`; tarih alanları **doğrulanmadı** |
| 3 | Giden e-fatura listesi, tarih penceresiyle | ☐ | `GetInvoice`, `DIRECTION=OUT`; aynı |
| 4 | UBL XML (ETTN ile) | ☐ | `GetInvoice` (`UUID`, `INVOICE_CONTENT_TYPE=XML`) |
| 5 | Giden e-arşiv listesi ve UBL'i | ☐ | Bulunan kaynakta yok; EDM'ye sorulur |
| 6 | Durum | ☐ | `GetInvoiceStatus` |
| 7 | Okurken durum değiştirmeme | ☐ | — |

EDM servisi İzibiz ile aynı aileden görünüyor (`REQUEST_HEADER`, `INVOICE_SEARCH_KEY`, `SESSION_ID` yapıları aynı). İzibiz şemasındaki tarih alanları (`DATE_TYPE`, `START_DATE`, `END_DATE`, `READ_INCLUDED`, `36` §5) EDM'de de varsa liste metodu aynı şekilde yazılır; **EDM WSDL'iyle doğrulanmadan** varsayılmaz.

## 3. Okuma kuralları

- Sadece `Login`, `Logout`, `GetInvoice`, `GetInvoiceStatus` (ve doğrulanırsa liste parametreleri).
- Okundu işareti koyan, gönderen, yanıt / iptal gönderen metotlar portal tarafından çağrılmaz (`30` §5). İzibiz'deki `MarkInvoice` karşılığı EDM'de varsa o da çağrılmaz.
- Listede okunmuşlar da gelsin (İzibiz'deki `READ_INCLUDED=true` karşılığı, varsa).
- `source_system = entegrator_edm`, `source_id` = ETTN.

## 4. Teknik notlar (önceki araştırmadan, eksik metot eklenecekse)

**Kaynak seviyesi:** ⚠️ Resmi doküman alınamadı. Topluluk kütüphanesi `rebasesoftware/edm-efatura` (v1.0.0, 2025). Easytrade'in elindeki resmi doküman bu notlardan önce gelir.

**Adresler:** Test WSDL `https://test.edmbilisim.com.tr/EFaturaEDM21ea/EFaturaEDM.svc?singleWsdl` (kütüphaneden). Canlı adres doğrulanmadı.

**İstek başlığı (kütüphaneden):** `REQUEST_HEADER`: `SESSION_ID`, `CLIENT_TXN_ID` (GUID), `ACTION_DATE`, `REASON`, `HOSTNAME`, `CHANNEL_NAME`, `APPLICATION_NAME`, `COMPRESSED=N`.

**`GetInvoice` (kütüphanedeki tek fatura çağrısı):** `INVOICE_CONTENT_TYPE=XML`, `HEADER_ONLY=N`, `INVOICE_SEARCH_KEY{ ID | UUID, DIRECTION=IN|OUT, LIMIT=1 }`.

**Yanıt `INVOICE` → `EntegratorDocHeader`:** `UUID` → `uuid`, `ID` → `invoiceNumber`, `HEADER/ISSUE_DATE` → `issueDate`, `HEADER/CDATE` → `createdAt`, `HEADER/SENDER` / `RECEIVER` → VKN'ler, `HEADER/SUPPLIER` / `CUSTOMER` → unvanlar, `HEADER/PROFILEID` → `profileId`, `HEADER/STATUS` → `statusRaw`, `CONTENT` → UBL (`31`).

**Durumlar** (kütüphanedeki `İŞLEM-SONUÇ` değerleri; EDM'nin resmi listesiyle karşılaştırılır ⚠️; mevcut istemcide eşleme varsa o kullanılır):

| Değer | Anlamı (kütüphane açıklaması) | `status_normalized` |
|---|---|---|
| `PACKAGE-PROCESSING`, `SEND-PROCESSING`, `SEND-WAIT_GIB_RESPONSE`, `SEND-WAIT_SYSTEM_RESPONSE`, `UNKNOWN-UNKNOWN` | Gönderim sürüyor | `draft` |
| `SEND-WAIT_APPLICATION_RESPONSE` | Alıcı yanıtı bekleniyor (giden) | `active` + `PENDING_RESPONSE` |
| `RECEIVE-WAIT_APPLICATION_RESPONSE` | Kabul/red yanıtı bekleniyor (gelen) | `active` + `PENDING_RESPONSE` |
| `SEND-SUCCEED`, `RECEIVE-SUCCEED` | Başarıyla iletildi / alındı | `active` |
| `ACCEPTED-SUCCEED`, `ACCEPT-SUCCEED` | Onaylandı / kabul yanıtı gönderildi | `active` |
| `REJECTED-SUCCEED`, `REJECT-SUCCEED` | Reddedildi / red yanıtı gönderildi | `cancelled` + `REJECTED` |
| `ACCEPT-*`, `REJECT-*` (`PROCESSING`, `WAIT_*`, `FAILED`) | Yanıt gönderimi sürüyor / başarısız | önceki durum + uyarı |
| `PACKAGE-FAIL`, `SEND-FAILED` | Gönderilemedi | `draft` + `DELIVERY_ERROR` |

E-arşiv iptal durumu bu listede yok.

## 5. EDM'ye sorulacaklar (backend mevcut dokümanla cevaplayamıyorsa)

- [ ] `GetInvoice` ile tarih aralığında listeleme (alanlar, `LIMIT` üst sınırı)
- [ ] E-arşiv faturalarının aynı servisten mi ayrı servisten mi okunduğu
- [ ] Durum kodları resmi listesi, e-arşiv iptal durumu
- [ ] Mükellefin kendi kullanıcısıyla Easytrade sunucusundan erişim için ek izin / IP tanımı gerekiyor mu
- [ ] Oturum süresi, hız sınırı

## Kaynaklar

- https://github.com/rebasesoftware/edm-efatura (`src/Client.php`, `src/Helper/Helper.php`, `doc/Client.md`)
