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

## Sürüm (#183)

Profiller ve sürüm numarası stratejisi: [`docs/07-surum-hazirligi.md`](../docs/07-surum-hazirligi.md).

- Görünen sürüm: `app.json` `expo.version` ile `package.json` `version` aynı tutulur.
- Derleme numarası EAS'te tutulur (`appVersionSource: remote`, `production` otomatik artırır).
- Mağaza meta veri taslakları `store/` altında; gönderim yapılmaz.

```bash
npx eas-cli build --profile development
```

Diğer profiller: `development-simulator`, `preview`, `production`.

## Bildirim ve bağlantılar (#201)

- **Bildirime dokunma:** backend push'un `data.url` alanına bildirimin web yolunu koyar, örneğin `/m/{mükellef}/tasks`. Uygulama bu yolu kendi ekranına çevirir (`src/core/push/links.ts`). Uygulama bildirimle soğuk açıldıysa hedef, oturum hazır olana kadar bekletilir. Bağlantı kullanıcının başka bir şirketine aitse önce o şirket seçilir.
- **Şema:** `hakan://tasks`, `hakan://task/{uuid}`, `hakan://receipts`, `hakan://receipts/{id}`, `hakan://capture`.
- **Universal / app link:** derlemede `EXPO_PUBLIC_APP_LINK_DOMAIN` verilirse (panelin alan adı, örneğin `app.example.com`) iki yapılandırma eklenir:
  - iOS: `applinks:<alan>` associated domain.
  - Android: `https://<alan>/m/...` için doğrulanan intent filter.

  Bu bağlantıların açılması için alan adının iki dosyayı da sunması gerekir:
  - `/.well-known/apple-app-site-association`: Team ID ve bundle id `app.hakan.mukellef`.
  - `/.well-known/assetlinks.json`: paket `app.hakan.mukellef` ve imza SHA-256'sı.

  Değişken boşsa yalnızca şema çalışır.
- Tanınmayan bağlantı ana ekranı açar (`src/app/+native-intent.tsx`).

## Hata, çevrimdışı ve hesap (#202)

- **Çevrimdışı açılış:** kayıtlı oturum açılışta doğrulanamazsa (ağ yok, zaman aşımı, sunucu hatası) oturum silinmez; "Bağlantı yok, tekrar dene" ekranı açılır. Yalnızca reddedilen oturum (401) çıkış yaptırır.
- **Hata ekranları:** kök `ErrorBoundary` "Tekrar dene" düğmesi sunar; bilinmeyen rota `+not-found` ekranını açar.
- **Parolamı unuttum:** giriş ekranından açılır. `/v1/auth/password/forgot` e-postaya kod gönderir, `/v1/auth/password/reset` yeni parolayı kaydeder.
- **Profil ve hesap:** gizlilik ve destek bağlantıları, çıkış ve hesap silme. Silme, `/v1/auth/account/deactivate/request` ile gelen e-posta koduyla onaylanır.
- **Yapılandırma** (derleme anında, `EXPO_PUBLIC_*`):
  - `EXPO_PUBLIC_PRIVACY_URL`: https adresi.
  - `EXPO_PUBLIC_SUPPORT_URL`: https ya da `mailto:` adresi. ClickFix widget'ı tanımlıysa Destek düğmesi uygulama içi ekranı açar; bu bağlantı yedektir.
  - `EXPO_PUBLIC_CLICKFIX_TAXPAYER_WIDGET_ID`: destek (#387). Ofis kendi widget kimliğini (Ayarlar → Destek) girmediyse kullanılan platform varsayılanı; ikisi de boşsa uygulama içi destek ekranı yoktur.
  - `EXPO_PUBLIC_CLICKFIX_EMBED_URL`: ClickFix webview sayfası (https); boşsa `https://getclickfix.com/widget-embed.html`.
  - `EXPO_PUBLIC_SENTRY_DSN`: boşsa Sentry kapalıdır. Kişisel veri gönderilmez (`sendDefaultPii: false`); kullanıcı yalnızca uuid ile işaretlenir.
  - Boş bağlantılar ekranda gösterilmez.
- **Sentry kaynak eşlemeleri:** `eas.json` içinde `SENTRY_DISABLE_AUTO_UPLOAD=true` olduğu için yükleme kapalıdır. Açmak için EAS secret'ına `SENTRY_AUTH_TOKEN`, `SENTRY_ORG` ve `SENTRY_PROJECT` eklenir ve bu değişken kaldırılır.

## Duman testleri ve sürüm (#203)

- **Maestro** (`.maestro/`): giriş → şirket seçimi → fiş çek → görevler (deep link dahil) → profil ve çıkış. Bir geliştirme ya da preview derlemesi kurulu emülatörde şöyle çalışır:

  ```bash
  maestro test -e MAESTRO_EMAIL=... -e MAESTRO_PASSWORD=... .maestro
  ```

  **Gece emülatör koşusu (#300):** `.github/workflows/mobile-e2e.yml` her gece (01:23 UTC) ve elle (Actions → "Mobile E2E (Maestro)" → Run workflow) iki uygulamanın akışlarını Android emülatöründe koşturur. Adımlar:
  - backend `scripts/e2e-up.sh` ile kalkar (`E2E_SKIP_FRONTEND=1`: compose, migration, demo seed, API);
  - uygulama `expo prebuild` ve `assembleRelease` (x86_64) ile derlenir; API adresi `http://10.0.2.2:8080`, yalnızca bu işte cleartext açılır;
  - akışlar `scripts/maestro-ci.sh` ile koşar.

  **Bilinen kırmızılar ve kök nedenleri (#454):**
  - *Uygulama açılışta kapanıyor, `login` akışı "Giriş yap"ı göremiyor:* Android 14'te GWP-ASan, Hermes'in `realloc` çağrısını örnekleyince yığın izini toplarken SIGSEGV ile düşüyordu (logcat: `gwp_asan` + `libhermesvm.so`, `android_unsafe_frame_pointer_chase`). Örnekleme rastgele olduğu için aralıklıydı. `app.config.js` manifest'e `android:gwpAsanMode="never"` yazar (iki uygulamada). Soğuk açılış için `login` "Giriş yap"ı 30 sn bekler.
  - *"Pixel Launcher isn't responding" penceresi:* yeni açılan emülatörün sistem uygulaması; `scripts/maestro-ci.sh` hata pencerelerini gizler ve sistemin oturmasını bekler.
  - *Gradle (`assembleRelease`) eklenti çözümlemesi:* `plugins.gradle.org` ara sıra eklenti işaretçisini ya da POM'u bulamıyor (`foojay-resolver-convention`, `kotlin-gradle-plugin` … `was not found` / `Could not parse POM`). Kod hatası değil, ağ/ayna kaynaklı. İş bir kez yeniden dener; ancak Gradle "bulunamadı" sonucunu önbelleğe aldığından (büyük olasılıkla) aynı işteki ikinci deneme de aynı hatayla düşüyor (ör. 17 Kasım, mobile işinde iki deneme de `foojay-resolver-convention`). Bu durumda işi Actions'tan yeniden çalıştırmak yeterli (yeni makine, boş önbellek). Kalıcı çözüm için yeniden denemenin `--refresh-dependencies` ile yapılması gerekir; CI akışı bu şeritte değiştirilmediği için ayrı issue'da.

  Sonuç iş özetinde akış başına tablo olarak görünür. `maestro-<uygulama>` artifact'ında JUnit raporu, hata anı ekran görüntüleri, ekran kaydı, logcat ve API logu var. İş zorunlu kontrol değil, yalnızca rapor üretir. Her push'taki mobil işte ayrıca şunlar koşar:
  - `npm test` (`src/core/config/maestro.test.ts`): akışların YAML'ı, `appId`, komutları, sırası ve kullandığı env değişkenleri denetlenir.
  - `expo prebuild` (Android): yerel proje yapılandırması üretilir. Eklentiler (expo-updates, Sentry, bildirim, bağlantılar) burada bozulursa iş düşer.
- **Sürüm bilgisi:** profil ekranı gerçek sürümü ve derleme numarasını (expo-application) ve varsa OTA güncellemesini gösterir.
- **OTA** (expo-updates): `runtimeVersion` politikası `appVersion`. Güncelleme yalnızca aynı uygulama sürümünün derlemelerine, kendi kanalında (`eas.json`: development, preview, production) gider. Yayın komutu: `npx eas-cli update --channel preview`. EAS proje kimliği yokken güncelleme kapalıdır.
- **Gönderim:** `eas.json` `submit` profilleri. Android: internal kanalda taslak. iOS: kimlikler `TBD_…` yer tutucusu. Eksik dış bilgilerin listesi: `store/README.md`.
- **Yerel preview derlemesi:** Android SDK ile `npx eas-cli build --profile preview --platform android --local`, ya da `npx expo prebuild` ve ardından `cd android && ./gradlew assembleRelease`.

## Dizinler

- `src/app/` ekranlar (expo-router). `(app)/` giriş ve şirket seçimi sonrası.
- `src/core/` API istemcisi, oturum, şirket seçimi, push, marka ve tema.
- `src/components/` ortak arayüz parçaları.
