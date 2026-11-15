# 05 · Uyarı Kodları ve `data_warning` Tablosu

Dokümanlarda geçen bütün uyarılar burada tek listede. Kod yazılırken uyarı metni değil **kod** üretilir; metin müşavir paneli tarafında koddan oluşturulur. Listede olmayan bir uyarı gerekiyorsa önce buraya eklenir.

## 1. `data_warning` ⊕

| Alan | Tip | Açıklama |
|---|---|---|
| id | UUID | |
| company_id | UUID | |
| period | CHAR(7) | Uyarının ait olduğu dönem (`2026-10`); bağlantı olaylarında `NULL` |
| entity_type | TEXT | `invoice`, `invoice_item`, `expense`, `financial_transaction`, `retail_sale`, `bank_transaction`, `entegrator_document`, `period` (dönem geneli kontroller) |
| entity_id | UUID | Dönem geneli kontrolde `NULL` |
| code | TEXT | §3 listesindeki kod |
| severity | TEXT | `info`, `warning`, `blocker` (§2) |
| params | JSONB | Metni üretmek için değerler (ör. `{ "expected": "120.00", "actual": "119.80" }`) |
| producer | TEXT | Üreten bileşen: `adapter:<source_system>`, `ubl_parser`, `excel_import`, `sync_engine`, `control:<kural>` |
| status | TEXT | `open`, `acknowledged` (müşavir gördü, kabul etti), `resolved` (müşavir düzeltti), `auto_resolved` (koşul ortadan kalktı) |
| resolved_by, resolved_at | UUID, TIMESTAMPTZ | |
| created_at, updated_at | TIMESTAMPTZ | |

Kurallar:

- Aynı `(entity_type, entity_id, code)` için en fazla bir `open` kayıt olur. Senkron tekrar çalıştığında aynı uyarı **yeniden üretilmez**, `updated_at` güncellenir.
- Koşul ortadan kalkarsa (ör. durum `PENDING_RESPONSE`'tan `Onaylandi`'ya döndü) üreten bileşen kaydı `auto_resolved` yapar.
- `GET /accountant/warnings` (`02` §8) bu tabloyu döner; filtreler: `filters[period]`, `filters[code]`, `filters[severity]`, `filters[status]`, `filters[entity_type]`.
- `GET /accountant/periods/{period}/readiness` (`02` §8): dönemde `blocker` seviyesinde `open` uyarı varsa dönem "beyana hazır" değildir.

## 2. Seviye

| Seviye | Anlamı |
|---|---|
| `info` | Bilgi. Beyanı etkilemez. |
| `warning` | Müşavir bakmalı; dönemi kilitlemeyi engellemez. |
| `blocker` | Müşavir `acknowledged` veya `resolved` yapmadan dönem "beyana hazır" sayılmaz. |

Aşağıdaki seviyeler **öneridir**; ürün tarafı (müşavir ihtiyacı) değiştirebilir. Seviye kodda sabit yazılmaz, yapılandırma tablosundan okunur.

## 3. Belge ve dönem uyarıları

### 3.1 Fatura ve UBL

| Kod | Seviye | Varlık | Ne zaman | Kaynak |
|---|---|---|---|---|
| `PENDING_RESPONSE` | `info` | invoice | Ticari fatura kabul/red yanıtı bekliyor | `30` §8 |
| `REJECTED` | `warning` | invoice | Alıcı reddetti; `status_normalized=cancelled` | `30` §8 |
| `RETURNED` | `warning` | invoice | İade yanıtı; `cancelled`, müşavir karar verir | `30` §8 |
| `DELIVERY_ERROR` | `blocker` | invoice | GİB / sistem hatası; fatura geçerli olmayabilir | `30` §8, `32` §8, `33` §9 |
| `STATUS_UNMAPPED` | `warning` | invoice | Entegratörün durum değeri tabloda yok | `32` §8, `33` §9 |
| `STATUS_UNVERIFIED` | `info` | invoice | Durum tablosu henüz doğrulanmamış entegratör (geçici kural) | `34` §5, `36` §7 |
| `EARCHIVE_REPORT_ERROR` | `warning` | invoice | E-arşiv GİB raporunda hata | `32` §8.2 |
| `DOC_KIND_MISMATCH` | `warning` | invoice | Listedeki e-fatura / e-arşiv bilgisi UBL `ProfileID` ile çelişiyor | `33` §7.3 |
| `UUID_MISMATCH` | `blocker` | entegrator_document | Listedeki ETTN ile UBL `cbc:UUID` farklı; kayıt yazılmadı | `37` §5.4 |
| `PARTY_MISMATCH` | `blocker` | invoice | UBL'deki taraf VKN'si bağlantının VKN'siyle uyuşmuyor | `31` §4 |
| `UNMAPPED_TYPE_CODE` | `warning` | invoice | Bilinmeyen `InvoiceTypeCode`; `SATIS`/`ALIS` yazıldı | `31` §3.1 |
| `UNMAPPED_PROFILE` | `info` | invoice | Bilinmeyen `ProfileID`; `OZELFATURA` yazıldı | `31` §3.2 |
| `UNKNOWN_TAX_CODE` | `warning` | invoice | UBL'de kod listesinde olmayan vergi tipi kodu | `31` §5.3, §8 |
| `LINE_TAXABLE_MISMATCH` | `warning` | invoice_item | Satır `LineExtensionAmount` ≠ KDV `TaxableAmount` (±0,01) | `31` §5.1 |
| `TAX_TOTAL_MISMATCH` | `warning` | invoice | Σ satır KDV ≠ başlık KDV, veya oran bazında toplam tutmuyor (±0,05) | `31` §6, `09` §3.2, `10`, `04` §4.5 |
| `BASE_TOTAL_MISMATCH` | `warning` | invoice | Σ satır matrah ≠ `TaxExclusiveAmount` (±0,05) | `31` §6 |
| `PAYABLE_MISMATCH` | `warning` | invoice | Matrah + KDV − tevkifat − stopaj ≠ ödenecek (±0,05) | `31` §6, `04` §4.5 |
| `HEADER_ESTIMATE` | `warning` | invoice | Satır yok; KDV kırılımı başlıktan tahmin edildi (`tax_summary_source=header_estimate`) | `01` §4.6, `31` §5.2, `30` §6 |
| `UBL_MISSING` | `warning` | entegrator_document | Listede var, UBL 5 denemede indirilemedi | `37` §5.5 |
| `MISSING_AT_SOURCE` | `warning` | entegrator_document | Önceden görülen belge iki ardışık tam taramada kaynakta yok | `37` §5.5 |
| `UNPROCESSED_INBOX` | `info` | invoice | Gelen kutusunda var, mükellef alış olarak işlememiş | `09` §3.1 |

### 3.2 Genel veri kalitesi

| Kod | Seviye | Varlık | Ne zaman | Kaynak |
|---|---|---|---|---|
| `POSSIBLE_DUPLICATE` | `warning` | invoice | Karşı taraf + tarih + tutar eşit, ETTN / numara farklı | `01` §6 kural 3 |
| `INVALID_TAX_ID` | `warning` | invoice, customer | VKN/TCKN 10 veya 11 hane değil | `01` §7 |
| `DELETED_AT_SOURCE` | `warning` | herhangi | Kaynakta silinmiş (`deleted_at_source` dolu) | `01` §7, `02` §3.2 |
| `LOCKED_PERIOD_CHANGE` | `blocker` | herhangi | Kilitli döneme değişiklik geldi; `change_request` oluştu | `02` §3.4, `20` §7.5 |
| `VAT_REPORT_MISMATCH` | `warning` | period | Kaynağın KDV raporu ≠ portalın KDV özeti | `01` §4.15, `09` §8 |
| `OUT_OF_PERIOD` | `warning` | herhangi | Excel satırı seçilen dönem dışında, müşavir onayıyla yazıldı | `04` §4.5 |
| `UNAPPROVED_EXPENSE` | `info` | expense | Onaysız masraf; beyan toplamına girmedi | `01` §4.12 |
| `RETAIL_ALREADY_INVOICED` | `info` | retail_sale | Faturaya dönüşmüş perakende satış; KDV toplamına ikinci kez girmedi | `01` §4.7 |

### 3.3 Müşavir talebinden gelen kontroller

Kuralların neden istendiği ve müşavirin sözü `06`'da. Eşikler ve kelime listeleri **ürün kararı**dır, mükellef bazında ayarlanabilir; kodda sabit yazılmaz.

| Kod | Seviye | Varlık | Ne zaman | Kaynak |
|---|---|---|---|---|
| `FIXED_ASSET_CANDIDATE` | `warning` | invoice_item | Gelen faturada kalem demirbaş olabilir (`line_type=fixed_asset_candidate`) | `06` T3 |
| `UNUSUAL_RECORD_TYPE` | `warning` | invoice_item | Kalem mükellefin olağan satış / alış kalemlerinden belirgin farklı | `06` T3 |
| `UNUSUAL_AMOUNT` | `warning` | invoice | Tutar, karşı tarafın (yoksa mükellefin) geçmiş ortalamasının ayarlanan katını aşıyor | `06` T3 |
| `NEW_PARTY_HIGH_AMOUNT` | `warning` | invoice | İlk kez görülen karşı taraf ve tutar ayarlanan sınırın üstünde | `06` T3 |
| `PLATE_NOT_REGISTERED` | `warning` | expense | Fişteki plaka mükellefin araç listesinde yok veya fiş tarihi geçerlilik dışında | `06` T8 |

Ödeme türü tutarlılığı (`06` T11) banka kontrolüdür: `BNK_PAYMENT_METHOD_MISMATCH`.

### 3.4 Banka

`21` §6.3'teki kontrol kodları: `BNK_UNINVOICED_COLLECTION`, `BNK_UNDOCUMENTED_PAYMENT`, `BNK_FEES_NOT_BOOKED`, `BNK_INTEREST`, `BNK_TAX_PAID`, `BNK_RENT_WITHHOLDING`, `BNK_SALARY_VS_PAYROLL`, `BNK_PAYMENT_METHOD_MISMATCH`, `BNK_CASH_LIMIT`, `BNK_POS_VS_SALES`, `BNK_BALANCE_MISMATCH`, `BNK_GAP`, `BNK_NON_BUSINESS`, `BNK_UNCLASSIFIED`. Anlamları ve hesaplama kuralları orada. Önerilen seviye: `BNK_GAP` ve `BNK_BALANCE_MISMATCH` `blocker`, diğerleri `warning`.

## 4. Bağlantı olayları (`data_warning`'e yazılmaz)

Bağlantıya ait sorunlar `source_connection.status` ve `source_connection.last_error` (`01` §4.2) üzerinden yürür, müşavire / mükellefe bildirim gider.

| Kod | Bağlantıya etkisi | Kaynak |
|---|---|---|
| `AUTH_FAILED` | `status=auth_failed` | `02` §5, `37` §4 |
| `IP_REJECTED` | `status=auth_failed` + iç uyarı (Easytrade operasyonu) | `32` §9 |
| `VKN_NOT_AUTHORIZED` | `status=auth_failed` + iç uyarı | `37` §4 |
| `VKN_MISMATCH` | Kurulumda `setup_state=failed` | `33` §4 |
| `LICENSE_EXPIRED` | `status=disabled` | `09` §1.3 |
| `RATE_LIMITED` | `status=rate_limited`, otomatik geri döner | `02` §5 |
| `AGENT_OFFLINE` | Bağlantı ajanı 30 dk sessiz | `02` §6 |
| `IZIBIZ_LIMIT_REACHED` | Yok; geliştirici uyarısı (1 günlük pencerede `LIMIT` doldu) | `36` §5.3 |
| `PROVIDER_ERROR` | Yok; tekrar denenir, ham yanıt saklanır | `37` §4 |
