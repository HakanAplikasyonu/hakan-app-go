// Dynamic additions to app.json (Expo passes the static config in as `config`).
//
// EAS: `eas init` writes the project id; until then it can come from EAS_PROJECT_ID.
// Without a project id push tokens cannot be fetched and the CI release job skips.
//
// Store builds (EXPO_PUBLIC_APP_ENV=production) must talk to an https API.
const { withAndroidManifest } = require('expo/config-plugins');

function assertStoreConfig() {
  if (process.env.EXPO_PUBLIC_APP_ENV !== 'production') return;
  const apiUrl = process.env.EXPO_PUBLIC_API_URL;
  if (!apiUrl || !/^https:\/\//i.test(apiUrl)) {
    throw new Error('Store build (EXPO_PUBLIC_APP_ENV=production): EXPO_PUBLIC_API_URL must be an https:// URL');
  }
}

// Universal links (iOS) and verified app links (Android) for the web panel's
// mükellef paths (/m/...), #201. EXPO_PUBLIC_APP_LINK_DOMAIN is the panel's
// host (e.g. app.example.com); empty: only the hakan:// scheme works. The
// domain must also serve apple-app-site-association and assetlinks.json
// (docs: mobile/README.md, "Bildirim ve bağlantılar").
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
          data: [{ scheme: 'https', host: domain, pathPrefix: '/m/' }],
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
  const projectId = config.extra?.eas?.projectId ?? process.env.EAS_PROJECT_ID?.trim();
  return withoutGwpAsan({
    ...config,
    ...appLinks(config),
    // OTA (#203): an update reaches only builds of the same app version
    // (runtimeVersion = version in app.json) on the build's EAS channel
    // (eas.json: development / preview / production). Without an EAS
    // project id updates are off.
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
