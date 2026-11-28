import assert from 'node:assert/strict';
import { test } from 'node:test';

import { receiptChecks } from './checks.ts';

test('receiptChecks maps the server codes to messages', () => {
  const got = receiptChecks({ checks: ['RECEIPT_BUYER_MISMATCH', 'PLATE_UNREADABLE'], plate_check: 'unreadable' });
  assert.deepEqual(
    got.map((c) => c.text),
    ['Bu fiş başka biri adına kesilmiş', 'Plaka okunamadı'],
  );
});

test('receiptChecks: a code the office turned off is not sent, nothing shown', () => {
  assert.deepEqual(receiptChecks({ checks: [], plate_check: 'unreadable' }), []);
  assert.deepEqual(receiptChecks(null), []);
});

test('receiptChecks falls back to plate_check and skips unknown codes', () => {
  assert.deepEqual(
    receiptChecks({ plate_check: 'not_registered' }).map((c) => c.text),
    ['Plaka mükellefin araçlarında yok'],
  );
  assert.deepEqual(receiptChecks({ checks: ['SOMETHING_NEW', 'PLATE_NOT_REGISTERED', 'PLATE_NOT_REGISTERED'] }).length, 1);
});
