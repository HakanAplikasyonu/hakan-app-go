# hakan-app-go

Mali müşavirlik ofisleri ve mükellefleri için veri toplama, kontrol, inceleme ve muhasebe programına aktarım platformu. "HAKAN" geçici geliştirme adıdır.

Durum: **iskelet.** Boilerplate tenant çekirdeği çalışıyor. Ürüne özgü kod sonraki issue'larda eklenecek.

## Kaynak

İskelet [`furkanmeclis/nextjs-go-boilerplate`](https://github.com/furkanmeclis/nextjs-go-boilerplate) reposundan türetildi. Git geçmişi taşınmadı.

| | |
|---|---|
| Kaynak commit | `fdd52e4011cfa70cf320c1ca33ead0f9d8d94373` (main, 11.11.2026) |
| Alınanlar | `backend/`, `frontend/`, `compose.local.yml`, `compose.prod.yml`, `deploy/`, `scripts/`, `Makefile`, `.github/workflows/ci.yml`, `.env.example`, `.env.server.example`, `.gitignore` |
| Go modülü | `github.com/HakanAplikasyonu/hakan-app-go/backend` (D12) |

Boilerplate'ten farklar:

- `TENANCY_ENABLED=true`. Her organizasyon bir mali müşavirlik ofisidir ve `/t/{slug}` altında açılır.
- `TENANCY_SELF_SERVE=false` (D14). Ofisleri yalnızca platform yöneticisi açar.
- `NAME_PREFIX=hakan`. Konteyner, volume ve ağ adları `hakan-*` olur.
- CI'dan `auto-pr` işi çıkarıldı (D20). Frontend işine `pnpm build`, backend işine ayrı bir `tenant_tables lint` adımı eklendi.

## Klasörler

| Klasör | İçerik |
|---|---|
| `backend/` | Go API ve Asynq worker (`:8080`, `/v1/...`) |
| `frontend/` | Next.js: platform paneli `/platform`, ofis paneli `/t/{slug}` |
| `mobile/` | Yer tutucu: Expo mükellef uygulaması |
| `desktop/` | Yer tutucu: Tauri v2 ve Go sidecar |
| `deploy/`, `compose.*.yml`, `scripts/` | Yerel altyapı, üretim ortamı ve env şeması |
| `docs/` | Planlama belgeleri ve kararlar |

## Gereksinimler

- Go 1.26 (`backend/go.mod`; `GOTOOLCHAIN=auto` doğru sürümü indirir)
- Node 22 ve pnpm
- Docker ve Docker Compose
- `golang-migrate` (postgres etiketiyle), `air`, `sqlc`, `golangci-lint`. Ayrıntı: `backend/TOOLCHAIN.md`.

## Yerelde çalıştırma

```bash
cp .env.example .env
```

```bash
make local-dev
```

Bu komut Docker'da Postgres, Redis, SeaweedFS, MailHog, Adminer, Centrifugo ve Meilisearch'ü başlatır. Ardından migration'ları uygular, backend'i `air` ile ve frontend'i `pnpm dev` ile host üzerinde çalıştırır. Worker yerelde API sürecinin içinde çalışır (`QUEUE_WORKER_INPROCESS=true`).

İlk platform yöneticisini başka bir terminalde oluştur:

```bash
make create-super-admin
```

Varsayılan hesap `admin@example.com` / `Password1`. Değiştirmek için:

```bash
make create-super-admin SA_EMAIL=sen@example.com SA_PASSWORD='Gizli1' SA_NAME=Ad SA_SURNAME=Soyad
```

### Adresler

| | |
|---|---|
| Platform girişi | http://localhost:3000/platform/login |
| Ofis paneli | http://localhost:3000/t/{slug} |
| API ve Scalar dokümanı | http://127.0.0.1:8080 · `/docs/` |
| Sağlık | `/healthz` · `/readyz` |
| MailHog | http://127.0.0.1:8025 |
| Adminer | http://127.0.0.1:8081 |

### İlk ofisi açmak

1. `/platform/login` adresinden süper admin ile gir.
2. **Organizasyonlar → Yeni** ile ofisin adını ve kısa adını (`slug`) gir. Sahip seçilmezse ofisi açan kullanıcı sahip olur.
3. `/t/{slug}` adresinde ofis paneli açılır.

## Kontroller

```bash
make -C backend test lint
```

```bash
cd frontend && pnpm typecheck && pnpm lint && pnpm build
```

```bash
make env-check
```

CI (`.github/workflows/ci.yml`) her push'ta değişen klasörlere göre bu kontrolleri çalıştırır. Tek zorunlu kontrol `ci` işidir.

## Belgeler

| Belge | İçerik |
|---|---|
| [docs/00-degerlendirme-ve-kararlar.md](docs/00-degerlendirme-ve-kararlar.md) | Entegrasyon paketinin eleştirel değerlendirmesi, ilk kararlar |
| [docs/01-cekirdek-hiyerarsi-ve-acik-kararlar.md](docs/01-cekirdek-hiyerarsi-ve-acik-kararlar.md) | Çekirdek (boilerplate, go-ubltr), hiyerarşi modeli, Basis ve rakip analizi, **güncel kararlar (§0)** |
| [docs/02-muhasebe-cikti-surucu-modeli.md](docs/02-muhasebe-cikti-surucu-modeli.md) | Luca ve formüllü Excel çıktısı için sürücü modeli |
| [docs/brand/](docs/brand/) | Logo |
| [docs/kaynak/](docs/kaynak/) | Kaynak talepler, görüşme transkripti ve entegrasyon paketi |
| `backend/docs/`, `frontend/README.md` | Boilerplate'ten gelen teknik belgeler |

## Yığın

- Backend: Go, boilerplate tenant çekirdeği
- UBL-TR: [`go-ubltr`](https://github.com/furkanmeclis/go-ubltr)
- Web: Next.js
- Mobil: Expo
- Masaüstü: Tauri v2 + Go sidecar

İş takibi: GitHub Projects (organizasyon projesi).
