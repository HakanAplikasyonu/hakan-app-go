// Kaynak: mobile/src/core/config/env.ts (#236'da kopyalandı; ortak paket ayrı issue).
/**
 * Runtime configuration from EXPO_PUBLIC_* variables (inlined by Metro at build time).
 *
 *   EXPO_PUBLIC_API_URL=https://api.example.com   Go API origin, without /v1
 *   EXPO_PUBLIC_APP_ENV=development|preview|production
 *
 * Unset API URL → http://localhost:8080 (Metro dev builds). app.config.js refuses a
 * production build without an https URL.
 */
export type AppEnv = 'development' | 'preview' | 'production';

const rawAppEnv = process.env.EXPO_PUBLIC_APP_ENV;

export const appEnv: AppEnv =
  rawAppEnv === 'production' || rawAppEnv === 'preview' || rawAppEnv === 'development'
    ? rawAppEnv
    : __DEV__
      ? 'development'
      : 'preview';

export const env = {
  apiUrl: (process.env.EXPO_PUBLIC_API_URL?.trim() || 'http://localhost:8080').replace(/\/$/, ''),
  requestTimeoutMs: 20000,
} as const;
