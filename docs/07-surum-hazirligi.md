# 07 · Mobil ve masaüstü sürüm hazırlığı (#183)

| | |
|---|---|
| Kapsam | İmzasız sürüm hazırlığı: Expo (EAS) profilleri, sürüm numarası stratejisi, mağaza meta veri taslakları; Tauri güncelleme kanalları ve imzalama/manifest iskeleti |
| Kapsam dışı | Gerçek mağaza gönderimi, imza anahtarlarının oluşturulması, güncelleme manifestinin yayımlanması |
| Tarih | 2026-11-16 |

## 1. Mobil (Expo / EAS)

### Profiller (`mobile/eas.json`)

| Profil | Dağıtım | `EXPO_PUBLIC_APP_ENV` | Android | iOS |
|---|---|---|---|---|
| `development` | internal | development | APK, geliştirme istemcisi | cihaz |
| `development-simulator` | internal | development | `development`'tan | simülatör |
| `preview` | internal | preview | APK | cihaz (ad hoc) |
| `production` | store | production | App Bundle (AAB) | mağaza |

- `production` için `EXPO_PUBLIC_API_URL` bir `https://` adresi olmalı. Değilse `app.config.js` derlemeyi durdurur. Adres koda yazılmaz: EAS ortam değişkeni (`eas env:create --environment production`) ya da CI sırrı olarak verilir.
- `cli.requireCommit`: EAS yalnızca commit'lenmiş çalışma ağacından derler.
- `submit.production` yalnızca Android **iç test kanalına taslak** olarak tanımlı (`track: internal`, `releaseStatus: draft`). iOS gönderim profili yok. CI'daki `mobile-release` işi yalnızca `eas build` çalıştırır, `eas submit` çalıştırmaz.

### Uygulama kimliği ve sürüm numarası

| Alan | Değer | Kural |
|---|---|---|
| Uygulama kimliği | `app.hakan.mukellef` (iOS `bundleIdentifier`, Android `package`) | Mağazaya bir kez yüklendikten sonra değişmez |
| Görünen sürüm | `expo.version` = `package.json` `version` (semver) | Elle artırılır; `src/core/config/release.test.ts` ikisinin eşit olduğunu denetler |
| Derleme numarası | iOS `buildNumber`, Android `versionCode` | `appVersionSource: remote`: EAS sunucusunda tutulur, `production` her derlemede artırır (`autoIncrement`) |
| Çalışma zamanı | `runtimeVersion: fingerprint` | Yerel kod değişince kendiliğinden değişir |

- `app.json`'daki `buildNumber` ve `versionCode` derlemede kullanılmaz; EAS'in uzak sayacı geçerlidir. Bu alanlar `push.ts` kaydında gösterilen `app_version` için durur (Karar 3).
- İlk mağaza derlemesinden önce sayaç bir kez ayarlanır:

  ```bash
  npx eas-cli build:version:set
  ```

- Sürüm akışı:
  1. `npm version minor --no-git-tag-version` ile `package.json`'u artırın ve `app.json`'daki `expo.version`'ı aynı yapın.
  2. Commit'leyin.
  3. main'e birleşince `mobile-release` işi `production` derlemesini kuyruğa alır (`EXPO_TOKEN` ve `eas init` gerekli).

### Mağaza meta veri taslakları (`mobile/store/`)

- `store.config.json`: App Store için EAS Metadata biçimi (`eas metadata:push` ile gönderilir; **gönderilmedi**). Türkçe ad, alt başlık, açıklama, anahtar kelimeler, sürüm notu, kategori ve inceleme hesabı yer tutucuları.
- `play/tr-TR/*.txt`: Google Play listesi (ad, kısa ve uzun açıklama, sürüm notu).
- Uzunluk sınırları `release.test.ts` ile denetlenir: App Store ad ve alt başlık ≤ 30, anahtar kelimeler ≤ 100; Play ad ≤ 30, kısa açıklama ≤ 80, açıklama ≤ 4000.
- Adresler (`example.com`) ve inceleme hesabı yer tutucudur (Karar 1).

## 2. Masaüstü (Tauri)

### Sürüm numarası

- Kaynak `desktop/package.json` `version`; `tauri.conf.json` oradan okur.
- Sürüm etiketi (`desktop-vX.Y.Z`) iş akışında `package.json`'a yazılır, depoya commit'lenmez.

### Güncelleme kanalları

| Etiket | Kanal | Uygulamanın baktığı manifest (`TAURI_UPDATER_BASE_URL` yoksa) |
|---|---|---|
| `desktop-v1.2.3` | stable | `…/releases/latest/download/latest.json` |
| `desktop-v1.3.0-beta.1` | beta | `…/releases/download/desktop-beta/latest.json` |

- Kanal sürümün ön sürüm ekinden seçilir (`desktop/scripts/updater-manifest.mjs`, `channelFor`).
- `TAURI_UPDATER_BASE_URL` değişkeni verilirse uç `<base>/<kanal>/latest.json` olur, paketler `<base>/<kanal>/<dosya>` altında aranır.
- Kanal adresi derlemede `plugins.updater.endpoints` olarak yapılandırmaya yazılır. Uygulama yalnızca kendi kanalının manifestine bakar; beta kullanıcıları kararlı sürüme otomatik geçmez.

### İmzalama ve manifest iskeleti (`.github/workflows/desktop-release.yml`)

- **Sırlar yoksa atlanır:**
  - macOS: `APPLE_CERTIFICATE`, `APPLE_ID` vb. yoksa paket imzasız ve notarize edilmemiş olur.
  - Windows: `WINDOWS_CERTIFICATE` yoksa paket imzasız olur.
  - Güncelleyici: `TAURI_SIGNING_PRIVATE_KEY` + `TAURI_UPDATER_PUBKEY` yoksa güncelleme paketi üretilmez ve güncelleyici derlemede kapalı olur.
- **`manifest` işi (yeni):** imzalı güncelleme paketlerinden (`.app.tar.gz` + `.sig`, NSIS `-setup.exe` + `.sig`) kanal başına `latest.json` üretir.
  - macOS universal paket hem `darwin-aarch64` hem `darwin-x86_64` için kullanılır.
  - Manifest `hakan-desktop-updater-<kanal>` artefaktı olarak yüklenir, **yayımlanmaz**. İmzalı paket yoksa iş atlar ve özetine yazar.
- Betik `npm test` ile CI'daki `desktop` işinde test edilir (`scripts/updater-manifest.test.mjs`).

## 3. Karar gerekiyor

1. **Mağaza kimlikleri ve adresleri:**
   - Apple Developer ve Google Play hesapları, App Store Connect uygulama kimliği (`ascAppId`).
   - Gizlilik, destek ve pazarlama adresleri.
   - Mağaza incelemesi için demo hesabı.

   Taslaklarda bunlar yer tutucu.
2. **Masaüstü güncelleme dağıtımı:** depo özelse güncelleyici GitHub Release adreslerine kimlik doğrulamasız erişemez. Manifest ve paketler herkese açık bir depolamaya mı (S3/CDN, `TAURI_UPDATER_BASE_URL`) yoksa herkese açık bir sürüm deposuna mı konacak? Manifestin yayımlanması kimin onayıyla olacak?
3. **Mobil derleme numarasının uygulama içinde gösterimi:** `push.ts` `app_version`'ı `app.json`'daki numarayla yazıyor; EAS'in uzak sayacı bu alana yansımaz. Gerçek derleme numarası için `expo-application` (`nativeBuildVersion`) eklensin mi?
4. **OTA güncelleme (`expo-updates`):** kurulu değil. Kanal tabanlı OTA istenirse profillere `channel` eklenir; `runtimeVersion: fingerprint` zaten uyumlu.
5. **Mağaza gönderim kanalı:** Android ilk gönderim iç test (taslak); iOS için TestFlight. Kararlı yayına geçiş ölçütü nedir?
