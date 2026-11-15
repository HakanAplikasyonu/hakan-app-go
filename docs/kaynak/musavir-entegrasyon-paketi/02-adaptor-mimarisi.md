# 02 · Adaptör Mimarisi

Her kaynak program için bir **adaptör** yazılır. Adaptörler aynı arayüzü uygular, ortak veri modeline (`01`) yazar ve aynı senkron motoru tarafından çalıştırılır. Portalın kendi API'si de Easytrade Muhasebe API kurallarını kullanır (§8).

## 1. Veri akışı

```
Kaynak program ──► Adaptör ──► DTO (01 şeması) ──► Senkron motoru ──► Portal veritabanı
 (Easytrade,        (kaynağa     (+ raw_payload)      (upsert, hash,       │
  Paraşüt, ...)      özel kod)                          mükerrer, dönem)    ▼
                                                                     Kontrol katmanı
                                                              (KDV, mükerrer, mutabakat)
                                                                            │
                                                                            ▼
                                                              Beyana hazır veri ──► Luca / EDM / QNB
```

Adaptör sadece okur ve dönüştürür. Veritabanına yazma, eşleştirme ve kontrol tek yerde, senkron motorunda yapılır.

Banka hesap hareketleri ayrı ve daha dar bir sözleşmeyle (`BankAdapter`) okunur, sabit çıkış IP'li ayrı worker havuzunda çalışır. Ayrıntı `20-banka-genel.md`.

## 2. Adaptör arayüzü

Dil bağımsız sözleşme (TypeScript ile gösterim). Varlık adları `01` ile birebir aynı:

```ts
type DateRange = { start: string; end: string }; // Y-m-d, iki uç dahil (Easytrade filters[...][start|end] kalıbı)

interface SourceAdapter {
  readonly sourceSystem: SourceSystem;    // 'easytrade' | 'parasut' | 'kolaybi' | ...
  readonly capabilities: Capability[];

  // Bağlantı
  connect(input: ConnectInput): Promise<ConnectionResult>;   // kimlik doğrulama + şirket/şube seçimi
  refreshAuth(conn: Connection): Promise<void>;
  healthCheck(conn: Connection): Promise<HealthStatus>;
  listCompanies?(conn: Connection): Promise<ExternalCompany[]>;
  listBranches?(conn: Connection): Promise<ExternalBranch[]>;

  // Referans veriler (Kalem 4 ve 9)
  fetchCustomers(conn: Connection, cursor?: Cursor): AsyncIterable<Batch<CustomerDTO>>;
  fetchCashBoxes(conn: Connection): AsyncIterable<Batch<CashBoxDTO>>;
  fetchExpenseCategories(conn: Connection): AsyncIterable<Batch<ExpenseCategoryDTO>>;

  // Hareket verileri (Kalem 1, 2, 3, 5)
  fetchOutgoingInvoices(conn: Connection, range: DateRange, cursor?: Cursor): AsyncIterable<Batch<InvoiceDTO>>;
  fetchIncomingInvoices(conn: Connection, range: DateRange, cursor?: Cursor): AsyncIterable<Batch<InvoiceDTO>>;
  fetchInboxInvoices?(conn: Connection, range: DateRange): AsyncIterable<Batch<InvoiceDTO>>; // işlenmemiş gelen e-fatura
  fetchFinancialTransactions(conn: Connection, range: DateRange, cursor?: Cursor): AsyncIterable<Batch<FinancialTransactionDTO>>;
  fetchCashBoxTransfers?(conn: Connection, range: DateRange): AsyncIterable<Batch<CashBoxTransferDTO>>;
  fetchExpenses(conn: Connection, range: DateRange, cursor?: Cursor): AsyncIterable<Batch<ExpenseDTO>>;

  // Kaynak veriyorsa (Kalem 6, 7, 8, 10)
  fetchRetailSales?(conn: Connection, range: DateRange): AsyncIterable<Batch<RetailSaleDTO>>;
  fetchRetailRefunds?(conn: Connection, range: DateRange): AsyncIterable<Batch<RetailRefundDTO>>;
  fetchPersonnelTransactions?(conn: Connection, range: DateRange): AsyncIterable<Batch<PersonnelTransactionDTO>>;
  fetchChecks?(conn: Connection): AsyncIterable<Batch<CheckDTO>>;
  fetchPromissoryNotes?(conn: Connection): AsyncIterable<Batch<PromissoryNoteDTO>>;
  fetchVatReport?(conn: Connection, period: string): Promise<VatReportDTO>;

  // Artımlı akış (kaynak destekliyorsa: Easytrade sync/pull, Logo DATE_MODIFIED)
  pullChanges?(conn: Connection, after?: string): AsyncIterable<ChangeBatch>;

  // Dosyalar
  fetchAttachment?(conn: Connection, ref: AttachmentRef): Promise<Binary>; // PDF, UBL XML, fiş görseli
}

type Capability =
  | 'outgoing_invoices' | 'incoming_invoices' | 'inbox_invoices' | 'invoice_items'
  | 'financial_transactions' | 'cash_box_transfers' | 'expense_categories' | 'expenses'
  | 'retail_sales' | 'personnel_transactions' | 'checks' | 'promissory_notes'
  | 'vat_report' | 'attachments' | 'ubl_xml' | 'incremental';

type Batch<T> = { items: T[]; nextCursor?: Cursor; raw: unknown[] };
```

Kurallar:
- DTO'lar **ortak modele normalize edilmiş** olur. Eşleme mantığı adaptörün içindedir. Eşleme kaynağı `03-alan-esleme-matrisi.md`.
- Her DTO `raw` alanında kaynağın ham kaydını taşır.
- Easytrade'in `{value, label}` enum'larından `value` alınır.
- Desteklenmeyen kalem için metot tanımlanmaz ve `capabilities` listesinde yer almaz. Senkron motoru sadece `capabilities` içindekileri çağırır.

## 3. Senkron motoru

### 3.1 İlk yükleme (backfill)

- Varsayılan pencere: **içinde bulunulan yıl + önceki yıl.** Müşavir isterse genişletir.
- Aylık dilimlerle çekilir. Bir dilim hata alırsa sadece o dilim tekrar denenir.
- Sıra: `expense_categories` → `cash_boxes` → `customers` → `outgoing` → `incoming` → `inbox` → `financial_transactions` → `cash_box_transfers` → `expenses` → `retail_sales` → `personnel_transactions` → `checks` → `vat_report`.

### 3.2 Artımlı senkron

1. **Kaynak artımlı akış veriyorsa** (`incremental` yeteneği: Easytrade `sync/pull`, Logo `DATE_MODIFIED`), birincil yöntem budur. Son konum `source_connection.sync_cursor`'da tutulur.
2. **Vermiyorsa** her çalışmada **açık dönemler** taranır: içinde bulunulan ay + önceki ay (KDV beyanı ayın 28'ine kadar verildiği için önceki ay da açıktır).
3. Her iki durumda da açık dönemlerin tam taraması **günde 1 kez** yapılır. Bu tarama silinen kayıtları yakalar.
4. `content_hash` değişmeyen kayıtlar yazılmaz.
5. Penceredeki bir kayıt kaynakta artık dönmüyorsa `deleted_at_source` damgalanır.

### 3.3 Zamanlama

| İş | Sıklık |
|---|---|
| Artımlı akış (destekleyen kaynakta) | 15 dakikada 1 |
| Açık dönem taraması | 4 saatte 1 (iş saatlerinde) + gece 1 |
| Referans veriler | Günde 1 |
| Müşavir "şimdi senkronla" butonu | Anlık, aynı bağlantı için en fazla 5 dakikada 1 |
| Dönem kapanışı öncesi (ayın 20–28'i) | 2 saatte 1 |
| KDV raporu mutabakatı (kaynak veriyorsa) | Her senkron sonrası |

### 3.4 Dönem kilidi

Müşavir dönemi kilitledikten sonra:
- O döneme ait kayıtta değişiklik gelirse kayıt **güncellenmez.**
- `change_request` tablosuna eski ve yeni hash ile fark özeti yazılır.
- Müşavir panelinde "kilitli dönemde değişiklik" uyarısı çıkar.

## 4. Kimlik bilgisi saklama

- Kimlik bilgileri (API anahtarı, OAuth token, kullanıcı/şifre) **asla** uygulama veritabanında açık tutulmaz. Gizli anahtar kasasında (KMS ile şifreli) saklanır, `source_connection.credentials_ref` ile referans verilir.
- Tercih sırası: **OAuth authorization_code > servis / API token > mükellefin ürettiği API anahtarı > kullanıcı adı/şifre.**
- Kullanıcı adı/şifre zorunluysa mükellefe **sadece entegrasyon için ayrı bir kullanıcı** açtırılır.
- Token yenileme adaptörün sorumluluğundadır. Yenileme başarısızsa bağlantı `auth_failed` olur ve mükellefe "bağlantıyı yenile" bildirimi gider.
- Loglarda token, şifre, API anahtarı **maskelenir.**

## 5. Hız sınırı ve hata yönetimi

| Durum | Davranış |
|---|---|
| 401 | 1 kez token yenile + tekrar. Yine olursa `auth_failed`, senkron durur. |
| 403 + lisans hatası (Easytrade `LICENSE_EXPIRED`) | Bağlantı `disabled`, tekrar yok, müşavire uyarı. |
| 403 diğer | `auth_failed`. |
| 404 (tek kayıt) | Kayıt silinmiş say, `deleted_at_source`. |
| 409 (`IDEMPOTENCY_MISMATCH`) | Yazma işleminde (Luca/entegratör çıkışı) anahtar çakışması. Tekrar yok, hata kaydı. |
| 422 / 400 | Tekrar yok. Ham istek ve yanıt saklanır. |
| 429 | `Retry-After` varsa ona uy, yoksa üstel geri çekilme (1, 2, 4, 8, 16 sn, en fazla 5 deneme). |
| 5xx, 502, zaman aşımı | Üstel geri çekilme, en fazla 5 deneme. Sonra dilim `failed`, bir sonraki çalışmada tekrar. |

- Her kaynak için **bağlantı başına token bucket** limitleyici. Değerler ilgili kaynak dosyasında. Belirsizse güvenli varsayılan: saniyede 1 istek.
- Aynı mükellef için aynı anda tek senkron işi çalışır (dağıtık kilit).

## 6. Kurulu programlar için bağlantı ajanı (Logo Tiger/GO, Mikro)

Bu programlar mükellefin kendi sunucusunda çalışır. İnternetten doğrudan erişilemez.

- **Ajan:** Mükellefin ağındaki bir Windows makinesine kurulan küçük servis (.NET 8 Worker Service önerilir).
- **Yön:** Ajan **dışarıya** bağlanır (HTTPS, 443). Mükellefin ağında port açılmaz.
- **Akış:** Ajan bulut tarafındaki iş kuyruğunu dinler (long-poll veya WebSocket). Gelen "şu dönemi çek" işini yerelde (Logo REST / Mikro API / SQL) çalıştırır. Sonucu sıkıştırıp bulut API'ye yükler.
- **Ajandaki adaptör:** Aynı `SourceAdapter` sözleşmesi, ama ajanın içinde çalışır. Bulut tarafı sadece DTO alır.
- **Kimlik:** Yerel kimlik bilgileri ajan makinesinde Windows DPAPI ile şifreli saklanır, buluta gönderilmez.
- **Kayıt:** Kurulumda müşavir panelinden üretilen tek kullanımlık eşleştirme kodu girilir.
- **Sağlık:** Ajan 5 dakikada bir heartbeat gönderir. 30 dakika sessizlikte müşavire "bağlantı koptu" uyarısı.
- **Güncelleme:** Ajan imzalı paketle kendini günceller.
- **SQL ile okuma gerekirse:** Ajana **salt okunur** bir SQL kullanıcısı tanımlanır. Ajan sadece önceden tanımlı, parametreli sorguları çalıştırır, serbest SQL çalıştırmaz.

## 7. Ek dosyalar (PDF / UBL XML / görsel)

- Kaynak geçici URL veriyorsa (Paraşüt PDF linki 1 saat, Easytrade `receipt_url`) dosya **hemen indirilir** ve nesne deposuna yazılır.
- UBL XML varsa (Easytrade `download-ubl`) PDF'ten önce indirilir. Vergi kırılımı için en güvenilir kaynak UBL'dir.
- Dosya boyutu sınırı 20 MB. SHA-256 ile mükerrer dosya saklanmaz.
- İndirme ayrı bir kuyrukta yapılır, fatura senkronunu bloklamaz.

## 8. Portalın kendi API'si: Easytrade kuralları

Müşavir paneli ve mükellef uygulaması portala Easytrade Muhasebe API ile **aynı kurallarla** konuşur. Backend ekibi yeni bir sözleşme öğrenmez.

| Konu | Kural |
|---|---|
| Yanıt zarfı | `{ "success", "message", "data", "meta" }` · hata: `{ "success": false, "message", "errors", "data": null, "error_code" }` |
| Durum kodları | `401`, `403`, `404`, `409`, `422`, `500` aynı anlamlarla |
| Sayfalama | `page`, `per_page` (en fazla 100), `sort`, `direction`, `search`, `filters[...]` · `meta`: `current_page`, `last_page`, `per_page`, `total` |
| Tarih filtresi | `filters[<alan>][start]`, `filters[<alan>][end]` (`Y-m-d`) |
| Bağlam | `X-Company-ID` = mükellef şirketi. Müşavir kullanıcısı birden fazla mükellefe erişir, istek hangi mükellef için ise o gönderilir. |
| Enum yanıtları | `{ "value", "label" }` |
| Yazma | `Idempotency-Key` başlığı (Luca aktarımı, dönem kilidi, masraf onayı gibi işlemler) |

Önerilen ana uçlar (adlandırma Easytrade kalıbında):

| Uç | İçerik |
|---|---|
| `GET /accountant/companies` | Müşavirin mükellefleri ve bağlantı durumları |
| `GET /accountant/companies/{company}/sources` | Mükellefin kaynak bağlantıları (`source_connection`) |
| `POST /accountant/companies/{company}/sources/{source}/sync` | Anlık senkron |
| `GET /accountant/invoices/sales`, `/purchases`, `/inbox` | Ortak modeldeki faturalar (`X-Company-ID` ile) |
| `GET /accountant/invoices/{invoice}` | Fatura + satırlar + `tax_summary` + kaynak bilgisi + uyarılar |
| `GET /accountant/financial-transactions`, `/cash-boxes`, `/expenses`, `/expense-categories`, `/retail-sales` | Diğer varlıklar |
| `GET /accountant/reports/vat?period=` | Portalın hesapladığı KDV özeti + kaynak KDV raporuyla fark |
| `GET /accountant/periods/{period}/readiness` | "Beyana hazır" kontrol listesi |
| `POST /accountant/periods/{period}/lock` | Dönem kilidi |
| `GET /accountant/warnings` | Kontrol katmanı uyarıları |

## 9. Gözlemlenebilirlik

Bağlantı başına şu metrikler tutulur ve müşavir panelinde özetlenir:
- Son başarılı senkron zamanı
- Dönem bazında çekilen belge sayısı ve toplam tutar
- Kaynak KDV raporu ile portal KDV özeti arasındaki fark (kaynak veriyorsa)
- Hata sayısı ve son hata mesajı
- `header_estimate` KDV kırılımı kullanılan belge sayısı (veri kalitesi göstergesi)

## 10. KVKK

- Easytrade bu akışta **veri işleyen** konumundadır. Müşavirle veri işleme sözleşmesi, mükelleften bağlantı onayı alınır. Onay metni ve zamanı `source_connection` üzerinde saklanır.
- Mükellef bağlantıyı istediği an kesebilir. Kesildiğinde token'lar silinir, çekilmiş veri müşavirin saklama yükümlülüğüne tabi olarak kalır.

## 11. Her adaptör için kabul testi listesi

Bir adaptör üretime alınmadan önce gerçek veya sandbox hesapta şu testler geçmeli:

1. Bağlantı kurma, token yenileme, yanlış kimlikte doğru hata.
2. 1 aylık satış faturası: sayı ve `amounts.payable_amount` toplamı kaynak ekranıyla birebir.
3. 1 aylık alış faturası: aynı kontrol.
4. Oran bazında KDV (`%1`, `%10`, `%20` ayrı ayrı `base` ve `vat`) kaynak KDV raporuyla birebir. Easytrade'de `reports/vat` ile otomatik.
5. İade faturası `type=IADE`, iptal faturası `status_normalized=cancelled` geliyor.
6. Tevkifatlı fatura: `withholding_code`, `withholding_rate` (yüzde) ve `withholding_amount` doğru.
7. Dövizli fatura: `currency`, `exchange_rate`, `amounts.payable_amount_try` doğru.
8. Kasa ve banka hareketleri: dönem sonu bakiyesi kaynakla aynı.
9. Faturaya bağlı tahsilat: `referenceable_*` / `allocations` doğru.
10. Kaynakta bir faturayı düzenle: sonraki senkronda `content_hash` değişiyor, kayıt güncelleniyor.
11. Kaynakta bir faturayı sil: sonraki senkronda `deleted_at_source` damgalanıyor.
12. Hız sınırına bilerek takıl: 429 sonrası iş kaybı olmadan devam ediyor.
13. Perakende satış (kaynak veriyorsa): faturaya dönüşmüş satış (`invoice_id` dolu) KDV toplamına **iki kez girmiyor.**
14. Masraf (kaynak veriyorsa): onaysız masraf beyan toplamına girmiyor.
