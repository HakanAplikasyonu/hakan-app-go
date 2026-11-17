// ClickFix Web Widget in a WebView (#387). Pure helpers (no React Native
// globals) so node --test can import them; the screen is ui/destek.tsx.
//
// Contract: ClickFix "Mobil: webview" guide and its bridge (bridge_version 1):
// the app opens <embed>?widget=<id>&locale=tr, waits for clickfix:ready, then
// sends clickfix:identify (verified, user_hash from our server) or
// clickfix:update (name / e-mail / attributes, unverified). clickfix:link
// opens in the system browser; clickfix:event "close" leaves the screen.

export const DEFAULT_CLICKFIX_EMBED_URL = 'https://getclickfix.com/widget-embed.html';

const WIDGET_ID = /^[A-Za-z0-9_-]{1,128}$/;

/** A widget id, or null (empty / malformed = no support screen). */
export function cleanWidgetId(value: string | null | undefined): string | null {
  const v = (value ?? '').trim();
  return WIDGET_ID.test(v) ? v : null;
}

/** The hosted embed page: https only, else the public ClickFix page. */
export function cleanEmbedUrl(value: string | null | undefined): string {
  const v = (value ?? '').trim();
  if (!v) return DEFAULT_CLICKFIX_EMBED_URL;
  try {
    const u = new URL(v);
    if (u.protocol === 'https:' && !u.search && !u.hash) return u.toString();
  } catch {
    // fall through
  }
  return DEFAULT_CLICKFIX_EMBED_URL;
}

/** Origin the WebView may navigate within (the embed page's own). */
export function embedOrigin(embedUrl: string): string {
  return new URL(cleanEmbedUrl(embedUrl)).origin;
}

export function embedPageUrl(
  embedUrl: string,
  widgetId: string,
  opts: { locale?: string; theme?: 'light' | 'dark' | 'auto' } = {},
): string {
  const u = new URL(cleanEmbedUrl(embedUrl));
  u.searchParams.set('widget', widgetId);
  u.searchParams.set('locale', opts.locale?.toLowerCase().startsWith('en') ? 'en' : 'tr');
  u.searchParams.set('theme', opts.theme ?? 'auto');
  return u.toString();
}

/** True when the WebView may load url (same origin as the embed page). */
export function isAllowedNavigation(url: string, origin: string): boolean {
  if (url === 'about:blank') return true;
  try {
    return new URL(url).origin === origin;
  } catch {
    return false;
  }
}

/** A link from clickfix:link, re-checked: https only. */
export function externalLink(url: unknown): string | null {
  if (typeof url !== 'string') return null;
  try {
    const u = new URL(url);
    return u.protocol === 'https:' ? u.toString() : null;
  } catch {
    return null;
  }
}

export type BridgeMessage =
  | { type: 'ready' }
  | { type: 'link'; url: string }
  | { type: 'close' }
  | { type: 'unread'; count: number }
  | { type: 'other' };

/** Widget → app message (ReactNativeWebView.postMessage text). */
export function parseBridgeMessage(raw: string): BridgeMessage | null {
  let m: unknown;
  try {
    m = JSON.parse(raw);
  } catch {
    return null;
  }
  if (!m || typeof m !== 'object') return null;
  const msg = m as Record<string, unknown>;
  switch (msg.type) {
    case 'clickfix:ready':
      return { type: 'ready' };
    case 'clickfix:link': {
      const url = externalLink(msg.url);
      return url ? { type: 'link', url } : { type: 'other' };
    }
    case 'clickfix:event': {
      if (msg.event === 'close') return { type: 'close' };
      if (msg.event === 'unread') {
        const data = (msg.payload ?? {}) as { count?: unknown };
        return { type: 'unread', count: Number(data.count) || 0 };
      }
      return { type: 'other' };
    }
    default:
      return typeof msg.type === 'string' && msg.type.startsWith('clickfix:') ? { type: 'other' } : null;
  }
}

export type SupportVisitor = {
  name?: string;
  email?: string;
  attributes?: Record<string, string>;
};

export type SupportIdentity = {
  user_id: string;
  user_hash: string;
  company?: { id: string; name?: string };
};

/** App → widget commands sent after clickfix:ready. */
export function visitorCommands(visitor: SupportVisitor, identity: SupportIdentity | null): object[] {
  const attributes = compactAttributes(visitor.attributes ?? {});
  if (identity) {
    return [
      {
        type: 'clickfix:identify',
        id: 'identify',
        user: {
          ...identity,
          ...(visitor.name ? { name: visitor.name } : {}),
          ...(visitor.email ? { email: visitor.email } : {}),
          ...(Object.keys(attributes).length ? { attributes } : {}),
        },
      },
    ];
  }
  const update: Record<string, unknown> = { ...attributes };
  if (visitor.name) update.name = visitor.name;
  if (visitor.email) update.email = visitor.email;
  return Object.keys(update).length ? [{ type: 'clickfix:update', update }] : [];
}

/** JavaScript that hands one command to window.ClickFixBridge. */
export function bridgeScript(command: object): string {
  const text = JSON.stringify(JSON.stringify(command));
  return `window.ClickFixBridge && window.ClickFixBridge.receive(${text}); true;`;
}

export function compactAttributes(attrs: Record<string, string | null | undefined>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(attrs)) {
    const s = (v ?? '').trim();
    if (s) out[k] = s;
  }
  return out;
}

/**
 * Runs in the embed page before the widget boots: when the device's last
 * support user differs from userKey, the widget's storage on that origin
 * (local/session storage, cookies) is cleared, so a shared device never
 * shows another account's conversations.
 */
export function forgetOtherUserScript(userKey: string): string {
  const key = JSON.stringify(userKey);
  return `(function(){try{var k="hakan_support_user";if(localStorage.getItem(k)!==${key}){localStorage.clear();sessionStorage.clear();document.cookie.split(";").forEach(function(c){var n=c.split("=")[0].trim();if(n)document.cookie=n+"=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/";});localStorage.setItem(k,${key});}}catch(e){}})();true;`;
}
