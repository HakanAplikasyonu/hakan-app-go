import assert from 'node:assert/strict';
import { test } from 'node:test';

import { MAX_REJECT_TEXT, REJECT_REASONS, rejectionBody } from './reject-reason.ts';

test('rejectionBody needs a code', () => {
  assert.deepEqual(rejectionBody(null, 'not'), { error: 'Bir gerekçe seçin.' });
});

test('rejectionBody: diger needs a text', () => {
  assert.ok('error' in rejectionBody('diger', '   '));
  assert.deepEqual(rejectionBody('diger', ' Ekim  ayına ait değil '), { body: { reason_code: 'diger', reason_text: 'Ekim ayına ait değil' } });
});

test('rejectionBody: a fixed reason without a text', () => {
  assert.deepEqual(rejectionBody('okunaksiz', ''), { body: { reason_code: 'okunaksiz' } });
});

test('rejectionBody: text limit', () => {
  assert.ok('body' in rejectionBody('mukerrer', 'ş'.repeat(MAX_REJECT_TEXT)));
  assert.ok('error' in rejectionBody('mukerrer', 'ş'.repeat(MAX_REJECT_TEXT + 1)));
});

test('REJECT_REASONS: six codes, diger last', () => {
  assert.equal(REJECT_REASONS.length, 6);
  assert.equal(REJECT_REASONS.at(-1)?.code, 'diger');
  assert.ok(REJECT_REASONS.every((r) => r.label));
});
