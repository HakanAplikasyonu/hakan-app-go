import assert from 'node:assert/strict';
import { test } from 'node:test';

import { actions, dayLabel, dueDateFromDays, taskTitle, validateRequest } from './format.ts';

test('taskTitle', () => {
  assert.equal(taskTitle({ kind: 'document_request', title: 'Kira kontratı' }), 'Kira kontratı');
  assert.equal(taskTitle({ kind: 'undocumented_payment', title: '' }), 'Ödeme için belge');
  assert.equal(taskTitle({ kind: 'yeni_tur' }), 'Eksik belge');
});

test('due date in Istanbul', () => {
  // 21:30 UTC is already the next day in Istanbul.
  assert.equal(dueDateFromDays(7, new Date('2026-11-16T21:30:00Z')), '2026-11-24');
  assert.equal(dueDateFromDays(1, new Date('2026-11-16T10:00:00Z')), '2026-11-17');
  assert.equal(dayLabel('2026-11-20'), '20 Kasım 2026');
});

test('validateRequest mirrors the backend', () => {
  assert.equal(validateRequest({ title: 'Kira', note: '', dueDays: 7 }), null);
  assert.match(validateRequest({ title: '  ', note: '', dueDays: 7 }) ?? '', /yazın/);
  assert.match(validateRequest({ title: 'x'.repeat(201), note: '', dueDays: 7 }) ?? '', /200/);
  assert.match(validateRequest({ title: 'x', note: '', dueDays: 91 }) ?? '', /90/);
});

test('actions by status', () => {
  assert.deepEqual(actions('open'), { complete: true, rerequest: false, cancel: true });
  assert.deepEqual(actions('submitted'), { complete: true, rerequest: true, cancel: true });
  assert.deepEqual(actions('matched'), { complete: false, rerequest: false, cancel: false });
});
