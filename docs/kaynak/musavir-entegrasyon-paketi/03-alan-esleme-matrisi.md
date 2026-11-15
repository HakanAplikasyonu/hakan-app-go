# 03 · Alan Eşleme Matrisi

Her ortak alanın (`01`) her kaynak sistemdeki karşılığı. Adaptör yazarken **tek başvuru tablosu** budur. Ayrıntılar ve uç noktalar ilgili kaynak dosyasında (`09`–`16`).

Gösterim:
- `alan` → kaynağın alan adı
- **—** → kaynakta yok (türetilir veya başka kaynaktan gelir)
- **Σ** → satırlardan toplanır
- **⚠️** → kaynak dokümanında doğrulanmalı
- **Kaynak seviyesi:** ET, PR, KB ve MK sütunları üreticinin resmi dokümanından. **LG** (Logo Objects REST) ve **BH** (BizimHesap B2B) sütunları topluluk kütüphanelerinden; bu iki sütundaki alan adları ilk gerçek bağlantıda doğrulanır (`12`, `14` başındaki not).
- **(E)** → İşbaşı, Robom ve BizimHesap'ta faturalar entegratör / UBL XML üzerinden gelir. Bu durumda alanlar UBL'den okunur (`31`).

Sütunlar: **ET** Easytrade · **PR** Paraşüt · **KB** KolayBi · **LG** Logo Tiger/GO · **MK** Mikro (SQL) · **BH** BizimHesap

## 1. Kapsam özeti

| Kalem | ET | PR | KB | LG | MK | BH | İşbaşı | Robom |
|---|---|---|---|---|---|---|---|---|
| Satış faturaları | ✅ | ✅ | ✅ | ✅ | ⚠️ SQL | (E) | (E) | (E) |
| Alış faturaları | ✅ | ✅ | ✅ | ✅ | ⚠️ SQL | (E) | (E) | (E) |
| Gelen kutusu (işlenmemiş) | ✅ `invoices/inbox` | ❌ | ✅ `e_document` inbound | — | — | (E) | (E) | (E) |
| Perakende satış | ✅ | — | — | ✅ (TYPE 7) | ⚠️ | — | — | — |
| Kasa / banka hareketleri | ✅ | ✅ | ✅ | ✅ | ⚠️ SQL | ⚠️ cari ekstre | — | — |
| Gelir / gider türleri | ✅ | ✅ | ✅ | ✅ hesap planı | ⚠️ | — | — | — |
| Masraf | ✅ | ✅ | ✅ | ✅ | ⚠️ | — | — | — |
| Personel ödemeleri | ✅ | ✅ maaş | — | — | — | — | — | — |
| Çek / senet | ✅ | — | ⚠️ cheques/bonds | ⚠️ | ⚠️ | — | — | — |
| KDV raporu (mutabakat) | ✅ | — | — | — | — | — | — | — |
| PDF | ✅ | ✅ | ✅ (view) | — | ⚠️ `FaturaPdfV2` | — | (E) | (E) |
| UBL XML | ✅ | — | — | — | — | — | (E) | (E) |
| Artımlı senkron | ⚠️ `sync/pull` | — | — | ✅ `DATE_MODIFIED` | ⚠️ | — | — | — |

## 2. Fatura türü eşlemesi (`direction` + `type` + `document_class`)

| Ortak model | ET | PR | KB | LG `TYPE` | MK |
|---|---|---|---|---|---|
| `outgoing` · `SATIS` | `direction=outgoing, type=SATIS` | `sales_invoices`, `item_type=invoice` | `type=sale_invoice`, `document_type=SATIS` | 7, 8, 9 | `cha_evrak_tip` ⚠️ |
| `outgoing` · `IADE` | `type=IADE` | `sales_invoices`, `item_type=refund` | `type=sale_return_invoice` veya `document_type=IADE` | 2, 3 | ⚠️ |
| `outgoing` · `TEVKIFAT` | `type=TEVKIFAT` | `vat_withholding_rate > 0` | `document_type=TEVKIFAT` | `DEDUCTIONPART1 > 0` | ⚠️ |
| `outgoing` · `ISTISNA` | `type=ISTISNA` | — (istisna kodu yok ⚠️) | `document_type=ISTISNA` | `VATEXCEPT_CODE` dolu | ⚠️ |
| `outgoing` · `OZELMATRAH` | `type=OZELMATRAH` | — | `document_type=OZELMATRAH` | ⚠️ | ⚠️ |
| `outgoing` · `IHRACKAYITLI` | `type=IHRACKAYITLI` | — | `document_type=IHRACKAYITLI` | ⚠️ | ⚠️ |
| `outgoing` · `SGK` | `type=SGK` | — | — | ⚠️ | ⚠️ |
| `incoming` · `ALIS` | `direction=incoming, type=ALIS` | `purchase_bills`, `item_type=purchase_bill`, `is_detailed=true` veya e-belgeli | `type=purchase_invoice` | 1, 4 | ⚠️ |
| `incoming` · `IADE` | `type=IADE` | `purchase_bills`, `item_type=refund` | `type=purchase_return_invoice` | 6 | ⚠️ |
| `incoming` · `TEVKIFAT` | `type=TEVKIFAT` | `vat_withholding_rate > 0` | `document_type=TEVKIFAT` | `DEDUCTIONPART1 > 0` | ⚠️ |
| İade + tevkifat | `type=IADE` + satır tevkifatı | `refund` + `vat_withholding_rate` | `document_type=TEVKIFATIADE` → `type=IADE` + satır tevkifatı | iade TYPE + `DEDUCTIONPART1` | ⚠️ |
| `document_class=price_difference` | — | — | — | 13 (alış), 14 (satış) | ⚠️ |
| `document_class=mustahsil` | — | — | — | 26 | ⚠️ |
| `document_class=smm` | — | e-SMM (`e_smms`) | `type=self_employment_receipt` | ⚠️ | ⚠️ |
| Çekilmez (proforma, şablon, irsaliye) | `quotes`, `despatches` | `estimate`, `recurring_*` | `sale_waybill`, `purchase_waybill`, `document_type=SEVK` | 5, 10 | — |
| `status_normalized=cancelled` | `status` ⚠️ + "Fatura İptal Edildi" hareketi | `item_type=cancelled` | ⚠️ | `CANCELLED=1` | `cha_iptal` ⚠️ |

**Öncelik kuralı:** Bir fatura hem iade hem tevkifatlı veya hem tevkifatlı hem istisnalıysa `type` Easytrade'in yaptığı gibi **tek değer** alır. Öncelik: `IADE` > `TEVKIFAT` > `ISTISNA` > `OZELMATRAH` > `IHRACKAYITLI` > `SATIS`/`ALIS`. Diğer özellikler satır alanlarında (`withholding_code`, `tax_exemption_code`) korunur.

## 3. Profil (e-belge senaryosu)

| Ortak `profile` | ET | PR | KB `document_scenario` | LG | MK |
|---|---|---|---|---|---|
| `TEMELFATURA` | aynı | `active_e_document` türü `e_invoices` → senaryo e-fatura detayından ⚠️ | `TEMELFATURA` | `PROFILE_ID` ⚠️ | `efd_pozisyon` ⚠️ |
| `TICARIFATURA` | aynı | ⚠️ | `TICARIFATURA` | `PROFILE_ID` ⚠️ | ⚠️ |
| `EARSIVFATURA` | aynı | `active_e_document` türü `e_archives` | `EARSIVFATURA` | `EINVOICE` ⚠️ | ⚠️ |
| `IHRACAT`, `KAMU`, `YOLCUBERABERFATURA`, `ILAC_TIBBICIHAZ` | aynı | ⚠️ | aynı adlar | ⚠️ | ⚠️ |
| `OZELFATURA` | aynı | — | — | ⚠️ | ⚠️ |
| `NULL` (kâğıt) | — | `active_e_document` yok | boş veya `document_type=MATBUDAN` | `EINVOICE` = kâğıt ⚠️ | e-fatura detayı yok |
| KB'ye özgü: `HKS`, `ENERJI`, `YATIRIMTESVIK` | — | — | aynı adlar | — | — |

`HKS`, `ENERJI`, `YATIRIMTESVIK` Easytrade enum'unda yok. Kaynak değeri olduğu gibi `profile`'a yazılır. Beyan açısından e-fatura sayılır.

## 4. Fatura başlığı

| Ortak alan | ET | PR | KB | LG | MK |
|---|---|---|---|---|---|
| `source_id` | `id` | `id` | `id` ⚠️ | `INTERNAL_REFERENCE` | `cha_Guid` ⚠️ |
| `uuid` | `uuid` | e-belge detayında ⚠️ | e-belge `uuid` ⚠️ | ⚠️ (`EARCHIVEDETR_*` / `GUID` değil) | `efd_uuid` |
| `invoice_number` | `invoice_number` | `invoice_series` + `invoice_id` (satış), `invoice_no` (alış) | `serial_no` | `DOC_NUMBER` | `cha_belge_no` / `cha_evrakno_seri`+`sira` ⚠️ |
| `issue_date` | `issue_date` | `issue_date` | `order_date` ⚠️ | `DATE` (veya `DOC_DATE` ⚠️) | `cha_tarihi` / `cha_belge_tarih` ⚠️ |
| `issue_time` | `issue_time` | — | — | — | — |
| `payment_due_date` | `payment_due_date` | `due_date` | `due_date` | ⚠️ | ⚠️ |
| `currency` | `currency.code` | `currency` (`TRL`→`TRY`) | `currency` | `CURR_INVOICE` (kod→ISO ⚠️) | ⚠️ |
| `exchange_rate` | `exchange_rate` | `exchange_rate` | `exchange_rate` | `TC_XRATE` | ⚠️ |
| `customer_*` | `customer.*` | `contact` ilişkisi + `tax_number`, `tax_office` | `associate` | `ARP_CODE` → `/Arps` | `cha_kod` → `CARI_HESAPLAR` |
| `amounts.line_extension_amount` | Σ | `gross_total` | ⚠️ | `TOTAL_GROSS` | ⚠️ |
| `amounts.allowance_total_amount` | Σ | `total_discount` + `total_invoice_discount` | `subtotal_discount_amount` + Σ satır | `TOTAL_DISCOUNTS` | ⚠️ |
| `amounts.tax_exclusive_amount` | `amounts.tax_exclusive_amount` | `before_taxes_total` (satış) / hesap (alış) | ⚠️ | `TOTAL_GROSS − TOTAL_DISCOUNTS` ⚠️ | `cha_aratoplam` ⚠️ |
| `amounts.tax_amount` | `amounts.tax_amount` | `total_vat` | ⚠️ | `TOTAL_VAT` | Σ `cha_vergi1..N` ⚠️ |
| `amounts.withholding_amount` | Σ `tax_breakdowns` ⚠️ | `vat_withholding` (satış) / hesap (alış) | Σ satır | Σ `DEDUCTION_TOT` | ⚠️ |
| `amounts.excise_amount` | — | `total_excise_duty` | — | `TOTAL_ADD_TAX` ⚠️ | ⚠️ |
| `amounts.communications_tax_amount` | — | `total_communications_tax` | — | ⚠️ | ⚠️ |
| `amounts.income_tax_withholding_amount` | — | `withholding` (satış) | Σ `stoppage_value` | ⚠️ | ⚠️ |
| `amounts.payable_amount` | `amounts.payable_amount` | `net_total` | ⚠️ | `TOTAL_NET` | `cha_meblag` ⚠️ |
| `global_tax_exemption_code` | `meta.global_tax_exemption_code` | — | `vat_exemption_reason_code` | `VATEXCEPT_CODE` | ⚠️ |
| `original_invoice_number` | `meta.original_invoice_number` | — | `return_invoice_references[].serial_no` | ⚠️ | ⚠️ |
| `payment_status` | — | `payment_status` | ⚠️ | — | — |
| `remaining_amount` | — | `remaining` | ⚠️ | — | — |
| `notes` | `notes` | `description` | `description` | `NOTES1..6` | ⚠️ |
| `source_updated_at` | `updated_at` | `updated_at` | ⚠️ | `DATE_MODIFIED` | ⚠️ |
| PDF | `download-pdf` | `e_archives/{id}/pdf`, `e_invoices/{id}/pdf` | `invoices/e-document/view` | — | `FaturaPdfV2` ⚠️ |
| UBL XML | `download-ubl` | — | — | — | — |

BizimHesap, İşbaşı, Robom: (E). Bu kaynaklarda tüm başlık alanları entegratörden gelen UBL'den okunur.

## 5. Fatura satırı

| Ortak alan | ET | PR | KB | LG `TRANSACTIONS` | MK `sth_` ⚠️ |
|---|---|---|---|---|---|
| `name` | `items[].name` | `product.name` / `description` | ürün adı ⚠️ | `MASTER_CODE` + ürün kartı | ⚠️ |
| `description` | `items[].description` | `description` | `description` | `DESCRIPTION` | ⚠️ |
| `product_code` | `items[].product.sku` | `product.code` ⚠️ | `product_id` → ürün kodu | `MASTER_CODE` | ⚠️ |
| `quantity` | `quantity` | `quantity` | `quantity` (string) | `QUANTITY` | ⚠️ |
| `unit_code` | `unit.code` | — (ürün birimi ⚠️) | ⚠️ | `UNIT_CODE` (Logo kodu → UBL ⚠️) | ⚠️ |
| `unit_price` | `unit_price` | `unit_price` | `unit_price` (string) | `PRICE` | ⚠️ |
| `line_extension_amount` | `line_extension_amount` | `quantity × unit_price` | `quantity × unit_price` | `TOTAL` | ⚠️ |
| `own_allowance_amount` | `own_allowance_amount` | `discount_type` + `discount_value` → tutar | `discount_amount` | `DISCOUNT_RATE` → tutar | ⚠️ |
| `tax_exclusive_amount` | `tax_exclusive_amount` | `net_total` ⚠️ | ⚠️ | `VAT_BASE` | ⚠️ |
| `tax_rate` | `tax_breakdowns` ⚠️ | `vat_rate` | `vat_rate` | `VAT_RATE` | ⚠️ |
| `total_tax_amount` | `total_tax_amount` | hesap: matrah × oran | ⚠️ | `VAT_AMOUNT` | ⚠️ |
| `withholding_code` | `tax_breakdowns` ⚠️ | — | `withholding_code` | `DEDUCT_CODE` | ⚠️ |
| `withholding_rate` (%) | `tax_breakdowns` ⚠️ | başlık `vat_withholding_rate` | `withholding_value` (`withholding_type=PERCENTAGE`) | `DEDUCTION_PART1 / DEDUCTION_PART2 × 100` | ⚠️ |
| `withholding_amount` | `tax_breakdowns` ⚠️ | hesap | `withholding_value` (`NUMERIC`) veya hesap | `DEDUCTION_TOT` | ⚠️ |
| `tax_exemption_code` | `tax_breakdowns` ⚠️ | — | başlıktan | `VATEXCEPT_CODE` | ⚠️ |
| `excise_rate` / `excise_amount` | — | `excise_duty_type` + `excise_duty_value` | — | `ADD_TAX_RATE` / `ADD_TAX_AMOUNT` | ⚠️ |
| `communications_tax_rate` | — | `communications_tax_rate` | — | ⚠️ | ⚠️ |
| `gtip_code` | `gtip_code` | — | `gtip_no` | ⚠️ | ⚠️ |
| `expense_category_id` | `expense_category_id` | başlık `category` | ⚠️ | `GL_CODE1` (hesap kodu ipucu) | ⚠️ |

## 6. Cari (`customer`)

| Ortak alan | ET | PR `contacts` | KB `associates` | LG `/Arps` | MK `CARI_HESAPLAR` ⚠️ | BH `/customers` |
|---|---|---|---|---|---|---|
| `type` | `type` | `account_type` (`customer`/`supplier`) | `associate_type` (eşleme `11` §7) | `ACCOUNT_TYPE` ⚠️ | ⚠️ | — (`both`) |
| `customer_legal_type` | `customer_legal_type` | `contact_type`: `company`→`corporate`, `person`→`individual` | `is_corporate` | `PERSCOMPANY` ⚠️ | ⚠️ | uzunluğa göre (10/11) |
| `trade_name` / `name` | `trade_name` / `name` | `name` | `name` (+ `surname`) | `TITLE` (+ `TITLE2`) | ⚠️ | `title` |
| `tax_number` (10) | `tax_number` | `tax_number` (10 hane ise) | `identity_number` (10 hane ise) | `TAX_ID` | ⚠️ | `taxno` (10 hane ise) |
| `identity_number` (11) | `identity_number` | `tax_number` (11 hane ise) | `identity_number` (11 hane ise) | `TCKNO` | ⚠️ | `taxno` (11 hane ise) |
| `tax_office` | `tax_office` | `tax_office` | `tax_office` | `TAX_OFFICE` | ⚠️ | `taxoffice` |
| `address`, `city`, `district` | aynı | `address`, `city`, `district` | `addresses[0].*` | ⚠️ | ⚠️ | `address` |
| `email`, `phone` | aynı | `email`, `phone` | `email`, `phone` | ⚠️ | ⚠️ | `email`, `phone` |
| `is_abroad` | — | `is_abroad` | `addresses[].is_abroad` | ⚠️ | ⚠️ | — |

## 7. Kasa / banka hesabı (`cash_box`)

| Ortak `type` | ET | PR `accounts.account_type` | KB `vaults.type` | LG | MK ⚠️ |
|---|---|---|---|---|---|
| `cash` | `cash` | `cash` | `safe_deposit` | `/safeDeposits` | `KASALAR` |
| `bank_account` | `bank_account` | `bank` | `bank_account` | `/bankAccounts` | `BANKALAR` |
| `credit_card` | `credit_card` | — | — | ⚠️ | ⚠️ |
| `valor` (POS) | `valor` | — | — | ⚠️ | ⚠️ |
| eşlenmez, ham saklanır | — | `sys` | — | — | — |

| Ortak alan | ET | PR | KB | LG | MK |
|---|---|---|---|---|---|
| `name` | `name` | `name` | ⚠️ | kart adı ⚠️ | ⚠️ |
| `iban` | `iban` | `iban` | ⚠️ | `IBAN` | ⚠️ |
| `bank_id` | `bank_id.value` | `bank_name` (metin → enum eşlemesi) | ⚠️ | ⚠️ | ⚠️ |
| `currency` | `currency` | `currency` | ⚠️ | ⚠️ | ⚠️ |
| `balance` | `balance` | `balance` | ⚠️ | ⚠️ | ⚠️ |

## 8. Finansal işlem (`financial_transaction`)

### 8.1 Alanlar

| Ortak alan | ET | PR `accounts/{id}/transactions` | KB `vaults/{id}/transactions` | LG | MK ⚠️ | BH `/abstract/{id}` |
|---|---|---|---|---|---|---|
| `source_id` | `id` | `id` | ⚠️ | `INTERNAL_REFERENCE` | `cha_Guid` | — (satır sırası + tarih) |
| `transaction_date` | `transaction_date` | `date` | ⚠️ | `DATE` | `cha_tarihi` | `trxdate` |
| `amount` | `amount` | `debit_amount` / `credit_amount` | ⚠️ | `AMOUNT` (kasa), `TOTAL_DEBIT`/`TOTAL_CREDIT` (banka, cari) | `cha_meblag` | `debit` / `credit` |
| `currency` | `currency` | `debit_currency` / `credit_currency` | ⚠️ | `CURR_TRANS` | ⚠️ | — |
| `amount_try` | hesap | `amount_in_trl` | ⚠️ | `RC_AMOUNT` ⚠️ | ⚠️ | — |
| `cash_box_id` | `cash_box_id` | sorgulanan hesap | sorgulanan vault | `SD_CODE` / `BNACCCODE` / `BANKACC_CODE` | ⚠️ | — |
| `customer_id` | `customer_id` | `payments` → fatura → cari ⚠️ | ⚠️ | `ARP_CODE` | `cha_kod` | sorgulanan cari |
| `description` | `description` | `description` | ⚠️ | `DESCRIPTION` / `NOTES1` | ⚠️ | `note` |
| `reference_document_no` | `reference_document_no` | — | `serial_no` | `DOC_NUMBER` | ⚠️ | — |
| `referenceable_*` (fatura bağlantısı) | `referenceable_type`, `referenceable_id` | fatura `payments.transaction` | `document_id` | ⚠️ | ⚠️ | — |

### 8.2 `transaction_type` / `transaction_subtype`

| Ortak | ET `transaction_type` | PR `transaction_type` ⚠️ | KB | LG fiş `TYPE` ⚠️ | BH `type` ⚠️ |
|---|---|---|---|---|---|
| `income` / `collection` | `income` ⚠️ | tahsilat değeri ⚠️ (hesap `debit` tarafı) | `cash_flow_direction = 1` | kasa/cari tahsilat fişi | tahsilat satırı |
| `expense` / `payment` | `expense` ⚠️ | ödeme değeri ⚠️ (hesap `credit` tarafı) | `cash_flow_direction = -1` | kasa/cari ödeme fişi | ödeme satırı |
| `retail_sale` | `retail_sale` | — | — | perakende tahsilat ⚠️ | — |
| `opening_balance` | `opening_balance` | açılış ⚠️ | ⚠️ | devir fişi ⚠️ | — |
| `cash_box_transfer` (ayrı tablo) | `cash-boxes/transfers` | hesaplar arası ⚠️ | ⚠️ | `SD_CODE_CROSS` dolu kasa fişi, banka virman fişi | — |
| `expense` / `bank_fee` | masraf kaydı | `bank_fees` ödemesi | ⚠️ | banka masraf fişi ⚠️ | — |
| `expense` / `salary_payment` | `personnel-transactions` (`salary_payment`) | `salaries` ödemesi | — | — | — |
| `expense` / `tax_payment` | masraf kaydı | `taxes` ödemesi | — | — | — |

### 8.3 `payment_method_type`

| Ortak | ET | PR | KB | LG | BH |
|---|---|---|---|---|---|
| `cash` | `cash` | hesap tipi `cash` | vault `safe_deposit` | kasa fişi | `payment` ⚠️ |
| `bank_transfer` | `bank_transfer` | hesap tipi `bank` | vault `bank_account` | banka fişi / `BANKACC_CODE` dolu cari fişi | ⚠️ |
| `credit_card` | `credit_card` | — | ⚠️ | kredi kartı cari fişi ⚠️ | ⚠️ |
| `check` | `check` | — | çek modülü ⚠️ | çek/senet bordrosu ⚠️ | ⚠️ |
| `promissory_note` | `promissory_note` | — | senet modülü ⚠️ | çek/senet bordrosu ⚠️ | ⚠️ |
| `credit` (veresiye) | `credit` | — | — | — | — |
| `unknown` ⊕ | — | — | — | — | varsayılan |

## 9. Masraf (`expense`)

| Ortak alan | ET `expenses` | PR | KB `invoices?type=general_expense` | LG `/safeDepositSlips` (masraf) |
|---|---|---|---|---|
| Kaynak kayıt | masraf | `purchase_bills` (`is_detailed=false`, e-belgesiz), `taxes` | `general_expense` | kasa masraf fişi ⚠️ |
| `expense_date` | `expense_date` | `issue_date` | `order_date` ⚠️ | `DATE` |
| `amount` | `amount` | `net_total` | toplam ⚠️ | `GROSS_AMOUNT` ⚠️ |
| `tax_rate` | `tax_rate` (`20_kdv`) | oran tahmini (`header_estimate`) | satır `vat_rate` | `VAT_RAT` |
| `tax_amount` ⊕ | hesap | `total_vat` | ⚠️ | `VAT_TOT` |
| `tax_exclusive_amount` ⊕ | hesap | `net_total − total_vat` | ⚠️ | `NET_AMOUNT` |
| `expense_category_id` | `expense_category_id` | `category` | gider tipi ⚠️ | — (hesap kodu `CASH_ACC_CODE` / karşı hesap) |
| `description` | `description` | `description` | `description` | `DESCRIPTION` |
| `personnel_id` | `personnel_id` | `spender` | — | — |
| `payment_method` | `payment_method` | hesap tipinden | vault tipinden | `cash` |
| `vehicle_plate` ⊕ | OCR / açıklama | açıklama regex | açıklama regex | `NUMBERPLATE` |
| `vendor_tax_number` ⊕ | OCR | `supplier.tax_number` | `associate` | `TAX_NR` / `TCKNO` |
| `receipt_number` ⊕ | OCR | `invoice_no` | `serial_no` | `DOC_NUMBER` |
| Fiş görseli | `receipt_url` | — | — | — |
| `status` (onay) | `status` ⚠️ | — (hepsi kesin) | — | `CANCELLED=0` |

Mikro ve BizimHesap: masraf API ile alınamıyor. Easytrade mobil uygulaması ve OCR birincil kanal.

## 10. Gelir / gider türü (`expense_category`)

| Ortak alan | ET | PR `item_categories` | KB | LG | MK ⚠️ |
|---|---|---|---|---|---|
| Kaynak liste | `expense-categories` | `category_type=Expenditure` (gider), `SalesInvoice` (gelir) | `financial_action_types` | `/GLAccounts` (`6xx`, `7xx`) | hesap planı / hizmet kartları |
| `name` | `name` | `name` | ⚠️ | hesap adı | ⚠️ |
| `parent_id` | `parent_id` | `parent_id` | — | hesap kodu hiyerarşisi | ⚠️ |
| `full_name` | `full_name` | `full_path` | `name` | `770.01.003 Akaryakıt` gibi | ⚠️ |
| `scope` ⊕ | `expense` | `Expenditure`→`expense`, `SalesInvoice`→`income` | `expense` | `6xx`→`income`, `7xx`→`expense` | ⚠️ |

## 11. Perakende, personel, çek/senet

| Varlık | ET | PR | KB | LG | MK |
|---|---|---|---|---|---|
| `retail_sale` | `retail-sales` | — | — | `salesInvoices` TYPE 7 → `invoice` olarak (perakende satış faturası) | ⚠️ |
| `retail_refund` | `retail-refunds` | — | — | TYPE 2 → `invoice` (IADE) | ⚠️ |
| `personnel_transaction` | `personnel-transactions` | `salaries` → `salary_accrual`, ödemeleri → `salary_payment` | çalışan carileri (`associate_type=*_employee`) | — | — |
| `check` / `promissory_note` | `checks`, `promissory-notes` | — | `cheques`, `bonds` ⚠️ | `/chequeAndPnotes` ⚠️ | ⚠️ |
