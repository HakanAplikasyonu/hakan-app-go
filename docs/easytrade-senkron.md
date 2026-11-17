# Easytrade senkronu (issue #385)

Easytrade bir kaynaktır (K1, `docs/00` §K1): portal Easytrade Muhasebe
API'sini salt okur. Uç ve alan kuralları `docs/kaynak/musavir-entegrasyon-paketi/09-easytrade.md`
dokümanındadır; bu doküman portal tarafındaki senkron sözleşmesini anlatır.
Kod: `backend/internal/modules/sources/easytrade`.

## 1. Zamanlanmış senkron

| | |
|---|---|
| Aralık | Mükellef başına `source_connections.sync_interval_minutes` (15–10080 dk). Yeni bağlantı 240 dk (09 §10 adım 5–8). `null` = yalnızca elle senkron. |
| Tetik | Worker zamanlayıcısı 5 dakikada bir `app:easytrade:sweep` çalıştırır; `next_sync_at` gelmiş, `active` / `rate_limited` bağlantıları bulur ve her biri için tek bir `app:easytrade:sync` görevi kuyruğa koyar (`asynq.Unique`, aynı bağlantıya ikinci görev girmez). |
| Okunan dönem | Açık dönemler: önceki ay ve içinde bulunulan ay (İstanbul takvimi), bugüne kadar. Kapanmış dönemler yalnızca elle senkronla yeniden okunur. |
| Pencere | Her ay × her kalem (`invoices`, `expenses`) bir `sync_windows` satırıdır (`pending` → `running` → `done` / `failed`). Başarısız pencere diğerlerini durdurmaz. |
| Çalışma kaydı | Her çalışma bir `sync_runs` satırıdır: `kind` = `incremental` (zamanlanmış) ya da `manual`; `status` = `success` / `partial` / `failed`; `counters` = içe aktarma raporu + `windows_done`, `windows_failed`. |
| Eşzamanlılık | Bağlantı başına Postgres advisory kilidi: zamanlanmış ve elle senkron üst üste binmez (`409 SYNC_RUNNING`). |

### Hata ve yeniden deneme

- Başarı: `next_sync_at = şimdi + aralık`, `sync_failures = 0`, `last_success_at` güncellenir.
- Kısmi ya da başarısız çalışma: `sync_failures + 1`; sonraki deneme 5, 10, 20, 40 … dakika sonra, hiçbir zaman aralıktan geç değil.
- Yeniden denemeyi kuyruk değil zamanlama yapar (görev `MaxRetry(0)`): deneme sayısı ve sıradaki zaman ekranda görünür.
- `401` (yenileme de başarısızsa) → `auth_failed`; `403 LICENSE_EXPIRED` → `disabled`. İkisinde de bağlantı taramadan düşer; müşavir bağlantıyı yeniden kurar.
- `429`, `5xx`, ağ hatası → pencere `failed`, zamanlama yeniden dener.

### API

| Uç | Amaç |
|---|---|
| `GET …/easytrade-connections` | Bağlantılar: `last_sync_at`, `last_success_at`, `next_sync_at`, `sync_interval_minutes`, `sync_failures`, `last_run` |
| `PATCH …/easytrade-connections/{uuid}` | `{"sync_interval_minutes": 60}` aç / değiştir, `null` kapat |
| `GET …/easytrade-connections/{uuid}/sync-runs` | Son 20 çalışma |
| `POST …/easytrade-connections/{uuid}/sync` | Elle senkron (`from`, `to`), 5 dakikada bir |

Kimlik bilgisi (30 günlük Sanctum token'ı) sır kasasındadır; anahtar
halkası (`SECRETS_KEYRING` / `ENCRYPTION_KEY`) geçersizse kasa kapalıdır,
tarama hiçbir bağlantıyı kuyruğa koymaz. Taban adres `EASYTRADE_API_BASE_URL`.

## 2. Webhook (push) — Easytrade tarafı hazır olduğunda

Easytrade bugün dışarıya bildirim göndermiyor; portal alıcıyı hazır tutar
(`POST /v1/webhooks/easytrade`). `EASYTRADE_WEBHOOK_SECRET` boşken alıcı
kapalıdır (`404`) ve tek tetik zamanlanmış senkrondur (polling).

| | |
|---|---|
| İmza | `X-Easytrade-Signature: sha256=<hex(HMAC-SHA256(secret, "{timestamp}.{gövde}"))>` |
| Zaman | `X-Easytrade-Timestamp`: Unix saniye, ±5 dakika (tekrar oynatma penceresi) |
| Gövde | `{"event_id", "event", "company_id", "resource", "resource_id", "occurred_at"}`; yalnızca `company_id` zorunlu |
| Etki | `company_id`'ye bağlı her etkin bağlantının `next_sync_at`'i şimdiye çekilir ve senkronu kuyruğa girer (bağlantı başına tek görev). Veri webhook'la taşınmaz; senkron normal API'den okur. |
| Yanıt | `202 {"accepted": n}`; bilinmeyen şirket de `202` ve `0` (şirket varlığı sızmaz). İmza/zaman hatası `401`. |

Örnek imza (kabuk):

```sh
ts=$(date +%s); body='{"event":"invoice.created","company_id":"42","resource":"invoice","resource_id":"7"}'
sig=$(printf '%s.%s' "$ts" "$body" | openssl dgst -sha256 -hmac "$EASYTRADE_WEBHOOK_SECRET" -hex | sed 's/^.* //')
curl -X POST "$API/v1/webhooks/easytrade" -H "Content-Type: application/json" \
  -H "X-Easytrade-Timestamp: $ts" -H "X-Easytrade-Signature: sha256=$sig" -d "$body"
```

## 3. Okunan kaynaklar ve yazıldıkları modüller

| Kalem (`sync_windows.item`) | Easytrade ucu | Portal | Not |
|---|---|---|---|
| `invoices` | `invoices/sales`, `invoices/purchases` + detay | `invoices` | 09 §3 |
| `expenses` | `expenses` + detay | `expenses` | Detaylı: `payment_method` (`payment_method_type` / `financial_transactions[]`), `receipt_number`, satıcı VKN/TCKN (`customer.tax_number`), plaka. Şablon (`is_main_expense`) ve reddedilen masraf yazılmaz. |
| `financial_transactions` | `financial-transactions` + detay | `source_financial_transactions` (yeni) | Tahsilat, ödeme, gelir/gider, perakende satış ödemesi. `payment_method_type`, `transaction_type`, `transaction_flow` Easytrade değerleriyle; `direction` (in/out) türetilir. Kişi/cari (`customer`), kasa/banka (`cash_box`). `referenceable_type = Invoice` → portal faturasına `invoice_id` bağlantısı (eşleştirme girdisi). İptal edilen yazılmaz. |
| `z_reports` | **Önerilen** `z-reports` + detay | `retail_sales` (`source_id = z:<id>`) | Easytrade'de yok; uç 404 dönerken kalem `unsupported` sayılır, çalışma başarısız olmaz. Ödeme ve KDV kırılımı ham veride. |

Ödeme türü değerleri (Easytrade `payment_method_type`): `cash` (nakit),
`bank_transfer` (havale/EFT), `credit_card` (kart/POS), `check` (çek),
`promissory_note` (senet), `credit` (veresiye); tanınmayan değer `unknown`
olur, ham değer korunur.

Dönem görünümleri:

- `GET …/easytrade-connections/{uuid}/financial-transactions?period=YYYY-MM`: satırlar, ödeme türü × yön toplamı, cari bazında giriş/çıkış, faturaya bağlanan referans sayısı.
- `GET …/easytrade-connections/{uuid}/z-reports?period=YYYY-MM`: Z raporları, dönem toplamı, ödeme türü dağılımı, `supported`.

Banka/kasa: Easytrade'in kasa/banka hareketleri `source_financial_transactions`
içinde `cash_box_*` alanlarıyla tutulur. `bank_transactions` banka kanalına
(`source_system = bank_*`) ayrılmış olduğu için oraya yazılmaz; banka
eşleştirmesi bu tabloyu ikinci girdi olarak okuyabilir (banka şeridi).

## 4. Easytrade'den istenecekler

Easytrade API'sinde bugün olmayan ya da belgelenmemiş, tam uyum için
gereken uçlar ve bilgiler. Terimler Easytrade'in kendi terimleridir.
`09` §9'daki 14 soru da geçerlidir; aşağıdakiler #385 kapsamında eklenenlerdir.

| # | İstek | Portal tarafı |
|---|---|---|
| 1 | **Webhook (giden bildirim):** şirkette `invoice`, `expense`, `financial_transaction`, `retail_sale`, `z_report` oluştuğunda/değiştiğinde/silindiğinde `POST {portal}/v1/webhooks/easytrade`; gövde `{event_id, event, company_id, resource, resource_id, occurred_at}`, imza `X-Easytrade-Signature: sha256=HMAC(secret, "{X-Easytrade-Timestamp}.{gövde}")`. Paylaşılan sırrın şirket ya da entegrasyon bazında üretilmesi. | Hazır (§2), sır boşken kapalı |
| 2 | **Z raporu ucu:** `GET /company/z-reports` (sayfalı, `sort=report_date`, şube bağlamı) ve `GET /company/z-reports/{id}`: `report_date`, `z_report_number`, `okc_serial_number`, `subtotal_amount`, `tax_amount`, `total_amount`, `currency`, `payments` (`payment_method_type` → tutar), `vat_breakdown[]` (`tax_rate`, `taxable_amount`, `tax_amount`). ÖKC/Paygo'dan gelen mali Z; `eod-sessions` kasa kapanışıdır, Z değildir. | Arabirim + fake ile test edildi (`ZReportSource`) |
| 3 | `GET /company/financial-transactions` için tarih filtresi (`filters[transaction_date][start|end]`) ve liste elemanında `transaction_date` | Bugün tarih sıralı tarama ile pencere kesiliyor |
| 4 | `financial_transaction.payment_method_type` değer listesi (havale ve EFT ayrımı var mı), `transaction_type` / `transaction_flow` anlamları (para girişi / çıkışı) | Yön türetmesi tahmine dayalı (`direction`) |
| 5 | `financial_transaction.customer` (ad, VKN/TCKN) ve `cash_box` (ad, `type`) detayda gömülü mü, yoksa yalnızca `customer_id` / `cash_box_id` mi | Gömülü değilse `customers` / `cash-boxes` ayrıca okunmalı |
| 6 | `referenceable_type` değerleri (`App\Models\Invoice`, `Sale` …) ve bir tahsilatın birden çok faturayı kapattığı durumda `allocations[]` | Fatura eşleştirmesi bugün tek referansla |
| 7 | `expense` detayında ödeme türü (`payment_method_type`), fiş no (`receipt_number`), satıcı VKN'si; `GET /company/expenses/{id}/receipt` dosyasının kalıcı adresi | Ödeme türü ve fiş no alınıyor; fiş görseli henüz indirilmiyor |
| 8 | Silinen kayıtların bildirimi (webhook `*.deleted` ya da `sync/pull` içinde) | `deleted_at_source` tespiti yok |
| 9 | Şirket API token'ı (`api_token` + `api_secret`) ile kimlik doğrulama yöntemi; kullanıcı şifresi istemeden bağlantı | Bugün kullanıcı girişi + 30 günlük token |
| 10 | Hız sınırı (istek/dakika) ve `429` yanıtında `Retry-After` | Zamanlama 5 dk'dan başlayan geri çekilme ile deniyor |
