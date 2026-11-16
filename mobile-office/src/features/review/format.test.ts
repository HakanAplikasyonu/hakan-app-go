import assert from 'node:assert/strict';
import { test } from 'node:test';

import { bulkTargets, confidenceBand, confidencePercent, isValidAccountCode, proposedCode, runSequential, swipeAction } from './format.ts';

test('proposedCode', () => {
  assert.equal(proposedCode({ account_code: '770.01' }), '770.01');
  assert.equal(proposedCode({ other: 1 }), null);
  assert.equal(proposedCode(null), null);
  assert.equal(proposedCode('770'), null);
});

test('account code format', () => {
  for (const ok of ['770', '770.01', '320.01.001', '153.01.A1']) assert.equal(isValidAccountCode(ok), true, ok);
  for (const bad of ['77', '770.', 'abc', '770..1', '']) assert.equal(isValidAccountCode(bad), false, bad);
});

test('confidence', () => {
  assert.equal(confidencePercent('0.8700'), '%87');
  assert.equal(confidencePercent('x'), '—');
  assert.equal(confidenceBand('0.5'), 'low');
  assert.equal(confidenceBand('0.7'), 'medium');
  assert.equal(confidenceBand('0.9'), 'high');
});

test('bulk targets: selected pending with engagement only', () => {
  const list = [
    { uuid: 'a', engagement_uuid: 'e', kind: 'k', target_field: 'f', proposed_value: {}, confidence: '1', status: 'pending' },
    { uuid: 'b', engagement_uuid: 'e', kind: 'k', target_field: 'f', proposed_value: {}, confidence: '1', status: 'accepted' },
    { uuid: 'c', kind: 'k', target_field: 'f', proposed_value: {}, confidence: '1', status: 'pending' },
    { uuid: 'd', engagement_uuid: 'e', kind: 'k', target_field: 'f', proposed_value: {}, confidence: '1', status: 'pending' },
  ];
  assert.deepEqual(bulkTargets(list, new Set(['a', 'b', 'c'])).map((s) => s.uuid), ['a']);
});

test('runSequential keeps going after a failure', async () => {
  const seen: number[] = [];
  const res = await runSequential([1, 2, 3], async (n) => {
    seen.push(n);
    if (n === 2) throw new Error('x');
  });
  assert.deepEqual(seen, [1, 2, 3]);
  assert.deepEqual(res.ok, [1, 3]);
  assert.equal(res.failed[0].item, 2);
});

test('swipe threshold', () => {
  assert.equal(swipeAction(200, 400), 'accept');
  assert.equal(swipeAction(-200, 400), 'reject');
  assert.equal(swipeAction(50, 400), null);
  assert.equal(swipeAction(70, 100), 'accept'); // minimum 60 px
});
