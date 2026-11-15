# 11 · KolayBi Adaptörü (API v1)

| | |
|---|---|
| Çalışma şekli | Bulut, REST, JSON |
| Base URL | Üretim: `https://ofis-api.kolaybi.com` · Sandbox: `https://ofis-sandbox-api.kolaybi.com` ✅ |
| Yol öneki | `/kolaybi/v1/...` ✅ |
| Kimlik doğrulama | API anahtarı → Bearer token + `Channel` header ✅ |
| 5 kalemin durumu | Satış ✅ · Alış ✅ · Kasa/Banka ✅ · Kategori ✅ · Masraf ✅ |
| Doğrulama kaynağı | developer.kolaybi.com (cURL örnekleri) ve dokümana bağlı güncel topluluk SDK'sı (`theposeidonas/laravel-kolaybi-api`, Ocak 2026) |

**Öne çıkan fark:** KolayBi, gelen e-faturaları API'den doğrudan listeleyebiliyor (`/e_document/invoices?direction=inbound`). Paraşüt'te bu yok. KolayBi mükelleflerinde entegratör bağlantısına ihtiyaç azalır.

## 1. Erişim

### 1.1 İki ayrı bilgi gerekiyor ✅

| Bilgi | Nereden | Kimden |
|---|---|---|
| `api_key` | KolayBi hesap ayarları | **Mükellef** üretir ve portala girer |
| `Channel` | KolayBi destek ekibi | **Easytrade** bir kez alır |

`Channel`'ın entegratör başına mı, mükellef başına mı verildiği dokümanda açık değil ⚠️. Beklenen: Easytrade için tek kanal, her mükellef kendi `api_key`'ini girer. KolayBi'ye ilk yazışmada netleştirilmeli.

### 1.2 Token alma ✅

```bash
curl -X POST "https://ofis-api.kolaybi.com/kolaybi/v1/access_token" \
  -H "Channel: EASYTRADE_CHANNEL" \
  -H "Content-Type: application/json" \
  -d '{"api_key": "MUKELLEF_API_KEY"}'
```

Sonraki tüm istekler:

```
Authorization: Bearer {access_token}
Channel: EASYTRADE_CHANNEL
```

- Token süresi dokümanda belirtilmiyor ⚠️.
- Uygulama: 401 alınca `api_key` ile yeni token al, isteği 1 kez tekrar et. Topluluk SDK'sı da bu şekilde çalışıyor.
- `api_key` uzun ömürlü kabul edilir ve kasada saklanır.

### 1.3 Firma seçimi ✅

```
GET /kolaybi/v1/companies
```

Kullanıcının yetkili olduğu firmalar döner. Seçilen firmanın `id`'si `source_connection.external_company_id`'ye yazılır. E-belge uçları `company_id` parametresini **zorunlu** istiyor.

## 2. Genel kurallar

| Konu | Değer |
|---|---|
| Tarih formatı (filtre) | `Y-m-d` ✅ |
| Gövde formatı (yazma) | `application/x-www-form-urlencoded` veya JSON ✅ |
| Sayfalama | Dokümanda bulunamadı ⚠️. İlk çağrıda yanıt yapısı incelenmeli (`meta`, `links`, `page`, `per_page`). |
| Hız sınırı | Dokümanda yok ⚠️. Güvenli değer: saniyede 1 istek, 429'da geri çekilme. |
| Para birimleri | `TRY`, `USD`, `EUR`, `GBP`, `CHF` ve 15 diğer ISO kodu ✅ |
| KDV oranları | `0`, `1`, `8`, `10`, `18`, `20` ✅ |

**Önemli:** Aşağıdaki **istek** parametreleri doğrulandı. **Yanıt** alanlarının adları ise dokümanda tam listelenmiyor. Tablolarda yanıt alanları için yazma uçlarının şemasından beklenen adlar verildi ve ⚠️ ile işaretlendi. İlk entegrasyon gününde bir sandbox faturasının ham yanıtı alınıp tablo kesinleştirilmeli.

## 3. Kalem 1 ve 2: Satış ve alış faturaları

KolayBi tek uç noktada bütün belge türlerini tutar. Tür, `type` parametresiyle seçilir.

### 3.1 Uç noktalar ✅

```
GET /kolaybi/v1/invoices
    ?type=sale_invoice
    &min_issue_date=2026-10-01
    &max_issue_date=2026-10-31
    &associate_id=     (isteğe bağlı)
    &description=      (isteğe bağlı)
    &has_products=     (isteğe bağlı, boolean)

GET /kolaybi/v1/invoices/{document_id}
GET /kolaybi/v1/invoices/{document_id}?include_draft=1   (taslaklar dahil)
```

### 3.2 `type` değerleri ✅ ve eşleme

| KolayBi `type` | Ortak model | Çekilsin mi |
|---|---|---|
| `sale_invoice` | `invoice`, `direction=outgoing`, `type` = `document_type`'tan (§3.4) | Evet |
| `sale_return_invoice` | `invoice`, `direction=outgoing`, `type=IADE` | Evet |
| `purchase_invoice` | `invoice`, `direction=incoming`, `type=ALIS` (veya `document_type`'tan) | Evet |
| `purchase_return_invoice` | `invoice`, `direction=incoming`, `type=IADE` | Evet |
| `general_expense` | `expense` (Kalem 5) | Evet |
| `self_employment_receipt` | `invoice`, `document_class=smm` (yöne dikkat ⚠️) | Evet |
| `sale_waybill`, `purchase_waybill` | irsaliye | **Hayır** |

Her `type` için ayrı liste çağrısı yapılır.

### 3.3 Başlık alanları

| KolayBi alanı | Ortak model | Durum |
|---|---|---|
| `id` / `document_id` | `source_id` | ⚠️ yanıttaki ad |
| `serial_no` | `invoice_number` | ✅ (yazma şemasında var) |
| `order_date` | `issue_date` | ✅ yazma şemasında "düzenleme tarihi" olarak kullanılıyor, ⚠️ yanıtta `issue_date` de olabilir |
| `due_date` | `payment_due_date` | ✅ |
| `currency`, `exchange_rate` | aynen | ✅ |
| `contact_id` / `associate_id` | `customer_id` | ✅ |
| `document_type` | Aşağıdaki tablo | ✅ |
| `document_scenario` | `profile` | ✅ |
| `vat_exemption_reason_code` | `global_tax_exemption_code` | ✅ |
| `subtotal_discount_amount` | `amounts.allowance_total_amount`'a eklenir | ✅ |
| `special_tax_base` (`reason_code`, `total`, `percentage`, `vat`) | Özel matrah | ✅ |
| `description` | `notes` | ✅ |
| Toplam alanları (ara toplam, KDV, genel toplam) | `amounts.line_extension_amount`, `amounts.tax_amount`, `amounts.payable_amount` | ⚠️ adları yanıttan alınmalı |
| E-belge UUID | `uuid` | ⚠️ |

### 3.4 `document_type` ✅ → ortak model

| KolayBi | Anlamı | Ortak modele etkisi |
|---|---|---|
| `SATIS` | Normal satış | `type=SATIS` (alışta `ALIS`) |
| `IADE` | İade | `type=IADE` |
| `TEVKIFAT` | Tevkifatlı | `type=TEVKIFAT`, satırlardaki tevkifat alanları zorunlu okunur |
| `TEVKIFATIADE` | Tevkifatlı iade | `type=IADE` + satır tevkifatı (Easytrade'de ayrı tür yok) |
| `ISTISNA` | KDV istisnası | `type=ISTISNA`, `global_tax_exemption_code` zorunlu |
| `OZELMATRAH` | Özel matrah | `type=OZELMATRAH`, `special_tax_base` okunur |
| `IHRACKAYITLI` | İhraç kayıtlı | `type=IHRACKAYITLI` |
| `MATBUDAN` | Kâğıt faturadan aktarım | `profile = NULL` (kâğıt) |
| `SEVK` | Sevk | Beyana girmez ⚠️ kontrol |

### 3.5 `document_scenario` ✅ → `profile`

| KolayBi | `profile` |
|---|---|
| `TEMELFATURA`, `TICARIFATURA`, `IHRACAT`, `KAMU`, `HKS`, `ENERJI`, `ILAC_TIBBICIHAZ`, `YATIRIMTESVIK`, `YOLCUBERABERFATURA` | `e_invoice` |
| `EARSIVFATURA` | `e_archive` |
| `SERBESTMESLEKMAKBUZU` | `e_smm` |
| `TEMELIRSALIYE` | irsaliye, çekilmez |
| boş | `paper` |

### 3.6 Satır alanları (`items[]`) ✅

| KolayBi alanı | Ortak model (`invoice_item`) |
|---|---|
| `product_id` | `product_code` (ürün kartından kod alınır) |
| `quantity` (string) | `quantity` (sayıya çevir) |
| `unit_price` (string) | `unit_price` |
| `vat_rate` | `tax_rate` |
| `discount_amount` | `own_allowance_amount` |
| `description` | `description` |
| `withholding_code` | `withholding_code` (GİB kodu, ör. `601`) |
| `withholding_type` (`PERCENTAGE` / `NUMERIC`) + `withholding_value` | `withholding_rate` veya `withholding_amount` |
| `stoppage_name` (`income_tax` / `corporate_tax`) + `stoppage_value` | Stopaj |
| `gtip_no` | `gtip_code` |

Satırda KDV tutarı ve net tutar alanlarının yanıttaki adları ⚠️. Gelmiyorsa hesaplanır.

Fatura başlığında da `vat_type` (`percentage` / `numeric`) ve `vat_value` var ✅. Bu alanlar, başlık düzeyinde uygulanan KDV düzeltmesi için kullanılıyor gibi görünüyor ⚠️.

### 3.7 Gelen ve giden e-belgeler ✅ (KolayBi'nin güçlü yanı)

```
GET /kolaybi/v1/e_document/invoices
    ?company_id={company_id}            (zorunlu)
    &direction=inbound                  (zorunlu: inbound | outbound)
    &min_issue_date=2026-10-01
    &max_issue_date=2026-10-31
    &document_type=                     (isteğe bağlı)
    &document_scenario=                 (isteğe bağlı)
    &min_total_amount= &max_total_amount=
    &party_name=
    &document_id=
```

- `direction=inbound`: Mükellefin gelen kutusuna düşen e-faturalar. Henüz alış faturasına dönüştürülmemiş olanlar dahil.
- Bu uç noktadan gelen kayıtlar, `invoices?type=purchase_invoice` sonuçlarıyla **ETTN üzerinden birleştirilir** (bkz. `01` §6). Gelen kutusunda olup alışa işlenmemiş faturalar müşavir panelinde "işlenmemiş gelen fatura" olarak ayrıca gösterilir.

Görüntü ✅:

```
GET /kolaybi/v1/invoices/e-document/view?uuid={ETTN}&direction=inbound|outbound
```

HTML/PDF döner ⚠️ (format doğrulanmalı). `attachment` olarak saklanır.

## 4. Kalem 3: Finansal işlemler

### 4.1 Kasa ve banka hesapları ✅

```
GET /kolaybi/v1/vaults?type=bank_account|safe_deposit&currency=TRY
GET /kolaybi/v1/vaults/{vault_id}
```

| KolayBi `type` | Ortak model `account_type` |
|---|---|
| `bank_account` | `bank_account` |
| `safe_deposit` | `cash` |

### 4.2 Hesap hareketleri ✅

```
GET /kolaybi/v1/vaults/{vault_id}/transactions
```

Bu uç nokta için tarih filtresi topluluk SDK'sında tanımlı değil ⚠️. Denenmesi gerekenler: `start_date`/`end_date` veya `min_issue_date`/`max_issue_date`. Filtre yoksa tamamı çekilir ve tarih bellekte süzülür.

Yön bilgisi için KolayBi'nin genel modelinde `cash_flow_direction` kullanılıyor: `1` giriş, `-1` çıkış, `0` nötr ✅. Hareket yanıtında aynı alanın dönmesi bekleniyor ⚠️.

### 4.3 Online banka hareketleri ✅

KolayBi'nin kendi banka entegrasyonu açıksa ham banka hareketleri de okunabilir:

```
GET /kolaybi/v1/online_bank/banks
GET /kolaybi/v1/online_bank/accounts
GET /kolaybi/v1/online_bank/accounts/{account_id}/transactions
    ?start_date=2026-10-01&end_date=2026-10-31
    &direction=in|out|all
    &is_mapped=true|false
    &min_amount= &max_amount= &receipt_no= &explanation=
```

- `is_mapped=false`: Mükellefin henüz cariye veya faturaya eşlemediği banka hareketleri. Müşavir panelindeki **"eşlenmemiş banka hareketi"** uyarısı doğrudan bu filtreyle beslenir.
- Bu veri, **ödeme türü mutabakatının** (nakit girilmiş ama bankadan gelmiş) en sağlam kaynağıdır.

### 4.4 Cari hareketleri ve tahsilat/ödeme ✅

```
GET /kolaybi/v1/associates/{associate_id}/transactions
```

Cari ekstresi. Faturaya bağlanmadan yapılan tahsilat ve ödemeler buradan alınır. Tahsilat ve ödeme kayıtlarında `vault_id`, `amount`, `issue_date`, `serial_no`, `description` alanları bulunur ✅ (yazma şemasından). Hangi kasaya veya bankaya girdiği `vault_id` ile belirlenir, buradan `payment_method_type` türetilir.

Çek ve senet modülleri de var (`cheques`, `bonds`). Çekle yapılan tahsilatlar için `payment_method_type = check` ⚠️ (uç noktalar ikinci fazda eklenir).

## 5. Kalem 4: Gelir ve gider türleri ✅

```
GET /kolaybi/v1/financial_action_types      (gider tipleri)
GET /kolaybi/v1/tags                        (etiketler)
```

- `financial_action_types`, gider türleri listesidir. `category` tablosuna `scope=expense` olarak yazılır.
- KolayBi'de gelir kategorisi için ayrı bir liste yok ⚠️. Gelir tarafında `tags` ve ürün grupları kullanılır.
- Etiketler (`tags`) müşavirin kural motorunda ek bilgi olarak kullanılabilir.

## 6. Kalem 5: Masraf fişleri ✅

```
GET /kolaybi/v1/invoices?type=general_expense&min_issue_date=...&max_issue_date=...
```

- `general_expense` kayıtları kalem zorunluluğu olmadan girilir (`items` zorunlu değil ✅).
- Eşleme: `expense`, `receipt_type` varsayılan `other`. Gider tipi `fuel` ise `fuel_receipt` yapılır.
- Gider tipi ilişkisi `financial_action_types` ile kurulur ⚠️ (yanıttaki alan adı doğrulanmalı).
- Plaka için ayrı alan yok. Açıklamada plaka regex'i çalıştırılır (bkz. `10-parasut.md` §7.4).
- Cariye gider yansıtma (`transfer`) çağrılarındaki `expenses[]` yapısı, gider kalemlerinin cari virmanlarda da tutulabildiğini gösteriyor ✅. İkinci fazda incelenir.

## 7. Karşı taraf (`associates`) ✅

```
GET /kolaybi/v1/associates?identity_no=&name=&code=&active=&group=&tag_ids[]=
GET /kolaybi/v1/associates/{id}
```

| KolayBi | Ortak model (`customer`) |
|---|---|
| `name` + `surname` | `trade_name` (kurumsal ise `name` tek başına unvan ⚠️) |
| `identity_no` (10–11 hane) | `tax_number / identity_number` |
| `is_corporate` | `customer_legal_type` |
| `associate_type` | `type` (aşağıda) |
| `tax_office` | `tax_office` |
| `addresses[]` (`city`, `district`, `address`, `country`, `is_abroad`) | ilk adres |
| `code`, `email`, `phone` | aynen |

`associate_type` ✅ → `type`:

| KolayBi | `type` |
|---|---|
| `customer`, `potential_customer`, `public_enterprise` | `customer` |
| `supplier`, `vendor`, `subcontractor`, `carrier`, `shipping_company`, `broker` | `supplier` |
| `full_time_employee`, `part_time_employee`, `freelancer_employee`, `student` | `employee` |
| `partner` | `both` |
| `abroad` | `customer` + `is_abroad=true` ⚠️ |

## 8. Senkron planı

| Adım | Çağrı | Sıklık |
|---|---|---|
| 1 | `companies` | Bağlantıda + günde 1 |
| 2 | `financial_action_types`, `tags` | Günde 1 |
| 3 | `vaults` | Günde 1 |
| 4 | `associates` | Günde 1 |
| 5 | `invoices` × 6 tip (açık dönemler) | 4 saatte 1 |
| 6 | `e_document/invoices` inbound + outbound (açık dönemler) | 4 saatte 1 |
| 7 | Her vault için `transactions` | 4 saatte 1 |
| 8 | `online_bank/accounts/{id}/transactions` (bağlıysa) | 4 saatte 1 |
| 9 | E-belge görüntüleri | Kuyruktan |

## 9. Doğrulanacaklar listesi

- [ ] `Channel` entegratör başına mı, mükellef başına mı
- [ ] Token süresi
- [ ] Sayfalama yapısı ve rate limit
- [ ] `invoices` liste yanıtının alan adları (özellikle toplamlar, ETTN, kategori, ödeme durumu)
- [ ] Liste yanıtı satırları içeriyor mu, yoksa her fatura için detay çağrısı gerekli mi
- [ ] `vaults/{id}/transactions` tarih filtresi ve yön alanı
- [ ] `self_employment_receipt`: mükellefin aldığı SMM mi, düzenlediği SMM mi
- [ ] `e-document/view` dönüş formatı (HTML / PDF / base64)
- [ ] `general_expense` kayıtlarında gider tipi ilişkisinin alan adı

## Kaynaklar

- KolayBi geliştirici dokümanı: https://developer.kolaybi.com
- cURL örnekleri: https://developer.kolaybi.com/docs/examples/curl
- Fatura listeleme: https://developer.kolaybi.com/docs/invoices/list/
- Gelen e-belgeler: https://developer.kolaybi.com/docs/e-documents/list
- Gider tipleri: https://developer.kolaybi.com/docs/financial-action-types/list
- Dokümana bağlı SDK: https://packagist.org/packages/theposeidonas/laravel-kolaybi-api
