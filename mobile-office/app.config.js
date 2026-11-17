// Kaynak: mobile/app.config.js (#236, #242'de güncellendi). app.json'a dinamik eklemeler.
//
// EAS: `eas init` proje kimliğini yazar; o zamana kadar EAS_OFFICE_PROJECT_ID'den okunur
// (mükellef uygulamasından ayrı bir EAS projesi). Kimlik yoksa push tokenı alınamaz ve
// OTA güncellemesi kapalıdır.
//
// Mağaza derlemesi (EXPO_PUBLIC_APP_ENV=production) https API ister.
const { withAndroidManifest } = require('expo/config-plugins');

function assertStoreConfig() {
  if (process.env.EXPO_PUBLIC_APP_ENV !== 'production') return;
  const apiUrl = process.env.EXPO_PUBLIC_API_URL;
  if (!apiUrl || !/^https:\/\//i.test(apiUrl)) {
    throw new Error('Store build (EXPO_PUBLIC_APP_ENV=production): EXPO_PUBLIC_API_URL must be an https:// URL');
  }
}

// Evrensel bağlantılar (iOS) ve doğrulanmış uygulama bağlantıları (Android):
// ofis panelinin yolları (/t/...), #241. EXPO_PUBLIC_APP_LINK_DOMAIN panelin
// alan adıdır (ör. app.example.com); boşsa yalnızca hakanofis:// çalışır. Alan
// adı apple-app-site-association ve assetlinks.json dosyalarını da sunmalı
// (store/README.md, "Eksik dış bilgiler").
function appLinks(config) {
  const domain = process.env.EXPO_PUBLIC_APP_LINK_DOMAIN?.trim().replace(/^https?:\/\//, '').replace(/\/.*$/, '');
  if (!domain) return {};
  return {
    ios: { ...config.ios, associatedDomains: [`applinks:${domain}`] },
    android: {
      ...config.android,
      intentFilters: [
        {
          action: 'VIEW',
          autoVerify: true,
          data: [{ scheme: 'https', host: domain, pathPrefix: '/t/' }],
          category: ['BROWSABLE', 'DEFAULT'],
        },
      ],
    },
  };
}

// GWP-ASan kapalı (#454): Android 14'te uygulamalar için örneklemeli açık
// gelir; Hermes'in realloc'u örneklenince GWP-ASan yığın izini çerçeve
// işaretçisiyle toplarken (android_unsafe_frame_pointer_chase) SIGSEGV ile
// düşüyor. Maestro koşusunda uygulama açılışta kapanıyor, akış "Giriş yap"ı
// göremeden düşüyordu (gece koşusu, logcat'te libhermesvm.so + gwp_asan).
// Örnekleme rastgele olduğu için hata da aralıklıydı.
function withoutGwpAsan(config) {
  return withAndroidManifest(config, (c) => {
    const application = c.modResults.manifest.application?.[0];
    if (application) application.$['android:gwpAsanMode'] = 'never';
    return c;
  });
}

module.exports = ({ config }) => {
  assertStoreConfig();
  const projectId = config.extra?.eas?.projectId ?? process.env.EAS_OFFICE_PROJECT_ID?.trim();
  return withoutGwpAsan({
    ...config,
    ...appLinks(config),
    // OTA (#242): güncelleme yalnızca aynı uygulama sürümünün derlemelerine
    // (runtimeVersion = app.json sürümü), derlemenin EAS kanalında gider
    // (eas.json: development / preview / production).
    runtimeVersion: { policy: 'appVersion' },
    updates: projectId
      ? { url: `https://u.expo.dev/${projectId}`, enabled: true, checkAutomatically: 'ON_LOAD', fallbackToCacheTimeout: 0 }
      : { enabled: false },
    extra: {
      ...config.extra,
      ...(projectId ? { eas: { ...config.extra?.eas, projectId } } : {}),
    },
  });
};
