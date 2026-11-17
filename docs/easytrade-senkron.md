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
