import assert from 'node:assert/strict';
import { test } from 'node:test';

import { crownFor, inboxA11yLabel, inboxEndpoint, inboxTime, normalizeInbox, unreadCount } from './inbox.ts';

test('inbox endpoints (#333)', () => {
  assert.equal(inboxEndpoint('office')?.list, '/v1/tenant/notifications');
  assert.equal(inboxEndpoint('office')?.read('u1'), '/v1/tenant/notifications/u1/read');
  assert.equal(inboxEndpoint('taxpayer')?.readAll, '/v1/taxpayer/notifications/read-all');
});

test('kind from template code or source event', () => {
  const items = normalizeInbox({
    items: [
      { uuid: 'a', title: 'Engelleyici', template_code: 'warnings.blocker', action_url: '/t/ofis/warnings' },
      { uuid: 'b', title: 'Belge yüklendi', source_event: 'doc_tasks.submitted' },
      { uuid: 'c', title: 'Ajan çevrimdışı', template_code: 'agents.offline' },
      { uuid: 'd', title: 'Hatırlatma', template_code: 'doc_tasks.reminder' },
      { uuid: 'e', title: 'Bilinmeyen', template_code: 'yeni.olay' },
    ],
    total: 5,
    unread: 5,
  });
  assert.deepEqual(items.map((i) => i.kind), ['blocker', 'upload', 'agent', 'reminder', 'other']);
  assert.equal(items[0].url, '/t/ofis/warnings');
});

test('normalizeInbox keeps valid rows and maps unknown kinds', () => {
  const items = normalizeInbox({
    items: [
      { uuid: 'a', title: 'Engelleyici: KDV toplamı tutmuyor', kind: 'blocker', created_at: '2026-11-16T05:31:00Z', read_at: null },
      { uuid: 'b', title: 'Atlas Gıda 12 fiş yükledi', kind: 'nope', read_at: '2026-11-16T07:00:00Z', url: '/t/ofis/review' },
      { uuid: '', title: 'x' },
      { uuid: 'c' },
      null,
    ],
  });
  assert.equal(items.length, 2);
  assert.equal(items[0].read, false);
  assert.equal(items[1].kind, 'other');
  assert.equal(items[1].read, true);
  assert.equal(items[1].url, '/t/ofis/review');
  assert.deepEqual(normalizeInbox('bozuk'), []);
  assert.equal(unreadCount(items), 1);
});

test('crown state by kind', () => {
  assert.equal(crownFor('blocker'), 'sorun');
  assert.equal(crownFor('upload'), 'yuklendi');
  assert.equal(crownFor('export'), 'aktarildi');
  assert.equal(crownFor('task'), 'bekleniyor');
});

test('inboxTime by Istanbul day', () => {
  const now = new Date('2026-11-16T09:00:00Z'); // 12:00
  assert.equal(inboxTime('2026-11-16T05:31:00Z', now), '08:31');
  assert.equal(inboxTime('2026-11-15T20:00:00Z', now), 'Dün');
  assert.equal(inboxTime('2026-11-14T10:00:00Z', now), '2 gün');
  assert.match(inboxTime('2026-10-12T10:00:00Z', now), /^12 Eki/);
  assert.equal(inboxTime('bozuk', now), '');
  // Türkiye'de gece yarısını geçen kayıt "Dün" değil bugündür.
  assert.equal(inboxTime('2026-11-15T21:30:00Z', now), '00:30');
});

test('a11y label says unread in words', () => {
  const [i] = normalizeInbox([{ uuid: 'a', title: 'Başlık', body: 'Ayrıntı', kind: 'upload', created_at: '2026-11-16T05:31:00Z' }]);
  assert.equal(inboxA11yLabel(i, new Date('2026-11-16T09:00:00Z')), 'Okunmadı, Yükleme, Başlık, Ayrıntı, 08:31');
});
