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
| Uygulama yığını | `compose.prod.yml` | Postgres, Redis, SeaweedFS (S3), Meilisearch, Centrifugo, `migrate`, backend, worker, isteğe bağlı `worker-egress`, `egress-proxy` (sabit çıkış IP'si, #256), frontend. Dokploy bu dosyayı kullanır. |
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

## Sabit çıkış IP'si: İşNet ve Vakıfbank (#256)

İşNet e-fatura servisleri ve Vakıfbank hesap hareketi servisi yalnızca izin
listelerindeki sabit IP'lerden gelen çağrıyı kabul eder (`docs/kaynak`
32 §2, 20 §4, 22 §1). İşNet tanımsız IP'ye hiç cevap vermez; çağrı zaman
aşımına düşer.

- **`egress-proxy` servisi** (squid): yalnızca `EGRESS_PROXY_HOSTS`'taki
  hostlara HTTPS (CONNECT 443) açar, gerisini 403 ile reddeder.
- **Backend ve worker** bu hostlara `EGRESS_PROXY_URL` üzerinden çıkar
  (varsayılan `http://egress-proxy.<ağ>:3128`). Diğer bütün trafik
  doğrudan çıkar. Banka adaptörü (API) ve entegratör senkronu (worker) aynı
  kuralı kullanır.
- **`EGRESS_PROXY_HOSTS`** varsayılanı:
  - İşNet test ve canlı: `einvoiceservicetest.isnet.net.tr`, `einvoiceservice.isnet.net.tr`
  - Vakıfbank test ve canlı: `vbtestservice.vakifbank.com.tr`, `vbservice.vakifbank.com.tr`
  - IP denetimi: `api.ipify.org`

  Virgülle ayrılır; `.alan.com.tr` biçimi alt alan adlarını da kapsar.
- **Vekili kapatmak için:** `EGRESS_PROXY_URL=` (boş). Başka bir sabit IP'li
  vekil kullanılacaksa onun adresi yazılır.

Çıkış IP'si, vekilin çalıştığı sunucunun IP'sidir. Bu yüzden vekil, IP'si
değişmeyen (rezerve) bir sunucuda çalışmalı. Sunucu değişirse bütün
mükelleflerin bankaya ve entegratöre yeniden başvurması gerekir (20 §4).

### Sabit IP'yi entegratöre ve bankaya bildirme

1. **Yığını ayağa kaldırın** ve vekilin dışarıya hangi IP'den çıktığını
   görün:

   ```bash
   docker compose -f deploy/compose.prod.yml exec backend /app/egress-check
   ```

   Çıktı `observed_ip=<IP> via_proxy=true` biçimindedir. Dokploy'da aynı
   komut `backend` konteynerinde çalıştırılır.
2. **Bu IP'yi** `.env.server` (ya da Dokploy ortamı) içinde
   `EGRESS_EXPECTED_IPS`'e yazın ve yeniden deploy edin. Worker IP'yi
   `EGRESS_CHECK_INTERVAL` aralığıyla denetler; IP değişirse günlüğe ve
   Sentry'ye yazar. Durum `GET /v1/platform/egress` ucundadır.
3. **Banka:** aynı IP'yi `BANK_EGRESS_IPS`'e yazın. Mükellefin şube
   talimatı formunda bu IP görünür.
4. **İşNet:** IP'yi test ortamı tanımı için İşNet'e iletin (32 §2, adım 2).
   Canlı tanım, test faturaları başarıyla tamamlandıktan sonra yapılır.
5. **Vakıfbank:** IP, mükellefin şube talimatıyla tanımlanır. Talimatta
   Easytrade teknik yetkili ve hizmet sağlayıcı olarak yer alır (20 §5.2).
6. **Bir hostun vekilden geçtiğini doğrulayın:**

   ```bash
   docker compose -f deploy/compose.prod.yml exec backend /app/egress-check -probe https://einvoiceservicetest.isnet.net.tr/
   ```

   Çıktıda `proxied=true` olmalı. Tanım yapılmadan önce İşNet cevap
   vermez; bu beklenen bir zaman aşımıdır.

`egress-check` çıkış kodları:

| Kod | Anlamı |
|---|---|
| 0 | tamam |
| 1 | hata |
| 2 | IP, `EGRESS_EXPECTED_IPS` içinde değil |
| 3 | vekil hostu reddetti; izin listesinde değil |

CI'daki duman testi (`scripts/prod-smoke.sh`) aynı aracı kullanır. İzin
listesindeki bir host vekilden geçmeli, listede olmayan bir host vekil
tarafından reddedilmelidir.

## Nesne deposu kapasitesi (SeaweedFS, #247)

SeaweedFS dosyaları sabit boyutlu birimlere (volume) yazar.

- **Ayarlar:**

  | Değişken | Varsayılan | Anlamı |
  |---|---|---|
  | `SEAWEEDFS_VOLUME_SIZE_MB` | `256` | Birim boyutu (önce 1024'tü) |
  | `SEAWEEDFS_VOLUME_MAX` | `0` | Birim sayısı; `0` boş diske göre hesaplar |

- **Disk dolunca:** yeni birim açılamaz ve yazma durur.
  - API bu durumda `503 STORAGE_UNAVAILABLE` döner (`Retry-After: 300`); önceden genel bir 500'dü. Arayüz ve mobil uygulama "biraz sonra tekrar deneyin" der.
  - Mobil kuyruk fişi telefonda tutup sonra yeniden dener.
- **Neden küçük birim:** küçük birim, yarı dolu birimlerde kalan kullanılmaz alanı azaltır. Var olan 1 GB'lık birimler olduğu gibi kalır; yeni birimler yeni boyutla açılır.
- **İzleme:** `storage_write_failures_total{reason="no_space"|"server"}` (API ve worker `/metrics`). Sıfırdan büyükse disk büyütülmeli ya da eski veri taşınmalı.

  Örnek uyarı:

  ```promql
  increase(storage_write_failures_total[15m]) > 0
  ```

- **Diski izleme:** `seaweedfs_data` biriminin bulunduğu diskin doluluğu sunucu izlemesinden takip edilmeli. Üretimde nesne deposu için ayrı disk önerilir.

## Yeni ofis açma ve kurulum sihirbazı (#273)

Pilot ofis platform yöneticisinin hesabıyla açılır. Betik sunucuya değil API'ye konuşur; yöneticinin bilgisayarından çalıştırılabilir (`curl` ve `jq` gerekir):

```bash
API_URL=https://api.ornek.test scripts/new-office.sh --name "Örnek Mali Müşavirlik" --slug ornek-smmm --owner-email sahip@ornek.test
```

- Yönetici e-postası ve parolası `ADMIN_EMAIL` / `ADMIN_PASSWORD` ortam değişkenlerinden okunur; yoksa betik sorar (parola gizli girilir, komut satırına ve kabuk geçmişine düşmez). Doğrulayıcı uygulaması açıksa kod `ADMIN_TOTP` ile verilir ya da sorulur.
- Betik ofisi açar (`POST /v1/platform/organizations`) ve sahibi `owner` rolüyle davet eder. Yönetici yeni ofisin ilk sahibi olarak kalır; sahip katıldıktan sonra üyeler sayfasından çıkarılabilir.
- Sahip daveti kabul edip ofise girdiğinde ana sayfada "Ofis kurulumunu tamamlayın" kartı görünür; sihirbaz `/t/<slug>/setup` adresindedir. Adımlar: ofis bilgileri, ekip davetleri, Excel / CSV'den mükellef listesi (satır satır doğrulama raporu), muhasebe programı (Luca / Zirve), entegratör bağlantıları (mükellef onayından sonra engagement sayfasından), mükelleflere toplu onay e-postası. Her adım atlanabilir; sihirbaz yarıda bırakılıp kaldığı yerden sürdürülür.
- Liste en çok 1000 satır olabilir. Toplu davet her geçerli satır için normal mükellef ekleme talebini açar (onay e-postası, #7). Ofis başına 15 dakikada en çok 3 gönderim ve 30 yükleme yapılabilir.

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
