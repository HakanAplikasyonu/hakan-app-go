# Sorgu performansı (issue #182)

Liste uçlarının nasıl ölçüldüğü, neyin düzeltildiği ve yük testinin nasıl çalıştırılacağı.

## 1. Veri seti ve ölçüm

- **Veri:** `make perf-seed` (`cmd/perf-seed`) bir ofis (`perf-smmm`), 500 mükellef ve her birine etkin bir engagement, 10.000 fatura ve 500 açık uyarı kurar.
  - Faturaların %30'u (3.000) ilk mükellefe aittir; böylece tek bir listenin derin sayfaları da ölçülür.
  - Kimlikler uydurmadır (VKN `98…`, `*.perf.test`). Komut `APP_ENV=production` ile çalışmaz.
- **Ortam:** yerel, tek makine. PostgreSQL 16, sunucu `cmd/server`, ofis sahibi oturumu.
- **Ölçüm araçları:**
  - Sorgu düzeyi: `EXPLAIN (ANALYZE, BUFFERS)`.
  - HTTP düzeyi: 10 eşzamanlı istemci, senaryo başına 500 istek.

> HTTP ölçümleri bu PR'ı hazırlayan ortamda k6 yerine küçük bir Go istemcisiyle alındı; k6 o ortamda çalıştırılamadı. Senaryolar aynıdır. k6 betiği (`scripts/perf/k6-invoices.js`) staging'de çalıştırılıp sonuçlar bu tabloya eklenmelidir.

## 2. Bulgular (EXPLAIN)

En ağır uç ofis fatura listesiydi: `GET /v1/tenant/engagements/{uuid}/invoices`, 3.000 faturalı mükellef.

1. **Görünürlük fonksiyonu satır başına çalışıyordu.**
   - `engagement_document_visible(engagement, issue_date)` bir `SECURITY DEFINER` SQL fonksiyonu olduğu için satır içine açılamaz.
   - Listede sıralamadan önce 3.000 satırın hepsinde çalışıyordu (~21 ms). Sayımda (`CountEngagementInvoices`) da aynısı oluyordu.
2. **Sıralamaya uyan indeks yoktu.** `(taxpayer_id)` indeksiyle bütün satırlar okunup sıralanıyor, `LIMIT 50` erken duramıyordu.
3. **OFFSET'te atlanan satırlar için de uyarı alt sorgusu çalışıyordu.** Satır başına açık uyarı sayımı `OFFSET 2900`'de 2.950 kez çalışıyordu (N+1 deseni).
4. **Genel plan sorunu vardı.**
   - pgx her sorguyu hazırlar (prepared statement). Beşinci çalıştırmadan sonra Postgres genel plana geçebilir.
   - Genel plan `LIMIT` değerini ve isteğe bağlı süzgeçlerin (`sqlc.narg`) dolu olup olmadığını bilmez.
   - Bu yüzden indeks yerine tam tarama ve sıralama seçiyordu. `EXPLAIN` 3,5 ms derken sunucuda istek 75 ms sürüyordu.

## 3. Düzeltmeler

| Değişiklik | Nerede |
|---|---|
| `idx_invoices_taxpayer_issue (taxpayer_id, issue_date DESC, id DESC)` | migration `000075` |
| İmleçli (keyset) sayfalama: `?cursor=`, yanıtta `next_cursor` | ofis ve mükellef fatura listeleri |
| Sayfa önce yalnızca kimliklerle seçilir; uyarı sayısı yalnızca dönen 50 satır için hesaplanır | `ListEngagementInvoices` |
| Sayım görünürlüğü her farklı tarih için bir kez sorar; `MATERIALIZED` CTE koşulun yeniden satır taramasına itilmesini engeller | `CountEngagementInvoices` |
| `plan_cache_mode=force_custom_plan` (oturum parametresi, `DB_PLAN_CACHE_MODE`) | `database.NewPostgresPool` |

İmleç hakkında:

- İmleç son satırın `(issue_date, id)` değeridir, base64 ile kodlanır.
- İstemci için opaktır ve kişisel veri taşımaz.
- İmleçle `offset` yok sayılır.
- Satır karşılaştırması `(issue_date, id) < (…)` indekste sınır koşuludur (`Index Cond`), bu yüzden derin sayfa ilk sayfa kadar ucuzdur.
- `offset` geriye uyumluluk için kaldı ama derin sayfalar için önerilmez.

## 4. Sonuçlar

**Sorgu (EXPLAIN ANALYZE, ofis listesi, 3.000 faturalı mükellef):**

| | Önce | Sonra |
|---|---|---|
| İlk sayfa (50) | 24,2 ms | 3,5 ms |
| `OFFSET 2900` | 137,4 ms | 22,7 ms |
| İmleç, ~2.900. satırdan sonra | — | 1,5 ms |
| Sayım | 25,9 ms | 2,9 ms |

Mükellef paneli listesinde görünürlük fonksiyonu yok:

| | Önce | Sonra |
|---|---|---|
| İlk sayfa | 2,7 ms | 0,19 ms |
| İmleç | — | 0,24 ms |

**HTTP (10 eşzamanlı istemci, 500 istek, p50 / p95):**

| Senaryo | Önce (main) | Sonra |
|---|---|---|
| Ofis listesi, ilk sayfa | 128 / 159 ms | 18 / 28 ms |
| Ofis listesi, `offset=2900` | 445 / 531 ms | 74 / 92 ms |
| Ofis listesi, imleç ~2.900 | — | 19 / 31 ms |
| Dönem matrisi (`/periods/matrix?months=6`, 500 mükellef) | 4,3 / 8,6 ms | 5,2 / 9,1 ms |

Saniyedeki istek sayısı ilk sayfada 78'den 544'e çıktı. Hiçbir senaryoda hata yok.

**Diğer ofis listeleri:** aynı veri setinde (500 mükellef) tek istek 1–4 ms sürdü. Bunlar engagement listesi, uyarılar, inceleme kuyruğu ve dönem matrisidir. Bunlarda değişiklik gerekmedi.

## 5. Yük testini çalıştırma

```bash
make perf-seed                 # ya da: make perf-seed ARGS=-reset
make backend-dev               # başka bir terminalde
make perf-load                 # k6 run scripts/perf/k6-invoices.js
```

**Ortam değişkenleri:**

| Değişken | Varsayılan |
|---|---|
| `BASE_URL` | `http://127.0.0.1:8080` |
| `PERF_EMAIL` | `perf-owner@perf.test` |
| `PERF_PASSWORD` | `Perf1234!` |
| `PERF_ORG` | `perf-smmm` |
| `PERF_ENGAGEMENT` | boşsa matristen ilk mükellef seçilir |
| `VUS` | 10 |
| `DURATION` | `1m` |

**Senaryo:** her sanal kullanıcı ilk sayfayı alır, imleçle 10 sayfa ilerler, ardından dönem matrisini ister.

**Eşikler:** p95 < 300 ms ve hata oranı < %1.

## 6. Açık kalanlar

- **Tablo büyüklüğü:** tam tablo taraması kalan sorgular, küçük tablolarda planlayıcının bilerek seçtiği sıralı taramalardır (ör. uyarı alt sorgusu 500 uyarıda `Seq Scan`). Tablolar büyüyünce aynı sorgular var olan `uq_data_warnings_open` indeksine geçer. 100 bin fatura ve üstü ölçülmedi.
- **Frontend:** ofis fatura ekranı hâlâ `offset` ile sayfalıyor. "Daha fazla" ya da sonsuz kaydırmada `next_cursor` kullanılmalı.
- **Plan önbelleği:** `plan_cache_mode` tüm uygulama sorgularını etkiler; her sorgu yeniden planlanır. Ölçülen ek planlama süresi ~0,7 ms, kazanç ise genel planın yanlış seçtiği listelerde 4–7 kat. Sorun çıkarsa `DB_PLAN_CACHE_MODE=auto` eski davranışa döner.
