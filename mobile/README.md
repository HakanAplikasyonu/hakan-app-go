# mobile/

Mükellef mobil uygulaması (D3, #37). Expo 58 + expo-router; başlangıç noktası `otopoly-go/mobile` (G10), yalnızca gereken parçalar alındı.

## Çalıştırma

```bash
npm ci
EXPO_PUBLIC_API_URL=http://<bilgisayar-ip>:8080 npx expo start
```

Push bildirimi ve güvenli depolama yerel modül ister: test cihazında Expo Go yerine geliştirme derlemesi kullanın (`npx eas-cli build --profile development`). Expo Go'da uygulama çalışır, push kapalı görünür.

| Değişken | Anlamı |
|---|---|
| `EXPO_PUBLIC_API_URL` | Go API kökü (`/v1` olmadan). Boşsa `http://localhost:8080`. Mağaza derlemesinde https zorunlu. |
| `EXPO_PUBLIC_APP_ENV` | `development` / `preview` / `production` (eas.json profilleri ayarlar). |
| `EAS_PROJECT_ID` | `eas init` öncesinde push için EAS proje kimliği. `eas init` sonrası `app.json`'a yazılır ve CI sürüm işi açılır. |

## Akış

`booting → signedOut (giriş) → selectTaxpayer (şirket seçici) → ready (ana sayfa)`

- Giriş: `POST /v1/auth/login`; 2FA açıksa (`MFA_REQUIRED`) kod alanı açılır. Tokenlar güvenli depoda (Keychain / Keystore) tutulur, `POST /v1/auth/refresh` ile yenilenir.
- Şirketler `GET /v1/auth/me` → `taxpayers`. Son kullanılan şirket (üyelik sürüyorsa) veya tek şirket doğrudan açılır, yoksa seçici gösterilir. Seçim `GET /v1/taxpayer/context` (`X-Taxpayer`) ile doğrulanır.
- Push: izin yalnızca kullanıcı "Bildirimleri aç" dediğinde istenir; token `POST /v1/push-devices` ile kaydedilir, çıkışta silinir.

## Komutlar

```bash
npm run lint
npm run typecheck
npm test               # node --test (saf modüller: src/**/*.test.ts)
npm run api:generate   # backend/docs/openapi.yaml → src/core/api/schema.d.ts
```

`backend/docs/openapi.yaml` değişen her PR'da `src/core/api/schema.d.ts` de yeniden üretilip işlenmeli; CI'daki `mobile` işi bunu denetler. `npm ci` yapmadan:

```bash
cd mobile && npx -y openapi-typescript@7.13.0 ../backend/docs/openapi.yaml -o src/core/api/schema.d.ts
```

## Dizinler

- `src/app/` ekranlar (expo-router). `(app)/` giriş ve şirket seçimi sonrası.
- `src/core/` API istemcisi, oturum, şirket seçimi, push, marka ve tema.
- `src/components/` ortak arayüz parçaları.
