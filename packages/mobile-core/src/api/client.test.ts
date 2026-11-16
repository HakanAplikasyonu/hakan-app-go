import assert from 'node:assert/strict';
import { test } from 'node:test';

import { createSession, type KeyValueStore } from '../auth/session.ts';
import { createApiClient, queryString } from './client.ts';
import { ApiError } from './envelope.ts';

type Call = { url: string; method: string; headers: Record<string, string>; body?: string };

function memoryStore(): KeyValueStore {
  const data = new Map<string, string>();
  return { get: async (k) => data.get(k) ?? null, set: async (k, v) => void data.set(k, v), remove: async (k) => void data.delete(k) };
}

function json(status: number, body: unknown, requestId = 'rid') {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', 'X-Request-ID': requestId } });
}

const ok = (data: unknown) => json(200, { success: true, data, meta: {} });
const tokens = (n: number) => ({
  access_token: `a${n}`,
  refresh_token: `r${n}`,
  token_type: 'Bearer',
  expires_in: 900,
  refresh_expires_at: 'x',
});

/** A client against a scripted fetch; `respond` answers each call. */
async function setup(respond: (c: Call, n: number) => Response | Promise<Response>, opts: { expiresIn?: number; timeoutMs?: number } = {}) {
  const session = createSession(memoryStore(), 'k');
  await session.setTokens({ ...tokens(1), expires_in: opts.expiresIn ?? 900 });
  const calls: Call[] = [];
  const fetchFn = (async (url: string, init: RequestInit) => {
    const c = { url, method: String(init.method), headers: init.headers as Record<string, string>, body: init.body as string | undefined };
    calls.push(c);
    if (init.signal?.aborted) throw new DOMException('aborted', 'AbortError');
    return respond(c, calls.length);
  }) as unknown as typeof fetch;
  const client = createApiClient({
    baseUrl: () => 'https://api.test',
    timeoutMs: opts.timeoutMs ?? 1000,
    session,
    scope: { option: 'taxpayer', header: 'X-Taxpayer' },
    fetch: fetchFn,
  });
  return { ...client, session, calls };
}

test('queryString skips empty values and encodes', () => {
  assert.equal(queryString(), '');
  assert.equal(queryString({ a: 1, b: '', c: null, d: undefined, e: 'x y', f: false }), '?a=1&e=x%20y&f=false');
});

test('request sends the token, the scope header and the JSON body', async () => {
  const { api, calls } = await setup(() => ok({ id: 7 }));
  assert.deepEqual(await api.post('/v1/things', { n: 1 }, { taxpayer: 'tp-1', query: { q: 'a' } }), { id: 7 });
  assert.equal(calls[0].url, 'https://api.test/v1/things?q=a');
  assert.equal(calls[0].method, 'POST');
  assert.equal(calls[0].headers.Authorization, 'Bearer a1');
  assert.equal(calls[0].headers['X-Taxpayer'], 'tp-1');
  assert.equal(calls[0].headers['Content-Type'], 'application/json');
  assert.equal(calls[0].body, '{"n":1}');

  await api.get('/v1/x');
  assert.equal(calls[1].headers['X-Taxpayer'], undefined);
  assert.equal(calls[1].headers['Content-Type'], undefined);
});

test('anonymous requests carry no token and need no session', async () => {
  const { request, calls, session } = await setup(() => ok('fine'));
  await session.clear();
  assert.equal(await request('POST', '/v1/auth/login', { anonymous: true, body: {} }), 'fine');
  assert.equal(calls[0].headers.Authorization, undefined);
  await assert.rejects(request('GET', '/v1/me'), (e: unknown) => e instanceof ApiError && e.code === 'UNAUTHENTICATED');
});

test('a 401 refreshes once for concurrent callers, then retries with the new token', async () => {
  const { api, calls, session } = await setup((c) => {
    if (c.url.endsWith('/v1/auth/refresh')) return ok(tokens(2));
    return c.headers.Authorization === 'Bearer a2'
      ? ok('yes')
      : json(401, { success: false, error: { code: 'UNAUTHENTICATED', message: 'x' } });
  });
  assert.deepEqual(await Promise.all([api.get('/v1/a'), api.get('/v1/b')]), ['yes', 'yes']);
  assert.equal(calls.filter((c) => c.url.endsWith('/v1/auth/refresh')).length, 1);
  assert.equal(JSON.parse(calls.find((c) => c.url.endsWith('/refresh'))!.body!).refresh_token, 'r1');
  assert.equal(session.get()?.refreshToken, 'r2');
});

test('a token about to expire is refreshed before the request', async () => {
  const { api, calls } = await setup((c) => (c.url.endsWith('/refresh') ? ok(tokens(2)) : ok('ok')), { expiresIn: 10 });
  await api.get('/v1/a');
  assert.deepEqual(
    calls.map((c) => c.url.replace('https://api.test', '')),
    ['/v1/auth/refresh', '/v1/a'],
  );
  assert.equal(calls[1].headers.Authorization, 'Bearer a2');
});

test('a rejected refresh ends the session; the 401 reaches the caller', async () => {
  const { api, session } = await setup(() => json(401, { success: false, error: { code: 'UNAUTHENTICATED', message: 'x' } }));
  const ended: unknown[] = [];
  session.subscribe((s) => ended.push(s));
  await assert.rejects(api.get('/v1/a'), (e: unknown) => e instanceof ApiError && e.status === 401);
  assert.equal(session.get(), null);
  assert.deepEqual(ended, [null]);
});

test('network trouble keeps the session', async () => {
  const { api, session } = await setup(() => {
    throw new TypeError('Network request failed');
  });
  await assert.rejects(api.get('/v1/a'), (e: unknown) => e instanceof ApiError && e.code === 'NETWORK_ERROR' && e.status === 0);
  assert.equal(session.get()?.refreshToken, 'r1');
});

test('a slow server times out', async () => {
  const { api } = await setup(
    (c) =>
      new Promise((_, reject) => {
        // Resolves only through the abort signal of the client's timer.
        void c;
        setTimeout(() => reject(new DOMException('aborted', 'AbortError')), 50);
      }),
    { timeoutMs: 10 },
  );
  await assert.rejects(api.get('/v1/a'), (e: unknown) => e instanceof ApiError && e.code === 'TIMEOUT');
});

test('the request id reaches the error', async () => {
  const { api } = await setup(() => json(409, { success: false, error: { code: 'CONFLICT', message: 'Zaten var' } }, 'req-9'));
  await assert.rejects(api.delete('/v1/a'), (e: unknown) => e instanceof ApiError && e.requestId === 'req-9' && e.code === 'CONFLICT');
});
