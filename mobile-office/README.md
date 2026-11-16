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
| `EAS_OFFICE_PROJECT_ID` | `eas init` öncesinde push için EAS proje kimliği (mükellef uygulamasınınkinden ayrı). |

## Akış

`booting → signedOut (giriş) → denied | selectOffice (ofis seçici) → ready`

- Giriş `POST /v1/auth/login`; 2FA açıksa (`MFA_REQUIRED`) kod alanı açılır. Tokenlar güvenli depoda, `POST /v1/auth/refresh` ile yenilenir.
- Ofisler `GET /v1/auth/me` → `organizations`. Yalnızca ofis üyeliği olan hesap girer; yalnızca mükellef üyeliği olan hesap `denied` ekranında mükellef uygulamasına yönlendirilir.
- Son kullanılan ofis (üyelik ve erişim sürüyorsa) ya da seçilebilir tek ofis doğrudan açılır; askıdaki veya erişim süresi dolmuş ofis seçilemez. Seçim `GET /v1/tenant/context` (`X-Organization: <slug>`) ile doğrulanır.

## Kopyalanan çekirdek

Aşağıdakiler `mobile/src/` altından kopyalandı ve dosya başında kaynağı yazıyor. Ortak paket çıkarmak ayrı bir issue; o zamana kadar düzeltmeler iki tarafa da uygulanmalı.

- `core/api/client.ts` (`X-Taxpayer` yerine `X-Organization`), `core/api/envelope.ts`, `core/api/schema.d.ts` (üretilir)
- `core/auth/session.ts`, `core/storage.ts` (anahtarlar `hakanofis.*`), `core/push/push.ts`, `core/config/env.ts`
- `components/ui.tsx`, `components/brand-mark.tsx`, `core/theme.ts`, `core/brand.ts`

## Komutlar

```bash
npm run lint
npm run typecheck
npm test               # node --test (saf modüller: src/**/*.test.ts)
npm run api:generate   # backend/docs/openapi.yaml → src/core/api/schema.d.ts
```

CI'daki `mobile-office` işi tipleri yeniden üretir (işlenmiş dosya eskiyse uyarı verir), ardından lint, tip ve test çalıştırır.

## Derleme

```bash
npx eas-cli build --profile development
```

Profiller: `development`, `development-simulator`, `preview`, `production`. Mağaza gönderimi ve sürüm hazırlığı #242'de.
