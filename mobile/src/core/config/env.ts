/**
 * Runtime configuration from EXPO_PUBLIC_* variables (inlined by Metro at build time).
 *
 *   EXPO_PUBLIC_API_URL=https://api.example.com   Go API origin, without /v1
 *   EXPO_PUBLIC_APP_ENV=development|preview|production
 *   EXPO_PUBLIC_PRIVACY_URL=https://…           gizlilik metni (profil ekranı); boş: gizlenir
 *   EXPO_PUBLIC_SUPPORT_URL=https://… | mailto:  destek (profil ekranı); boş: gizlenir
 *   EXPO_PUBLIC_SENTRY_DSN=https://…@…/…         çökme raporu; boş: Sentry kapalı
 *   EXPO_PUBLIC_CLICKFIX_TAXPAYER_WIDGET_ID=…     destek (#387): ofisin kendi widget'ı yoksa
 *                                                 kullanılan platform varsayılanı; ikisi de boşsa
 *                                                 destek ekranı yok (EXPO_PUBLIC_SUPPORT_URL'e düşer)
 *   EXPO_PUBLIC_CLICKFIX_EMBED_URL=https://…      ClickFix webview sayfası; boş: getclickfix.com
 *
 * Unset API URL → http://localhost:8080 (Metro dev builds). app.config.js refuses a
 * production build without an https URL.
 */
import { safeLink } from './safe-link';

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
  privacyUrl: safeLink(process.env.EXPO_PUBLIC_PRIVACY_URL),
  supportUrl: safeLink(process.env.EXPO_PUBLIC_SUPPORT_URL, true),
  sentryDsn: safeLink(process.env.EXPO_PUBLIC_SENTRY_DSN),
  clickfixTaxpayerWidgetId: process.env.EXPO_PUBLIC_CLICKFIX_TAXPAYER_WIDGET_ID?.trim() ?? '',
  clickfixEmbedUrl: safeLink(process.env.EXPO_PUBLIC_CLICKFIX_EMBED_URL),
} as const;
