# 09 · Easytrade Adaptörü (Easytrade Muhasebe API)

| | |
|---|---|
| Çalışma şekli | Bulut, REST, JSON (Laravel Sanctum) |
| Base URL | `https://pro.easytradetr.com/api` |
| Kimlik doğrulama | Bearer token (bkz. §1) |
| Bağlam | `X-Company-ID`, `X-Branch-ID` başlıkları |
| Kapsam | Satış ✅ · Alış ✅ · Gelen e-fatura kutusu ✅ · Perakende ✅ · Kasa/Banka ✅ · Kategori ✅ · Masraf ✅ · Personel ✅ · Çek/Senet ✅ · KDV raporu ✅ · PDF/UBL ✅ |
| Kaynak | Easytrade Muhasebe API dokümanı (`easytrade-muhasebe-api.md`) |

Ortak veri modeli (`01`) bu API'nin adlarıyla kurulduğu için bu adaptör **neredeyse dönüşümsüzdür.** İşi: doğru uçları doğru sırayla çağırmak, `{value, label}` enum'larından `value`'yu almak, `string` dönen tutarları sayıya çevirmek ve ⊕ alanları türetmek.

## 1. Kimlik doğrulama ve bağlam

### 1.1 Mevcut seçenekler (dokümandaki)

| Yöntem | Uç | Not |
|---|---|---|
| Kullanıcı girişi | `POST /auth/login` (`email`, `password`, `device_name`) → `data.token` | Token **30 gün** geçerli. Yenileme: `POST /auth/refresh`. Kullanıcı şifresi gerektirir. |
| QR ile giriş | `POST /qr-login/verify`, `/approve`, `/reject` | Mükellefin mobil uygulamadan onay vermesi |
| Şirket API token'ı | `POST /company/api-tokens` (`name`, `branch_id`) → `api_token` + `api_secret` (secret bir kez gösterilir) | Bu token'la isteklerin **nasıl imzalanacağı dokümanda yok** (§9, soru 1) |
| İç servis token'ı | `/internal/v1/*` uçlarında "internal token" kullanılıyor | Sipariş mikroservisi için var. Portal için aynı model uygun mu: §9, soru 1 |

Portal için bağlantı yöntemi §9'daki ilk soru cevaplanınca kesinleşir. Adaptör kodu bu karardan bağımsız yazılır: tüm istekler `Authorization: Bearer {token}` ile gider. Token kaynağı yapılandırmadan gelir.

### 1.2 Bağlam başlıkları

| Başlık | Kullanım |
|---|---|
| `X-Company-ID` | Her `/company/*` isteğinde zorunlu. `source_connection.external_company_id`. |
| `X-Branch-ID` | Şube bağlamı isteyen uçlarda. Liste uçları aktif şube yoksa `400 "Aktif şube bulunamadı"` döner. |

**Çok şubeli mükellef:** Önce `GET /company/branches` ile şubeler alınır. Şube bağlamı isteyen her liste ucu **her şube için ayrı** çağrılır. Kayıtlar `branch_id` ile ayrılır ama tek mükellef altında toplanır. (Bir ucun şube bağlamı olmadan tüm şirketi döndürüp döndürmediği §9, soru 4.)

### 1.3 Hata kodları

| Kod | Adaptör davranışı |
|---|---|
| `401` | Token yenile, 1 kez tekrar. Olmazsa `auth_failed`. |
| `403` + `error_code: LICENSE_EXPIRED` | Senkron durur, bağlantı `disabled`, müşavire "mükellefin Easytrade lisansı bitmiş" uyarısı. **Tekrar denenmez.** |
| `403` diğer | Yetki eksik, `auth_failed`. |
| `404` | Kayıt silinmiş veya şirkete ait değil → `deleted_at_source`. |
| `422` | Tekrar yok, hata kaydı. |
| `500` | Üstel geri çekilme. |
| `502` (dosya indirme) | "Dosya şu an hazırlanamıyor" → kuyrukta sonra tekrar. |

## 2. Liste kuralları

```
GET /company/{kaynak}?page=1&per_page=100&sort=<alan>&direction=asc&filters[...]=...
X-Company-ID: {company_id}
X-Branch-ID: {branch_id}
Authorization: Bearer {token}
```

- `per_page` en fazla **100**.
- Döngü: `meta.current_page < meta.last_page` olduğu sürece `page+1`.
- **Tarih filtresi:** Dokümanda tek örnek kalıp var: `filters[transaction_date][start]` / `filters[transaction_date][end]` (`Y-m-d`). Faturalar, masraflar ve perakende için filtre anahtarları dokümanda yok (§9, soru 2). Cevap gelene kadar güvenli yöntem: `sort=<tarih_alanı>&direction=desc` ile okuyup pencere başlangıcından eski ilk kayıtta durmak.

### 2.1 Artımlı senkron: `GET /sync/pull`

Easytrade'de değişiklik akışı ucu var:

```
GET /sync/pull?after={next_after}
→ data.changes, data.next_after, data.has_more, data.server_time
```

- `has_more = true` olduğu sürece `next_after` ile devam edilir. Son `next_after` `source_connection.sync_cursor` içinde saklanır.
- Bu uç doğrulanırsa, açık dönem taraması yerine **gerçek artımlı senkron** yapılır.
- Dokümanda istek parametreleri üretilememiş (`MAX_LIMIT` hatası) ve `changes` yapısı yok (§9, soru 3). Doğrulanana kadar `02`'deki pencere taraması kullanılır.

## 3. Kalem 1–2: Satış ve alış faturaları

### 3.1 Uçlar

| Amaç | Uç |
|---|---|
| Satış faturaları | `GET /company/invoices/sales` |
| Alış faturaları (işlenmiş) | `GET /company/invoices/purchases` |
| **Entegratörden gelen, henüz işlenmemiş faturalar** | `GET /company/invoices/inbox` |
| Detay (satırlar dahil) | `GET /company/invoices/{invoice}` |
| PDF | `GET /company/invoices/{invoice}/download-pdf` |
| **UBL XML** | `GET /company/invoices/{invoice}/download-ubl` |
| Alternatif dosya ucu | `GET /files/invoices/{id}/{format}` (`502` = sonra tekrar) |
| Tevkifat kodları | `GET /company/withholding-codes` |
| İstisna kodları | `GET /company/tax-exemption-codes` |

**Liste yanıtlarının şeması dokümanda boş** (`data: [null]`, §9, soru 5). Güvenli yöntem: Listeden `id` toplanır, her fatura için `GET /company/invoices/{invoice}` çağrılır. Detay yanıtı tam belgelenmiş.

**İşlenmemiş gelen fatura:** `inbox` listesindeki faturalar `purchases` listesinde yoksa müşavir panelinde "mükellefin işlemediği gelen fatura" olarak gösterilir. Beyana girip girmeyeceğine müşavir karar verir. Eşleşme `uuid` ile.

### 3.2 Detay alanları → ortak model

Alan adları birebir aynı. Dönüşüm gerekenler:

| Easytrade | Ortak model | Dönüşüm |
|---|---|---|
| `data.id` | `source_id` | |
| `data.uuid` | `uuid` | |
| `data.invoice_number` | `invoice_number` | |
| `data.direction.value` | `direction` | `{value,label}` → `value` |
| `data.type.value` | `type` | |
| `data.profile.value` | `profile` | |
| `data.status.value` | `status` | `status_normalized` türetilir (§9, soru 6) |
| `data.issue_date`, `data.issue_time` | aynı | |
| `data.payment_due_date` | aynı | |
| `data.currency.code` | `currency` | |
| `data.amounts.tax_exclusive_amount` | aynı | sayıya çevir |
| `data.amounts.tax_amount` | aynı | **`string` dönüyor**, sayıya çevir |
| `data.amounts.payable_amount` | aynı | |
| `data.customer.*` | `customer` + `customer_tax_number`, `customer_identity_number`, `customer_name` | |
| `data.sgk` | `sgk` | |
| `data.notes` | `notes` | |
| `data.items[].name`, `description`, `quantity`, `unit_price` | aynı | |
| `data.items[].unit.code` | `unit_code` | |
| `data.items[].line_extension_amount` | aynı | |
| `data.items[].own_allowance_amount` | aynı | |
| `data.items[].allowance_total_amount` | aynı | |
| `data.items[].tax_exclusive_amount` | aynı | |
| `data.items[].total_tax_amount` | aynı | |
| `data.items[].tax_inclusive_amount` | aynı | |
| `data.items[].expense_category_id` | aynı | |
| `data.items[].export_registered_code`, `gtip_code`, `diib_line_code` | aynı | |
| `data.items[].tax_breakdowns` | `tax_breakdowns` (ham) → `tax_rate`, `withholding_code`, `withholding_rate`, `withholding_amount`, `tax_exemption_code` | **Yapısı dokümanda yok** (§9, soru 7) |
| `data.items[].product.sku` / `barcode` | `product_code` | |
| `data.movements[]` | ⊕ denetim izi | "Fatura İptal Edildi" hareketi → `status_normalized = cancelled` destek bilgisi |
| `data.responses[]` | ⊕ | GİB durum kodları (`gib_status_code`, `gib_status_description`) |
| `data.updated_at` | `source_updated_at` | |

Header'da türetilen ⊕ alanlar:
- `amounts.line_extension_amount = Σ items.line_extension_amount`
- `amounts.allowance_total_amount = Σ items.allowance_total_amount`
- `amounts.tax_inclusive_amount = Σ items.tax_inclusive_amount`
- `amounts.withholding_amount = Σ items.withholding_amount`
- Tutarlılık kontrolü: `Σ items.total_tax_amount` ile `amounts.tax_amount` farkı 0,05 TL'yi geçerse uyarı.

`meta` alanları (`original_invoice_number`, `global_tax_exemption_code`, `order_reference`, `is_internet_sale`) detay yanıtında `data.meta` altında dönüyor. İçerik şeması belgelenmemiş ⚠️. İstek şemasındaki adlar esas alınır.

## 4. Kalem 6: Perakende satışlar

| Amaç | Uç |
|---|---|
| Liste | `GET /company/retail-sales` |
| Detay | `GET /company/retail-sales/{sale}` |
| İadeler | `GET /company/retail-refunds`, `GET /company/retail-refunds/{refund}` |
| Değişimler | `GET /company/retail-exchanges`, `GET /company/retail-exchanges/{exchange}` |
| Gün sonu oturumları | `GET /company/eod-sessions/{id}` |

Detay alanları `01` §4.7 ile birebir. Kritik kurallar:

1. **`invoice_id` doluysa** satış faturaya dönüşmüştür. KDV fatura üzerinden beyana girer, perakende satış toplamından **düşülür.**
2. Oran bazında KDV `items[].tax_rate`, `taxable_amount`, `tax_amount` üzerinden toplanır.
3. Ödeme türü dağılımı `financial_transactions[].payment_method_type` üzerinden alınır.
4. **Gün sonu oturumu (EOD) mali Z raporu değildir.** Kasa kapanışıdır: nakit/kart satış, iade, masraf, tahsilat toplamları ve kasa farkı (`discrepancy_amount`) içerir. Portalda **kasa mutabakatı kontrolü** için kullanılır, beyan kaynağı olarak değil.
5. ÖKC fiş no, Z no, ÖKC sicil no alanları perakende satışta yok ⊕. Z raporu verisi ÖKC/Paygo entegrasyonundan ayrıca gelir (kapsam dışı doküman).

## 5. Kalem 3: Finansal işlemler

| Amaç | Uç |
|---|---|
| Kasa / banka / kart / POS hesapları | `GET /company/cash-boxes`, `GET /company/cash-boxes/{cash_box}` |
| Hesap hareketleri | `GET /company/cash-boxes/{cash_box}/transactions` |
| Cari finansal işlemler (tahsilat, ödeme) | `GET /company/financial-transactions`, `GET /company/financial-transactions/{financial_transaction}` |
| Cari bazlı işlemler | `GET /company/customers/{customer}/financial-transactions` |
| Kasa virmanları | `GET /company/cash-boxes/transfers?start_date=&end_date=&cash_box_id=` |
| Cari virmanları | `GET /company/customers/transfers` |
| Cari ekstre | `GET /company/customers/{customer}/statement?start_date=&end_date=` |
| Cari bakiyeleri | `GET /company/customers/balances` |

Notlar:
- `cash-boxes/transfers` tarih filtresini **açık parametre** olarak alıyor (`start_date`, `end_date`). Belgelenmiş tek tarih filtreli liste bu.
- `cash-boxes/{id}/transactions` liste şeması boş ⚠️ (§9, soru 5). Hareketin tam içeriği `financial-transactions/{id}` detayından alınır.
- Detay alanları `01` §4.9 ile birebir. `payment_detail` (çek/senet) ve `referenceable` (`Invoice`/`Sale`) bağlantıları korunur.
- `transaction_type` değerlerinin (`income`, `expense`, `retail_sale`, `opening_balance`) anlamı ve `transaction_flow` değerleri netleşmeli (§9, soru 8).

## 6. Kalem 4–5: Gider kategorileri ve masraflar

| Amaç | Uç |
|---|---|
| Kategoriler | `GET /company/expense-categories`, `GET /company/expense-categories/{expenseCategory}` |
| Kategori seçenekleri (aktifler) | `GET /company/expense-categories/options` |
| Masraflar | `GET /company/expenses`, `GET /company/expenses/{expense}` |
| Kategoriye göre masraf | `GET /company/expense-categories/{expenseCategory}/expenses` |
| Fiş dosyası | `GET /company/expenses/{expense}/receipt` → `receipt_url`, `file_name`, `mime_type`, `size_bytes` |

Kurallar:
1. **Beyana sadece onaylı masraf girer.** Onay durumu `status` alanında. Değer listesi §9, soru 9.
2. Tekrarlayan masraflarda **şablon** (`is_main_expense = true`) beyana girmez, sadece üretilen masraflar (`is_derived_expense`) girer ⚠️.
3. `tax_rate` (`20_kdv`) → hesaplama için `20`. `tax_exclusive_amount` ve `tax_amount` ⊕ türetilir. `amount` KDV dahil mi: §9, soru 10.
4. Fiş görseli `receipt_url` ile hemen indirilir (`attachment`).
5. Plaka, fişi kesen VKN, fiş no, Z no Easytrade'de yok ⊕. Portalın OCR katmanı fiş görselinden doldurur. `description` alanında plaka regex'i de çalıştırılır (`10-parasut.md` §7.4).

## 7. Kalem 7–8: Personel ve çek/senet

| Amaç | Uç |
|---|---|
| Personel işlemleri | `GET /company/personnel-transactions` |
| Personel ekstresi | `GET /company/personnel/{personnel}/transactions` |
| Bekleyen maaşlar | `GET /company/personnel/bulk-salary-payment` |
| Çekler | `GET /company/checks`, `GET /company/checks/{check}` |
| Senetler | `GET /company/promissory-notes`, `GET /company/promissory-notes/{promissory_note}` |

`personnel_transaction.transaction_type`: `salary_accrual`, `salary_payment`, `daily_wage`, `advance`, `bonus`, `deduction`, `refund`, `opening_balance` (Easytrade enum).

## 8. Kalem 10: KDV raporu (mutabakat)

```
GET /company/reports/vat?period=2026-10-01
GET /company/reports/vat/months?year=2026
```

Dönüş: `sales` ve `purchases` için `invoice_count`, `excluded_invoice_count`, `rates`, `totals.base`, `totals.vat`, `totals.withholding`, ayrıca `summary`, `carry_chain_start` (devreden KDV zinciri) ve `excluded_statuses`.

Kullanım:
1. Her dönem senkronundan sonra rapor çekilip `vat_report` tablosuna yazılır.
2. Portalın kendi `invoice_tax_summary` toplamı ile karşılaştırılır (oran bazında `base`, `vat`, `withholding`).
3. Fark varsa: hangi faturaların eksik veya fazla olduğu `invoice_count` ve oran kırılımından bulunur, müşavire uyarı gider.
4. `excluded_statuses` listesi, Easytrade'in hangi fatura durumlarını beyana almadığını söyler. `status_normalized` eşlemesi bununla kalibre edilir.

Bu uç, kabul testi #4'ü (`02` §11) **otomatik** hale getirir.

## 9. Easytrade backend ekibinden netleşmesi gerekenler

Easytrade API'sinde değişiklik önerisi değildir. Dokümanda eksik kalan ve entegrasyon için bilinmesi gereken bilgilerdir.

| # | Soru | Neden gerekli |
|---|---|---|
| 1 | Portal (mali müşavir paneli) Easytrade'e hangi kimlikle bağlanacak? Şirket API token'ı (`api_token` + `api_secret`) isteklerde nasıl kullanılıyor, yoksa `/internal/v1` tarzı bir servis token'ı mı kullanılacak? | Mükellef şifresi istemeden bağlantı |
| 2 | `invoices/sales`, `invoices/purchases`, `invoices/inbox`, `expenses`, `retail-sales`, `financial-transactions`, `cash-boxes/{id}/transactions`, `personnel-transactions` için desteklenen `filters[...]` anahtarları (özellikle tarih aralığı) ve `sort` alanları | Dönem bazında çekim |
| 3 | `GET /sync/pull` istek parametreleri (`after`, limit, varlık seçimi), `changes` yapısı ve hangi varlıkları kapsadığı | Gerçek artımlı senkron |
| 4 | Şube bağlamı isteyen liste uçları `X-Branch-ID` olmadan tüm şirketi döndürebiliyor mu | Çok şubeli mükellefte istek sayısı |
| 5 | Liste uçlarının (`invoices/*`, `cash-boxes/{id}/transactions`, `personnel-transactions`) `data[]` eleman şeması | Detay çağrısına gerek kalmaması |
| 6 | `invoice.status.value` değer listesi (taslak, gönderildi, hata, iptal, kabul, red vb.) | Beyana hangisinin gireceği |
| 7 | `items[].tax_breakdowns` yapısı (vergi türü kodu, oran, matrah, tutar, tevkifat kodu/oranı/tutarı, istisna kodu) | Oran bazında KDV ve tevkifat |
| 8 | `financial_transaction.transaction_type` ve `transaction_flow` değerlerinin anlamı (para girişi / çıkışı) | Kasa yönü |
| 9 | `expense.status` değer listesi (onay bekliyor, onaylandı, reddedildi) | Beyana hangi masrafın gireceği |
| 10 | `expense.amount` KDV dahil mi | Matrah ve KDV hesabı |
| 11 | `retail_sale.status` ve `payment_status` değer listeleri | İptal/iade ayrımı |
| 12 | `cash_box.type = valor` tam anlamı | Hesap planı önerisi (`108` / `102`) |
| 13 | `GET /company/reports/vat` yanıtında `sales.rates` ve `summary` yapısı | Oran bazında mutabakat |
| 14 | Fatura ve masrafların silinmesi durumunda `sync/pull` veya liste uçları silinen kaydı bildiriyor mu | `deleted_at_source` tespiti |

## 10. Senkron planı

| Adım | Çağrı | Sıklık |
|---|---|---|
| 1 | `GET /auth/me`, `GET /company/branches` | Bağlantıda + günde 1 |
| 2 | `withholding-codes`, `tax-exemption-codes` | Haftada 1 |
| 3 | `expense-categories`, `cash-boxes`, `customers` | Günde 1 |
| 4 | `sync/pull` (doğrulanınca) | 15 dakikada 1 |
| 5 | Yoksa: `invoices/sales`, `invoices/purchases`, `invoices/inbox` + detaylar (açık dönemler) | 4 saatte 1 |
| 6 | `retail-sales` (+ iade/değişim) | 4 saatte 1 |
| 7 | `financial-transactions`, `cash-boxes/transfers` | 4 saatte 1 |
| 8 | `expenses` + fiş dosyaları | 4 saatte 1 |
| 9 | `personnel-transactions`, `checks`, `promissory-notes` | Günde 1 |
| 10 | `reports/vat` (açık dönemler) | Her senkron sonrası |
| 11 | UBL XML ve PDF | Kuyruktan |
