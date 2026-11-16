/**
 * JSON client of the Go API (#295; mobile/ and mobile-office/ share it).
 *
 * Every request carries the access token; one refresh runs for all
 * concurrent callers when the token is about to expire or a request gets 401.
 * A rejected refresh ends the session; network trouble keeps it.
 *
 * Pure (fetch, AbortController and timers only) so `node --test` runs it.
 */
import type { Session, Tokens } from '../auth/session.ts';
import { ApiError, unwrap } from './envelope.ts';

export type Query = Record<string, string | number | boolean | undefined | null>;

type BaseOptions = {
  query?: Query;
  body?: unknown;
  /** No auth header, no refresh (login, refresh, public endpoints). */
  anonymous?: boolean;
};

/** Request options; `S` is the app's scope option (`taxpayer`, `organization`). */
export type RequestOptions<S extends string = never> = BaseOptions & { [K in S]?: string };

export type ApiClientConfig<S extends string> = {
  /** API origin without /v1 (read on every request). */
  baseUrl: () => string;
  timeoutMs: number;
  session: Session;
  /**
   * The scope option and the header it fills, e.g. `taxpayer` → `X-Taxpayer`
   * (mükellef app) or `organization` → `X-Organization` (office app).
   */
  scope: { option: S; header: string };
  fetch?: typeof fetch;
};

export function queryString(query?: Query) {
  if (!query) return '';
  const parts: string[] = [];
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined || v === null || v === '') continue;
    parts.push(`${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`);
  }
  return parts.length ? `?${parts.join('&')}` : '';
}

export function createApiClient<S extends string>(cfg: ApiClientConfig<S>) {
  const { session } = cfg;
  const doFetch = cfg.fetch ?? ((...args: Parameters<typeof fetch>) => fetch(...args));

  async function send(method: string, path: string, opts: RequestOptions<S>, accessToken: string | null) {
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (opts.body !== undefined) headers['Content-Type'] = 'application/json';
    if (accessToken) headers.Authorization = `Bearer ${accessToken}`;
    const scope = (opts as Record<string, unknown>)[cfg.scope.option];
    if (typeof scope === 'string' && scope) headers[cfg.scope.header] = scope;

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), cfg.timeoutMs);
    try {
      const res = await doFetch(`${cfg.baseUrl()}${path}${queryString(opts.query)}`, {
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
        const res = await send('POST', '/v1/auth/refresh', { body: { refresh_token: current.refreshToken } } as RequestOptions<S>, null);
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
  async function request<T>(method: string, path: string, opts: RequestOptions<S> = {} as RequestOptions<S>): Promise<T> {
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

  const api = {
    get: <T>(path: string, opts?: RequestOptions<S>) => request<T>('GET', path, opts),
    post: <T>(path: string, body?: unknown, opts?: RequestOptions<S>) => request<T>('POST', path, { ...opts, body } as RequestOptions<S>),
    put: <T>(path: string, body?: unknown, opts?: RequestOptions<S>) => request<T>('PUT', path, { ...opts, body } as RequestOptions<S>),
    delete: <T>(path: string, opts?: RequestOptions<S>) => request<T>('DELETE', path, opts),
  };

  return { request, api };
}
