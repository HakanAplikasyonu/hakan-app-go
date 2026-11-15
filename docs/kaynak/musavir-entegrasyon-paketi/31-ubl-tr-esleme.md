# 31 · UBL-TR XML → Ortak Model

Bütün entegratörlerden (ve mükellefin portala yüklediği XML / ZIP dosyalarından) gelen e-fatura ve e-arşiv XML'i **tek bir ayrıştırıcıyla** `invoice`, `invoice_item`, `invoice_tax_summary` tablolarına (`01` §4.4–4.6) yazılır. Entegratör adaptörleri sadece XML'i indirir; içeriği yorumlamaz.

Kaynak standart: GİB UBL-TR 1.2 (Fatura ve Ortak Elemanlar kılavuzları). Elemanların yeri ve anlamı bu kılavuzlardan; **kod listeleri** (vergi tipi kodları, istisna kodları, birim kodları) GİB "UBL-TR Kod Listeleri" kılavuzundan tablo olarak yüklenir, kodda sabit yazılmaz.

## 1. Girdi

| Girdi | İşlem |
|---|---|
| Ham XML (`<Invoice …>`) | Doğrudan ayrıştır |
| Base64 metin | Çöz |
| ZIP (tek veya çok XML) | Aç, her XML'i ayrı işle (İzibiz `COMPRESSED=Y`, QNB indirme yanıtları ZIP dönebiliyor) |
| Zarf (`StandardBusinessDocument`) | İçindeki `Invoice` elemanlarını çıkar |

Kök eleman `Invoice` olmalı (`urn:oasis:names:specification:ubl:schema:xsd:Invoice-2`). Ad alanları: `cac` = `…:CommonAggregateComponents-2`, `cbc` = `…:CommonBasicComponents-2`. İmza bölümü (`ext:UBLExtensions`) okunmaz, olduğu gibi saklanır.

Ham XML her zaman `attachment` (`kind=ubl_xml`, SHA-256) olarak saklanır. Yeniden işleme gerektiğinde entegratöre tekrar gidilmez.

## 2. Başlık → `invoice`

| UBL-TR | Ortak model | Not |
|---|---|---|
| `cbc:UUID` | `uuid` | ETTN. Mükerrer kontrolünün birinci anahtarı (`01` §6). |
| `cbc:ID` | `invoice_number` | 16 karakter (3 seri + 4 yıl + 9 sıra) |
| `cbc:IssueDate` | `issue_date` | Dönem buna göre |
| `cbc:IssueTime` | `issue_time` | |
| `cbc:ProfileID` | `profile` | §3.2 |
| `cbc:InvoiceTypeCode` | `type` | §3.1, `direction` ile birlikte |
| `cbc:DocumentCurrencyCode` | `currency` | |
| `cac:PricingExchangeRate/cbc:CalculationRate` | `exchange_rate` | Dövizli faturada |
| `cbc:Note` (çoklu) | `notes` | Satır sonlarıyla birleştirilir |
| `cac:BillingReference/cac:InvoiceDocumentReference/cbc:ID` | `original_invoice_number` | İade faturasında asıl fatura |
| `cac:BillingReference/cac:InvoiceDocumentReference/cbc:IssueDate` | `original_invoice_issue_date` | |
| `cac:OrderReference` (`cbc:ID`, `cbc:IssueDate`) | `order_reference` | |
| `cac:DespatchDocumentReference` | `raw_payload` (+ irsaliye eşleşmesi) | |
| `cac:PaymentMeans/cbc:PaymentDueDate` | `payment_due_date` | |
| Karşı taraf | `customer_*` | §4 |
| `cac:LegalMonetaryTotal/cbc:LineExtensionAmount` | `amounts.line_extension_amount` | |
| `cac:LegalMonetaryTotal/cbc:AllowanceTotalAmount` | `amounts.allowance_total_amount` | |
| `cac:LegalMonetaryTotal/cbc:TaxExclusiveAmount` | `amounts.tax_exclusive_amount` | |
| `cac:LegalMonetaryTotal/cbc:TaxInclusiveAmount` | `amounts.tax_inclusive_amount` | |
| `cac:LegalMonetaryTotal/cbc:PayableAmount` | `amounts.payable_amount` | |
| `cac:LegalMonetaryTotal/cbc:ChargeTotalAmount`, `PayableRoundingAmount` | `raw_payload` | |
| `cac:TaxTotal` içindeki `TaxTypeCode=0015` alt toplamları | `amounts.tax_amount` | KDV. `cac:TaxTotal/cbc:TaxAmount` bütün vergilerin toplamıdır, KDV'ye doğrudan yazılmaz. |
| `cac:TaxTotal` içindeki diğer vergi tipleri | `amounts.excise_amount`, `amounts.communications_tax_amount` veya `tax_breakdowns` | Vergi tipi kodu → alan eşlemesi kod listesi tablosundan (§5.3) |
| `cac:WithholdingTaxTotal/cbc:TaxAmount` | `amounts.withholding_amount` | Tevkif edilen KDV (fatura geneli) |
| hesaplanan | `amounts.payable_amount_try` | `payable_amount × exchange_rate` (TRY ise aynı) |

Sabit değerler: `has_items=true`, `tax_summary_source=items`, `source_system=entegrator_<id>` (veya yüklemede `ubl_upload`), `source_id=uuid`, `capture_method=api` (yüklemede `excel_import` değil, ⊕ `ubl_upload`).

## 3. Tür ve senaryo

### 3.1 `cbc:InvoiceTypeCode` → `type`

| UBL-TR değeri | `direction=outgoing` | `direction=incoming` |
|---|---|---|
| `SATIS` | `SATIS` | `ALIS` |
| `IADE` | `IADE` | `IADE` |
| `TEVKIFAT` | `TEVKIFAT` | `TEVKIFAT` |
| `ISTISNA` | `ISTISNA` | `ISTISNA` |
| `OZELMATRAH` | `OZELMATRAH` | `OZELMATRAH` |
| `IHRACKAYITLI` | `IHRACKAYITLI` | `IHRACKAYITLI` |
| `SGK` | `SGK` | `SGK` |
| `TEVKIFATIADE` | `IADE` (tevkifat satırları korunur) | `IADE` |
| Listede olmayan her değer (ör. `KOMISYONCU`) | `SATIS` / `ALIS` + uyarı `UNMAPPED_TYPE_CODE` | aynı |

Ham değer her zaman `invoice_type_code_raw` ⊕ alanında saklanır. Ortak modelin `type` enum'u Easytrade enum'udur (`01` §4.4) ve genişletilmez; eşleşmeyen değer müşavire gösterilir.

`TEVKIFATIADE` ve `KOMISYONCU` değerlerinin varlığı İşNet `InvoiceType` listesinden (`Komisyoncu`, `TevkifatIade`) doğrulandı. Tam liste GİB kod listesinden alınır.

### 3.2 `cbc:ProfileID` → `profile`

| UBL-TR | `profile` |
|---|---|
| `TEMELFATURA` | `TEMELFATURA` |
| `TICARIFATURA` | `TICARIFATURA` |
| `EARSIVFATURA` | `EARSIVFATURA` |
| `IHRACAT` | `IHRACAT` |
| `YOLCUBERABERFATURA` | `YOLCUBERABERFATURA` |
| `KAMU` | `KAMU` |
| `ILAC_TIBBICIHAZ` | `ILAC_TIBBICIHAZ` |
| diğer (ör. `HKS`) | `OZELFATURA` + uyarı `UNMAPPED_PROFILE`. Ham değer her zaman `profile_raw` ⊕ alanında. |

`profile=NULL` ortak modelde "kâğıt / bilinmiyor" demektir (`01` §4.4); UBL'den gelen fatura hiçbir zaman `NULL` almaz.

## 4. Karşı taraf

Yön belirler: `direction=outgoing` ise karşı taraf `cac:AccountingCustomerParty/cac:Party`, `incoming` ise `cac:AccountingSupplierParty/cac:Party`. Mükellefin kendi tarafı, adaptörün bağlandığı VKN ile **doğrulanır** (eşleşmezse fatura işlenmez, uyarı `PARTY_MISMATCH`).

| UBL-TR (`cac:Party` altında) | Ortak model |
|---|---|
| `cac:PartyIdentification/cbc:ID[@schemeID="VKN"]` | `customer_tax_number` |
| `cac:PartyIdentification/cbc:ID[@schemeID="TCKN"]` | `customer_identity_number` |
| `cac:PartyName/cbc:Name` | `customer_name` (kurumsal) |
| `cac:Person/cbc:FirstName` + `cbc:FamilyName` | `customer_name` (bireysel, `PartyName` yoksa) |
| `cac:PartyTaxScheme/cac:TaxScheme/cbc:Name` | `customer.tax_office` |
| `cac:PostalAddress/cbc:CityName`, `cbc:CitySubdivisionName`, `cbc:StreetName`, `cbc:BuildingNumber`, `cbc:PostalZone` | `customer.city`, `district`, `address`, `postal_code` |
| `cac:Contact/cbc:ElectronicMail`, `cbc:Telephone` | `customer.email`, `phone` |

`PartyIdentification` birden fazla olabilir (ör. `MERSISNO`, `TICARETSICILNO`); sadece `VKN` ve `TCKN` şemaları kimlik olarak alınır, diğerleri `raw_payload`'da kalır.

Karşı taraf `customer` tablosunda VKN/TCKN ile bulunur veya oluşturulur (`01` §4.3 birleştirme kuralı). E-arşivde nihai tüketiciye kesilen faturalarda TCKN `11111111111` gibi genel değer gelebilir; bu değer `customer.is_default_retail_customer=true` olan kayda bağlanır.

## 5. Satırlar → `invoice_item`

### 5.1 Alanlar

| UBL-TR (`cac:InvoiceLine` altında) | Ortak model | Not |
|---|---|---|
| `cbc:ID` | `line_no` | |
| `cac:Item/cbc:Name` | `name` | |
| `cac:Item/cbc:Description` | `description` | |
| `cac:Item/cac:SellersItemIdentification/cbc:ID` | `product_code` | |
| `cbc:InvoicedQuantity` | `quantity` | |
| `cbc:InvoicedQuantity/@unitCode` | `unit_code` | UBL birim kodu (`C62`, `KGM` …) |
| `cac:Price/cbc:PriceAmount` | `unit_price` | |
| hesaplanan `quantity × unit_price` | `line_extension_amount` | Ortak modelde "miktar × fiyat" |
| `cac:AllowanceCharge[cbc:ChargeIndicator="false"]/cbc:Amount` toplamı | `own_allowance_amount` | Satır iskontosu |
| `cbc:LineExtensionAmount` | `tax_exclusive_amount` | UBL-TR'de satır tutarı iskonto düşülmüş net tutardır (KDV matrahı). Kontrol: aynı satırın KDV `TaxableAmount` değeriyle eşit olmalı (±0,01), değilse uyarı. |
| `cac:TaxTotal/cac:TaxSubtotal[TaxTypeCode=0015]/cbc:Percent` | `tax_rate` | |
| aynı alt toplamın `cbc:TaxAmount` | `total_tax_amount` (KDV) | |
| aynı alt toplamın `cac:TaxCategory/cbc:TaxExemptionReasonCode` | `tax_exemption_code` | KDV %0 satırlarda istisna kodu |
| `cac:WithholdingTaxTotal/cac:TaxSubtotal/cac:TaxCategory/cac:TaxScheme/cbc:TaxTypeCode` | `withholding_code` | `601`–`650` |
| aynı alt toplamın `cbc:Percent` | `withholding_rate` | Yüzde olarak (`50`, `70`, `90` …), `01` §1.1 kuralı |
| aynı alt toplamın `cbc:TaxAmount` | `withholding_amount` | |
| Diğer vergi tipi alt toplamları (ÖTV, ÖİV …) | `excise_*`, `communications_tax_*` veya `tax_breakdowns` | §5.3 |
| bütün `TaxSubtotal`'lar ham | `tax_breakdowns` | JSONB, kayıpsız |
| `tax_exclusive_amount + total_tax_amount + diğer vergiler` | `tax_inclusive_amount` | |

### 5.2 Özel durumlar

| Durum | Kural |
|---|---|
| Özel matrah (`OZELMATRAH`) | Özel matrah kodu (`801`–`811`) ve tutarı ilgili `TaxSubtotal`/`TaxCategory`'den `special_tax_base_code`, `special_tax_base_amount` |
| İhraç kayıtlı (`IHRACKAYITLI`) | İhraç kayıtlı kodu (`701`–`704`) `export_registered_code` |
| İhracat (`IHRACAT`) | GTİP kodu → `gtip_code` (satır teslimat bilgisindeki yeri kılavuzdan teyit edilecek ⚠️). Ayrıntılar `raw_payload`. |
| Fatura geneli istisna | Bütün KDV alt toplamlarında aynı `TaxExemptionReasonCode` varsa `invoice.global_tax_exemption_code` |
| Satırsız XML (bozuk / eksik) | `has_items=false`, başlıktan `header_estimate` (`01` §4.6), uyarı |

Özel matrah ve ihraç kayıtlı kodlarının UBL-TR'deki tam yeri kılavuzdan teyit edilerek ayrıştırıcıya yazılır ⚠️; teyit edilene kadar bu faturalarda `tax_breakdowns` ham saklanır ve müşavire uyarı gider.

### 5.3 Vergi tipi kodları

- KDV: `TaxTypeCode = 0015` (GİB UBL-TR Ortak Elemanlar kılavuzundaki örneklerde geçen kod).
- Tevkifat: `cac:WithholdingTaxTotal` altında, kod `601`–`650` (Easytrade `GET /company/withholding-codes` listesiyle aynı set).
- Diğer vergiler (ÖTV listeleri, ÖİV, damga vergisi, stopaj vb.): **GİB UBL-TR Kod Listeleri** kılavuzundaki "Vergi Türü Kodları" tablosu `tax_type_code` tablosu olarak yüklenir; her koda ortak modelde hangi alana yazılacağı (`excise`, `communications_tax`, `income_tax_withholding`, `other`) bu tabloda tutulur. Listede olmayan kod `tax_breakdowns`'ta kalır ve uyarı üretir.

## 6. `invoice_tax_summary`

Satırlardan üretilir (`01` §4.6 kural 1):

```
GROUP BY tax_rate, withholding_code, tax_exemption_code
  base        = Σ tax_exclusive_amount
  vat         = Σ total_tax_amount
  withholding = Σ withholding_amount
```

Kontroller (tutmazsa uyarı, kayıt yazılır):

| Kontrol | Tolerans |
|---|---|
| Σ satır KDV = başlık KDV (`0015` alt toplamları) | ±0,05 |
| Σ satır matrah = `TaxExclusiveAmount` | ±0,05 |
| `TaxInclusiveAmount − WithholdingTaxTotal (+ yuvarlama) = PayableAmount` | ±0,05 |
| Başlık `cac:TaxTotal` alt toplamlarının oran bazında toplamı = satır özetinin oran bazında toplamı | ±0,05 |

## 7. Durum

UBL XML'de fatura durumu (kabul/red/iptal) **yoktur**. Durum entegratörün liste/durum servisinden gelir ve `status`, `status_normalized` alanlarına `30` §8 kurallarıyla yazılır.

## 8. Kabul testleri (ayrıştırıcı)

GİB'in örnek faturaları ve her entegratörden en az birer gerçek XML ile:

1. Temel fatura, tek oran: başlık, karşı taraf, tek satır, özet.
2. Ticari fatura, üç oran (`%1`, `%10`, `%20`) ve satır iskontosu.
3. Tevkifatlı fatura (ör. `%50`): satır ve başlık tevkifatı, `PayableAmount` kontrolü.
4. İstisnalı fatura (KDV %0 + istisna kodu).
5. İade faturası: `BillingReference` → asıl fatura no/tarih.
6. Dövizli fatura: `CalculationRate`, `payable_amount_try`.
7. E-arşiv nihai tüketici faturası (TCKN `11111111111`).
8. ZIP içinde birden çok XML.
9. Kendi tarafı bağlanılan VKN ile eşleşmeyen XML reddediliyor.
10. Bilinmeyen `InvoiceTypeCode` / `ProfileID` / vergi kodu → kayıt yazılıyor + uyarı.

## Kaynaklar

- GİB UBL-TR Ortak Elemanlar Kılavuzu v1.2 (örnek `TaxTypeCode 0015`, `PartyIdentification/@schemeID` VKN/TCKN): Uyumsoft topluluk paketinde kopyası, https://github.com/gkvdt/uyumsoft-einvoice
- İşNet E-Fatura Web Servis Arayüz Dokümanı (V2) `InvoiceType` listesi: https://github.com/EfeSorogluu/NetteFatura-API
- e-Fatura kılavuzları: https://ebelge.gib.gov.tr
