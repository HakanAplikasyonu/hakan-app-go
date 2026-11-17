/**
 * ClickFix Web Widget (#387): portalın destek sistemi. Ayrı bir destek
 * modülü yok; talepler ClickFix'e gider.
 *
 * İki kanal var:
 *  - `provider`: ofis paneli → yazılım sağlayıcısı (platform widget'ı,
 *    `NEXT_PUBLIC_CLICKFIX_WIDGET_ID`).
 *  - `taxpayer`: mükellef portalı → mükellefin kendi mali müşavirlik ofisi
 *    (ofis ayarındaki widget kimliği; boşsa platform varsayılanı
 *    `NEXT_PUBLIC_CLICKFIX_TAXPAYER_WIDGET_ID`).
 *
 * Kimlik boşsa hiçbir şey yüklenmez. Değerler imaja gömülmez: konteyner
 * ortamından /api/client-env ile çalışma anında gelir.
 *
 * Embed biçimi ClickFix'in herkese açık dokümanından (Web widget →
 * JavaScript API): kuyruk fonksiyonu + `ClickFix("init", {...})` +
 * `<script src=".../widget.js" async>`.
 */

export type ClickFixChannel = "provider" | "taxpayer";

export const DEFAULT_CLICKFIX_SCRIPT_URL = "https://getclickfix.com/widget.js";

/** Runtime config delivered by /api/client-env. */
export type ClickFixRuntimeEnv = {
  scriptUrl?: string;
  widgetId?: string;
  taxpayerWidgetId?: string;
};

const WIDGET_ID = /^[A-Za-z0-9_-]{1,128}$/;

/** A widget id the loader accepts, or null (empty / malformed = off). */
export function cleanWidgetId(value: string | null | undefined): string | null {
  const v = (value ?? "").trim();
  return WIDGET_ID.test(v) ? v : null;
}

/**
 * Script URL: https only (http is allowed for localhost while developing a
 * widget locally). Anything else falls back to the public ClickFix URL.
 */
export function cleanScriptUrl(value: string | null | undefined): string {
  const v = (value ?? "").trim();
  if (!v) return DEFAULT_CLICKFIX_SCRIPT_URL;
  try {
    const url = new URL(v);
    if (url.protocol === "https:") return url.toString();
    if (
      url.protocol === "http:" &&
      (url.hostname === "localhost" || url.hostname === "127.0.0.1")
    )
      return url.toString();
  } catch {
    // fall through
  }
  return DEFAULT_CLICKFIX_SCRIPT_URL;
}

/**
 * Widget id of a channel: the taxpayer channel prefers the office's own
 * widget (office settings) and falls back to the platform default.
 */
export function resolveWidgetId(
  channel: ClickFixChannel,
  env: ClickFixRuntimeEnv | null | undefined,
  officeWidgetId?: string | null,
): string | null {
  if (channel === "provider") return cleanWidgetId(env?.widgetId);
  return cleanWidgetId(officeWidgetId) ?? cleanWidgetId(env?.taxpayerWidgetId);
}

/** Who is writing and from where; passed to the widget as visitor data. */
export type ClickFixVisitor = {
  name?: string;
  email?: string;
  /** Custom-field values (ClickFix → Ayarlar → Özel alanlar keys). */
  attributes?: Record<string, string>;
};

/** Verified identity (user_hash computed server-side), when available. */
export type ClickFixIdentity = {
  user_id: string;
  user_hash: string;
  company?: { id: string; name?: string };
};

export type ClickFixInit = {
  widgetId: string;
  locale: "tr" | "en";
  hideLauncher: true;
};

export function buildInit(widgetId: string, locale: string): ClickFixInit {
  return {
    widgetId,
    locale: locale.toLowerCase().startsWith("en") ? "en" : "tr",
    // Balon yok: widget üst çubuktaki "Destek" düğmesinden açılır (ofis
    // panelinde alt sekme çubuğu, portalda içerik ile çakışmasın).
    hideLauncher: true,
  };
}

/** Drops empty values; ClickFix rejects unknown/empty custom fields loudly. */
export function compactAttributes(
  attrs: Record<string, string | null | undefined>,
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(attrs)) {
    const s = (v ?? "").trim();
    if (s) out[k] = s;
  }
  return out;
}

type ClickFixFn = ((...args: unknown[]) => unknown) & { q?: unknown[] };
type ClickFixWindow = Window & { ClickFix?: ClickFixFn };

/** The SDK's queue snippet: calls made before widget.js loads are queued. */
export function clickFix(): ClickFixFn {
  const win = window as ClickFixWindow;
  if (!win.ClickFix) {
    const fn: ClickFixFn = (...args: unknown[]) => {
      (fn.q = fn.q || []).push(args);
    };
    win.ClickFix = fn;
  }
  return win.ClickFix;
}

/** Initializes the widget and passes the visitor; returns the cleanup. */
export function startClickFix(
  init: ClickFixInit,
  visitor: ClickFixVisitor,
  identity: ClickFixIdentity | null,
  onUnread: (count: number) => void,
): () => void {
  const cf = clickFix();
  cf("init", init);
  if (identity) {
    cf("identify", {
      ...identity,
      name: visitor.name,
      email: visitor.email,
      attributes: visitor.attributes,
    });
  } else {
    const profile: Record<string, string> = {};
    if (visitor.name) profile.name = visitor.name;
    if (visitor.email) profile.email = visitor.email;
    if (Object.keys(profile).length) cf("update", profile);
    if (visitor.attributes && Object.keys(visitor.attributes).length)
      cf("setAttributes", visitor.attributes);
  }
  cf("on", "unread", (e: { count?: number } | undefined) =>
    onUnread(Number(e?.count) || 0),
  );
  // Shutdown only: the visitor (and its conversations) survives navigation
  // between shells; `resetClickFix` forgets it on sign-out.
  return () => (window as ClickFixWindow).ClickFix?.("shutdown");
}

/** Forgets the visitor (sign-out), so the next user on this browser starts clean. */
export function resetClickFix(): void {
  (window as ClickFixWindow).ClickFix?.("reset");
}

/** Opens the chat; false when the widget is not on the page. */
export function openClickFix(): boolean {
  const fn = (window as ClickFixWindow).ClickFix;
  if (!fn) return false;
  fn("open");
  return true;
}
