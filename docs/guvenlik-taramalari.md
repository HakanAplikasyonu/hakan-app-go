# CI güvenlik taramaları (#192)

`.github/workflows/ci.yml` içindeki `security` işi ve `.github/dependabot.yml`.
`security`, tek gerekli kontrol olan `ci` kapısına bağlıdır; bulgu varsa iş
kırmızı olur ve PR birleşmez.

## Taramalar

| Tarama | Ne zaman | Kırma ölçütü | Yerelde |
|---|---|---|---|
| **gitleaks** v8.30.1 | Her push, tüm git geçmişi | Her bulgu | `gitleaks git --redact .` |
| **govulncheck** v1.9.0 | Backend ya da masaüstü sidecar değişince, haftalık tam çalıştırma | Kodun gerçekten çağırdığı her açık | `cd backend && go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` |
| **gosec** (golangci-lint üzerinden) | Backend değişince | Önem derecesi HIGH, güven ≥ MEDIUM; test kodu hariç | `cd backend && golangci-lint run -c .golangci.gosec.yml ./...` |
| **pnpm audit** (frontend) | Frontend değişince | high ve critical (geliştirme bağımlılıkları dahil) | `cd frontend && pnpm audit --audit-level high` |
| **npm audit** (mobile, desktop, e2e) | Her çalıştırmada | high ve critical | `npm audit --audit-level=high` |
| **Dependabot** | Haftalık, pazartesi 05:00 (İstanbul) | — | — |

Dependabot şunları izler: Go modülleri (`backend`, `desktop/sidecar`), npm
(`frontend`, `mobile`, `desktop`, `e2e`), GitHub Actions, Dockerfile temel
imajları (`backend`, `frontend`, `deploy/backup`) ve Cargo (`desktop/src-tauri`).
Küçük ve yama sürümleri ekosistem başına tek PR'da gruplanır. **Büyük
sürümler hiçbir ekosistemde önerilmez** (`ignore` `"*"` semver-major): bunlar
geçiş adımlarıyla birlikte elle planlanır. Ayrıca `golang` Docker imajının küçük
sürümü (1.26 → 1.27) önerilmez; Go araç zinciri `go.mod` ile birlikte elle
yükseltilir. React Native'in küçük sürümleri Expo SDK'yı izler
(`npx expo install --fix`). Güvenlik güncellemeleri bu kurallardan etkilenmez.

## Bu PR'da düzeltilenler

- **govulncheck**: kodun çağırdığı 25 açık (8 modül). Yükseltilenler: excelize
  v2.11.1, golang.org/x/net, golang.org/x/image, google.golang.org/grpc,
  OpenTelemetry (otel, sdk, otlptrace), andybalholm/brotli. Sonuç: 0.
- **gosec (HIGH)**: 37 bulgu.
  - Sorgu dizgesinden gelen `limit`/`offset` değerleri `strconv.Atoi` ile
    okunup `int32`'ye çevriliyordu (G109). Hepsi `pkg/intconv.ParseInt32` ile
    32 bit ayrıştırılıp sınırlandırılıyor.
  - `int32(len(...))` ve yapılandırma sayıları gibi daraltan dönüşümler (G115)
    `intconv.Int32/Int16/Uint32` ile sınırlandırıldı.
  - Kalan 5'i gerekçeli yanlış pozitif (aşağıda).
- **pnpm audit (frontend)**: 24 high/critical.
  - next 16.2.10 → 16.4.6, eslint-config-next 16.4.6, zod 4.6.7.
  - `xlsx`: npm'deki 0.18.5 artık güncellenmiyor ve düzeltmesi yok. SheetJS'in
    kendi dağıtımındaki 0.20.3 tarball'ı kullanılıyor
    (`https://cdn.sheetjs.com/xlsx-0.20.3/xlsx-0.20.3.tgz`). Bu sürümü
    Dependabot güncelleyemez; elle izlenmeli.
  - Geçişli bağımlılıklar için `frontend/pnpm-workspace.yaml`'da `overrides`
    eklendi: brace-expansion, js-yaml, nanoid, source-map-js. Üst paket düzeltmeyi
    kendisi getirince ilgili satır silinir.
- **gitleaks**: 3 yanlış pozitif (aşağıda).
- **mobile, desktop, e2e**: high veya critical bulgu yok.

## Yanlış pozitifler ve nasıl yok sayılır

Yok sayma her zaman **bulgunun yerinde ve gerekçesiyle** yapılır; bir taramayı
ya da kuralı toptan kapatmak yasaktır.

- **gosec**: satır sonuna `//nolint:gosec // <gerekçe>`. Gerekçe zorunludur
  (`nolintlint`, `require-explanation`). Mevcutlar:
  - `entegrator/service.go`: advisory lock anahtarının yarısı `int32(connID)`.
    Taşma 2^32 modunda sarar; çakışma yalnızca iki senkronu sıraya sokar.
  - `orgctx/orgcache`: önbellek kuşak belirteci için `math/rand`. Sır değil,
    yalnızca benzersizlik gerekiyor.
  - `metrics`: kapatma, istek bağlamı bittikten sonra çalıştığı için kendi
    süreli bağlamını kullanıyor.
  - `entegrator/audit.go` (G704 SSRF): URL ya entegratörün sabit adresi ya da
    sır kasasındaki `Credentials.Endpoint`. Bu alanı bugün yalnızca operatör
    yazabiliyor, API'den yazılmıyor. **Bu alanı yazan bir API eklenirse adres
    doğrulanmalı**: yalnızca https, iç ağ ve loopback adresleri reddedilmeli.
- **gitleaks**: `.gitleaksignore` dosyasına bulgunun parmak izi
  (`<commit>:<dosya>:<kural>:<satır>`) ve üstüne gerekçesi yazılır. Mevcutlar:
  - `vault_test.go`: kasanın düz metin saklamadığını kanıtlayan sabit test parolası.
  - `.env.server.example`: boş `REDIS_PASSWORD` satırı ve altındaki üretici
    açıklaması. Değer yok.
- **audit**: düzeltme varsa yükseltilir ya da `overrides` ile taban sürüm
  verilir. Düzeltme yoksa paketin yerine başka paket kullanılır. Bunlar da
  olmuyorsa yok sayma burada, gerekçesi ve son tarihiyle yazılır. Şu an böyle
  bir kayıt yok.

## Gerçek bir sır sızarsa

1. Sırrı hemen sağlayıcıda iptal et ya da döndür. Geçmişten silmek tek başına
   yetmez: sır klonlarda ve önbelleklerde kalır.
2. `.env.server`'daki değeri `scripts/prod-env.sh upgrade --rotate KEY` ile
   yenile. `@norotate` anahtarlar için `backend/docs/sir-kasasi.md` ve
   `backend/docs/alan-sifreleme.md` dosyalarındaki adımları izle.
3. Kodu düzelt (sır env'e ya da kasaya taşınır). Gerekirse geçmişi temizleme
   kararını depo sahipleri verir.
