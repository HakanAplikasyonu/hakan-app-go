import assert from 'node:assert/strict';
import { test } from 'node:test';

import { notificationUrl } from '../push/data.ts';
import { safeLink } from './safe-link.ts';

test('safeLink accepts https (and mailto when allowed) only', () => {
  assert.equal(safeLink(' https://example.com/gizlilik '), 'https://example.com/gizlilik');
  assert.equal(safeLink('http://example.com'), '');
  assert.equal(safeLink('javascript:alert(1)'), '');
  assert.equal(safeLink(undefined), '');
  assert.equal(safeLink('mailto:destek@example.com'), '');
  assert.equal(safeLink('mailto:destek@example.com', true), 'mailto:destek@example.com');
});

test('notificationUrl reads a string data.url only', () => {
  assert.equal(notificationUrl({ url: '/m/x/tasks', kind: 'k' }), '/m/x/tasks');
  assert.equal(notificationUrl({ url: '' }), null);
  assert.equal(notificationUrl({ url: 3 }), null);
  assert.equal(notificationUrl(null), null);
  assert.equal(notificationUrl('url'), null);
});
