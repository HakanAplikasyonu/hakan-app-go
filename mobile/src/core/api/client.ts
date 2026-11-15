import { env } from '../config/env';
import { session, type Tokens } from '../auth/session';
import { ApiError, unwrap } from './envelope';

export type Query = Record<string, string | number | boolean | undefined | null>;

export type RequestOptions = {
  query?: Query;
  body?: unknown;
  /** Mükellef scope of `/v1/taxpayer/*` requests (`X-Taxpayer`). */
  taxpayer?: string;
  /** No auth header, no refresh (login, refresh, public endpoints). */
  anonymous?: boolean;
};

function queryString(query?: Query) {
  if (!query) return '';
  const parts: string[] = [];
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined || v === null || v === '') continue;
    parts.push(`${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`);
  }
  return parts.length ? `?${parts.join('&')}` : '';
}

async function send(method: string, path: string, opts: RequestOptions, accessToken: string | null) {
  const headers: Record<string, string> = { Accept: 'application/json' };
  if (opts.body !== undefined) headers['Content-Type'] = 'application/json';
  if (accessToken) headers.Authorization = `Bearer ${accessToken}`;
  if (opts.taxpayer) headers['X-Taxpayer'] = opts.taxpayer;

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), env.requestTimeoutMs);
  try {
    const res = await fetch(`${env.apiUrl}${path}${queryString(opts.query)}`, {
      method,
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      signal: controller.signal,
    });
    const payload = await res.json().catch(() => undefined);
    return { status: res.status, payload, requestId: res.headers.get('X-Request-ID') ?? undefined };
  } catch {
    const timedOut = controller.signal.aborted;
    throw new ApiError({ status: 0, code: timedOut ? 'TIMEOUT' : 'NETWORK_ERROR', message: 'Network request failed' });
  } finally {
    clearTimeout(timer);
  }
}

let refreshing: Promise<boolean> | null = null;

/** Rotates the refresh token once for all concurrent callers. False → session is over. */
function refresh(): Promise<boolean> {
  refreshing ??= (async () => {
    const current = session.get();
    if (!current) return false;
    try {
      const res = await send('POST', '/v1/auth/refresh', { body: { refresh_token: current.refreshToken } }, null);
      const tokens = unwrap<Tokens>(res.status, res.payload, res.requestId);
      await session.setTokens(tokens);
      return true;
    } catch (e) {
      // Network trouble keeps the session; a rejected token ends it.
      if (e instanceof ApiError && e.status === 0) throw e;
      await session.clear();
      return false;
    }
  })().finally(() => {
    refreshing = null;
  });
  return refreshing;
}

/** JSON request to the Go API; returns the envelope's `data`. */
export async function request<T>(method: string, path: string, opts: RequestOptions = {}): Promise<T> {
  if (opts.anonymous) {
    const res = await send(method, path, opts, null);
    return unwrap<T>(res.status, res.payload, res.requestId);
  }
  const current = session.get();
  if (!current) throw new ApiError({ status: 401, code: 'UNAUTHENTICATED', message: 'Not signed in' });
  if (current.accessExpiresAt - Date.now() < 30_000) await refresh();

  let res = await send(method, path, opts, session.get()?.accessToken ?? null);
  if (res.status === 401 && (await refresh())) {
    res = await send(method, path, opts, session.get()?.accessToken ?? null);
  }
  return unwrap<T>(res.status, res.payload, res.requestId);
}

export const api = {
  get: <T>(path: string, opts?: RequestOptions) => request<T>('GET', path, opts),
  post: <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('POST', path, { ...opts, body }),
  put: <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('PUT', path, { ...opts, body }),
  delete: <T>(path: string, opts?: RequestOptions) => request<T>('DELETE', path, opts),
};
