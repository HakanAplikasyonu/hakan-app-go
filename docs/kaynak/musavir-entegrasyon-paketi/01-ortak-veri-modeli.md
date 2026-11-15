# 01 · Ortak Veri Modeli (Easytrade standardı)

Bütün kaynak adaptörleri veriyi **bu şemaya** dönüştürür. Portalın kontrol katmanı (mükerrer fatura, KDV uyarısı, ödeme türü mutabakatı, plaka kontrolü) ve Luca'ya çıkış sadece bu modeli okur.

**Temel karar:** Alan adları, enum değerleri, yanıt zarfı ve liste kuralları **Easytrade Muhasebe API ile birebir aynıdır** (`https://pro.easytradetr.com/api`). Backend ekibi zaten bildiği yapıyla çalışır. Easytrade kullanan mükellefin verisi neredeyse dönüşümsüz akar. Diğer programlar bu yapıya eşlenir.

## 0. İşaretler

| İşaret | Anlamı |
|---|---|
| (yok) | Alan Easytrade API'de **aynı adla** var |
| **⊕** | **Portal uzantısı.** Easytrade API'de karşılığı yok, ama beyan veya kontrol için gerekli. Easytrade'e eklenmesi önerilmez. Sadece portal tablosunda tutulur. |
| **⚠️** | Easytrade dokümanında alan var ama değer listesi veya yapısı belirtilmemiş. `09-easytrade.md` §9'daki sorular cevaplanınca kesinleşir. |

## 1. Genel kurallar

### 1.1 Easytrade'den aynen alınan kurallar

| Konu | Kural |
|---|---|
| Yanıt zarfı | `{ "success", "message", "data", "meta" }` · hata: `{ "success": false, "message", "errors", "data": null, "error_code" }` |
| Sayfalama | `page`, `per_page` (en fazla 100), `sort`, `direction` (`asc`/`desc`), `search`, `filters[...]` · `meta`: `current_page`, `last_page`, `per_page`, `total` |
| Tarih aralığı filtresi | `filters[<tarih_alanı>][start]` / `filters[<tarih_alanı>][end]`, `Y-m-d` |
| Bağlam başlıkları | `X-Company-ID` (mükellef şirketi), `X-Branch-ID` (şube) |
| Tekrar denenebilir yazma | `Idempotency-Key` başlığı. Aynı anahtar + farklı içerik → `409 IDEMPOTENCY_MISMATCH` |
| Enum yanıtları | `{ "value": "...", "label": "..." }` biçiminde. Portal **`value`** saklar. |
| Para birimi | `TRY`, `USD`, `EUR`, `GBP` (Easytrade'in desteklediği set). Diğer kaynaklardan gelen ISO kodları (`CHF` vb.) olduğu gibi saklanır. |
| Birim | UBL birim kodları (`C62`, `KGM`, `LTR`, `MTR` ...) |
| Tevkifat oranı | **Yüzde olarak** tutulur: `5/10` → `50`, `9/10` → `90` (Easytrade `withholding_rate` kuralı) |
| Tevkifat kodları | GİB kodları `601`–`627`, `650` (Easytrade `GET /company/withholding-codes`) |
| İstisna kodları | GİB kodları `101`–`351` (Easytrade `GET /company/tax-exemption-codes`) |
| Özel matrah | `801`–`811` · İhraç kayıtlı: `701`–`704` |

### 1.2 Portalın eklediği kurallar

1. **Kaynak kimliği korunur.** Her kayıtta `source_system` ⊕ + `source_id` ⊕ + `capture_method` ⊕ (`api`, `excel_import`, `ocr`, `manual`, `qr`, `bank_match`: banka hareketinden müşavir onayıyla oluşturuldu, `21` §6; `ubl_upload`: mükellefin yüklediği e-fatura XML/ZIP, `31`) bulunur. Benzersiz anahtar: `(company_id, source_system, source_id)`. Excel ile gelen kayıtta `source_system` asıl programın adıdır (bkz. `04`).
2. **Ham veri saklanır.** `raw_payload` ⊕ (JSONB) her zaman tutulur. Eşleme hatası çıkarsa kaynağa tekrar gitmeden yeniden işlenir.
3. **Para tipi.** `NUMERIC(18,4)`, float değil. Easytrade bazı tutarları `string` döndürüyor. Adaptör sayıya çevirir.
4. **Tarih.** Belge tarihleri `DATE`, zaman damgaları `TIMESTAMPTZ`, saat dilimi `Europe/Istanbul`.
5. **İşaret.** Tutarlar **pozitif** saklanır. Yön `direction`, `transaction_type` gibi alanlarla verilir.
6. **Dönem.** Her belgeye `period` ⊕ (`YYYY-MM`) atanır. Kilitli dönemde değişiklik kaydı değiştirmez, `change_request` olarak düşer (bkz. `02`).
7. **Uyarılar.** Bütün veri uyarıları `data_warning` ⊕ tablosuna kodla yazılır. Kod listesi ve tablo: `05`.

### 1.3 Veritabanı kuralları (kod yazılırken)

| Konu | Kural |
|---|---|
| Kimlik | Bütün tablolarda `id UUID` |
| Ortak kolonlar | Kaynaktan gelen her varlık tablosunda (`customer`, `invoice`, `retail_sale`, `retail_refund`, `cash_box`, `financial_transaction`, `cash_box_transfer`, `expense_category`, `expense`, `personnel_transaction`, `check`, `promissory_note`, `bank_transaction`): `company_id`, `source_system`, `source_id`, `capture_method`, `raw_payload`, `content_hash`, `deleted_at_source`, `created_at`, `updated_at`; dönemli olanlarda ayrıca `period`. Tablolarda tekrar yazılmadıysa da vardır. Alt tablolar (`invoice_item`, `invoice_tax_summary`) üst kaydın kimliğini taşır. |
| Tekil anahtar | `(company_id, source_system, source_id)`; `invoice`'ta ayrıca `(company_id, uuid)` mükerrer kontrolü için indeks (tekil değil; `01` §6) |
| `amounts.*` | Veritabanında **düz kolon** (`tax_exclusive_amount`, `tax_amount`, `payable_amount` …; nokta öncesi kısım atılır). API yanıtında Easytrade gibi `amounts` nesnesi altında döner. |
| Enum | Veritabanı enum tipi yerine `TEXT` + kontrol kısıtı (değer listesi büyüyebilir). API'de `{ value, label }`. |
| Çok kiracılık | Her sorgu `company_id` ile, `company.tenant_id` üzerinden müşavir ofisine bağlı. Müşavir kullanıcısı sadece kendi ofisinin mükelleflerini görür. |
| Para / tarih | §1.2 kural 3 ve 4 |

## 2. Kapsam: beyan için çekilen veri kalemleri

| # | Kalem | Ortak varlık | Beyandaki yeri |
|---|---|---|---|
| 1 | Satış faturaları | `invoice` (`direction=outgoing`) | KDV1 hesaplanan KDV, matrah, tevkifat, istisna |
| 2 | Alış faturaları | `invoice` (`direction=incoming`) | KDV1 indirilecek KDV, tevkifat |
| 3 | Finansal işlemler | `financial_transaction`, `cash_box_transfer` | Mizan, tahsilat/ödeme, ödeme türü mutabakatı |
| 4 | Gelir ve gider türleri | `expense_category` | Hesap planı eşlemesi |
| 5 | Masraf ve giderler | `expense` | İndirilecek KDV (fişli giderler), gider kaydı |
| 6 | Perakende satışlar ve iadeleri | `retail_sale`, `retail_refund` | Faturasız satış KDV'si (ÖKC/Z raporu karşılığı) |
| 7 | Personel ödemeleri | `personnel_transaction` | Ücret ödemeleri (muhtasar için bilgi) |
| 8 | Çek ve senetler | `check`, `promissory_note` | Portföy, tahsilat |
| 9 | Referans kartlar | `customer`, `cash_box` | Cari ve hesap eşlemesi |
| 10 | Kontrol verisi | `vat_report` | Kaynak KDV toplamıyla mutabakat |
| 11 | Banka hareketleri | `bank_transaction` ⊕, `bank_balance_snapshot` ⊕ | Faturasız tahsilat, belgesiz ödeme, banka masrafı, faiz, vergi/SGK ödemesi, bakiye mutabakatı (`21`) |

İlk beş kalem her kaynaktan beklenir. 6–10 kaynak veriyorsa çekilir (hangi kaynağın neyi verdiği `03` §1). Kalem 11 ön muhasebe programından değil, doğrudan bankadan veya banka dosyasından gelir (`20`–`23`).

## 3. Varlıklar

```
company (mükellef şirketi, Easytrade: user.company)
 ├── source_connection ⊕
 │          └── entegrator_document ⊕ (entegratör belge indeksi, `37` §5.1)
 ├── customer (cari)
 ├── invoice ── items[] ── tax_breakdowns[]
 │          └── tax_summary[] ⊕
 ├── retail_sale ── items[]          retail_refund
 ├── cash_box ── financial_transaction
 │          └── cash_box_transfer
 ├── expense_category ── expense
 ├── personnel_transaction
 ├── check, promissory_note
 └── attachment ⊕ (PDF, UBL XML, fiş görseli)
```

## 4. Tablolar

### 4.1 `company` (mükellef)

| Alan | Tip | Easytrade | Açıklama |
|---|---|---|---|
| id | UUID | | Portal kimliği |
| tenant_id | UUID | ⊕ | Mali müşavir ofisi |
| name | TEXT | `company.name` | |
| trade_name | TEXT | `company.trade_name` | Unvan |
| tax_number | VARCHAR(11) | `company.tax_number` | VKN (10) veya TCKN (11) |
| tax_office | TEXT | `company.tax_office` | |
| ledger_type | ENUM | ⊕ | `bilanco`, `isletme`, `serbest_meslek`, `basit_usul` |
| is_e_invoice_user | BOOL | ⊕ | GİB e-fatura mükellefi mi (Easytrade `GET /company/e-invoice/check-user` ile sorgulanabilir) |
| is_e_ledger_user | BOOL | ⊕ | |
| vat_period | ENUM | ⊕ | `monthly`, `quarterly` |

### 4.2 `source_connection` ⊕

| Alan | Tip | Açıklama |
|---|---|---|
| id | UUID | |
| company_id | UUID | |
| source_system | ENUM | `easytrade`, `parasut`, `kolaybi`, `bizimhesap`, `logo_isbasi`, `logo_tiger`, `logo_go`, `mikro`, `robom`, `manual_upload`, `entegrator_*` |
| external_company_id | TEXT | Easytrade `X-Company-ID`, Paraşüt `company_id`, Logo `firmno` vb. |
| external_branch_ids | JSONB | Easytrade şubeleri (`X-Branch-ID`) |
| credentials_ref | TEXT | Gizli anahtar kasası referansı. Kimlik bilgisi bu tabloda açık tutulmaz. |
| setup_state | TEXT | **Kurulum akışı.** Programlar ve entegratörler: `draft` → `credentials_entered` (İşNet'te `authorization_requested`) → `verifying` → `active` / `failed` (`30` §4.3). Bankalar: `draft` → `form_ready` → `submitted_to_bank` → `credentials_entered` → `verifying` → `active` / `failed` (`20` §5.1). |
| status | ENUM | **Çalışma durumu** (kurulum `active` olduktan sonra): `pending`, `active`, `auth_failed`, `rate_limited`, `disabled` |
| last_error | JSONB | Son hata: `{ code, message, at, provider_code }` (`05` §4) |
| meta | JSONB | Kaynağa özel bağlantı bilgisi (İşNet birim listesi, Uyumsoft `HasEInvoice`, e-fatura kullanıcısı mı vb.) |
| consent_text, consent_at | TEXT, TIMESTAMPTZ | Mükellef bağlantı onayı (`02` §10) |
| sync_cursor | JSONB | Kaynak bazında artımlı senkron durumu (Easytrade için `sync/pull` `next_after`, QNB `belgeSiraNo`) |
| capabilities | JSONB | Bu bağlantının hangi kalemleri verebildiği |
| last_full_sync_at, last_success_at | TIMESTAMPTZ | |

`source_system` değerlerinin tam listesi: `easytrade`, `parasut`, `kolaybi`, `bizimhesap`, `logo_isbasi`, `logo_tiger`, `logo_go`, `mikro`, `robom`, `manual_upload` (programı bilinmeyen Excel, mükellefin yüklediği UBL XML/ZIP, OCR; programı bilinen Excel'de `source_system` o programın adıdır, `capture_method=excel_import`, §1.2 kural 1), `entegrator_isnet`, `entegrator_uyumsoft`, `entegrator_izibiz`, `entegrator_qnb_efinans`, `entegrator_edm`, `bank_<bank_id>` (ör. `bank_vakifbank`; `bank_id` değerleri `22` özet tablosu), `bank_file` (ekstre yükleme).

### 4.3 `customer` (cari)

| Alan | Tip | Easytrade | Açıklama |
|---|---|---|---|
| id | UUID | | |
| company_id | UUID | | |
| source_system, source_id | | ⊕ | |
| type | ENUM | `type` | `customer`, `supplier`, `both` · ⊕ `employee` (diğer kaynaklarda personel cari olarak gelebiliyor) |
| customer_legal_type | ENUM | `customer_legal_type` | `individual`, `corporate` |
| trade_name | TEXT | `trade_name` | Kurumsal unvan |
| first_name, last_name | TEXT | `first_name`, `last_name` | Bireysel |
| name | TEXT | `name` / `full_name` | Görünen ad |
| tax_number | VARCHAR(10) | `tax_number` | VKN. **Eşleşmenin ana anahtarı.** |
| identity_number | VARCHAR(11) | `identity_number` | TCKN |
| tax_office | TEXT | `tax_office` | |
| address, city, district, postal_code | TEXT | aynı adlar | |
| email, phone | TEXT | aynı adlar | |
| is_default_retail_customer | BOOL | `is_default_retail_customer` | Perakende "genel müşteri" kartı |
| is_abroad | BOOL | ⊕ | Yurt dışı |
| bank_accounts | JSONB | `bank_accounts` | IBAN'lar |
| raw_payload | JSONB | ⊕ | |

Kural: Aynı şirkette aynı `tax_number` / `identity_number` iki kaynaktan gelirse tek `customer` kaydında birleşir (`customer_source_link` ⊕ ara tablosu).

### 4.4 `invoice` (satış ve alış faturası)

| Alan | Tip | Easytrade | Açıklama |
|---|---|---|---|
| id | UUID | | |
| company_id | UUID | | |
| source_system, source_id | | ⊕ | |
| uuid | UUID | `uuid` | **ETTN.** Mükerrer kontrolünde birinci anahtar. |
| invoice_number | VARCHAR(32) | `invoice_number` | Seri + sıra |
| direction | ENUM | `direction` | `outgoing` (satış), `incoming` (alış) |
| type | ENUM | `type` | `SATIS`, `ALIS`, `IADE`, `TEVKIFAT`, `ISTISNA`, `OZELMATRAH`, `IHRACKAYITLI`, `SGK` |
| profile | ENUM | `profile` | `TEMELFATURA`, `TICARIFATURA`, `EARSIVFATURA`, `IHRACAT`, `YOLCUBERABERFATURA`, `OZELFATURA`, `KAMU`, `ILAC_TIBBICIHAZ` · **`NULL` = kâğıt / bilinmiyor** |
| document_class | ENUM | ⊕ | `invoice` (varsayılan), `smm` (serbest meslek makbuzu), `mustahsil`, `price_difference` (fiyat farkı) |
| status | VARCHAR | `status.value` ⚠️ | Kaynağın ham durumu |
| status_normalized | ENUM | ⊕ | `draft`, `active`, `cancelled`. **Beyan sadece `active` alır.** |
| issue_date | DATE | `issue_date` | Düzenleme tarihi. Beyan dönemini belirler. |
| issue_time | TIME | `issue_time` | |
| payment_due_date | DATE | `payment_due_date` | |
| period | CHAR(7) | ⊕ | `2026-10` |
| customer_id | UUID | `customer.id` | Karşı taraf |
| customer_tax_number | VARCHAR(10) | `customer.tax_number` | Denormalize |
| customer_identity_number | VARCHAR(11) | `customer.identity_number` | Denormalize |
| customer_name | TEXT | `customer.name` | Denormalize |
| currency | CHAR(3) | `currency.code` | |
| exchange_rate | NUMERIC(18,6) | `exchange_rate` (istek alanı) ⚠️ yanıtta | |
| amounts.line_extension_amount | NUMERIC | ⊕ (satırlarda var) | Satır tutarları toplamı (iskonto öncesi) |
| amounts.allowance_total_amount | NUMERIC | ⊕ (satırlarda var) | Toplam iskonto |
| amounts.tax_exclusive_amount | NUMERIC | `amounts.tax_exclusive_amount` | KDV matrahı toplamı |
| amounts.tax_amount | NUMERIC | `amounts.tax_amount` | Hesaplanan KDV |
| amounts.tax_inclusive_amount | NUMERIC | ⊕ (satırlarda var) | |
| amounts.withholding_amount | NUMERIC | ⊕ (satır `tax_breakdowns`'tan) | Tevkif edilen KDV |
| amounts.excise_amount | NUMERIC | ⊕ | ÖTV |
| amounts.communications_tax_amount | NUMERIC | ⊕ | ÖİV |
| amounts.income_tax_withholding_amount | NUMERIC | ⊕ | Stopaj |
| income_tax_withholding_rate | NUMERIC(5,2) | ⊕ | |
| amounts.payable_amount | NUMERIC | `amounts.payable_amount` | Ödenecek / tahsil edilecek |
| amounts.payable_amount_try | NUMERIC | ⊕ | TL karşılığı |
| global_tax_exemption_code | VARCHAR(3) | `meta.global_tax_exemption_code` | Fatura geneli istisna kodu |
| original_invoice_number | VARCHAR(32) | `meta.original_invoice_number` | İade faturasında asıl fatura |
| original_invoice_issue_date | DATE | `meta.original_invoice_issue_date` | |
| order_reference | JSONB | `meta.order_reference` | |
| is_internet_sale | BOOL | `meta.is_internet_sale` | |
| sgk | JSONB | `sgk` | SGK faturası alanları |
| despatch_id | | `despatch_id` | Bağlı irsaliye |
| notes | TEXT | `notes` | |
| payment_status | ENUM | ⊕ | `paid`, `partially_paid`, `unpaid`, `overdue`, `unknown` |
| remaining_amount | NUMERIC | ⊕ | Açık bakiye |
| has_items | BOOL | ⊕ | Satır detayı geldi mi |
| tax_summary_source | ENUM | ⊕ | `items`, `source_summary`, `header_estimate` |
| content_hash | CHAR(64) | ⊕ | Değişiklik tespiti |
| source_updated_at | TIMESTAMPTZ | `updated_at` | |
| raw_payload | JSONB | ⊕ | |

**`type` ve `direction` birlikte okunur.** Örnek: alış iadesi `direction=incoming, type=IADE`. Tevkifatlı satış `direction=outgoing, type=TEVKIFAT`. Kaynağın tek bir "tür" alanı varsa adaptör ikisine böler (eşleme `03` §2'de).

### 4.5 `invoice_item`

| Alan | Tip | Easytrade | Açıklama |
|---|---|---|---|
| id | UUID | | |
| invoice_id | UUID | | |
| line_no | INT | ⊕ | |
| name | TEXT | `items[].name` | Kalem adı. **Demirbaş ve kayıt türü uyarıları bu alandan beslenir.** |
| description | TEXT | `items[].description` | |
| product_code | TEXT | `items[].product.sku` ⚠️ | Kaynaktaki ürün/hizmet kodu |
| quantity | NUMERIC(18,6) | `items[].quantity` | |
| unit_code | VARCHAR(8) | `items[].unit.code` | UBL kodu |
| unit_price | NUMERIC(18,6) | `items[].unit_price` | |
| line_extension_amount | NUMERIC | `items[].line_extension_amount` | Miktar × fiyat |
| own_allowance_amount | NUMERIC | `items[].own_allowance_amount` | Satırın kendi iskontosu |
| allowance_total_amount | NUMERIC | `items[].allowance_total_amount` | Satır + fatura geneli iskonto payı |
| tax_exclusive_amount | NUMERIC | `items[].tax_exclusive_amount` | KDV matrahı |
| tax_rate | NUMERIC(5,2) | `lines[].tax_rate` (istek) · yanıtta `tax_breakdowns` ⚠️ | `0`, `1`, `8`, `10`, `18`, `20` |
| total_tax_amount | NUMERIC | `items[].total_tax_amount` | |
| tax_inclusive_amount | NUMERIC | `items[].tax_inclusive_amount` | |
| withholding_code | VARCHAR(3) | `lines[].withholding_code` | `601`–`650` |
| withholding_rate | NUMERIC(5,2) | `lines[].withholding_rate` | Yüzde: `5/10` → `50` |
| withholding_amount | NUMERIC | ⊕ (`tax_breakdowns`'tan) | |
| tax_exemption_code | VARCHAR(3) | ⊕ (satır bazında) | Fatura geneli için `global_tax_exemption_code` |
| special_tax_base_code | VARCHAR(3) | `lines[].special_tax_base_code` | `801`–`811` |
| special_tax_base_amount | NUMERIC | `lines[].special_tax_base_amount` | |
| export_registered_code | VARCHAR(3) | `items[].export_registered_code` | `701`–`704` |
| gtip_code | VARCHAR(12) | `items[].gtip_code` | |
| diib_line_code | TEXT | `items[].diib_line_code` | |
| excise_rate, excise_amount | NUMERIC | ⊕ | ÖTV |
| communications_tax_rate, communications_tax_amount | NUMERIC | ⊕ | ÖİV |
| expense_category_id | UUID | `items[].expense_category_id` | Gelen faturada kalem masraf olarak işaretlendiyse |
| line_type | ENUM | ⊕ | `goods`, `service`, `fixed_asset_candidate`, `discount`, `expense`, `unknown` |
| tax_breakdowns | JSONB | `items[].tax_breakdowns` ⚠️ | Ham vergi kırılımı |

### 4.6 `invoice_tax_summary` ⊕ (oran bazında)

KDV1'in doğrudan beslendiği tablo. Alan adları Easytrade KDV raporunun (`GET /company/reports/vat`) `totals` yapısıyla aynı: `base`, `vat`, `withholding`.

| Alan | Tip |
|---|---|
| invoice_id | UUID |
| tax_rate | NUMERIC(5,2) |
| base | NUMERIC(18,4) |
| vat | NUMERIC(18,4) |
| withholding | NUMERIC(18,4) |
| withholding_code | VARCHAR(3) |
| tax_exemption_code | VARCHAR(3) |

Üretim sırası:
1. Satırlar varsa satırlardan `GROUP BY tax_rate, withholding_code, tax_exemption_code`.
2. Kaynak oran bazında özet veriyorsa onu kullan (`tax_summary_source = source_summary`).
3. İkisi de yoksa: `tax_rate = round(tax_amount / tax_exclusive_amount × 100)` en yakın geçerli orana yuvarlanır, `tax_summary_source = header_estimate` yazılır ve kontrol katmanında **uyarı** üretilir.

### 4.7 `retail_sale` ve `retail_refund` (perakende)

Faturalanmamış perakende satışlar. ÖKC'den kesilen satışların ve Z raporunun karşılığı. **`invoice_id` doluysa satış zaten faturaya dönüşmüştür ve beyana iki kez girmemelidir.**

`retail_sale`:

| Alan | Easytrade | Açıklama |
|---|---|---|
| sale_number | `sale_number` | |
| sale_date | `sale_date` | |
| customer_id | `customer_id` | |
| subtotal_amount | `subtotal_amount` | |
| total_discount_exclusive | `total_discount_exclusive` | |
| tax_amount | `tax_amount` | |
| total_amount | `total_amount` | |
| currency | `currency` | |
| payment_status | `payment_status` ⚠️ | |
| status | `status` ⚠️ | |
| invoice_id | `invoice_id` | **Çift sayım kontrolü** |
| items[] | `items[]`: `product_name`, `quantity`, `unit_price`, `discount_amount_exclusive`, `taxable_amount`, `tax_rate`, `tax_amount`, `total_item_price`, `refunded_quantity` | Oran bazında KDV buradan |
| payments[] | `financial_transactions[]` | Ödeme türü dağılımı (`payment_method_type`) |
| okc_serial_number, z_report_number, receipt_number | ⊕ | ÖKC fişi kaynaklı satışlarda |

`retail_refund`: `exchange_number`, `exchange_date`, `status`, `sale_id`, `sale_number`, `cash_box_id`, `payment_method`, `refunded_amount`, `new_amount`, `price_difference`, `currency`, `reference_document_no`.

### 4.8 `cash_box` (kasa / banka / kart / POS)

| Alan | Easytrade | Açıklama |
|---|---|---|
| name | `name` | |
| type | `type` | `cash`, `bank_account`, `credit_card`, `valor` (POS / blokeli hesap) |
| bank_id | `bank_id.value` | `akbank`, `garanti`, `is-bankasi`, ... (Easytrade enum) |
| iban | `iban` | Boşluksuz, büyük harf. **Banka verisiyle eşleşme anahtarı.** |
| currency | `currency` | |
| balance | `balance` | |
| opening_balance | `opening_balance` | |
| is_official | `is_official` | Resmi (beyana konu) hesap mı |
| block_days, commission_rate | aynı adlar | POS |
| branch_id | `branch_id` | |
| is_active | `is_active` | |
| suggested_ledger_code | ⊕ | `100`, `102`, `108`, `300` gibi öneri. Müşavir eşlemesi ayrı tabloda. |

### 4.9 `financial_transaction` (tahsilat, ödeme, cari hareket)

| Alan | Easytrade | Açıklama |
|---|---|---|
| customer_id | `customer_id` | |
| cash_box_id | `cash_box_id` | |
| valor_cash_box_id | `valor_cash_box_id` | POS'tan valörlü geçiş |
| transaction_type | `transaction_type` | `income`, `expense`, `retail_sale`, `opening_balance` ⚠️ (anlam: §5) |
| transaction_subtype | ⊕ | `collection`, `payment`, `bank_fee`, `salary_payment`, `tax_payment`, `adjustment`, `other` |
| payment_method_type | `payment_method_type` | `cash`, `bank_transfer`, `credit_card`, `check`, `promissory_note`, `credit` · ⊕ `unknown` |
| amount | `amount` | Pozitif |
| currency | `currency` | |
| exchange_rate | `exchange_rate` | |
| amount_try | ⊕ | |
| transaction_date | `transaction_date` | |
| due_date | `due_date` | |
| valor_date | `valor_date` | |
| period | ⊕ | |
| description | `description` | |
| reference_document_no | `reference_document_no` | Makbuz / dekont no |
| referenceable_type, referenceable_id | `referenceable_type`, `referenceable_id` | `Invoice` veya `Sale` bağlantısı |
| status | `status` ⚠️ | |
| transaction_flow | `transaction_flow` ⚠️ | |
| cari_borc, cari_alacak | `cari_borc`, `cari_alacak` | |
| allocations[] | ⊕ | Bir tahsilat birden fazla faturayı kapatıyorsa (bazı kaynaklarda var) |

**Ödeme türü mutabakatı** (müşavirin istediği kontrol) bu tablo ile banka verisi üzerinden çalışır: `payment_method_type = cash` olan bir tahsilat, aynı gün ve tutarda banka hareketiyle eşleşiyorsa uyarı.

### 4.10 `cash_box_transfer` (virman)

| Alan | Easytrade |
|---|---|
| from_cash_box_id | `from_cash_box_id` |
| to_cash_box_id | `to_cash_box_id` |
| payment_method_type | `payment_method_type` |
| amount | `amount` |
| transaction_date | `transaction_date` |
| description | `description` |

### 4.11 `expense_category` (gelir / gider türü)

| Alan | Easytrade | Açıklama |
|---|---|---|
| name | `name` | |
| parent_id | `parent_id` | |
| full_name | `full_name` | `Araç > Akaryakıt` |
| is_active | `is_active` / `status.value` | |
| scope | ⊕ | `expense` (varsayılan), `income`. Easytrade'de sadece gider kategorisi var. Diğer kaynakların gelir kategorileri `scope=income` ile aynı tabloya yazılır. |
| canonical_type | ⊕ | Standart tür, aşağıda |

Standart türler (`canonical_type`). Kaynağın kategori adı bu listeye kural tabanlı eşlenir (anahtar kelime + müşavirin elle düzeltmesi, öğrenilen eşleme kalıcı saklanır):

| canonical_type | Örnek kaynak adları |
|---|---|
| `fuel` | akaryakıt, benzin, mazot, motorin, yakıt |
| `vehicle` | araç bakım, lastik, otopark, köprü otoyol, HGS, OGS |
| `rent` | kira, işyeri kirası |
| `utilities` | elektrik, su, doğalgaz |
| `telecom` | telefon, internet, GSM |
| `food` | yemek, mutfak, gıda |
| `office` | kırtasiye, ofis malzemesi, temizlik |
| `travel` | yol, seyahat, konaklama, uçak |
| `marketing` | reklam, ilan |
| `software` | yazılım, abonelik, lisans |
| `professional_services` | danışmanlık, muhasebe, avukat |
| `salary` | maaş, ücret, prim |
| `social_security` | SGK |
| `tax` | vergi, harç, damga |
| `bank_fee` | banka masrafı, komisyon, POS komisyonu |
| `finance_expense` | kredi faizi, KMH faizi, finansman gideri |
| `fixed_asset` | demirbaş, bilgisayar, makine, araç alımı |
| `cogs` | ticari mal alımı, hammadde |
| `other` | diğer |
| gelir: `sales_goods`, `sales_service`, `interest_income`, `other_income` | |

### 4.12 `expense` (masraf / gider)

| Alan | Easytrade | Açıklama |
|---|---|---|
| expense_category_id | `expense_category_id` | |
| expense_date | `expense_date` | |
| amount | `amount` | ⚠️ KDV dahil mi netleşmeli |
| currency | `currency` | |
| tax_rate | `tax_rate` | `0_kdv`, `1_kdv`, `10_kdv`, `20_kdv` · ⊕ `8_kdv`, `18_kdv` (eski dönem belgeleri ve diğer kaynaklar için) |
| tax_exclusive_amount | ⊕ | Matrah. Easytrade'de yok, `amount` ve `tax_rate`'ten hesaplanır. |
| tax_amount | ⊕ | KDV |
| description | `description` | |
| notes | `notes` | |
| is_paid | `is_paid` | |
| payment_method | `payment_method` | `cash`, `bank_transfer`, `credit_card`, `check`, `promissory_note`, `credit` |
| cash_box_id | `financial_transaction.cash_box.id` | |
| personnel_id | `personnel_id` | Personel masrafı |
| status | `status` ⚠️ | Onay durumu. **Beyana sadece onaylı masraf girer.** |
| receipt_url | `receipt_url` | Fiş görseli / PDF (`GET /company/expenses/{id}/receipt`) |
| is_recurring, parent_expense_id | aynı adlar | Tekrarlayan masraf |
| branch_id | `branch_id` | |
| receipt_type | ⊕ | `okc_receipt`, `fuel_receipt`, `expense_voucher` (gider pusulası), `smm`, `bank_fee`, `salary`, `tax_payment`, `other` |
| vendor_tax_number | ⊕ | Fişi kesen işyerinin VKN'si |
| vendor_name | ⊕ | |
| receipt_number | ⊕ | Fiş no |
| z_report_number | ⊕ | ÖKC Z no |
| okc_serial_number | ⊕ | ÖKC sicil no |
| vehicle_plate | ⊕ | **Plaka kontrolü.** Normalize: büyük harf, boşluksuz (`34ABC123`) |
| is_deductible | ⊕ | `unknown` (varsayılan), `yes`, `no`. **Müşavir karar verir.** |
| capture_method | ⊕ | `api`, `excel_import`, `ocr`, `manual`, `qr` |
| ocr_confidence | ⊕ | |

### 4.13 `personnel_transaction`

| Alan | Easytrade |
|---|---|
| personnel_id | `personnel_id` |
| transaction_type | `transaction_type`: `salary_accrual`, `salary_payment`, `daily_wage`, `advance`, `bonus`, `deduction`, `refund`, `opening_balance` |
| payment_method_type | `payment_method_type` |
| amount, currency, exchange_rate | aynı adlar |
| transaction_date | `transaction_date` |
| period_year, period_month | aynı adlar |
| reference_document_no | `reference_document_no` |
| cash_box_id | `cash_box_id` |

### 4.14 `check` ve `promissory_note`

Çek: `check_number`, `serial_number`, `drawer_bank_name`, `drawer_branch_name`, `payee_name`, `amount`, `currency`, `issue_date`, `due_date`, `status`, `flow_type`, `collected_cash_box_id`, `collection_transaction_id`.

Senet: `note_number`, `debtor_name`, `debtor_identity_number`, `creditor_name`, `amount`, `currency`, `issue_date`, `due_date`, `place_of_issue`, `status`, `flow_type`.

Durum (Easytrade ekstre filtresinden): `pending`, `completed`, `partially_paid`, `bounced`, `protested` ⚠️.

### 4.15 `vat_report` (kontrol verisi)

Kaynak program kendi KDV özetini veriyorsa (Easytrade `GET /company/reports/vat?period=`) dönem bazında saklanır. Portalın kendi `invoice_tax_summary` toplamıyla karşılaştırılır. Fark varsa uyarı üretilir.

| Alan | Easytrade |
|---|---|
| period | `period` |
| sales.invoice_count, sales.excluded_invoice_count | aynı adlar |
| sales.rates | `sales.rates` ⚠️ |
| sales.totals.base / vat / withholding | aynı adlar |
| purchases.* | aynı yapı |
| excluded_statuses | `excluded_statuses` |

### 4.16 `attachment` ⊕

| Alan | Açıklama |
|---|---|
| owner_type, owner_id | `invoice`, `expense`, `retail_sale` |
| kind | `pdf`, `ubl_xml`, `image`, `html`, `source_file` (içe aktarılan Excel/CSV) |
| storage_key | Nesne deposu anahtarı |
| source_url | Kaynağın geçici URL'si. Kalıcı saklama için hemen indirilir. |
| sha256 | |

### 4.17 Yardımcı tablolar ⊕

**`customer_source_link`**: aynı cari birden fazla kaynaktan geldiğinde (§4.3 kuralı).

| Alan | Açıklama |
|---|---|
| customer_id | Birleşik `customer` |
| source_system, source_id | Kaynaktaki cari |
| raw_payload | |

**`change_request`**: kilitli döneme gelen değişiklik (`02` §3.4).

| Alan | Açıklama |
|---|---|
| company_id, period | |
| entity_type, entity_id | Değişen kayıt |
| old_hash, new_hash | `content_hash` |
| diff | JSONB, alan bazında eski / yeni |
| new_payload | Gelen kaydın ortak model hâli (onaylanırsa uygulanır) |
| status | `open`, `applied`, `rejected` |
| decided_by, decided_at | |

**`sync_window`**: senkron dilimlerinin durumu (`02` §3.1, `37` §5.2). Bir dilim yarıda kalırsa sadece o dilim tekrar edilir.

| Alan | Açıklama |
|---|---|
| connection_id | |
| item | Kalem / belge türü (`outgoing_invoices`, `e_archive` …) |
| direction | Varsa |
| date_field | `issue` / `created` / kaynağa özel |
| range_start, range_end | |
| status | `pending`, `running`, `done`, `failed` |
| page_cursor | Kaldığı sayfa / imleç |
| attempts, last_error | |
| started_at, finished_at | |

**`sync_run`**: her senkron çalışmasının özeti (`02` §9 metrikleri buradan).

| Alan | Açıklama |
|---|---|
| connection_id | |
| kind | `backfill`, `incremental`, `full_scan`, `status_refresh`, `manual` |
| started_at, finished_at | |
| status | `success`, `partial`, `failed` |
| counters | JSONB: okunan, yazılan, değişen, atlanan, hata |
| error | Son hata |

## 5. Enum anlamları (Easytrade'de netleşecek olanlar dahil)

| Alan | Değer | Anlamı (portal yorumu) | Durum |
|---|---|---|---|
| `financial_transaction.transaction_type` | `income` | Kasaya / bankaya para girişi (tahsilat) | ⚠️ |
| | `expense` | Para çıkışı (ödeme) | ⚠️ |
| | `retail_sale` | Perakende satışın ödemesi | ⚠️ |
| | `opening_balance` | Açılış bakiyesi | |
| `invoice.type` | `IADE` | `direction` ile birlikte: satış iadesi veya alış iadesi | |
| `cash_box.type` | `valor` | POS / blokeli tahsilat hesabı | ⚠️ |

## 6. Mükerrer ve kaynaklar arası eşleme

Aynı belge birden fazla kaynaktan gelebilir (ör. Paraşüt'ten ve entegratörden aynı e-fatura, ya da Easytrade `invoices/purchases` ve `invoices/inbox`). Sıra:

1. `uuid` (ETTN) eşitse aynı belge.
2. Değilse: `customer_tax_number` (veya `customer_identity_number`) + `invoice_number` + `issue_date` eşitse aynı belge.
3. Değilse: karşı taraf + `issue_date` + `amounts.payable_amount` (±0,01) eşitse **olası mükerrer** uyarısı.

Birleştirmede öncelik: entegratör / UBL XML > ön muhasebe programı > OCR / manuel.

## 7. Normalizasyon (tüm adaptörler)

| Konu | Kural |
|---|---|
| Para birimi | `TRL`, `TL`, `YTL` → `TRY` |
| VKN / TCKN | Sadece rakam. 10 hane → `tax_number`, 11 hane → `identity_number`. Diğer uzunluklar `NULL` + uyarı. |
| Fatura no | Boşluk ve tire temizlenir, büyük harf. Seri ve sıra ayrı geliyorsa yan yana yazılır. |
| KDV oranı | Ondalık gelirse (`0.20`) yüzdeye çevrilir (`20`). |
| Tevkifat oranı | Kesir (`5/10`), pay/payda (`5`,`10`) veya ondalık (`0.5`) gelirse yüzdeye çevrilir (`50`). |
| Easytrade masraf KDV'si | `20_kdv` → `20` (hesaplama için), ham değer korunur. |
| İptal belgeler | Silinmez. `status_normalized = cancelled`, beyana girmez. |
| Proforma, teklif, taslak, şablon | Çekilmez. Çekildiyse `status_normalized = draft`, beyana girmez. |
| Kaynakta silinen kayıt | Fiziksel silme yok. `deleted_at_source` ⊕ damgası + uyarı. |
| Tarih | Belge tarihi olarak kaynaktaki **düzenleme tarihi** kullanılır, oluşturma tarihi değil. |
