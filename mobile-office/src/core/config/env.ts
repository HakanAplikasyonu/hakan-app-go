// Kaynak: mobile/src/core/config/env.ts (#236'da kopyalandı; ortak paket ayrı issue).
/**
 * Runtime configuration from EXPO_PUBLIC_* variables (inlined by Metro at build time).
 *
 *   EXPO_PUBLIC_API_URL=https://api.example.com   Go API origin, without /v1
 *   EXPO_PUBLIC_APP_ENV=development|preview|production
 *   EXPO_PUBLIC_HELP_URL=https://…/yardim       web yardım merkezi (#522); boş: Yardım düğmesi gizlenir
 *   EXPO_PUBLIC_SENTRY_DSN=https://…@…/…         çökme raporu (#241); boş: Sentry kapalı
 *   EXPO_PUBLIC_CLICKFIX_WIDGET_ID=…              destek (#387): ofis → yazılım sağlayıcısı
 *                                                 ClickFix widget'ı; boş: destek ekranı yok
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
  helpUrl: safeLink(process.env.EXPO_PUBLIC_HELP_URL).replace(/\/$/, ''),
  sentryDsn: safeLink(process.env.EXPO_PUBLIC_SENTRY_DSN),
  clickfixWidgetId: process.env.EXPO_PUBLIC_CLICKFIX_WIDGET_ID?.trim() ?? '',
  clickfixEmbedUrl: safeLink(process.env.EXPO_PUBLIC_CLICKFIX_EMBED_URL),
} as const;
