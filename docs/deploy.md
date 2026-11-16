# Üretim deploy hattı (#177)

HAKAN'ın sunucuya nasıl çıktığı: imajlar CI'da derlenir, sunucu derlemez,
yalnızca çeker. Migration her deploy'da tek seferlik bir iş olarak çalışır.
Gerçek sunucu adresi ya da kimlik bilgisi repoda yoktur; hepsi sunucudaki
`.env.server` dosyasındadır (`deploy/env/schema`'dan üretilir).

## Parçalar

| Parça | Dosya | Not |
|---|---|---|
| Backend imajı | `backend/Dockerfile` | Çok aşamalı: Go derleme → alpine. İçinde `server`, `worker`, `migrate` CLI, `release-record`, `create-super-admin`, `secrets-rotate`, `field-crypt` ve `migrations/`. Kullanıcı `appuser` (10001). Rol `APP_ROLE` ile seçilir (`deploy/docker/entrypoint.sh`). |
| Web imajı | `frontend/Dockerfile` | Çok aşamalı Next.js `standalone` çıktısı. |
| Yedek imajı | `deploy/backup/Dockerfile` | Zamanlanmış şifreli yedek ve geri yükleme: `docs/yedekleme.md`. |
| İmaj listesi | `deploy/images.json` | CI bu listeyi matris olarak gezer. |
| Uygulama yığını | `compose.prod.yml` | Postgres, Redis, SeaweedFS (S3), Meilisearch, Centrifugo, `migrate`, backend, worker, isteğe bağlı `worker-egress`, frontend. Dokploy bu dosyayı kullanır. |
| Düz Docker sunucusu | `deploy/compose.prod.yml` | Yukarıdaki yığın ile Caddy ters vekil (`deploy/proxy.yml`, `deploy/caddy/Caddyfile`). |
| Ortam şeması | `deploy/env/schema` | `scripts/prod-env.sh init/upgrade/check` (betiğin başındaki açıklama). |

## Sağlık uçları

| Uç | Ne söyler | Kim kullanır |
|---|---|---|
| backend `GET /healthz` | Süreç ayakta | compose healthcheck (Postgres/Redis kesintisi API'yi düşürmesin diye) |
| backend `GET /readyz` | Postgres ve Redis'e erişiyor | Deploy sonrası kontrol, duman testi |
| frontend `GET /health` | Next.js sunucusu ayakta | compose healthcheck, Traefik/Caddy |
| centrifugo `healthcheck` | WebSocket sunucusu ayakta | compose healthcheck |

## Migration adımı

`migrate` servisi backend imajıyla, `APP_ROLE=migrate` olarak bir kez çalışır.
Önce `migrate up`, ardından bu sürümün `app_releases` kaydını yazar ve çıkar.
Backend ve worker `depends_on: migrate: service_completed_successfully` ile
bekler; migration başarısız olursa yeni sürüm başlamaz, eski konteynerler
çalışmaya devam eder. Uygulama konteynerlerinde `AUTO_MIGRATE=false` olduğundan
migration'ı yalnızca bu iş çalıştırır.

## CI

`.github/workflows/ci.yml`:

- `image-build`: altyapı değişikliğinde (Dockerfile, `compose.prod.yml`,
  `deploy/**`, migration, bağımlılık dosyaları), haftalık ve elle tetiklemede
  her imajı derler. Push etmez.
- `prod-smoke`: aynı koşulda backend, frontend ve yedek imajlarını yerelde derler. Ardından
  `scripts/prod-smoke.sh` boş runner'da `deploy/compose.prod.yml`'ı tek
  kullanımlık sırlarla ayağa kaldırır ve şunları kontrol eder: migration son
  sürümde ve kirli değil, backend `/readyz`, Caddy → frontend `/health`, Caddy
  → BFF → API `/v1/app/config`, bir şifreli yedek çalıştırması (`backup-run`). Sonunda her şeyi (volume'lar dahil) siler.
- `publish-images`: yalnızca `main` push'unda, testler yeşilse imajları
  `ghcr.io/<owner>/<repo>-<service>` olarak `:main`, `:sha-<kısa>` ve `:v0.N`
  etiketleriyle iter. Kimlik `secrets.GITHUB_TOKEN`; başka sır yok.
- `release-tag`: imajlar yayımlanınca `v0.N` etiketini keser. Dokploy bu
  etiketle deploy eder.

Yerelde aynı duman testi (Docker ve Compose 2.20+ gerekir):

```bash
docker build -t local/hakan-backend:main backend
```

```bash
docker build -t local/hakan-frontend:main frontend
```

```bash
docker build -t local/hakan-backup:main deploy/backup
```

```bash
scripts/prod-smoke.sh
```

## Deploy: Dokploy

1. Sunucuda `.env.server`'ı üret: `scripts/prod-env.sh init` (APP_URL ve
   IMAGE_REPO sorulur, sırlar üretilir). Şema değişince `scripts/prod-env.sh upgrade`.
2. Dokploy'da compose uygulaması: dosya `compose.prod.yml`, ortam `.env.server`
   içeriği. İmajlar özel paketse `ghcr.io` için `read:packages` yetkili bir
   kayıt ekle.
3. Alan adları: `frontend` (3000) ve `centrifugo` (8000). Traefik
   `dokploy-network` üzerinden yönlendirir.
4. Dokploy `v0.*` etiketlerinde deploy eder. Sıra: imaj çekilir → `migrate` →
   backend/worker → frontend.

## Deploy: düz Docker sunucusu (Dokploy yok)

```bash
scripts/prod-env.sh init
```

```bash
docker compose -f deploy/compose.prod.yml up -d
```

- `.env.server` depo kökünde olmalı; `deploy/compose.prod.yml` onu ve
  `deploy/plain-host.env` dosyasını okur. `plain-host.env`, Dokploy'un ağını
  dışarıdan beklemek yerine Compose'un oluşturmasını sağlar.
- Caddy 80 ve 443'ü dinler. `PROXY_APP_SITE` (varsayılan `APP_URL`) frontend'e,
  `PROXY_REALTIME_SITE` Centrifugo'ya gider. `https://` adresleri için Let's
  Encrypt sertifikası otomatik alınır; DNS kayıtları sunucuyu göstermeli.
  Sertifikalar `<NAME_PREFIX>-prod-caddy-data` volume'unda kalır.
- `CENTRIFUGO_WS_URL`, `PROXY_REALTIME_SITE` ile aynı alan adını göstermeli
  (`wss://realtime.<alan>/connection/websocket`).
- Bir sunucuda birden fazla kurulum varsa her biri için `-p <ad>` ya da
  `COMPOSE_PROJECT_NAME` ver (varsayılan proje adı `hakan-prod`).
- Güncelleme: `.env.server`'daki `BACKEND_IMAGE` / `FRONTEND_IMAGE`
  etiketini değiştir (ya da `:main` ile kal) ve `up -d`'yi tekrar çalıştır.

## Geri alma

1. `BACKEND_IMAGE` / `FRONTEND_IMAGE`'ı önceki `v0.N` etiketine çevir ve `up -d`.
2. Migration'lar geri alınmaz: yeni migration'lar geriye uyumlu yazılır
   (kolon ekle, sonra kullan; sil, bir sonraki sürümde). Bir migration'ın
   gerçekten geri alınması gerekiyorsa backend imajındaki `migrate` CLI ile
   `down 1` çalıştırılır. Bu, veri kaybı riski yüzünden elle yapılan bir
   karardır.
3. Alan şifrelemesi gibi veri biçimi değişikliklerinde sürümün belgesindeki
   geri alma adımı önce çalıştırılır (örnek: `backend/docs/alan-sifreleme.md`).

## Sırlar

Sırlar yalnızca `.env.server`'da durur (git'e girmez, `.gitignore`). CI imaj
yayımlamak için yalnızca `GITHUB_TOKEN` kullanır. `@norotate` işaretli
anahtarlar (`APP_ENCRYPTION_KEY`, `FIELD_ENCRYPTION_KEY`, `DB_PASSWORD`)
`upgrade --rotate` ile değiştirilemez; değiştirilirlerse veri okunamaz hale gelir.
