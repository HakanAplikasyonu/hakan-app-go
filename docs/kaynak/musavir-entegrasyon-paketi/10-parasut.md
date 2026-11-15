# 10 · Paraşüt Adaptörü (API v4)

| | |
|---|---|
| Çalışma şekli | Bulut, REST, JSON:API standardı |
| Base URL | `https://api.parasut.com/v4/{company_id}` ✅ |
| Kimlik doğrulama | OAuth2 (`authorization_code` veya `password`) ✅ |
| 5 kalemin durumu | Satış ✅ · Alış ✅ · Kasa/Banka ✅ · Kategori ✅ · Masraf ✅ |
| Doğrulama kaynağı | Paraşüt'ün resmi OpenAPI tanımından üretilmiş `parasut.js` v4 SDK'sı ve apidocs.parasut.com |

## 1. Erişim

### 1.1 Easytrade tarafında bir kez yapılacak

1. `destek@parasut.com` adresine yazılarak **entegratör uygulaması** kaydı istenir. Paraşüt şunları verir: `CLIENT_ID`, `CLIENT_SECRET`. ✅
2. Aynı yazışmada **özel `REDIRECT_URL`** kaydı istenir (ör. `https://portal.easytrade.com.tr/connect/parasut/callback`). Varsayılan `urn:ietf:wg:oauth:2.0:oob`'dir ve çok kiracılı akış için uygun değildir. ✅

### 1.2 Her mükellef için bağlantı akışı (önerilen: authorization_code)

```
1. Mükellef portalda "Paraşüt'ü bağla"ya basar.
2. Tarayıcı yönlendirilir:
   GET https://api.parasut.com/oauth/authorize
       ?client_id=CLIENT_ID
       &redirect_uri=REDIRECT_URL
       &response_type=code
3. Mükellef Paraşüt'e girip onaylar, REDIRECT_URL?code=XXXX adresine döner.
4. Backend token alır:
   POST https://api.parasut.com/oauth/token
   (multipart/form-data)
     grant_type=authorization_code
     client_id=CLIENT_ID
     client_secret=CLIENT_SECRET
     code=XXXX
     redirect_uri=REDIRECT_URL
5. Yanıt:
   { "access_token": "...", "token_type": "bearer",
     "expires_in": 7200, "refresh_token": "..." }
6. GET /v4/me → kullanıcının erişebildiği firmalar. Mükellef doğru firmayı seçer,
   company_id source_connection.external_company_id alanına yazılır.
```
✅ (akış ve alanlar resmi dokümanda)

### 1.3 Token yenileme

```
POST https://api.parasut.com/oauth/token
  grant_type=refresh_token
  client_id=CLIENT_ID
  client_secret=CLIENT_SECRET
  refresh_token=REFRESH_TOKEN
```

- `access_token` **2 saat** geçerli. ✅
- Her yenilemede **yeni bir `refresh_token` döner**, eskisi geçersiz olur. Yeni değer atomik olarak kasaya yazılmalı. Aynı bağlantı için paralel yenileme yapılmamalı (dağıtık kilit). ✅

### 1.4 Alternatif: password grant

`grant_type=password` + `username` + `password` ile de token alınabilir. ✅ Sadece mükellef OAuth akışını tamamlayamıyorsa kullanılır. Bu durumda mükellefe Paraşüt'te **ayrı bir entegrasyon kullanıcısı** açtırılır.

## 2. Genel kurallar

| Konu | Değer |
|---|---|
| Header | `Authorization: Bearer {access_token}`, `Content-Type: application/vnd.api+json` ✅ |
| Sayfalama | `page[number]` (varsayılan 1), `page[size]` (varsayılan 15) ✅. Üst sınır dokümanda yok ⚠️. 25 ile başlayıp 100 deneyin, hata alırsanız düşürün. |
| Filtre | `filter[alan]=değer` ✅ |
| Sıralama | `sort=alan` veya `sort=-alan` (azalan) ✅ |
| İlişkiler | `include=a,b.c` ✅. Gelen ilişkiler yanıtın `included` dizisindedir (JSON:API). |
| Hız sınırı | Resmi dokümanda **dakikada 60 istek** ✅. Topluluk paketleri **10 saniyede 10 istek** uyguluyor. Güvenli değer: bağlantı başına **saniyede 1 istek**. |
| Yanıt meta | JSON:API `meta` içinde sayfa bilgisi döner. Alan adları (`total_count`, `total_pages` vb.) ⚠️ ilk istekte doğrulanmalı. Döngü, boş `data` gelene kadar devam ettirilebilir. |

### 2.1 Tarih filtresi ⚠️

`filter[issue_date]` parametresi var ✅, ama **aralık sözdizimi dokümanda belirtilmiyor.** İlk entegrasyon testinde şunlar denenmeli:

1. `filter[issue_date]=2026-10-01..2026-10-31`
2. Tek tarih `filter[issue_date]=2026-10-15`

Aralık desteklenmiyorsa güvenli yöntem şu:
`sort=-issue_date` ile azalan sırada sayfa sayfa okuyup, `issue_date` pencerenin başlangıcından küçük ilk kayıtta durmak.

## 3. Kalem 1: Satış faturaları

### 3.1 Uç nokta

```
GET /v4/{company_id}/sales_invoices
  ?filter[issue_date]=...
  &sort=-issue_date
  &page[number]=1&page[size]=25
  &include=contact,details,details.product,category,payments,payments.transaction,active_e_document
```

Diğer filtreler ✅: `filter[due_date]`, `filter[contact_id]`, `filter[invoice_id]`, `filter[invoice_series]`.
Sıralanabilir alanlar ✅: `id, issue_date, due_date, remaining, remaining_in_trl, description, net_total`.

Tek kayıt: `GET /v4/{company_id}/sales_invoices/{id}?include=...` ✅

### 3.2 `item_type` değerleri ✅ ve eşleme

| Paraşüt `item_type` | Ortak model | Çekilsin mi |
|---|---|---|
| `invoice` | `direction=outgoing`, `type=SATIS` (tevkifatlıysa `TEVKIFAT`), `status_normalized=active` | Evet |
| `refund` | `direction=outgoing`, `type=IADE`, `status_normalized=active` | Evet |
| `cancelled` | `direction=outgoing`, `type=SATIS`, `status_normalized=cancelled` | Evet (beyana girmez) |
| `estimate` | proforma | **Hayır** |
| `recurring_invoice`, `recurring_estimate` | şablon | **Hayır** |

`archived=true` faturalar da çekilir. Arşivlenmiş olmak beyandan çıkarmaz.

### 3.3 Başlık alanları (attributes) ✅

| Paraşüt alanı | Açıklama | Ortak model |
|---|---|---|
| `invoice_series` + `invoice_id` | Seri + sıra | `invoice_number` (yan yana) |
| `issue_date` | Düzenleme tarihi | `issue_date`, `period` |
| `due_date` | Vade | `payment_due_date` |
| `currency` | `TRL`, `USD`, `EUR`, `GBP` | `currency` (`TRL` → `TRY`) |
| `exchange_rate` |  | `exchange_rate` |
| `gross_total` | Ara toplam | `amounts.line_extension_amount` |
| `total_discount` + `total_invoice_discount` | Satır + fatura altı iskonto | `amounts.allowance_total_amount` |
| `before_taxes_total` | Vergiler hariç toplam | `amounts.tax_exclusive_amount` |
| `total_vat` | Toplam KDV | `amounts.tax_amount` |
| `vat_withholding` | Tevkifat tutarı | `amounts.withholding_amount` |
| `vat_withholding_rate` | Tevkifat oranı | `withholding_rate` (yüzde: `5/10` → `50`; Paraşüt değerinin biçimi doğrulanmalı ⚠️: `50` mi `0.5` mi) |
| `withholding` | Stopaj tutarı | `amounts.income_tax_withholding_amount` |
| `withholding_rate` | Stopaj oranı | `income_tax_withholding_rate` |
| `total_excise_duty` | ÖTV | `amounts.excise_amount` |
| `total_communications_tax` | ÖİV | `amounts.communications_tax_amount` |
| `net_total` | Genel toplam | `amounts.payable_amount` |
| `remaining`, `remaining_in_trl` | Açık bakiye | `remaining_amount` |
| `payment_status` | `paid`, `overdue`, `unpaid`, `partially_paid` | `payment_status` |
| `tax_number`, `tax_office` | Fatura üzerindeki müşteri bilgisi | `customer_tax_number` (10 hane) / `customer_identity_number` (11 hane), öncelikli. `customer_name` için `contact.name` |
| `is_abroad` | Yurt dışı | ihracat işareti |
| `description` |  | `notes` |
| `created_at`, `updated_at` |  | `source_updated_at` |

**Dikkat:** Paraşüt'te `gross_total` "Ara toplam", `net_total` "Genel toplam" anlamına geliyor. İsimler ters gibi duruyor ama SDK açıklamaları bu şekilde. ✅

### 3.4 Satır alanları (`details`, tip `sales_invoice_details`) ✅

| Paraşüt alanı | Ortak model (`invoice_item`) |
|---|---|
| `quantity` | `quantity` |
| `unit_price` | `unit_price` |
| `vat_rate` | `tax_rate` |
| `discount_type` (`percentage` / `amount`) + `discount_value` | `own_allowance_amount` (yüzde ise hesapla) |
| `excise_duty_type` + `excise_duty_value` | `excise_rate` / `excise_amount` |
| `communications_tax_rate` | `communications_tax_rate` |
| `net_total` | `tax_exclusive_amount` ⚠️ (KDV hariç olduğu doğrulanmalı) |
| `description` | `description` |
| `product` (ilişki) → `name`, `code` | `product_code`, açıklama boşsa `description` |

Satır KDV tutarı gelmiyor. Hesaplanır: `total_tax_amount = tax_exclusive_amount × tax_rate / 100`. Toplamı başlıktaki `total_vat` ile kuruş farkıyla eşleşmeli. Fark 0,05 TL'yi geçerse uyarı üretilir.

### 3.5 Karşı taraf (`contact`) ✅

`name`, `tax_number` (VKN/TCKN), `tax_office`, `contact_type` (`person` / `company`), `account_type` (`customer` / `supplier`), `city`, `district`, `address`, `is_abroad`, `iban`, `email`, `phone`.

### 3.6 E-belge ve PDF ✅

1. `include=active_e_document` ile resmileşmiş belge ilişkisi gelir. Türü `e_archives` veya `e_invoices`'dır. Buradan `profile` belirlenir: `e_archives` → `EARSIVFATURA`; `e_invoices` → senaryo e-fatura detayından (`TEMELFATURA` / `TICARIFATURA` ⚠️). İlişki yoksa `profile = NULL` (kâğıt).
2. PDF için:
   - `GET /v4/{company_id}/e_archives/{e_doc_id}/pdf`
   - `GET /v4/{company_id}/e_invoices/{e_doc_id}/pdf`
3. PDF hazır değilse **204** döner. 204 almayana kadar aralıklı tekrar edilir.
4. Dönen URL **1 saat geçerlidir.** Hemen indirilip depoya yazılır.

ETTN (UUID) alanının e_invoice/e_archive kaydındaki adı SDK'da görünmüyor ⚠️. `GET /e_invoices/{id}` yanıtında kontrol edilmeli. Varsa `uuid`'ye yazılır.

### 3.7 Tahsilatlar

`include=payments,payments.transaction` ile her tahsilat ve onu oluşturan kasa/banka hareketi gelir. Bunlar `allocations` tablosunu doldurur (bkz. Kalem 3).

## 4. Kalem 2: Alış faturaları

### 4.1 Uç nokta

```
GET /v4/{company_id}/purchase_bills
  ?filter[issue_date]=...
  &sort=-issue_date
  &page[number]=1&page[size]=25
  &include=supplier,details,details.product,category,spender,pay_to,payments,payments.transaction,active_e_document
```

Filtreler ✅: `filter[issue_date]`, `filter[due_date]`, `filter[spender_id]`.
Sıralama ✅: `id, issue_date, due_date, remaining, remaining_in_trl`.

**Not:** Listeleme için dokümanda `include` listesinde `supplier` yok, ama ilişki modelde mevcut (`PurchaseBillRelationships.supplier`). ⚠️ `include=supplier` denenmeli. Çalışmazsa her kayıt için `GET /purchase_bills/{id}` ile detay çekilir.

### 4.2 `item_type` değerleri ✅

| Paraşüt `item_type` | Ortak model |
|---|---|
| `purchase_bill` | `direction=incoming`, `type=ALIS` (tevkifatlıysa `TEVKIFAT`), `status_normalized=active` |
| `refund` | `direction=incoming`, `type=IADE` |
| `cancelled` | `direction=incoming`, `status_normalized=cancelled` |
| `recurring_purchase_bill` | **Çekilmez** |

### 4.3 Fatura mı masraf fişi mi? (kritik)

Paraşüt'te "Giderler > Fiş/Fatura" ekranındaki her şey `purchase_bills` içindedir. Ayrım `is_detailed` ile yapılır: ✅

| Durum | Anlamı | Ortak modele yazılacak yer |
|---|---|---|
| `is_detailed = true` | Kalemli alış faturası | `invoice` (`direction=incoming`) |
| `is_detailed = false` ve `active_e_document` var | Kalemsiz girilmiş e-fatura | `invoice` (`has_items=false`) |
| `is_detailed = false` ve e-belge yok | Fiş, gider pusulası, kalemsiz masraf | `expense` (bkz. Kalem 5) |

### 4.4 Başlık alanları ✅

`invoice_no`, `issue_date`, `due_date`, `currency`, `exchange_rate`, `gross_total`, `total_discount`, `total_invoice_discount`, `total_vat`, `total_excise_duty`, `total_communications_tax`, `net_total`, `withholding_rate`, `vat_withholding_rate`, `remaining`, `remaining_in_trl`, `total_paid`, `payment_status`, `is_detailed`, `e_invoices_count`, `description`.

Eşleme satış faturasıyla aynıdır. Farklar:
- Alışta `before_taxes_total`, `withholding` ve `vat_withholding` **tutar alanı yok**, sadece oranlar var ✅. Tevkifat tutarı satırlardan hesaplanır: `Σ(satır KDV) × tevkifat oranı`.
- `invoice_no` tek alan halinde gelir.

### 4.5 Satırlar (`purchase_bill_details`) ✅

Satış satırlarıyla aynı alanlar: `quantity`, `unit_price`, `vat_rate`, `discount_type`, `discount_value`, `excise_duty_type`, `excise_duty_value`, `communications_tax_rate`, `net_total`, `description`.

### 4.6 Diğer ilişkiler

- `spender`: Masrafı cebinden ödeyen çalışan. `expense.personnel_id` için kullanılır (personel adına ödenen masraf).
- `pay_to`: Ödeme yapılacak cari.
- `category`: Gider kategorisi (Kalem 4).

### 4.7 Gelen e-faturalar hakkında önemli not ❌

`GET /e_invoice_inboxes` **gelen faturaları listelemez.** Bu uç nokta, bir VKN'nin e-fatura mükellefi olup olmadığını sorgular. Dönen alanlar: `vkn`, `e_invoice_address`, `name`, `inbox_type`, `address_registered_at`, `registered_at`. ✅

Mükellefin Paraşüt'e düşmüş ama henüz alış faturasına dönüştürmediği gelen e-faturalar, Paraşüt v4 API'sinden **okunamaz.** Bunlar e-belge entegratörü üzerinden alınmalıdır.

## 5. Kalem 3: Finansal işlemler

### 5.1 Hesaplar ✅

```
GET /v4/{company_id}/accounts?page[size]=25
```

| Paraşüt alanı | Ortak model (`cash_box`) |
|---|---|
| `account_type`: `cash` / `bank` / `sys` | `type`: `cash` / `bank_account` / eşlenmez (`sys` sistem hesabıdır, ör. çek/senet ara hesabı ⚠️, ham saklanır) |
| `name` | `name` |
| `currency` | `currency` |
| `bank_name`, `bank_branch`, `bank_account_no`, `iban` | aynen |
| `balance` | `balance` |
| `bank_integration_type` | Paraşüt'ün kendi banka entegrasyonu var mı (bilgi amaçlı) |
| `archived` | arşivlenmiş hesaplar da çekilir |

Filtreler ✅: `filter[name]`, `filter[currency]`, `filter[bank_name]`, `filter[bank_branch]`, `filter[account_type]`, `filter[iban]`.

### 5.2 Hesap hareketleri ✅

```
GET /v4/{company_id}/accounts/{account_id}/transactions
  ?filter[date]=...
  &include=debit_account,credit_account
  &page[size]=25
```

Her hesap için ayrı çağrılır.

| Paraşüt alanı | Ortak model (`financial_transaction`) |
|---|---|
| `date` | `transaction_date` |
| `description` | `description` |
| `transaction_type` | `transaction_type` + `transaction_subtype` (eşleme tablosu aşağıda ⚠️) |
| `debit_amount` + `debit_currency` | Bu hesaba göre **giriş mi çıkış mı** belirlenir |
| `credit_amount` + `credit_currency` |  |
| `amount_in_trl` | `amount_try` |
| `debit_account` / `credit_account` (ilişki) | `cash_box_id` = sorgulanan hesap. İki taraf da kasa/banka ise hareket `cash_box_transfer` olarak yazılır (`from_cash_box_id` = credit, `to_cash_box_id` = debit) |

**Yön kuralı:** Sorgulanan hesap `debit_account` ise para **bu hesaba girmiştir** (`transaction_type = income`). `credit_account` ise **çıkmıştır** (`transaction_type = expense`). ⚠️ İlk testte gerçek bir tahsilat ve ödeme ile teyit edilmeli.

**`transaction_type` değerleri SDK'da enum olarak listelenmiyor** ⚠️. İlk senkronda gelen tüm farklı değerler loglanmalı ve şu eşleme tablosu doldurulmalı:

| Beklenen anlam | Ortak model `transaction_type` / `transaction_subtype` |
|---|---|
| Müşteriden tahsilat | `income` / `collection` |
| Tedarikçiye ödeme | `expense` / `payment` |
| Hesaplar arası transfer | `cash_box_transfer` kaydı |
| Maaş ödemesi | `expense` / `salary_payment` |
| Vergi ödemesi | `expense` / `tax_payment` |
| Banka gideri | `expense` / `bank_fee` |
| Açılış bakiyesi | `opening_balance` |
| Düzeltme | `income` veya `expense` / `adjustment` |

`payment_method_type` Paraşüt'te ayrı alan olarak yok. Hesap tipinden türetilir: `cash` hesap → `cash`, `bank` hesap → `bank_transfer`.

### 5.3 Faturaya bağlama

Fatura ile `include=payments.transaction` alındığında `payments[].relationships.transaction.data.id` hareket kimliğini verir ✅. `payments` öznitelikleri: `date`, `amount`, `currency`, `notes`. Bunlardan `allocations` satırı üretilir.

### 5.4 Cari bazlı tahsilat ve ödemeler

Faturaya bağlanmadan doğrudan cariye yapılan tahsilat ve ödemeler de hesap hareketlerinde görünür. Cari bağlantısı için hareketin `payments` ilişkisi kontrol edilir ⚠️.

## 6. Kalem 4: Gelir ve gider türleri ✅

```
GET /v4/{company_id}/item_categories
  ?filter[category_type]=Expenditure
  &include=parent_category,subcategories
  &page[size]=25
```

| `category_type` ✅ | Ortak model |
|---|---|
| `Expenditure` | `expense_category`, `scope=expense` |
| `SalesInvoice` | `expense_category`, `scope=income` |
| `Product`, `Contact`, `Employee` | Beyan dışı. Çekilmez. |

Alanlar: `name`, `full_path`, `parent_id`, `bg_color`, `text_color`.

`Expenditure` ve `SalesInvoice` için ayrı çağrı yapılır.

`full_path` (ör. `Araç > Akaryakıt`) `canonical_type` eşlemesinde ana girdi olarak kullanılır (bkz. `01` §4.11).

## 7. Kalem 5: Masraf fişleri

Paraşüt'te masraf ve gider üç kaynaktan okunur. Maaşlar Easytrade'deki ayrıma uygun olarak `personnel_transaction`'a yazılır (§7.2):

| Kaynak | Uç nokta | Ortak model `receipt_type` |
|---|---|---|
| Kalemsiz fiş/fatura (e-belgesiz) | `purchase_bills` (`is_detailed=false`, e-belge yok) | `okc_receipt` / `other` |
| Vergiler | `GET /v4/{company_id}/taxes` ✅ | `tax_payment` |
| Banka giderleri | `bank_fees` ⚠️ (aşağıda) | `bank_fee` |

### 7.1 Kalemsiz fiş/fatura

- `purchase_bills` listesinden süzülür (bkz. §4.3).
- `total_vat` ve `net_total` başlıktan alınır.
- Oran kırılımı yoksa `header_estimate` kuralı uygulanır.
- Fişin görseli API'den alınamıyor ⚠️. Paraşüt'te dosya eki uç noktası SDK'da yok.

### 7.2 Maaş ve vergi

```
GET /v4/{company_id}/salaries?filter[issue_date]=...&include=employee,category,payments
GET /v4/{company_id}/taxes?filter[issue_date]=...&include=category,payments
```

Alanlar ✅: `description`, `issue_date`, `due_date`, `net_total`, `currency` (vergide yok), `exchange_rate`, `total_paid`, `remaining`, `remaining_in_trl`, `archived`.
Filtreler ✅: `filter[issue_date]`, `filter[due_date]`, `filter[currency]`, `filter[remaining]`.

Eşleme:
- `salaries` → `personnel_transaction`, `transaction_type=salary_accrual`, `amount=net_total`, `transaction_date=issue_date`, personel `employee` ilişkisinden. Ödemeleri (`payments`) → `transaction_type=salary_payment`.
- `taxes` → `expense`, `receipt_type=tax_payment`, `amount=net_total`, `expense_date=issue_date`, `tax_rate=0_kdv`.

### 7.3 Banka giderleri ⚠️

`bank_fees` için **listeleme uç noktası yok**, sadece `GET /bank_fees/{id}` var ✅. Banka giderlerine ulaşmak için hesap hareketlerinde banka gideri ödemesi olan kayıtlardan `payments → payable` ilişkisi izlenir. Bu yol test edilmeden üretime alınmamalı.

### 7.4 Plaka

Paraşüt'te plaka için ayrı bir alan yok. `description` ve satır açıklamalarında plaka regex'i çalıştırılır:

```
\b(0[1-9]|[1-7][0-9]|8[01])\s?[A-Z]{1,3}\s?\d{2,4}\b
```

Regex, metin büyük harfe çevrildikten (Türkçe locale ile) sonra çalıştırılır. Bulunan plaka boşluksuz hale getirilip `vehicle_plate`'e yazılır, `capture_method = api` olarak işaretlenir.

## 8. Senkron planı

| Adım | Çağrı | Sıklık |
|---|---|---|
| 1 | `GET /me` (firma doğrulama) | Bağlantıda + günde 1 |
| 2 | `item_categories` (5 tip) | Günde 1 |
| 3 | `accounts` | Günde 1 |
| 4 | `contacts` (`sort=id`, tam liste) | Günde 1 |
| 5 | `sales_invoices` (açık dönemler) | 4 saatte 1 |
| 6 | `purchase_bills` (açık dönemler) | 4 saatte 1 |
| 7 | Her hesap için `accounts/{id}/transactions` (açık dönemler) | 4 saatte 1 |
| 8 | `salaries` (→ personel), `taxes` (→ masraf) (açık dönemler) | Günde 1 |
| 9 | Yeni veya değişen e-belgeli faturaların PDF'i | Kuyruktan |

İstek bütçesi (dakikada 60 sınırıyla): ayda 300 fatura ve 3 hesabı olan bir mükellefin açık dönem taraması yaklaşık 40–60 istek tutar. Bağlantı başına hız sınırlayıcı şart.

## 9. Doğrulanacaklar listesi (sandbox veya gerçek hesapta ilk iş)

- [ ] `filter[issue_date]` aralık sözdizimi
- [ ] `page[size]` üst sınırı ve `meta` alan adları
- [ ] `purchase_bills` listesinde `include=supplier` çalışıyor mu
- [ ] `vat_withholding_rate` değer biçimi
- [ ] Satır `net_total`'ın KDV hariç olduğu
- [ ] `transaction_type` değerlerinin tam listesi
- [ ] Hareketlerde debit/credit yön kuralı
- [ ] E-fatura/e-arşiv kaydında ETTN alan adı
- [ ] Webhook desteği (topluluk paketlerinde `Webhooks` geçiyor, resmi spec'te teyit edilmeli). Varsa artımlı senkron için kullanılır.

## Kaynaklar

- Paraşüt API dokümanı: https://apidocs.parasut.com
- Resmi OpenAPI'den üretilmiş SDK: npm `parasut.js` v4.0.0 (alan ve uç nokta listeleri buradan alındı)
- Topluluk paketi (rate limit, grant tipleri): https://packagist.org/packages/northlab/parasut-laravel
