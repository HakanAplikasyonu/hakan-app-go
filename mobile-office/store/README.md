# Mağaza meta veri taslakları (#242)

**Taslak, gönderilmedi.** Kalıp: `mobile/store/` (#203). Mükellef uygulamasından ayrı mağaza kaydıdır (`app.hakan.office`).

| Dosya | Mağaza | Gönderme komutu (yalnızca karar sonrası) |
|---|---|---|
| `store.config.js` | App Store (EAS Metadata; dış bilgiler ortamdan) | `npx eas-cli metadata:push` |
| `play/tr-TR/title.txt` | Google Play, ad (≤ 30) | Play Console ya da `fastlane supply` |
| `play/tr-TR/short-description.txt` | Google Play, kısa açıklama (≤ 80) | |
| `play/tr-TR/full-description.txt` | Google Play, açıklama (≤ 4000) | |
| `play/tr-TR/release-notes.txt` | Google Play, sürüm notu | |

- Uzunluk sınırları `src/core/config/release.test.ts` ile denetlenir.
- Ekran görüntüleri henüz yok. Gerekenler: iPhone 6,9" ve 6,5", iPad (tablet desteği açık), Android telefon; ekranlar pano, inceleme kuyruğu, öneri ayrıntısı, mükellef ayrıntısı, belge talebi.
- İkon ve açılış görseli `assets/src/office-mark.svg`'den üretilir (mavi zemin; mükellef uygulamasınınki lacivert).

## Eksik dış bilgiler (#207)

Gönderimden önce hepsi dolmalı. Dolmayan değer yer tutucu kalır: `example.com` ya da `TBD`.
`node -e "console.log(require('./store/store.config.js').missingStoreValues())"` eksikleri listeler.

| Bilgi | Nerede | Not |
|---|---|---|
| `STORE_OFFICE_MARKETING_URL` | App Store tanıtım adresi | env |
| `STORE_OFFICE_SUPPORT_URL` | App Store destek adresi | env |
| `STORE_OFFICE_PRIVACY_URL` | App Store gizlilik politikası | env |
| `STORE_OFFICE_REVIEW_FIRST_NAME`, `STORE_OFFICE_REVIEW_LAST_NAME`, `STORE_OFFICE_REVIEW_EMAIL`, `STORE_OFFICE_REVIEW_PHONE` | App Review iletişim kişisi | env |
| `STORE_OFFICE_DEMO_USERNAME`, `STORE_OFFICE_DEMO_PASSWORD` | İnceleme hesabı: canlıda demo ofisin üyesi | env; EAS secret, repoya yazılmaz |
| `ascAppId`, `appleTeamId` | `eas.json` → `submit.production.ios` | Şu an `TBD_…`; App Store Connect'te ayrı uygulama kaydı |
| `secrets/play-service-account.json` | `eas.json` → `submit.production.android` | Play servis hesabı anahtarı; git dışında (`.gitignore`) |
| `EAS_OFFICE_PROJECT_ID` | `app.config.js` (`eas init`) | Ayrı EAS projesi; push belirteci ve OTA için, yokken güncelleme kapalı |
| `EXPO_PUBLIC_SENTRY_DSN` | EAS secret | Boşsa çökme raporu kapalı (#241) |
| `EXPO_PUBLIC_APP_LINK_DOMAIN` ile Team ID ve imza SHA-256'sı | Evrensel / uygulama bağlantıları (`/t/...`, #241) | Panel alan adında `.well-known` dosyalarında `app.hakan.office` da yer almalı |
