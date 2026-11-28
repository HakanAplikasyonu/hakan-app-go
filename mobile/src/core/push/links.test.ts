import assert from 'node:assert/strict';
import { test } from 'node:test';

import { notificationUrl, resolveLink, systemPath } from './links.ts';

const TP = '0b6a5d1e-2f3c-4a5b-8c7d-9e0f1a2b3c4d';
const TASK = 'aa11bb22-cc33-4d44-8e55-ff6677889900';

test('notification web paths map to app routes with their taxpayer', () => {
  assert.deepEqual(resolveLink(`/m/${TP}/tasks`), { route: '/tasks', taxpayer: TP });
  assert.deepEqual(resolveLink(`/m/${TP}/tasks/${TASK}`), { route: `/task/${TASK}`, taxpayer: TP });
  assert.deepEqual(resolveLink(`/m/${TP}/receipts/42`), { route: '/receipts/42', taxpayer: TP });
  assert.deepEqual(resolveLink(`/m/${TP}/receipts`), { route: '/receipts', taxpayer: TP });
  assert.deepEqual(resolveLink(`/m/${TP}`), { route: '/', taxpayer: TP });
  assert.deepEqual(resolveLink(`/m/${TP}/uploads`), { route: '/capture', taxpayer: TP });
});

test('universal links use the same paths', () => {
  assert.deepEqual(resolveLink(`https://app.example.com/m/${TP}/tasks?x=1#top`), { route: '/tasks', taxpayer: TP });
  assert.deepEqual(resolveLink(`https://app.example.com/m/${TP.toUpperCase()}/tasks`), { route: '/tasks', taxpayer: TP });
});

test('scheme links', () => {
  assert.deepEqual(resolveLink('hakan://tasks'), { route: '/tasks' });
  assert.deepEqual(resolveLink('hakan://receipts/r-123'), { route: '/receipts/r-123' });
  assert.deepEqual(resolveLink(`hakan://task/${TASK}`), { route: `/task/${TASK}` });
  assert.deepEqual(resolveLink('hakan:///tasks'), { route: '/tasks' });
  assert.deepEqual(resolveLink('hakan://'), { route: '/' });
});

test('unknown or malformed links are refused', () => {
  for (const bad of [
    '',
    null,
    undefined,
    '/platform/bulk/x',
    '/t/ofis/journals',
    `/m/not-a-uuid/tasks`,
    `/m/${TP}/tasks/not-a-uuid`,
    `/m/${TP}/receipts/a/b`,
    'hakan://receipts/../../etc',
    'hakan://settings',
    `/m/${TP}/receipts/%E0%A4%A`,
  ]) {
    assert.equal(resolveLink(bad as string), null, String(bad));
  }
});

test('receipt decision opens Gönderdiklerim (#780 Senaryo B)', () => {
  assert.deepEqual(resolveLink(`/m/${TP}/gonderdiklerim`), { route: '/gonderdiklerim', taxpayer: TP });
  assert.deepEqual(resolveLink('hakan://gonderdiklerim'), { route: '/gonderdiklerim' });
  assert.equal(resolveLink(`/m/${TP}/gonderdiklerim/x`), null);
});

test('push payload url', () => {
  assert.equal(notificationUrl({ url: '/m/x/tasks', kind: 'k' }), '/m/x/tasks');
  assert.equal(notificationUrl({ url: 3 }), null);
  assert.equal(notificationUrl(null), null);
});

test('native intent falls back to home', () => {
  assert.equal(systemPath(`/m/${TP}/tasks`), '/tasks');
  assert.equal(systemPath('/somewhere/else'), '/');
});
