# @hakan/mobile-core

`mobile/` (mükellef) ve `mobile-office/` (müşavir) uygulamalarının ortak çekirdeği (#295). Daha önce bu kod iki uygulamada kopya olarak duruyordu.

| Modül | İçerik |
|---|---|
| `api/envelope` | API zarfı (`unwrap`, `ApiError`), Türkçe hata metni (`createErrorMessage`; uygulama kendi kodlarını ekler) |
| `api/client` | JSON istemcisi (`createApiClient`): erişim belirteci, tek seferlik eşzamanlı yenileme, zaman aşımı. Kapsam başlığı uygulamadan gelir (`taxpayer` → `X-Taxpayer`, `organization` → `X-Organization`) |
| `auth/session` | Oturum (`createSession`): güvenli depoda saklanır, sona erince aboneler haberdar edilir |
| `auth/launch` | Açılışta oturum denetimi hatasının anlamı (yalnızca 401 oturumu kapatır) |
| `secure-store` | Keychain / Keystore; web derlemesinde localStorage |
| `push/push` | Expo push (`createPush`): izin, belirteç, cihaz kaydı, dokunuşlar. Anahtar ve Android kanal adı uygulamadan gelir |
| `push/data` | Push verisindeki `url` |
| `observability/sentry` | Çökme raporu (`createReporter`); DSN yoksa her çağrı boş |
| `config/safe-link` | Yalnızca https (ve izin verilirse mailto) bağlantı |
| `ui` (`ui/ui.tsx`), `ui/theme`, `ui/palette` | Ekran yapı taşları, renk paleti, açık/koyu tema |
| `ui/ikon` | Tasarımdaki ikon seti (react-native-svg, çizgi 1.75) |
| `ui/layout` | Kırılımlar (`yerlesim`: telefon < 768, tablet 768–1279, geniş ≥ 1280), 44 px dokunma hedefi, rozet metni |
| `ui/sekme-cubugu` | Alt sekme çubuğu (telefon) ve 84 px ikon rayı (tablet); expo-router `Tabs` `tabBar` prop'u ile |
| `ui/parcalar` | Arayüz v1 parçaları: kesik köşe, ekran başlığı, hap, segment, ilerleme çubuğu, kart, satır, Görünüm ayarı |
| `notifications/inbox` | Mobil bildirim listesi modeli ve uçları (uç #333 ile gelecek; o zamana kadar `null`) |
| `screens` | Kök hata ekranı (expo-router `ErrorBoundary`) ve bağlantısız açılış ekranı; logo ve raporlayıcı uygulamadan gelir |

Uygulamaya özgü olanlar uygulamada kalır: ortam değişkenleri (`core/config/env.ts`), saklama anahtarları (`core/storage.ts`), bağlantı eşlemesi (`core/push/links.ts`), oturum sağlayıcısı, logo ve marka adı. Uygulamaların `core/*` dosyaları bu paketi uygulamanın değerleriyle kuran ince sarmalayıcılardır; ekranlar eskisi gibi `@/core/...` yollarından içe aktarır.

## Bağlantı

- Uygulamalar paketi `"@hakan/mobile-core": "file:../packages/mobile-core"` ile alır; `npm ci` `node_modules/@hakan/mobile-core` altına bağlantı (symlink) kurar.
- Paketin kendi `node_modules`'ı yoktur. `react`, `react-native`, `expo-*` ve Sentry eş bağımlılıktır (`peerDependencies`), uygulamanın kurduğu sürüm kullanılır.
- Metro (`metro.config.js`): paket klasörü `watchFolders`'a eklenir, paketin içe aktardığı modüller uygulamanın `node_modules`'ından çözülür (`resolver.nodeModulesPaths`). Böylece React tek kopya kalır.
- TypeScript (`tsconfig.json`): `preserveSymlinks`, paket dosyalarını bağlantı yolundan (`<uygulama>/node_modules/@hakan/mobile-core/...`) görür; içe aktarmalar uygulamanın tiplerinden çözülür.
- Paket TypeScript kaynağı olarak dağıtılır (derleme adımı yok). Paket içi içe aktarmalar `.ts` uzantılıdır ki `node --test` doğrudan çalıştırabilsin.

## Testler

```bash
npm test
```

Saf modüllerin (istemci, zarf, oturum, açılış, bağlantı) birim testleri. Tip denetimi ve lint iki uygulamanın kendi adımlarında koşar. CI'da `packages/mobile-core/**` değişince `mobile` ve `mobile-office` işleri birlikte çalışır.
