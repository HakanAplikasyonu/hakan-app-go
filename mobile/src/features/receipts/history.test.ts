import assert from 'node:assert/strict';
import { test } from 'node:test';

import { historyStatusLabel, serverOnly } from './history.ts';
import { markUploaded, newItem, type ReceiptResult } from './queue.ts';

const result = (uuid: string): ReceiptResult => ({ uuid, status: 'draft', kind: 'fuel_receipt', fields: {}, duplicate_of: null, error: null });

test('server history shows receipts that are not on this phone, newest first', () => {
  const onPhone = markUploaded(newItem({ id: 'a', taxpayer: 'tp1', kind: 'fuel_receipt', fileUri: 'file:///a.jpg', mimeType: 'image/jpeg', now: 1 }), result('r1'));
  const otherCompany = markUploaded(newItem({ id: 'b', taxpayer: 'tp2', kind: 'fuel_receipt', fileUri: 'file:///b.jpg', mimeType: 'image/jpeg', now: 1 }), result('r2'));
  const history = [
    { uuid: 'r1', created_at: '2026-11-10T10:00:00Z' },
    { uuid: 'r2', created_at: '2026-11-09T10:00:00Z' },
    { uuid: 'r3', created_at: '2026-11-12T10:00:00Z' },
  ];
  assert.deepEqual(serverOnly(history, [onPhone, otherCompany], 'tp1').map((h) => h.uuid), ['r3', 'r2']);
});

test('history status labels', () => {
  assert.equal(historyStatusLabel('approved'), 'Onaylandı');
  assert.equal(historyStatusLabel('something_new'), 'something_new');
});
