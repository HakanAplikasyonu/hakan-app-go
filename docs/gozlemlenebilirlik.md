# Gözlemlenebilirlik (issue #179)

Backend'in (API ve worker) log, metrik, iz ve hata raporlama düzeni. Bu belge kodun gerçekte ne yaptığını anlatır.

## 1. Log

- **Biçim:** `log/slog` ile JSON, stdout'a (`LOG_FORMAT=json`, varsayılan). `LOG_FORMAT=text` yalnızca yerel geliştirme içindir.
- **Bağlam:** istek bağlamıyla yazılan her kayda kendiliğinden iki alan eklenir:
  - `request_id`: `X-Request-ID` başlığından gelir, yoksa üretilir.
  - `organization_id`: ofisin iç kimliğidir, kişisel veri değildir.
- **Kişisel veri:** hiçbir hedefe gitmez. Hedefler stdout, `app_logs` tablosu ve hata izleyicidir. Hepsinden önce `logging.Redact` çalışır:
  - Mesaj ve her metin, hata ya da yapılandırılmış alan `observability.ScrubString`'den geçer. Temizlenenler:
    - e-posta,
    - telefon,
    - Bearer ya da JWT belirteci,
    - `password=` ve benzeri çiftler,
    - IBAN,
    - **VKN**: tek başına duran 10 hane,
    - **TCKN**: tek başına duran, 0 ile başlamayan 11 hane.
  - Harf ya da rakamla bitişik haneler maskelenmez. Böylece UUID ve iz kimlikleri bozulmaz. Daha uzun numaralar (fatura no) olduğu gibi kalır.
  - Anahtarı hassas görünen alanların değeri `[redacted]` olur. Bu anahtarlar: `password`, `token`, `email`, `phone`, `vkn`, `tckn`, `iban`, `tax_id`, `tax_number` ….
  - Sayı, mantıksal değer, zaman ve süre alanları değişmez.
- **Test:** `internal/logging/redact_test.go` ve `internal/observability/observability_test.go`.

## 2. Metrikler (Prometheus)

Metrikler API'den ayrı bir portta yayınlanır, böylece dış vekilden erişilemez:

| Süreç | Değişken | Varsayılan |
|---|---|---|
| API | `METRICS_ADDR` | `:9091` |
| Worker | `WORKER_METRICS_ADDR` | `:9092` |

- `METRICS_TOKEN` doluysa `Authorization: Bearer <token>` zorunludur. Boş değer dinleyiciyi kapatır.
- `compose.prod.yml` portları dışarı açmaz. Prometheus aynı ağdan `backend:9091` ve `worker:9092` adreslerini toplar.

| Metrik | Etiketler | Kaynak |
|---|---|---|
| `http_requests_total` | `method`, `route`, `status` | API |
| `http_request_duration_seconds` (histogram) | `method`, `route` | API |
| `entegrator_requests_total` | `provider`, `operation`, `outcome` | Entegratör istemcisi |
| `entegrator_request_duration_seconds` (histogram) | `provider`, `operation` | Entegratör istemcisi |
| `queue_tasks` | `queue`, `state` (pending, active, scheduled, retry, archived) | Worker |
| `queue_latency_seconds` | `queue` | Worker (en eski bekleyen iş) |
| `queue_inspect_up` | `queue` | Worker |
| `go_*`, `process_*` | | İkisi de |

- **`route`:** rota kalıbıdır, ör. `/v1/tenant/invoices/{uuid}`. Ham yol hiçbir zaman etiket olmaz. Eşleşmeyen istekler `unmatched` olarak sayılır.
- **`outcome`:** `ok` ya da sağlayıcının hata türüdür (`auth`, `rate-limit`, `unavailable` …). Ayrıca `timeout`, `canceled` ve `error` değerleri vardır.

Örnek uyarılar:

```promql
# Entegratör hata oranı, sağlayıcı başına (auth hataları kimlik bilgisi sorunudur)
sum by (provider) (rate(entegrator_requests_total{outcome!="ok"}[15m]))
  / sum by (provider) (rate(entegrator_requests_total[15m])) > 0.2

# API p95 gecikmesi
histogram_quantile(0.95, sum by (le, route) (rate(http_request_duration_seconds_bucket[5m]))) > 2

# Kuyruk birikmesi
max by (queue) (queue_latency_seconds) > 600
```

## 3. İzleme (OpenTelemetry)

- **Açma:** `OTEL_EXPORTER_OTLP_ENDPOINT` (ya da `..._TRACES_ENDPOINT`) boşsa izleme kapalıdır ve maliyeti yoktur. Doluysa OTLP/HTTP ile gönderilir.
- **Ayarlar:** başlık ve protokol gibi diğer ayarları exporter standart `OTEL_EXPORTER_OTLP_*` değişkenlerinden okur.
- **Servis adı:** `OTEL_SERVICE_NAME` (varsayılan `APP_NAME`) ve ardından `-api` ya da `-worker`.
- **Örnekleme:** üst iz tabanlı oran, `OTEL_TRACES_SAMPLER_ARG` (varsayılan 0.1). Gelen `traceparent` başlığı sürdürülür.
- **Span içeriği:** yalnızca method, rota kalıbı ve durum kodu. URL, sorgu dizesi, başlık ve gövde kaydedilmez. `/health*` izlenmez.
- **Test:** `internal/tracing/tracing_test.go`.

Sentry'nin kendi performans izlemesi (`SENTRY_TRACES_SAMPLE_RATE`) ayrıdır ve değişmedi.

## 4. Hata raporlama

- **Çağrı yerleri:** yalnızca `observability.CaptureError` ve `CaptureMessage` kullanılır.
- **Arka uç:** `observability.Reporter` arayüzüdür. Varsayılanı Sentry protokolüdür (`SENTRY_DSN`; GlitchTip gibi kendi barındırılan izleyiciler de olur).
- **Başka arka uç:** `observability.AddReporter` ile eklenir. Eklenen arka uca giden hata metni, mesaj, etiket ve ek alanlar önceden temizlenir. Yani yeni bir arka uç, yerleşik olanın göndermeyeceği bir veriyi alamaz.
- **Kapanış:** `FlushAll`.
