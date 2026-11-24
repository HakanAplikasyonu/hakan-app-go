import assert from 'node:assert/strict';
import { test } from 'node:test';

import { formatPlate, normalizePlate, plateValid } from './plaka.ts';

test('normalizePlate follows the server rule', () => {
  assert.equal(normalizePlate(' 34 abc-123 '), '34ABC123');
  assert.equal(normalizePlate('06 şçi 42'), '06SCI42');
});

test('plateValid checks the province code and shape', () => {
  assert.ok(plateValid('34 ABC 123'));
  assert.ok(plateValid('81 a 12345'));
  assert.ok(!plateValid('00 ABC 123'));
  assert.ok(!plateValid('82 AB 12'));
  assert.ok(!plateValid('34 ABCD 12'));
  assert.ok(!plateValid(''));
});

test('formatPlate spaces the parts', () => {
  assert.equal(formatPlate('34ABC123'), '34 ABC 123');
  assert.equal(formatPlate('garip'), 'garip');
});
