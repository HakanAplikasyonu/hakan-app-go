import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  addCorrection,
  backoffMs,
  BLOCKED_RETRY_MS,
  classify,
  correctionOutcome,
  markCorrections,
  correctionsDue,
  due,
  markFailed,
  markUploaded,
  markUploading,
  newItem,
  recover,
  retry,
  summarize,
  type ReceiptResult,
} from './queue.ts';

const T = 1_000_000;
const item = (id: string, at: number, taxpayer = 'tp1') =>
  newItem({ id, taxpayer, kind: 'expense_receipt', fileUri: `file:///${id}.jpg`, mimeType: 'image/jpeg', now: at });
const ok: ReceiptResult = { uuid: 'r1', status: 'draft', kind: 'expense_receipt', fields: {}, duplicate_of: null, error: null };

test('a receipt shot offline waits, then uploads when the network returns', () => {
  let a = item('a', T);
  // offline: the request never reached the server
  a = markFailed(markUploading(a), 'network', 'Network request failed', T + 1);
  assert.equal(a.status, 'pending');
  assert.equal(a.nextAttemptAt, T + 1 + 30_000);
  assert.deepEqual(due([a], 'tp1', T + 2), []);
  // network back: the sync runs forced, regardless of the backoff
  assert.deepEqual(due([a], 'tp1', T + 2, { force: true }).map((i) => i.id), ['a']);
  a = markUploaded(markUploading(a), ok);
  assert.equal(a.status, 'uploaded');
  assert.equal(a.result?.uuid, 'r1');
  assert.deepEqual(due([a], 'tp1', T + 3, { force: true }), []);
});

test('backoff doubles and is capped', () => {
  assert.equal(backoffMs(1), 30_000);
  assert.equal(backoffMs(2), 60_000);
  assert.equal(backoffMs(3), 120_000);
  assert.equal(backoffMs(20), 30 * 60_000);
});

test('status classification', () => {
  assert.equal(classify(0), 'network');
  assert.equal(classify(503), 'server');
  assert.equal(classify(429), 'server');
  assert.equal(classify(401), 'auth');
  assert.equal(classify(404), 'unsupported');
  assert.equal(classify(405), 'unsupported');
  assert.equal(classify(400), 'rejected');
  assert.equal(classify(413), 'rejected');
});

test('missing server endpoint keeps the photo on the phone', () => {
  const a = markFailed(item('a', T), 'unsupported', 'HTTP_404', T);
  assert.equal(a.status, 'blocked');
  assert.equal(a.nextAttemptAt, T + BLOCKED_RETRY_MS);
  assert.deepEqual(due([a], 'tp1', T + 1, { force: true }).map((i) => i.id), ['a']);
});

test('a rejected photo stops until the user retries', () => {
  const a = markFailed(item('a', T), 'rejected', 'too large', T);
  assert.equal(a.status, 'failed');
  assert.deepEqual(due([a], 'tp1', T + 10, { force: true }), []);
  assert.deepEqual(due([retry(a)], 'tp1', T + 10).map((i) => i.id), ['a']);
});

test('only the active company, oldest first', () => {
  const list = [item('b', T + 5), item('a', T), item('x', T, 'tp2')];
  assert.deepEqual(due(list, 'tp1', T + 10).map((i) => i.id), ['a', 'b']);
});

test('an interrupted upload is retried after a restart', () => {
  const [a] = recover([markUploading(item('a', T))]);
  assert.equal(a.status, 'pending');
});

test('corrections replace per field and wait for upload', () => {
  let a = item('a', T);
  a = addCorrection(a, { field: 'total_amount', value: '100.00' });
  a = addCorrection(a, { field: 'total_amount', value: '120.00' });
  assert.deepEqual(a.corrections, [{ field: 'total_amount', value: '120.00' }]);
  assert.deepEqual(correctionsDue([a], 'tp1'), []); // not uploaded yet
  a = markUploaded(a, ok);
  assert.deepEqual(correctionsDue([a], 'tp1').map((i) => i.id), ['a']);
  assert.deepEqual(correctionsDue([{ ...a, correctionsSent: true }], 'tp1'), []);
});

test('summary counts per company', () => {
  const list = [
    item('a', T),
    markFailed(item('b', T), 'unsupported', '', T),
    markFailed(item('c', T), 'rejected', '', T),
    markUploaded(item('d', T), ok),
    item('e', T, 'tp2'),
  ];
  assert.deepEqual(summarize(list, 'tp1'), { waiting: 1, blocked: 1, failed: 1, uploaded: 1 });
});

test('classify: the receipt endpoint answers (#199)', () => {
  assert.equal(classify(409, 'NO_ACTIVE_ENGAGEMENT'), 'unsupported');
  assert.equal(classify(409, 'CONFLICT'), 'rejected');
  assert.equal(classify(503), 'server');
  assert.equal(classify(405), 'unsupported');
  assert.equal(classify(400, 'VALIDATION_ERROR'), 'rejected');
});

test('corrections refused by the server are not retried; network errors are', () => {
  assert.equal(correctionOutcome(0), 'retry');
  assert.equal(correctionOutcome(503), 'retry');
  assert.equal(correctionOutcome(401), 'retry');
  assert.equal(correctionOutcome(400), 'drop');
  assert.equal(correctionOutcome(409), 'drop');
  let a = markUploaded(item('c', T), ok);
  a = addCorrection(a, { field: 'total_amount', value: 'abc' });
  a = markCorrections(a, 'total_amount value "abc" is not valid');
  assert.equal(a.correctionsSent, true);
  assert.deepEqual(correctionsDue([a], 'tp1'), []);
  // A new correction clears the refusal and is sent again.
  a = addCorrection(a, { field: 'total_amount', value: '1.250,00' });
  assert.equal(a.correctionsError, null);
  assert.deepEqual(correctionsDue([a], 'tp1').map((i) => i.id), ['c']);
});
