import assert from 'node:assert/strict';
import { test } from 'node:test';

import { launchFailure } from './launch.ts';
import { createSession, type KeyValueStore } from './session.ts';

function memoryStore(): KeyValueStore & { data: Map<string, string> } {
  const data = new Map<string, string>();
  return {
    data,
    get: async (k) => data.get(k) ?? null,
    set: async (k, v) => {
      data.set(k, v);
    },
    remove: async (k) => {
      data.delete(k);
    },
  };
}

const tokens = { access_token: 'a1', refresh_token: 'r1', token_type: 'Bearer', expires_in: 900, refresh_expires_at: '2026-12-01T00:00:00Z' };

test('setTokens stores the session under the app key and load reads it back', async () => {
  const store = memoryStore();
  const s = createSession(store, 'app.session', () => 1_000);
  await s.setTokens(tokens);
  assert.deepEqual(s.get(), { accessToken: 'a1', refreshToken: 'r1', accessExpiresAt: 901_000, refreshExpiresAt: '2026-12-01T00:00:00Z' });
  assert.deepEqual([...store.data.keys()], ['app.session']);

  const again = createSession(store, 'app.session');
  assert.equal(await again.load(), again.get());
  assert.equal(again.get()?.refreshToken, 'r1');
});

test('a corrupt stored session is dropped', async () => {
  const store = memoryStore();
  store.data.set('k', '{not json');
  const s = createSession(store, 'k');
  assert.equal(await s.load(), null);
  assert.equal(store.data.has('k'), false);
  assert.equal(await createSession(memoryStore(), 'k').load(), null);
});

test('clear removes the session and notifies subscribers once', async () => {
  const store = memoryStore();
  const s = createSession(store, 'k');
  const seen: unknown[] = [];
  const off = s.subscribe((v) => seen.push(v));
  await s.clear(); // nothing to clear: no event
  await s.setTokens(tokens);
  await s.clear();
  assert.deepEqual(seen, [null]);
  assert.equal(store.data.has('k'), false);
  off();
  await s.setTokens(tokens);
  await s.clear();
  assert.deepEqual(seen, [null]);
});

test('launchFailure: only 401 signs out', () => {
  assert.equal(launchFailure({ status: 401 }), 'signedOut');
  assert.equal(launchFailure({ status: 0, code: 'TIMEOUT' }), 'offline');
  assert.equal(launchFailure({ status: 503 }), 'offline');
  assert.equal(launchFailure(new TypeError('Network request failed')), 'offline');
  assert.equal(launchFailure(null), 'offline');
});
