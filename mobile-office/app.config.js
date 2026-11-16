// Kaynak: mobile/app.config.js (#236). app.json'a dinamik eklemeler.
//
// EAS: `eas init` proje kimliğini yazar; o zamana kadar EAS_OFFICE_PROJECT_ID'den okunur
// (mükellef uygulamasından ayrı bir EAS projesi). Kimlik yoksa push tokenı alınamaz.
//
// Mağaza derlemesi (EXPO_PUBLIC_APP_ENV=production) https API ister.
function assertStoreConfig() {
  if (process.env.EXPO_PUBLIC_APP_ENV !== 'production') return;
  const apiUrl = process.env.EXPO_PUBLIC_API_URL;
  if (!apiUrl || !/^https:\/\//i.test(apiUrl)) {
    throw new Error('Store build (EXPO_PUBLIC_APP_ENV=production): EXPO_PUBLIC_API_URL must be an https:// URL');
  }
}

module.exports = ({ config }) => {
  assertStoreConfig();
  const projectId = config.extra?.eas?.projectId ?? process.env.EAS_OFFICE_PROJECT_ID?.trim();
  return {
    ...config,
    runtimeVersion: { policy: 'fingerprint' },
    extra: {
      ...config.extra,
      ...(projectId ? { eas: { ...config.extra?.eas, projectId } } : {}),
    },
  };
};
