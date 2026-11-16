# mobile-office/

Mali müşavir ve ofis çalışanları için mobil uygulama (M12, #236). Mükellef uygulamasından (`mobile/`) ayrı bir Expo projesidir: aynı Go API ve oturum modeli, ayrı paket kimliği ve mağaza kaydı.

| | Mükellef (`mobile/`) | Ofis (`mobile-office/`) |
|---|---|---|
| Ad | Hakan | Hakan Ofis |
| Paket kimliği | `app.hakan.mukellef` | `app.hakan.office` |
| Şema | `hakan://` | `hakanofis://` |
| Kapsam başlığı | `X-Taxpayer` | `X-Organization` |

## Çalıştırma

```bash
npm ci
EXPO_PUBLIC_API_URL=http://<bilgisayar-ip>:8080 npx expo start
```

| Değişken | Anlamı |
|---|---|
| `EXPO_PUBLIC_API_URL` | Go API kökü (`/v1` olmadan). Boşsa `http://localhost:8080`. Mağaza derlemesinde https zorunlu. |
| `EXPO_PUBLIC_APP_ENV` | `development` / `preview` / `production` (eas.json profilleri ayarlar). |
| `EXPO_PUBLIC_SENTRY_DSN` | Sentry DSN (https). Boşsa çökme raporu kapalı (#241). |
| `EAS_OFFICE_PROJECT_ID` | `eas init` öncesinde push için EAS proje kimliği (mükellef uygulamasınınkinden ayrı). |

## Akış

`booting → signedOut (giriş) → denied | selectOffice (ofis seçici) → ready`

- Giriş `POST /v1/auth/login`; 2FA açıksa (`MFA_REQUIRED`) kod alanı açılır. Tokenlar güvenli depoda, `POST /v1/auth/refresh` ile yenilenir.
- Ofisler `GET /v1/auth/me` → `organizations`. Yalnızca ofis üyeliği olan hesap girer; yalnızca mükellef üyeliği olan hesap `denied` ekranında mükellef uygulamasına yönlendirilir.
- Son kullanılan ofis (üyelik ve erişim sürüyorsa) ya da seçilebilir tek ofis doğrudan açılır; askıdaki veya erişim süresi dolmuş ofis seçilemez. Seçim `GET /v1/tenant/context` (`X-Organization: <slug>`) ile doğrulanır.

## Bildirimler ve derin bağlantılar (#241)

Push kaydı oturum açılınca yapılır; izin yalnızca ana sayfadaki “Bildirimleri aç” ile istenir. Bildirime dokununca `data.url` (ofis panelinin yolu) uygulama ekranına eşlenir (`src/core/push/links.ts`); bağlantı başka bir ofise aitse önce o ofis seçilir.

| Bildirim | Tür (`kind`) | Açılan ekran |
|---|---|---|
| Mükellef belge yükledi / görev tamamlandı | `doc_task_submitted` | Görev ayrıntısı (`/t/{slug}/engagements/{eng}?task={uuid}`) |
| Kritik (engelleyici) uyarı | `warnings_blocker` | Mükellefin uyarıları (`/t/{slug}/warnings?engagement={eng}`) |

Şema bağlantıları: `hakanofis://warnings`, `hakanofis://review`, `hakanofis://clients/{eng}`, `hakanofis://tasks/{uuid}?engagement={eng}`. Bilinmeyen bağlantı ana sayfayı açar; bilinmeyen ekran `+not-found`.

Hata ekranı (`ErrorBoundary`), bağlantısız açılış ekranı (oturum korunur, “Tekrar dene”) ve Sentry `mobile/` ile aynı kalıptadır.

## Kopyalanan çekirdek

Aşağıdakiler `mobile/src/` altından kopyalandı ve dosya başında kaynağı yazıyor. Ortak paket çıkarmak ayrı bir issue; o zamana kadar düzeltmeler iki tarafa da uygulanmalı.

- `core/api/client.ts` (`X-Taxpayer` yerine `X-Organization`), `core/api/envelope.ts`, `core/api/schema.d.ts` (üretilir)
- `core/auth/session.ts`, `core/storage.ts` (anahtarlar `hakanofis.*`), `core/push/push.ts`, `core/config/env.ts`
- `components/ui.tsx`, `components/brand-mark.tsx`, `core/theme.ts`, `core/brand.ts`
- `core/observability/sentry.ts`, `core/auth/launch.ts`, `core/config/safe-link.ts`, `app/+not-found.tsx`; `core/push/links.ts` ve `use-notification-routing.ts` aynı kalıpla ofis yollarına uyarlandı

## Komutlar

```bash
npm run lint
npm run typecheck
npm test               # node --test (saf modüller: src/**/*.test.ts)
npm run api:generate   # backend/docs/openapi.yaml → src/core/api/schema.d.ts
```

CI'daki `mobile-office` işi tipleri yeniden üretir (işlenmiş dosya eskiyse uyarı verir), ardından lint, tip ve test çalıştırır.

## Duman testleri ve sürüm (#242)

- **Maestro** (`.maestro/`): giriş → ofis seçimi → pano (derin bağlantı dahil) → inceleme onayı → belge talebi → çıkış. Geliştirme ya da preview derlemesi kurulu bir emülatörde, demo verisi olan bir backend'e karşı:

  ```bash
  maestro test -e MAESTRO_EMAIL=... -e MAESTRO_PASSWORD=... .maestro
  ```

  Hesap bir ofis kullanıcısı olmalı (ör. `make demo-seed` ile gelen `musavir@demo.hakan.local`). Kuyrukta bekleyen öneri yoksa onay adımı atlanır. Emülatör çalıştırması CI'da yok (`mobile/` ile aynı gerekçe: yerel Android derlemesi ve test verili backend gerektirir). CI'da `npm test` akışların YAML'ını, `appId`'yi, komutları, sırayı ve dokunulan `testID`'lerin ekranlarda olduğunu denetler; `expo prebuild` (Android) eklentilerin yerel projeyi ürettiğini denetler.
- **OTA** (expo-updates): `runtimeVersion` politikası `appVersion`; güncelleme kendi kanalına gider (`npx eas-cli update --channel preview`). EAS proje kimliği yokken kapalı.
- **Gönderim:** `eas.json` `submit` profilleri (Android internal taslak, iOS kimlikleri `TBD_…`). Eksik dış bilgiler: `store/README.md`.

## Derleme

```bash
npx eas-cli build --profile development
```

Profiller: `development`, `development-simulator`, `preview`, `production`. Mağaza gönderimi ve sürüm hazırlığı #242'de.
