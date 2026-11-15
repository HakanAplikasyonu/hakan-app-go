// Dynamic additions to app.json (Expo passes the static config in as `config`).
//
// EAS: `eas init` writes the project id; until then it can come from EAS_PROJECT_ID.
// Without a project id push tokens cannot be fetched and the CI release job skips.
//
// Store builds (EXPO_PUBLIC_APP_ENV=production) must talk to an https API.
function assertStoreConfig() {
  if (process.env.EXPO_PUBLIC_APP_ENV !== 'production') return;
  const apiUrl = process.env.EXPO_PUBLIC_API_URL;
  if (!apiUrl || !/^https:\/\//i.test(apiUrl)) {
    throw new Error('Store build (EXPO_PUBLIC_APP_ENV=production): EXPO_PUBLIC_API_URL must be an https:// URL');
  }
}

module.exports = ({ config }) => {
  assertStoreConfig();
  const projectId = config.extra?.eas?.projectId ?? process.env.EAS_PROJECT_ID?.trim();
  return {
    ...config,
    runtimeVersion: { policy: 'fingerprint' },
    extra: {
      ...config.extra,
      ...(projectId ? { eas: { ...config.extra?.eas, projectId } } : {}),
    },
  };
};
