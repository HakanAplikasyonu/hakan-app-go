/**
 * Maps notification URLs and deep links to app routes (#201).
 *
 * The backend puts the web path of a notification in the push `data.url`
 * (backend/internal/modules/notifications/usecase/expo.go): the mükellef panel
 * lives under `/m/{taxpayer}/...`. The same paths arrive as universal / app
 * links (`https://{domain}/m/...`), and the app's own scheme gives short forms
 * (`hakan://tasks`, `hakan://receipts/{id}`).
 *
 * Pure functions only: no React Native import, so `node --test` runs them.
 */

/** Where to go: an app route and, when the link names one, the taxpayer it belongs to. */
export type LinkTarget = { route: AppRoute; taxpayer?: string };

export type AppRoute = '/' | '/tasks' | `/task/${string}` | '/receipts' | `/receipts/${string}` | '/capture';

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const ID = /^[A-Za-z0-9_-]{1,64}$/;

/** Path segments of a URL, scheme link or plain path; [] when unusable. */
function segments(input: string): string[] {
  let s = input.trim();
  if (!s) return [];
  const scheme = /^([a-z][a-z0-9+.-]*):\/\/(.*)$/i.exec(s);
  if (scheme) {
    const proto = scheme[1].toLowerCase();
    const rest = scheme[2];
    if (proto === 'http' || proto === 'https') {
      const slash = rest.indexOf('/');
      s = slash < 0 ? '/' : rest.slice(slash);
    } else {
      // hakan://tasks → host is the first segment; hakan:///tasks works too.
      s = '/' + rest;
    }
  }
  s = s.split(/[?#]/)[0];
  return s
    .split('/')
    .filter(Boolean)
    .map((p) => {
      try {
        return decodeURIComponent(p);
      } catch {
        return '';
      }
    });
}

function app(parts: string[]): AppRoute | null {
  const [head, id, ...rest] = parts;
  if (rest.length > 0) return null;
  switch (head) {
    case undefined:
      return '/';
    case 'tasks':
    case 'task':
      if (id === undefined) return '/tasks';
      return UUID.test(id) ? `/task/${id.toLowerCase()}` : null;
    case 'receipts':
      if (id === undefined) return '/receipts';
      return ID.test(id) ? `/receipts/${id}` : null;
    case 'capture':
    case 'uploads':
      return id === undefined ? '/capture' : null;
    default:
      return null;
  }
}

/**
 * resolveLink maps a notification URL or a deep link to an app route, or null
 * when the link points somewhere the app does not show (the caller then
 * stays where it is).
 *
 *   /m/{taxpayer}/tasks           → /tasks (taxpayer)
 *   /m/{taxpayer}/tasks/{uuid}    → /task/{uuid}
 *   /m/{taxpayer}/receipts/{id}   → /receipts/{id}
 *   https://{domain}/m/...        → same as the path
 *   hakan://tasks, hakan://receipts/{id}, hakan://task/{uuid}
 */
export function resolveLink(input: string | null | undefined): LinkTarget | null {
  if (typeof input !== 'string') return null;
  const parts = segments(input);
  if (parts.length === 0) return input.trim() ? { route: '/' } : null;
  if (parts[0] === 'm') {
    const taxpayer = parts[1];
    if (taxpayer === undefined) return { route: '/' };
    if (!UUID.test(taxpayer)) return null;
    const route = app(parts.slice(2));
    return route ? { route, taxpayer: taxpayer.toLowerCase() } : null;
  }
  const route = app(parts);
  return route ? { route } : null;
}

/** The `url` of a push payload (data.url), if it is a string. */
export function notificationUrl(data: unknown): string | null {
  if (!data || typeof data !== 'object') return null;
  const url = (data as { url?: unknown }).url;
  return typeof url === 'string' && url ? url : null;
}

/**
 * Rewrites an incoming system path (universal link, scheme link) for
 * expo-router's +native-intent: a known link becomes its app route, anything
 * else opens the home screen instead of a "not found" page.
 */
export function systemPath(path: string): string {
  return resolveLink(path)?.route ?? '/';
}
