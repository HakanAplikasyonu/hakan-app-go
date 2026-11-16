/**
 * Crash reporting (#202, #295). Sentry is on only when a DSN is set; without
 * it every call here is a no-op. No personal data is attached: no user e-mail
 * or name, only the user's uuid when signed in.
 */
import * as Sentry from '@sentry/react-native';

export function createReporter(cfg: { dsn: string; environment: string }) {
  const sentryEnabled = cfg.dsn !== '';
  let started = false;

  function initSentry() {
    if (!sentryEnabled || started) return;
    started = true;
    Sentry.init({
      dsn: cfg.dsn,
      environment: cfg.environment,
      sendDefaultPii: false,
      tracesSampleRate: 0,
    });
  }

  function reportError(error: unknown, context?: Record<string, string>) {
    if (!sentryEnabled) return;
    Sentry.captureException(error, context ? { tags: context } : undefined);
  }

  /** Ties reports to the signed-in user by uuid only (null signs out). */
  function setReportUser(uuid: string | null) {
    if (!sentryEnabled) return;
    Sentry.setUser(uuid ? { id: uuid } : null);
  }

  return { sentryEnabled, initSentry, reportError, setReportUser, wrapRoot: Sentry.wrap };
}
