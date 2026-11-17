"use client";

import Script from "next/script";
import { useEffect, useMemo, useSyncExternalStore } from "react";

import {
  buildInit,
  cleanScriptUrl,
  resolveWidgetId,
  startClickFix,
  type ClickFixChannel,
  type ClickFixIdentity,
  type ClickFixRuntimeEnv,
  type ClickFixVisitor,
} from "@/lib/support/clickfix";
import { useSupportStore } from "@/lib/support/support-store";
import { useLocale } from "@/providers/locale-provider";

function runtimeClickFixEnv(): ClickFixRuntimeEnv | null {
  const env = (window as { __APP_ENV__?: { clickfix?: ClickFixRuntimeEnv } })
    .__APP_ENV__;
  return env?.clickfix ?? null;
}

const noSubscribe = () => () => {};

/** ClickFix runtime config (/api/client-env); null on the server. */
export function useClickFixEnv(): ClickFixRuntimeEnv | null {
  return useSyncExternalStore(noSubscribe, runtimeClickFixEnv, () => null);
}

/**
 * Loads the ClickFix web widget for one channel (#387). Renders nothing and
 * loads nothing when the channel has no widget id. The launcher bubble is
 * hidden; `SupportButton` opens the chat.
 */
export function ClickFixWidget({
  channel,
  officeWidgetId,
  visitor,
  identity = null,
}: {
  channel: ClickFixChannel;
  /** Taxpayer channel: the office's own widget id (office settings). */
  officeWidgetId?: string | null;
  visitor: ClickFixVisitor;
  identity?: ClickFixIdentity | null;
}) {
  const { locale } = useLocale();
  const setAvailable = useSupportStore((s) => s.setAvailable);
  const setUnread = useSupportStore((s) => s.setUnread);
  // __APP_ENV__ is set once by a beforeInteractive script; the server
  // snapshot is null so server render and hydration match.
  const env = useSyncExternalStore(noSubscribe, runtimeClickFixEnv, () => null);

  const widgetId = env ? resolveWidgetId(channel, env, officeWidgetId) : null;
  const scriptUrl = cleanScriptUrl(env?.scriptUrl);
  // Stable key so the widget restarts only when who/where actually changes.
  const visitorKey = useMemo(() => JSON.stringify(visitor), [visitor]);
  const identityKey = identity ? JSON.stringify(identity) : "";

  useEffect(() => {
    if (!widgetId) return;
    const stop = startClickFix(
      buildInit(widgetId, locale),
      JSON.parse(visitorKey) as ClickFixVisitor,
      identityKey ? (JSON.parse(identityKey) as ClickFixIdentity) : null,
      setUnread,
    );
    setAvailable(true);
    return () => {
      stop();
      setAvailable(false);
    };
  }, [widgetId, locale, visitorKey, identityKey, setAvailable, setUnread]);

  if (!widgetId) return null;
  return <Script id="clickfix-widget" src={scriptUrl} strategy="lazyOnload" />;
}
