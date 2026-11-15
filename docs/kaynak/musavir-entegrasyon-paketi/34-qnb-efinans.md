# 34 · QNB eFinans (QNB eSolutions): Easytrade'de mevcut entegrasyon

| | |
|---|---|
| Durum | **Easytrade backend'inde QNB eFinans entegrasyonu mevcut.** Portal yeni bir istemci yazmaz; mevcut istemciyi kullanır. |
| Bu dosyanın işi | Portalın mevcut istemciden neye ihtiyaç duyduğunu listelemek (§2) ve eksik çıkan metotlar için önceki araştırmadan teknik notları vermek (§5) |
| Ortak kurallar | `30` (okuma kuralları, durum ailesi, kabul testleri), `37` (senkron akışı, indeks tablosu, hata modeli) |

## 1. İki durum

| Mükellef | Veri nereden gelir |
|---|---|
| Easytrade kullanıcısı (faturasını Easytrade üzerinden QNB ile kesiyor) | **Easytrade adaptörü** (`09`): satış, alış, gelen kutusu (`invoices/inbox`), UBL (`download-ubl`). Entegratörden ayrıca çekilmez. |
| Easytrade kullanıcısı değil, kendi QNB eFinans hesabı var | Mevcut QNB istemcisi, **mükellefin kendi** QNB kullanıcı bilgileriyle çağrılır (bu dosya) |

İkinci durumda istemcinin Easytrade'in kendi hesabına değil, bağlantıdaki mükellef kimlik bilgilerine (`source_connection.credentials_ref`, `02` §4) göre çalışabilmesi gerekir.

## 2. Portalın ihtiyaç listesi (backend dolduracak)

| # | İhtiyaç | Mevcut istemcide var mı | Yoksa (§5'teki not) |
|---|---|---|---|
| 1 | Mükellefin kendi kullanıcı adı / şifresiyle çağrı (çok kiracılı) | ☐ | — |
| 2 | Gelen e-fatura listesi, tarih penceresiyle | ☐ | `gelenBelgeleriListeleExt` |
| 3 | Gelen e-faturanın UBL XML'i | ☐ | `gelenBelgeleriIndirExt` (`belgeFormati=UBL`) |
| 4 | Giden e-fatura listesi, tarih penceresiyle | ☐ | `gidenBelgeleriListele` ⚠️ |
| 5 | Giden e-faturanın UBL XML'i | ☐ | `gidenBelgeleriIndirExt` |
| 6 | Giden e-arşiv listesi, tarih penceresiyle | ☐ | **Bulunan kaynaklarda yok**; QNB'ye sorulur (§4) |
| 7 | E-arşiv UBL XML'i (ETTN ile) | ☐ | `faturaSorgula` |
| 8 | Durum (kabul, red, iptal) | ☐ | `gidenBelgeDurumSorgulaExt`; gelen için QNB'ye sorulur |
| 9 | Okurken durum değiştirmeme (§3) | ☐ | — |

Mevcut istemci Easytrade'in **fatura gönderme** akışı için yazıldıysa 2, 4, 6 numaralı liste metotları muhtemelen eksiktir; bu durumda §5'teki notlarla eklenir.

## 3. Okuma kuralları

- `gelenBelgeleriListeleExt` çağrısında `belgelerAlindiMi` parametresi **gönderilmez**. Etkisi (belgeyi "alındı" olarak işaretleyip işaretlemediği) doğrulanmadı ⚠️. Mevcut istemci bu parametreyi gönderiyorsa portal çağrılarında kapatılır.
- Gönderme, iptal, yanıt metotları portal tarafından çağrılmaz (`30` §5).
- `source_system = entegrator_qnb_efinans`, `source_id` = ETTN (`30` §6).

## 4. QNB'ye sorulacaklar (backend mevcut dokümanla cevaplayamıyorsa)

- [ ] Tarih aralığıyla e-arşiv listeleme / rapor metodu
- [ ] Giden belge listesi yanıt alanları ve belge OID'si
- [ ] Durum kodları listesi, gelen fatura kabul/red durumu
- [ ] `belgelerAlindiMi` parametresinin etkisi
- [ ] Mükellefin kendi kullanıcısıyla, Easytrade sunucusundan erişim için ek bir izin / IP tanımı gerekiyor mu
- [ ] Hız sınırı

Easytrade'in QNB ile mevcut ilişkisi olduğu için bu sorular mevcut iletişim kanalından sorulabilir.

## 5. Teknik notlar (önceki araştırmadan, eksik metot eklenecekse)

**Kaynak seviyesi:** ⚠️ Resmi doküman alınamadı. İki bağımsız topluluk kütüphanesinden: `Hasokeyk/qnb-finansbank-invoice-php` (2026) ve `muhittingulap/qnbfinansbank` (2020). Easytrade'in elindeki resmi doküman bu notlardan önce gelir.

**Adresler (topluluk kütüphanesinden):**

| Ortam | Servis | Adres |
|---|---|---|
| Canlı | ConnectorService (e-fatura) | `https://connector.qnbesolutions.com.tr/connector/ws/connectorService?wsdl` |
| Canlı | e-Arşiv | `https://earsiv.qnbesolutions.com.tr/earsiv/ws/EarsivWebService?wsdl` |
| Test | ConnectorService | `https://erpefaturatest1.qnbesolutions.com.tr/efatura/ws/connectorService?wsdl` |
| Test | e-Arşiv | `https://earsivtest.qnbesolutions.com.tr/earsiv/ws/EarsivWebService?wsdl` |

**Kimlik:** WS-Security `UsernameToken` veya UserService `wsLogin` ile oturum çerezi. İsteklerde mükellef VKN/TCKN (`vergiTcKimlikNo` / `vkn`).

**`gelenBelgeleriListeleExt`:**

| Parametre | Değer |
|---|---|
| `vergiTcKimlikNo` | VKN |
| `belgeTuru` | `FATURA` |
| `sonAlinanBelgeSiraNumarasi` | Son alınan `belgeSiraNo` (ilk çağrıda `0`). Artımlı imleç; `source_connection.sync_cursor`'da saklanır. |
| `faturaTarihiBaslangic`, `faturaTarihiBitis` | Dönem taraması |
| `gelisTarihiBaslangic`, `gelisTarihiBitis` | Artımlı alternatif |
| `belgelerAlindiMi` | Gönderilmez (§3) |

Yanıt öğesi: `ettn` → `uuid`, `belgeNo` → `invoiceNumber`, `belgeSiraNo` → imleç, `belgeTarihi` (`yyyyMMdd`) → `issueDate`, `faturaGelisTarihi` → `createdAt`, `gonderenVknTckn` / `saticiUnvan` → karşı taraf, `odenecekTutar` → kontrol. Durum alanı listede görülmedi.

**`gelenBelgeleriIndirExt`:** `vergiTcKimlikNo`, `belgeEttn`, `belgeTuru=FATURA`, `belgeFormati=UBL` → base64 ZIP (`37` §3.4).

**`gidenBelgeleriListele`** (2020 kütüphanesi): `vkn`, `belgeTuru=FATURA`, `baslangicGonderimTarihi`, `bitisGonderimTarihi` (`yyyyMMdd`). Yanıt alanları örneklenmemiş. İçerik için `gidenBelgeleriIndirExt` `belgeOidListesi` istiyor; OID'nin listede gelip gelmediği doğrulanmadı.

**`gidenBelgeDurumSorgulaExt`:** `vergiTcKimlikNo` + `parametreler{ belgeNo, belgeNoTipi, belgeTuru=FATURA_UBL }` → `durum`, `gonderimDurumu`, `gonderimCevabiKodu`, `yanitDurumu`, `yanitDetayi`, `ulastiMi`, `ettn` …

**`faturaSorgula`** (e-Arşiv servisi): tek fatura, `faturaUuid` + `donenBelgeFormati`.

**Durum:** Kod anlamları bulunamadı. Mevcut istemcide durum eşlemesi varsa o kullanılır; yoksa geçici kural: `ulastiMi=true` → `active`; yanıt detayı red bildiriyorsa `cancelled` + `REJECTED`; diğerleri `active` + `STATUS_UNVERIFIED`.

## Kaynaklar

- https://github.com/Hasokeyk/qnb-finansbank-invoice-php (`docs/01-baslangic.md`, `src/service/invoice_service.php`)
- https://github.com/muhittingulap/qnbfinansbank (`src/libraries/efatura.php`, `src/libraries/earsiv.php`)
