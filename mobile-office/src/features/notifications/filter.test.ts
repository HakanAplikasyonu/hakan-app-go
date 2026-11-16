import assert from 'node:assert/strict';
import { test } from 'node:test';

import { normalizeInbox } from '../../../../packages/mobile-core/src/notifications/inbox.ts';

import { filterInbox } from './filter.ts';

test('filters by kind', () => {
  const items = normalizeInbox([
    { uuid: 'a', title: 'Engel', kind: 'blocker' },
    { uuid: 'b', title: 'Yükleme', kind: 'upload' },
    { uuid: 'c', title: 'Ajan', kind: 'agent' },
  ]);
  assert.equal(filterInbox(items, 'all').length, 3);
  assert.deepEqual(filterInbox(items, 'blocker').map((i) => i.uuid), ['a']);
  assert.deepEqual(filterInbox(items, 'upload').map((i) => i.uuid), ['b']);
});
