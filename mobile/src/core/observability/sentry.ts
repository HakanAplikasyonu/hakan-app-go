/**
 * Crash reporting (#202; shared, @hakan/mobile-core #295). Sentry is on only
 * when EXPO_PUBLIC_SENTRY_DSN is set.
 */
import { createReporter } from '@hakan/mobile-core/observability/sentry';

import { appEnv, env } from '../config/env';

export const { sentryEnabled, initSentry, reportError, setReportUser, wrapRoot } = createReporter({
  dsn: env.sentryDsn,
  environment: appEnv,
});
