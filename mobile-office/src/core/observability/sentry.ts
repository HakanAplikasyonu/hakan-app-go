// Kaynak: mobile/src/core/observability/sentry.ts (#241'de kopyalandı).
/**
 * Crash reporting (#202). Sentry is on only when EXPO_PUBLIC_SENTRY_DSN is set;
 * without it every call here is a no-op. No personal data is attached: no
 * user e-mail or name, only the user's uuid when signed in.
 */
import * as Sentry from '@sentry/react-native';

import { appEnv, env } from '../config/env';

export const sentryEnabled = env.sentryDsn !== '';

let started = false;

export function initSentry() {
  if (!sentryEnabled || started) return;
  started = true;
  Sentry.init({
    dsn: env.sentryDsn,
    environment: appEnv,
    sendDefaultPii: false,
    tracesSampleRate: 0,
  });
}

export function reportError(error: unknown, context?: Record<string, string>) {
  if (!sentryEnabled) return;
  Sentry.captureException(error, context ? { tags: context } : undefined);
}

/** Ties reports to the signed-in user by uuid only (null signs out). */
export function setReportUser(uuid: string | null) {
  if (!sentryEnabled) return;
  Sentry.setUser(uuid ? { id: uuid } : null);
}

export const wrapRoot = Sentry.wrap;
