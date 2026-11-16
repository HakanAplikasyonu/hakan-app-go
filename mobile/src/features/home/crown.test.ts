import assert from 'node:assert/strict';
import { test } from 'node:test';

import { dueLine, lateBadge, receiptCrown, taskCrown, waitingLine } from './crown.ts';

test('receipt status to crown', () => {
  assert.equal(receiptCrown('draft'), 'yuklendi');
  assert.equal(receiptCrown('approved'), 'onaylandi');
  assert.equal(receiptCrown('failed'), 'sorun');
  assert.equal(receiptCrown('duplicate'), 'sorun');
  assert.equal(receiptCrown(undefined), 'bekleniyor');
});

test('task status to crown', () => {
  assert.equal(taskCrown('open'), 'bekleniyor');
  assert.equal(taskCrown('open', true), 'sorun');
  assert.equal(taskCrown('submitted', true), 'yuklendi');
  assert.equal(taskCrown('matched'), 'onaylandi');
});

test('due line on the Istanbul calendar', () => {
  const now = new Date('2026-11-16T09:00:00Z');
  assert.deepEqual(dueLine('2026-11-14', now), { text: '2 gün gecikti', late: true });
  assert.deepEqual(dueLine('2026-11-16', now), { text: 'bugün son gün', late: false });
  assert.deepEqual(dueLine('2026-11-21T20:59:00Z', now), { text: '5 gün kaldı', late: false });
  assert.equal(dueLine(null, now), null);
  assert.equal(dueLine('bozuk', now), null);
});

test('badges', () => {
  assert.equal(lateBadge([{ overdue: true }, {}, { overdue: false }]), '1 gecikti');
  assert.equal(lateBadge([]), null);
  assert.equal(waitingLine(2), '2 fiş telefonda bekliyor');
  assert.equal(waitingLine(0), null);
});
