# 04 · Excel İçe Aktarma (API'si olmayan kaynaklar)

API'si olmayan veya okuma ucu yetersiz olan kaynaklarda (BizimHesap, Logo İşbaşı, Robom, Mikro JumpBulut ve listede olmayan her program) veri, **programın kendi Excel dışa aktarımı** ile alınır:

1. Mükellef veya müşavir kendi programından listeyi dışa aktarır.
2. Dosyayı portala yükler.
3. Portal dosyayı tanır, kolonları ortak modele eşler (AI destekli), doğrular.
4. Müşavir önizlemeyi onaylar.
5. Veri, API'den gelmiş gibi **aynı ortak modele** (`01`) ve aynı senkron motoruna (`02`) yazılır.

Bu yöntemle gelen kayıtlar `source_system = manual_upload` değil, **asıl kaynak programın adıyla** işaretlenir (`bizimhesap`, `logo_isbasi` ...). `capture_method = excel_import` ⊕ ayrıca tutulur. Böylece ileride o programın API'si açılırsa aynı kayıtlar mükerrer olmadan eşleşir.

## 1. Desteklenen veri türleri

| `data_type` | Ortak varlık | Asgari zorunlu kolonlar |
|---|---|---|
| `outgoing_invoices` | `invoice` (`direction=outgoing`) | Tarih, fatura no, karşı taraf adı veya VKN/TCKN, KDV hariç tutar veya genel toplam |
| `incoming_invoices` | `invoice` (`direction=incoming`) | Aynı |
| `invoice_items` | `invoice_item` | Fatura no, kalem adı, miktar veya tutar, KDV oranı |
| `financial_transactions` | `financial_transaction` | Tarih, tutar, yön (giriş/çıkış ya da borç/alacak kolonları) |
| `expenses` | `expense` | Tarih, tutar, açıklama veya kategori |
| `customers` | `customer` | Ad / unvan |
| `expense_categories` | `expense_category` | Ad |
| `retail_sales` | `retail_sale` | Tarih, tutar |
| `bank_statement` | `bank_transaction` ⊕ | Tarih, açıklama, tutar (işaretli tek kolon veya borç/alacak kolonları). Ek kurallar `23` §4. |

Bir dosyada birden fazla sayfa (sheet) varsa her sayfa ayrı `data_type` ile işlenebilir.

## 2. Dosya kuralları

| Konu | Kural |
|---|---|
| Biçim | `.xlsx`, `.xls`, `.csv` (`;` veya `,` ayraçlı, UTF-8 / Windows-1254) |
| Boyut | En fazla 20 MB, en fazla 100.000 satır |
| Başlık satırı | İlk satır olmak zorunda değil. Logo, banner, boş satır ve birleştirilmiş başlıklar atlanır (§4.1) |
| Toplam satırları | "Toplam", "Genel Toplam", "Ara Toplam" satırları veri sayılmaz |
| Saklama | Orijinal dosya `attachment` olarak saklanır (`kind = source_file` ⊕). İçe aktarma geri alınabilir. |

## 3. Akış ve durumlar

```
uploaded ──► analyzing ──► mapping_review ──► validating ──► preview ──► committed
                │                │                 │             │
                └──► failed      └──► (müşavir     └──► has_errors (satır hataları)
                                       eşlemeyi
                                       düzeltir)
```

| Durum | Anlamı |
|---|---|
| `uploaded` | Dosya alındı |
| `analyzing` | Sayfa, başlık satırı ve kaynak program tespiti, kolon eşleme önerisi |
| `mapping_review` | Eşleme önerisi hazır. Güven skoru düşük kolonlar müşavire sorulur. **Kayıtlı şablon tam eşleşirse bu adım atlanır.** |
| `validating` | Satırlar ortak modele çevrilip doğrulanıyor |
| `has_errors` | Hatalı satırlar var. Müşavir düzeltir, atlar veya dosyayı yeniden yükler. |
| `preview` | Eklenecek / güncellenecek / mükerrer kayıt özeti |
| `committed` | Veri ortak modele yazıldı |
| `failed` | Dosya okunamadı |
| `rolled_back` | İçe aktarma geri alındı (bu içe aktarmanın oluşturduğu kayıtlar silinir) |

## 4. Tanıma motoru

### 4.1 Yapı tespiti (deterministik)

1. Her sayfa için ilk 50 satır okunur.
2. **Başlık satırı:** Dolu hücre sayısı en yüksek, çoğunluğu metin olan ve altındaki satırlarda tarih/sayı içeren satır.
3. **İki satırlı başlık** (üstte "KDV", altta "%1 / %10 / %20" gibi) birleştirilir: `KDV %20`.
4. Veri bloğunun sonu: ilk tamamen boş satır veya toplam satırı.

### 4.2 Kaynak program tespiti (parmak izi)

- Başlık satırındaki kolon adlarının sıralı listesinden bir parmak izi (hash) üretilir.
- Daha önce onaylanmış bir **şablon** (§6) aynı parmak izine sahipse eşleme doğrudan şablondan gelir.
- Yoksa AI eşleme adımına geçilir.

### 4.3 AI destekli kolon eşleme

AI'ya giden içerik:
- Başlık satırı
- İlk 20 veri satırı (**maskelenmiş:** VKN/TCKN, IBAN, telefon ve e-posta değerleri desenleri korunarak `1234******` biçiminde)
- Beklenen `data_type` (müşavir seçtiyse) ve o türün ortak model alan listesi

AI'dan beklenen çıktı (JSON):

```json
{
  "data_type": "outgoing_invoices",
  "source_guess": "bizimhesap",
  "header_row": 3,
  "columns": [
    { "column": "B", "header": "Fatura Tarihi", "field": "issue_date", "confidence": 0.98 },
    { "column": "C", "header": "Belge No", "field": "invoice_number", "confidence": 0.95 },
    { "column": "E", "header": "Vergi No", "field": "customer_tax_number", "confidence": 0.93 },
    { "column": "H", "header": "KDV %20", "field": "tax_summary[20].vat", "confidence": 0.90 },
    { "column": "K", "header": "Toplam", "field": "amounts.payable_amount", "confidence": 0.88 },
    { "column": "M", "header": "Plasiyer", "field": null, "confidence": 0.99 }
  ],
  "warnings": ["İskonto kolonu bulunamadı"]
}
```

Kurallar:
1. **AI sadece eşleme yapar.** Tutar hesaplamaz, satır üretmez, değer değiştirmez. Dönüşüm ve hesaplar deterministik kodla yapılır.
2. `confidence < 0.80` olan kolonlar müşavire sorulur. Zorunlu bir alan eşlenemediyse içe aktarma `mapping_review`'da bekler.
3. Eşlenmeyen kolonlar (`field: null`) kaybolmaz, satırın `raw_payload`'ına yazılır.
4. AI yanıtı geçersiz JSON'sa veya şemaya uymazsa bir kez tekrar denenir. Yine olmazsa eşleme tamamen müşaviredir.

Oran bazında KDV kolonları için hedef alan kalıbı: `tax_summary[<oran>].base` ve `tax_summary[<oran>].vat` (ör. "Matrah %10", "KDV %10"). Bu kolonlar varsa `invoice_tax_summary` doğrudan dolar ve `tax_summary_source = source_summary` olur. Yoksa `header_estimate` kuralı (`01` §4.6) uygulanır.

### 4.4 Değer dönüşümü (deterministik)

| Tür | Kural |
|---|---|
| Sayı | Türkçe biçim `1.234,56` ve İngilizce `1,234.56` otomatik ayrılır (kolonun tüm değerlerine bakılarak). `₺`, `TL`, boşluk temizlenir. Parantez veya `-` negatif → mutlak değer + yön alanı. |
| Tarih | `dd.MM.yyyy`, `dd/MM/yyyy`, `yyyy-MM-dd`, Excel seri tarihi (`45231`) |
| VKN / TCKN | Sadece rakam. 10 hane → `tax_number`, 11 hane → `identity_number` |
| KDV oranı | `%20`, `20`, `0,20`, `0.2` → `20` |
| Tevkifat | `5/10`, `%50`, `0,5` → `withholding_rate = 50` |
| Para birimi | `TL`, `TRL`, `₺` → `TRY` |
| Fatura türü metni | "Satış", "Satış İade", "Alış", "Alış İade", "Tevkifatlı" → `direction` + `type` (`03` §2 tablosu) |
| E-belge türü metni | "e-Fatura", "e-Arşiv", "Kağıt" → `profile` |
| Borç / alacak kolonları | Kasa/banka listesinde borç = giriş (`income`), alacak = çıkış (`expense`). Cari ekstresinde tersi. Hangisi olduğu `data_type` ve müşavir onayıyla netleşir. |
| Yön tek kolonda (+/−) | Pozitif = giriş, negatif = çıkış |

### 4.5 Doğrulama

Her satır için:

| Kontrol | Sonuç |
|---|---|
| Zorunlu alan boş | Hata |
| Tarih geçersiz veya seçilen dönem dışında | Hata (dönem dışı ise uyarı + müşavir onayı) |
| VKN/TCKN hane sayısı yanlış | Uyarı |
| `amounts.tax_exclusive_amount + amounts.tax_amount ≈ amounts.payable_amount` (±0,05, tevkifat ve stopaj düşülerek) | Tutmazsa uyarı |
| Oran bazında KDV toplamı başlık KDV'siyle tutmuyor | Uyarı |
| Aynı dosyada aynı fatura no + karşı taraf iki kez | Hata (dosya içi mükerrer) |
| Veritabanında aynı belge var (`01` §6 kuralları) | Önizlemede "mevcut, güncellenecek" veya "olası mükerrer" |
| Kilitli döneme ait satır | Yazılmaz, `change_request` olur |

## 5. Önizleme ve onay

Müşavire gösterilen özet:
- Eklenecek kayıt sayısı ve toplam tutar
- Güncellenecek kayıt sayısı
- Mükerrer / atlanan kayıt sayısı
- Oran bazında KDV özeti (`base`, `vat`, `withholding`)
- Hata ve uyarı listesi (satır numarasıyla)

Onaydan sonra kayıtlar senkron motoru üzerinden yazılır. `source_id` olarak:
1. Dosyada kaynağın kendi kayıt kimliği varsa o,
2. Yoksa `invoice_number` + karşı taraf + tarih,
3. Hareket kayıtlarında `tarih + tutar + açıklama + satır sırası` özetinin hash'i.

Aynı dosya tekrar yüklenirse aynı `source_id`'ler üretildiği için kayıtlar **çoğalmaz,** güncellenir.

## 6. Şablonlar (öğrenen eşleme)

Onaylanan her eşleme şablon olarak saklanır:

| Alan | Açıklama |
|---|---|
| `fingerprint` | Başlık parmak izi |
| `source_system` | `bizimhesap`, `logo_isbasi`, ... |
| `data_type` | |
| `header_row` | |
| `columns` | Kolon → alan eşlemesi |
| `scope` | `company` (sadece bu mükellef), `tenant` (müşavirin tüm mükellefleri), `global` (Easytrade'in hazır şablonu) |

- Aynı programın aynı raporu bir kez eşlendikten sonra o müşavirin diğer mükelleflerinde **soru sorulmadan** işlenir.
- Easytrade ekibi, her program için örnek dosyalar toplandıkça `global` şablonlar yayınlar. Böylece AI adımı çoğu zaman hiç çalışmaz.

**İlk iş:** BizimHesap, İşbaşı, Robom ve Mikro'dan fatura listesi, fatura detayı (kalemli), kasa/banka hareketleri ve masraf listesi dışa aktarımlarının **örnek dosyaları** toplanıp `global` şablonları hazırlanmalı.

## 7. Portal API'si (Easytrade kuralları)

| Uç | Açıklama |
|---|---|
| `POST /accountant/imports` | `multipart/form-data`: `file`, `source_system`, `data_type` (isteğe bağlı), `period` (isteğe bağlı). `X-Company-ID` zorunlu. Yanıt: `data.id`, `data.status` |
| `GET /accountant/imports` | İçe aktarma listesi (`page`, `per_page`, `filters[status]`, `filters[created_at][start]`/`[end]`) |
| `GET /accountant/imports/{import}` | Durum, tespit edilen sayfalar, başlık satırı, önerilen eşleme, güven skorları, örnek satırlar |
| `PUT /accountant/imports/{import}/mapping` | Müşavirin düzelttiği eşleme. `save_template: true` ise şablon olarak kaydedilir. |
| `POST /accountant/imports/{import}/validate` | Doğrulamayı çalıştırır |
| `GET /accountant/imports/{import}/rows` | Satır bazında sonuç (`filters[result]=error\|warning\|ok`) |
| `PATCH /accountant/imports/{import}/rows/{row}` | Tek satırı düzelt veya atla |
| `GET /accountant/imports/{import}/preview` | Önizleme özeti (§5) |
| `POST /accountant/imports/{import}/commit` | Yazma. **`Idempotency-Key` zorunlu.** |
| `POST /accountant/imports/{import}/rollback` | Bu içe aktarmanın oluşturduğu kayıtları geri alır (kilitli dönem hariç) |
| `GET /accountant/import-templates` | Şablon listesi |
| `DELETE /accountant/import-templates/{template}` | Şablon sil |

Analiz ve doğrulama uzun sürebilir. Bu yüzden asenkron çalışır, istemci `GET /accountant/imports/{import}` ile durumu izler. Durum değişince müşavire bildirim gider.

## 8. Mükellef tarafı

Mükellef de dosyayı kendi uygulamasından yükleyebilir. Bu durumda:
- Yükleme müşavirin önüne `mapping_review` veya `preview` durumunda düşer.
- **Commit yetkisi sadece müşavirdedir.** Mükellef yüklemesi doğrudan beyan verisine yazılmaz.

## 9. Kabul testleri

1. Her hedef program için örnek dosya: yükle → şablon tanınıyor → sıfır müdahaleyle `preview`.
2. Bilinmeyen bir başlık yapısı: AI eşlemesi zorunlu alanları %80 üzeri güvenle buluyor.
3. Türkçe ve İngilizce sayı biçimli iki dosya doğru okunuyor.
4. Excel seri tarihi doğru çevriliyor.
5. Toplam satırı veri olarak alınmıyor.
6. Aynı dosya iki kez yükleniyor: kayıt sayısı artmıyor.
7. Oran bazında KDV kolonlu dosyada `invoice_tax_summary` kaynak programın KDV toplamıyla birebir.
8. Rollback sonrası kayıtlar ve özetler ilk hale dönüyor.
9. Kilitli döneme ait satırlar yazılmıyor, `change_request` oluşuyor.
10. AI'ya giden örnekte VKN/TCKN, IBAN, telefon ve e-posta maskelenmiş.
