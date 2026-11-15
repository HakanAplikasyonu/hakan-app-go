# 14 · Logo Tiger / GO Adaptörü (Logo Objects REST)

| | |
|---|---|
| Çalışma şekli | **Kurulu yazılım.** Mükellefin kendi sunucusunda (MS SQL + Logo uygulama sunucusu) |
| API | Logo Objects REST Service. Tiger, GO, Wings ve 3 serisi (Tiger 3, GO 3) ⚠️ |
| Base URL | `http://{SUNUCU}:32001/api/v1` (genelde) ⚠️ |
| Kimlik doğrulama | OAuth2 password grant, Basic client bilgisi + firma no ⚠️ |
| Erişim yolu | **Bağlantı ajanı şart** (`02` §6) |
| 5 kalemin durumu | Beş kalem de Logo'da var (tam muhasebe programı). REST nesne ve alan adları ⚠️ (topluluk kaynağı). |
| Doğrulama kaynağı | Logo Objects REST servisinin 156 nesnesini kapsayan topluluk istemci kütüphanesi (`ankapix/logo-objects-php`, Temmuz 2026). **Bu çalışmada Logo'nun Objects REST servisi için herkese açık resmi dokümanına ulaşılamadı**; resmi doküman Logo / bayi üzerinden alınmalı. |

> ⚠️ Bu dosyadaki uç nokta, sorgu kuralı ve alan adları topluluk kütüphanesinden alındı. İlk mükellefin Logo sunucusunda (ajan üzerinden) her nesne için tek kayıt çekilip alan adları doğrulanmadan üretime alınmaz.

**Önemli fark:** Tiger ve GO tam muhasebe programlarıdır. **Muhasebe fişleri ve hesap planı da içlerinde.** Mükellefin muhasebesini müşavir Luca'da değil Logo'da tutuyorsa bu kaynak, Luca'ya aktarım kadar değerlidir.

**Kapsam dışı:** Netsis farklı bir üründür, bu adaptör onu kapsamaz. İşbaşı için bkz. `13`.

## 1. Ön koşullar (mükellef tarafında)

1. **Logo REST Service kurulu ve lisanslı olmalı.** Logo bayisi tarafından açılır ⚠️ (bazı lisanslarda ek modül ücreti var).
2. REST servisi için **client id / client secret** (Logo bayisinden).
3. Entegrasyon için ayrı bir **Logo kullanıcısı.** Sadece okuma yetkili rol önerilir.
4. **Firma no** (`firmno`) ve **dönem no.**
5. Ajanın kurulacağı, Logo sunucusuna ağdan erişen bir Windows makinesi.

Kurulum kontrol listesi müşavir panelinde mükellefe/bayiye gösterilir.

## 2. Kimlik doğrulama ⚠️

```
POST http://{SUNUCU}:32001/api/v1/token
Authorization: Basic base64("{clientId}:{clientSecret}")
Content-Type: application/x-www-form-urlencoded

grant_type=password&username={KULLANICI}&password={SIFRE}&firmno={FIRMA_NO}
```

- Dönen `access_token` sonraki isteklerde `Authorization: Bearer {token}` olarak kullanılır.
- 401 alınınca token bir kez yenilenip istek tekrarlanır.
- Kimlik bilgileri **ajan makinesinde** DPAPI ile şifreli saklanır, buluta gönderilmez.

## 3. Genel sorgu kuralları ⚠️

| İşlem | İstek |
|---|---|
| Listeleme | `GET /{nesne}` |
| Tek kayıt | `GET /{nesne}/{id}` |
| Kriterle arama | `GET /{nesne}?q=...` |
| Sayım | `GET /{nesne}?count=true` |

Desteklenen sorgu parametreleri: `limit`, `offset`, `fields`, `sort`, `q`, `expand`, `expandLevel`, `withCount`, `count`.

```
GET /salesInvoices?limit=100&offset=0&withCount=true
    &q=DATE gte '2026-10-01' and DATE lte '2026-10-31'
    &expand=TRANSACTIONS
```

- `q` operatörleri ⚠️: `eq`, `neq`, `gt`, `gte`, `lt`, `lte`, `like`, `and`, `or`. Metin değerleri tek tırnak içinde.
- Tarih literal biçimi kurulu sürüme göre değişebilir ⚠️. İstemci kütüphanesi örneği `'01.01.2026'` biçimini kullanıyor, `'2026-01-01'` de denenmeli.
- **Artımlı senkron** ⚠️: Her nesnede `DATE_MODIFIED` (+ `HOUR_MODIFIED`, `MIN_MODIFIED`) alanı var. `q=DATE_MODIFIED gte '...'` ile sadece değişen kayıtlar çekilebilir. Diğer kaynaklardan farklı olarak gerçek artımlı senkron mümkün.
- Satırlar (`TRANSACTIONS`) için `expand` gerekir.
- Sayfalama: `limit` + `offset`. `withCount=true` ile toplam kayıt sayısı alınır.
- Hız sınırı: Yerel servis, dış limit yok. Mükellefin Logo sunucusunu yormamak için **saniyede 2 istek** ve **gece toplu tarama** önerilir.

## 4. Kalem 1 ve 2: Satış ve alış faturaları ⚠️

```
GET /salesInvoices      (satış faturaları)
GET /purchaseInvoices   (alış faturaları)
```

Logo'da satış ve alış ayrımı uç noktayla yapılır. Alt tür `TYPE` alanındadır.

### 4.1 `TYPE` (fatura türü) → ortak model ⚠️

Logo fatura türü kodları (kurulu sürümde doğrulanmalı):

| TYPE | Anlamı | `direction` | Ortak model |
|---|---|---|---|
| 1 | Satınalma faturası | incoming | `type=ALIS` |
| 4 | Alınan hizmet faturası | incoming | `type=ALIS` |
| 6 | Satınalma iade faturası | incoming | `type=IADE` |
| 13 | Satınalma fiyat farkı faturası | incoming | `type=ALIS`, `document_class=price_difference` |
| 26 | Müstahsil makbuzu | incoming | `type=ALIS`, `document_class=mustahsil` |
| 7 | Perakende satış faturası | outgoing | `type=SATIS` |
| 8 | Toptan satış faturası | outgoing | `type=SATIS` |
| 9 | Verilen hizmet faturası | outgoing | `type=SATIS` |
| 2 | Perakende satış iade faturası | outgoing | `type=IADE` |
| 3 | Toptan satış iade faturası | outgoing | `type=IADE` |
| 14 | Satış fiyat farkı faturası | outgoing | `type=SATIS`, `document_class=price_difference` |
| 5, 10 | Alınan / verilen proforma |  | **Çekilmez** |

### 4.2 Başlık alanları ⚠️

| Logo alanı | Ortak model |
|---|---|
| `INTERNAL_REFERENCE` | `source_id` ⚠️ |
| `GUID` | ek kimlik (değişmez) |
| `NUMBER` | Logo içi fiş no |
| `DOC_NUMBER` | **Belge no** → `invoice_number` |
| `DATE` | `issue_date` |
| `DOC_DATE` | Belge tarihi (`DATE` ile farklıysa `DOC_DATE` öncelikli ⚠️, müşavirle netleştir) |
| `TYPE` | `direction`, `type`, `document_class` (yukarıdaki tablo). `DEDUCTIONPART1 > 0` ise `type=TEVKIFAT`, `VATEXCEPT_CODE` doluysa `type=ISTISNA` (öncelik: `03` §2) |
| `CANCELLED` | `1` → `status_normalized=cancelled` |
| `ARP_CODE` | Cari kodu → `/Arps` ile `customer` eşlemesi |
| `TOTAL_GROSS` | `amounts.line_extension_amount` |
| `TOTAL_DISCOUNTS` | `amounts.allowance_total_amount` |
| `TOTAL_VAT` | `amounts.tax_amount` |
| `TOTAL_ADD_TAX` | Ek vergi / ÖTV → `amounts.excise_amount` ⚠️ |
| `TOTAL_NET` | `amounts.payable_amount` |
| `CURR_INVOICE`, `TC_XRATE`, `TC_NET` | Döviz kodu, kur, döviz tutarı |
| `RC_XRATE`, `RC_NET` | Raporlama dövizi |
| `DEDUCTIONPART1` / `DEDUCTIONPART2` | Tevkifat oranı pay / payda → `withholding_rate = P1 / P2 × 100` (`5/10` → `50`) |
| `VATEXCEPT_CODE`, `VATEXCEPT_REASON` | `global_tax_exemption_code` |
| `EINVOICE` | E-belge türü → `profile` (`EARSIVFATURA` / e-fatura / kâğıt=`NULL`) ⚠️ değer anlamları doğrulanmalı |
| `PROFILE_ID` | E-fatura senaryosu (temel / ticari) |
| `PAYMENT_TYPE` | Ödeme şekli |
| `CREDIT_CARD_NO` | Ödeme kartı **(okunmaz, saklanmaz)** |
| `TYPE_CODE` | Özel tür kodu |

Tevkifat tutarı başlıkta ayrı alan olarak yok. Satırlardaki `DEDUCTION_TOT` toplanır.

ETTN alanı: Satış faturalarında e-arşiv/e-fatura detayları alt nesnelerde (`EARCHIVEDETR_*`) ⚠️. ETTN alan adı kurulu sürümde doğrulanmalı. Bulunamazsa ETTN entegratör verisinden eşlenir.

### 4.3 Satır alanları (`TRANSACTIONS`) ⚠️

| Logo alanı | Ortak model (`invoice_item`) |
|---|---|
| `TYPE` | `line_type` (malzeme, hizmet, indirim, masraf, promosyon ⚠️ kod listesi doğrulanmalı) |
| `MASTER_CODE` | `product_code` |
| `DESCRIPTION` | `description` |
| `QUANTITY` | `quantity` |
| `UNIT_CODE` | `unit_code` |
| `PRICE` | `unit_price` |
| `TOTAL` | Satır brüt |
| `DISCOUNT_RATE` | İndirim oranı |
| `VAT_RATE` | `tax_rate` |
| `VAT_BASE` | `tax_exclusive_amount` (KDV matrahı) |
| `VAT_AMOUNT` | `total_tax_amount` |
| `VAT_INCLUDED` | Fiyat KDV dahil mi |
| `DEDUCTION_PART1` / `DEDUCTION_PART2` | Satır tevkifat oranı |
| `DEDUCTION_TOT` | `withholding_amount` |
| `DEDUCT_CODE` | `withholding_code` |
| `VATEXCEPT_CODE` | `tax_exemption_code` |
| `ADD_TAX_RATE`, `ADD_TAX_AMOUNT`, `ADD_TAX_CODE` | ÖTV / ek vergi |
| `GL_CODE1` … `GL_CODE5` | **Muhasebe hesap kodları** (Luca eşlemesi için ipucu) |
| `CANCELLED` | İptal satır |

`GL_CODE*` alanları, mükellefin Logo'da kullandığı hesap kodlarını doğrudan verir. Kural motoru ilk eşlemeyi buradan önerir.

## 5. Kalem 3: Finansal işlemler ⚠️

### 5.1 Kasa fişleri

```
GET /safeDepositSlips?q=DATE gte '...' and DATE lte '...'
```

| Logo alanı | Ortak model |
|---|---|
| `TYPE` | İşlem türü (tahsilat, ödeme, masraf, virman ⚠️ kod listesi doğrulanmalı) → `transaction_type` + `transaction_subtype` |
| `SD_CODE` | Kasa kodu → `cash_box` |
| `SD_CODE_CROSS` | Virmanda karşı kasa → `to_cash_box_id` |
| `DATE` | `transaction_date` |
| `NUMBER`, `DOC_NUMBER` | `reference_document_no` |
| `AMOUNT` | `amount` |
| `SIGN` | Yön (borç/alacak) → `transaction_type` (`income` / `expense`) ⚠️ (0/1 anlamı doğrulanmalı) |
| `DESCRIPTION` | `description` |
| `CURR_TRANS`, `TC_XRATE`, `TC_AMOUNT` | Döviz |
| `CANCELLED` | İptal |
| `CASH_ACC_CODE` | Kasa muhasebe hesabı (`100.xx`) |
| `TCKNO` | Karşı taraf TCKN |
| `NUMBERPLATE` | **Plaka** (masraf fişlerinde) → `vehicle_plate` |
| `VAT_RAT`, `VAT_TOT`, `NET_AMOUNT`, `GROSS_AMOUNT` | KDV'li kasa masraflarında vergi kırılımı |
| `EINVOICE` | E-belgeli mi |

Kasadan girilen KDV'li masraflar (fiş, gider pusulası) **kasa fişi** olarak durur. Bunlar `expense`'e de yazılır (bkz. Kalem 5).

### 5.2 Banka fişleri

```
GET /bankSlips?q=DATE gte '...' and DATE lte '...'&expand=TRANSACTIONS
```

| Logo alanı | Ortak model |
|---|---|
| `TYPE` | Banka işlem türü (gelen/giden havale, virman, masraf ⚠️) |
| `DATE`, `NUMBER` |  |
| `BNACCCODE` | Banka hesap kodu → `cash_box` |
| `TOTAL_DEBIT`, `TOTAL_CREDIT` | Fiş toplamı |
| `SIGN` | Yön |
| `CANCELLED` | İptal |
| `TRANSACTIONS` | Satırlar (karşı cari, tutar, açıklama ⚠️ alan adları doğrulanmalı) |

### 5.3 Cari hesap fişleri (tahsilat, ödeme, dekont)

```
GET /ArpSlips?q=DATE gte '...' and DATE lte '...'&expand=TRANSACTIONS
```

| Logo alanı | Kullanım |
|---|---|
| `TYPE` | Nakit tahsilat, nakit ödeme, borç dekontu, alacak dekontu, kredi kartı fişi, virman ⚠️ |
| `ARP_CODE` | Cari |
| `BANKACC_CODE` | Bankadan ise banka hesabı → **ödeme türü mutabakatı için kritik** |
| `TOTAL_DEBIT`, `TOTAL_CREDIT` | Tutar |
| `DOC_NUMBER`, `DATE` | |

### 5.4 Hesap kartları

```
GET /safeDeposits     (kasalar)
GET /bankAccounts     (banka hesapları, `IBAN` alanı dahil ⚠️)
GET /banks
```

## 6. Kalem 4: Gelir ve gider türleri

Logo'da gelir/gider türleri iki yerden gelir:

1. **Hesap planı** ⚠️: `GET /GLAccounts`. `6xx` gelir, `7xx` gider hesapları `category` olarak yazılır. `full_path` için hesap kodu hiyerarşisi kullanılır (`770.01.003`).
2. **Masraf merkezleri / özel kodlar** ⚠️: Faturaların `AUXIL_CODE`, `PROJECT_CODE` alanları ek kırılım olarak kullanılabilir.

Bu, diğer ön muhasebe programlarından daha zengin bir yapıdır. Müşavirin Luca hesap planına eşleme neredeyse birebir yapılabilir.

## 7. Kalem 5: Masraf fişleri

| Kaynak | Uç | Not |
|---|---|---|
| Kasa masraf fişleri | `/safeDepositSlips` (masraf türleri) | `NUMBERPLATE`, `VAT_RAT`, `VAT_TOT` alanlarıyla |
| Alınan hizmet faturaları | `/purchaseInvoices` (`TYPE=4`) | Fatura olarak `invoice`'a da yazılır |
| Satınalma masrafları | `/purchaseExpenses` ⚠️ | Alış faturasına yansıtılan masraflar |
| Muhasebe fişi ile girilen giderler | `/GLSlips` ⚠️ | Müşavir doğrudan muhasebe fişi kesiyorsa |

## 8. Muhasebe fişleri (bonus)

```
GET /GLSlips?q=DATE gte '...' and DATE lte '...'&expand=TRANSACTIONS
```

Mükellefin muhasebesi Logo'da tutuluyorsa yevmiye kayıtlarının tamamı buradan okunur. Bu, Luca'ya aktarımda "hazır fiş" kaynağıdır. Kontrol katmanı (mükerrer, KDV) yine faturalardan beslenir.

## 9. REST yoksa: SQL ile okuma (yedek yol) ⚠️

REST servisi lisanslı değilse ajan, Logo veritabanından **salt okunur SQL kullanıcısıyla** okuyabilir. Logo tablo adlandırması `LG_{FFF}_{PP}_{TABLO}` biçimindedir (`FFF` = firma no, 3 hane; `PP` = dönem no, 2 hane).

| Veri | Tablo (doğrulanmalı) |
|---|---|
| Fatura başlıkları | `LG_FFF_PP_INVOICE` |
| Fatura / stok satırları | `LG_FFF_PP_STLINE` |
| Kasa hareketleri | `LG_FFF_PP_KSLINES` |
| Banka fişleri / satırları | `LG_FFF_PP_BNFICHE`, `LG_FFF_PP_BNFLINE` |
| Cari fişleri / satırları | `LG_FFF_PP_CLFICHE`, `LG_FFF_PP_CLFLINE` |
| Cari kartlar | `LG_FFF_CLCARD` (firmaya bağlı, döneme bağlı değil) |
| Hesap planı | `LG_FFF_EMUHACC` |
| Muhasebe fişleri | `LG_FFF_PP_EMFICHE`, `LG_FFF_PP_EMFLINE` |

Kurallar:
- Ajan sadece önceden tanımlı, parametreli sorgular çalıştırır.
- Sorgular `WITH (NOLOCK)` ile, mesai dışında çalıştırılır.
- REST alan adları ile SQL kolon adları büyük ölçüde aynıdır (`TRCODE` ↔ `TYPE` gibi farklar olabilir ⚠️).

## 10. Senkron planı (ajan üzerinde)

| Adım | Çağrı | Sıklık |
|---|---|---|
| 1 | `/Arps`, `/safeDeposits`, `/bankAccounts`, `/GLAccounts` | Gece 1 |
| 2 | `/salesInvoices`, `/purchaseInvoices` (açık dönemler, `expand=TRANSACTIONS`) | 4 saatte 1 + gece |
| 3 | `/safeDepositSlips`, `/bankSlips`, `/ArpSlips` (açık dönemler) | 4 saatte 1 + gece |
| 4 | `/GLSlips` (açık dönemler, müşavir istediyse) | Gece 1 |

## 11. Doğrulanacaklar listesi (ilk pilot mükellefte)

- [ ] Kurulu sürüm (Tiger 3 / GO 3 / Wings) ve REST servis sürümü
- [ ] `q` tarih literal biçimi
- [ ] Fatura `TYPE` kod listesi
- [ ] Satır `TYPE` kod listesi
- [ ] Kasa, banka ve cari fişi `TYPE` kod listeleri
- [ ] `SIGN` alanının anlamı
- [ ] ETTN alan adı
- [ ] `EINVOICE` değerleri
- [ ] `DATE` mı `DOC_DATE` mı (müşavir tercihi)
- [ ] Dönem değişiminde (yıl sonu) yeni dönem no'nun otomatik algılanması

## Kaynaklar

- Logo Objects REST istemci kütüphanesi (156 nesne, alan listeleri): https://packagist.org/packages/ankapix/logo-objects-php
- Logo Objects REST servis kullanım örneği: https://www.ibm.com/support/pages/maximo-rest-client-example-calling-external-rest-service-logo-tiger-3-maximo-client
