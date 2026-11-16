/**
 * Bildirim adreslerini ve derin bağlantıları uygulama ekranlarına eşler (#241).
 * Kalıp: mobile/src/core/push/links.ts (#201); ofis paneli yollarına uyarlandı.
 *
 * Backend bildirimin web yolunu push `data.url` içinde gönderir. Ofis paneli
 * `/t/{ofis-slug}/...` altındadır:
 *   /t/{slug}                                   → /           (ofis)
 *   /t/{slug}/engagements/{eng}?task={uuid}     → /tasks/{uuid}?engagement={eng}
 *   /t/{slug}/engagements/{eng}                 → /clients/{eng}
 *   /t/{slug}/warnings[?engagement={eng}]       → /warnings
 *   /t/{slug}/review                            → /review
 * Aynı yollar evrensel bağlantı olarak (`https://{alan}/t/...`), kısa biçimler
 * uygulamanın şemasıyla gelir: `hakanofis://warnings`, `hakanofis://review`,
 * `hakanofis://clients/{eng}`, `hakanofis://tasks/{uuid}?engagement={eng}`.
 *
 * Saf modül: React Native içe aktarmaz, node --test ile sınanır.
 */

export type LinkTarget = { route: string; params?: Record<string, string>; office?: string };

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const SLUG = /^[a-z0-9][a-z0-9-]{0,63}$/i;

type Parsed = { parts: string[]; query: URLSearchParams };

function parse(input: string): Parsed | null {
  let s = input.trim();
  if (!s) return null;
  const scheme = /^([a-z][a-z0-9+.-]*):\/\/(.*)$/i.exec(s);
  if (scheme) {
    const proto = scheme[1].toLowerCase();
    const rest = scheme[2];
    if (proto === 'http' || proto === 'https') {
      const slash = rest.indexOf('/');
      s = slash < 0 ? '/' : rest.slice(slash);
    } else {
      // hakanofis://warnings → ilk parça host; hakanofis:///warnings da olur.
      s = '/' + rest;
    }
  }
  const [pathAndQuery] = s.split('#');
  const q = pathAndQuery.indexOf('?');
  const path = q < 0 ? pathAndQuery : pathAndQuery.slice(0, q);
  const query = new URLSearchParams(q < 0 ? '' : pathAndQuery.slice(q + 1));
  const parts = path
    .split('/')
    .filter(Boolean)
    .map((p) => {
      try {
        return decodeURIComponent(p);
      } catch {
        return '';
      }
    });
  if (parts.some((p) => p === '')) return null;
  return { parts, query };
}

const lower = (s: string) => s.toLowerCase();

function app(parts: string[], query: URLSearchParams): Omit<LinkTarget, 'office'> | null {
  const [head, id, ...rest] = parts;
  if (rest.length > 0) return null;
  const engagement = query.get('engagement');
  switch (head) {
    case undefined:
      return { route: '/' };
    case 'warnings':
      if (id !== undefined) return null;
      return engagement && UUID.test(engagement) ? { route: '/warnings', params: { engagement: lower(engagement) } } : { route: '/warnings' };
    case 'review':
      return id === undefined ? { route: '/review' } : null;
    case 'clients':
    case 'taxpayers':
      if (id === undefined) return { route: '/clients' };
      return UUID.test(id) ? { route: '/clients/[engagement]', params: { engagement: lower(id) } } : null;
    case 'engagements': {
      if (id === undefined || !UUID.test(id)) return null;
      const task = query.get('task');
      if (task && UUID.test(task)) return { route: '/tasks/[uuid]', params: { uuid: lower(task), engagement: lower(id) } };
      return { route: '/clients/[engagement]', params: { engagement: lower(id) } };
    }
    case 'tasks':
      if (id === undefined || !UUID.test(id) || !engagement || !UUID.test(engagement)) return null;
      return { route: '/tasks/[uuid]', params: { uuid: lower(id), engagement: lower(engagement) } };
    default:
      return null;
  }
}

/**
 * Bildirim adresini ya da derin bağlantıyı ekrana eşler; uygulamada karşılığı
 * olmayan bağlantı için null (çağıran olduğu yerde kalır).
 */
export function resolveLink(input: string | null | undefined): LinkTarget | null {
  if (typeof input !== 'string') return null;
  const p = parse(input);
  if (!p) return null;
  if (p.parts.length === 0) return input.trim() ? { route: '/' } : null;
  if (p.parts[0] === 't') {
    const office = p.parts[1];
    if (office === undefined) return { route: '/' };
    if (!SLUG.test(office)) return null;
    const target = app(p.parts.slice(2), p.query);
    return target ? { ...target, office: lower(office) } : null;
  }
  return app(p.parts, p.query);
}

/** Push verisindeki `url` (data.url). */
export function notificationUrl(data: unknown): string | null {
  if (!data || typeof data !== 'object') return null;
  const url = (data as { url?: unknown }).url;
  return typeof url === 'string' && url ? url : null;
}

/** expo-router yolu: `/tasks/[uuid]` + params → `/tasks/x?engagement=y`. */
export function hrefOf(t: Pick<LinkTarget, 'route' | 'params'>): string {
  let path = t.route;
  const rest = new URLSearchParams();
  for (const [k, v] of Object.entries(t.params ?? {})) {
    if (path.includes(`[${k}]`)) path = path.replace(`[${k}]`, encodeURIComponent(v));
    else rest.set(k, v);
  }
  const qs = rest.toString();
  return qs ? `${path}?${qs}` : path;
}

/**
 * +native-intent için: bilinen bağlantı uygulama yoluna dönüşür, bilinmeyen
 * bağlantı "bulunamadı" yerine ana sayfayı açar. Ofis değiştirme yalnızca
 * bildirime dokununca yapılır (use-notification-routing).
 */
export function systemPath(path: string): string {
  const t = resolveLink(path);
  return t ? hrefOf(t) : '/';
}
