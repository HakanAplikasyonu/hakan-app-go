/**
 * Runtime browser config as a tiny script (`window.__APP_ENV__ = {...}`).
 * Loaded before hydration by the root layout so instrumentation-client.ts can
 * read the Sentry DSN from the container env (SENTRY_DSN) instead of a
 * build-time NEXT_PUBLIC_* variable. A DSN is public by design.
 *
 * ClickFix widget ids (#387) come the same way: the image is prebuilt by CI,
 * so a `process.env.NEXT_PUBLIC_*` member access would be inlined (empty) at
 * build time. `runtimeEnv` reads by computed key, which Next leaves alone.
 */
export const dynamic = "force-dynamic";

function runtimeEnv(key: string): string {
  return process.env[key]?.trim() || "";
}

export function GET() {
  const payload = {
    sentryDsn: runtimeEnv("SENTRY_DSN"),
    sentryEnvironment:
      runtimeEnv("SENTRY_ENVIRONMENT") || process.env.NODE_ENV || "",
    clickfix: {
      scriptUrl: runtimeEnv("NEXT_PUBLIC_CLICKFIX_SCRIPT_URL"),
      widgetId: runtimeEnv("NEXT_PUBLIC_CLICKFIX_WIDGET_ID"),
      taxpayerWidgetId: runtimeEnv("NEXT_PUBLIC_CLICKFIX_TAXPAYER_WIDGET_ID"),
    },
  };
  const json = JSON.stringify(payload).replace(/</g, "\\u003c");
  // instrumentation-client.ts registers __onAppEnv before this runs.
  const body = `window.__APP_ENV__=${json};window.__onAppEnv&&window.__onAppEnv(window.__APP_ENV__);`;
  return new Response(body, {
    headers: {
      "Content-Type": "application/javascript; charset=utf-8",
      "Cache-Control": "no-store",
    },
  });
}
