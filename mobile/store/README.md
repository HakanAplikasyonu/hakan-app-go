# Mağaza meta veri taslakları (#183)

**Taslak, gönderilmedi.** Ayrıntı ve açık kararlar: [`docs/07-surum-hazirligi.md`](../../docs/07-surum-hazirligi.md).

| Dosya | Mağaza | Gönderme komutu (yalnızca karar sonrası) |
|---|---|---|
| `store.config.js` | App Store (EAS Metadata biçimi; dış bilgiler ortamdan) | `npx eas-cli metadata:push` |
| `play/tr-TR/title.txt` | Google Play, ad (≤ 30) | Play Console ya da `fastlane supply` |
| `play/tr-TR/short-description.txt` | Google Play, kısa açıklama (≤ 80) | |
| `play/tr-TR/full-description.txt` | Google Play, açıklama (≤ 4000) | |
| `play/tr-TR/release-notes.txt` | Google Play, sürüm notu | |

- Uzunluk sınırları ve Türkçe liste `src/core/config/release.test.ts` ile denetlenir.
- Ekran görüntüleri henüz yok. Gerekenler: iPhone 6,9" ve 6,5", Android telefon; ekranlar giriş, görevler, fiş çek, fişlerim.

## Eksik dış bilgiler (#203)

Gönderimden önce hepsi dolmalı. Dolmayan değer yer tutucu olarak kalır: `example.com` ya da `TBD`.
`node -e "console.log(require('./store/store.config.js').missingStoreValues())"` hâlâ eksik olanları listeler.

| Bilgi | Nerede | Not |
|---|---|---|
| `STORE_MARKETING_URL` | App Store tanıtım adresi | env |
| `STORE_SUPPORT_URL` | App Store destek adresi | env; uygulama içi `EXPO_PUBLIC_SUPPORT_URL` ile aynı olmalı |
| `STORE_PRIVACY_URL` | App Store gizlilik politikası | env; uygulama içi `EXPO_PUBLIC_PRIVACY_URL` ile aynı olmalı |
| `STORE_REVIEW_FIRST_NAME`, `STORE_REVIEW_LAST_NAME`, `STORE_REVIEW_EMAIL`, `STORE_REVIEW_PHONE` | App Review iletişim kişisi | env |
| `STORE_DEMO_USERNAME`, `STORE_DEMO_PASSWORD` | İnceleme hesabı (canlıda açılır) | env; EAS secret, repoya yazılmaz |
| `ascAppId`, `appleTeamId` | `eas.json` → `submit.production.ios` | Şu an `TBD_…`; App Store Connect'ten |
| `secrets/play-service-account.json` | `eas.json` → `submit.production.android` | Google Play servis hesabı anahtarı; git dışında (`.gitignore`) |
| `EAS_PROJECT_ID` | `app.config.js` (`eas init`) | Push belirteci ve OTA güncellemesi için; yokken güncelleme kapalı |
| `EXPO_PUBLIC_APP_LINK_DOMAIN` ile Team ID ve imza SHA-256'sı | universal / app link (#201) | `.well-known` dosyaları |
| `EXPO_PUBLIC_SENTRY_DSN`, `SENTRY_ORG`, `SENTRY_PROJECT`, `SENTRY_AUTH_TOKEN` | çökme raporu (#202) | DSN boşsa kapalı |
| Ekran görüntüleri | App Store / Play | Henüz yok |
