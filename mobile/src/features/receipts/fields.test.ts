import assert from 'node:assert/strict';
import { test } from 'node:test';

import { fieldsToCheck, orderedFields, statusLabel } from './fields.ts';
import { addCorrection, markFailed, markUploaded, newItem } from './queue.ts';

const base = newItem({ id: 'a', taxpayer: 't', kind: 'fuel_receipt', fileUri: 'f', mimeType: 'image/jpeg', now: 1 });
const result = {
  uuid: 'r',
  status: 'draft' as const,
  kind: 'fuel_receipt',
  duplicate_of: null,
  error: null,
  fields: {
    zzz_extra: { raw: 'x', value: 'x', confidence: 0.99, valid: true },
    total_amount: { raw: '1.250,00', value: '1250.00', confidence: 0.95, valid: true },
    vehicle_plate: { raw: '34 AB 12', value: '34AB12', confidence: 0.6, valid: true },
    date: { raw: '31/1O/2026', value: '', confidence: 0.9, valid: false },
  },
};

test('known fields come first in a fixed order', () => {
  assert.deepEqual(orderedFields(result.fields).map(([k]) => k), ['date', 'total_amount', 'vehicle_plate', 'zzz_extra']);
});

test('low-confidence and invalid fields need a check until corrected', () => {
  const up = markUploaded(base, result);
  assert.deepEqual(fieldsToCheck(up), ['date', 'vehicle_plate']);
  assert.deepEqual(fieldsToCheck(addCorrection(up, { field: 'date', value: '2026-10-31' })), ['vehicle_plate']);
  assert.deepEqual(fieldsToCheck(base), []);
});

test('status labels are Turkish', () => {
  assert.equal(statusLabel(base), 'Sırada');
  assert.equal(statusLabel(markFailed(base, 'network', '', 1)), 'Bağlantı bekleniyor');
  assert.equal(statusLabel(markFailed(base, 'unsupported', '', 1)), 'Telefonda bekliyor');
  assert.equal(statusLabel(markUploaded(base, result)), 'Müşavir onayında');
  assert.equal(statusLabel(markUploaded(base, { ...result, status: 'duplicate' })), 'Daha önce gönderilmiş');
});
